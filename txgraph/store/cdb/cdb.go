package cdb

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

var (
	allWalletAddressesQuery = `select address from wallet`

	allBlockNumbersQuery = `select "number" from block order by "number" desc`

	findBlockQuery = `select "number", processed from block where "number"=$1`

	findWalletQuery = "select address from wallet where address=$1"

	unprocessedBlocksQuery = `select "number" from block where processed=false limit 1000000`

	walletsInPartitionQuery = `select address from wallet where address >= $1 and address < $2`

	walletTxsQuery = `select * from tx where "from"=$1`

	// Compile-time check for ensuring CDBGraph implements Graph.
	_ graph.Graph = (*CDBGraph)(nil)
)

type blockCache struct {
	mu				sync.RWMutex
	maxSize		int
	cacheMap		map[int]struct{}
	cacheList	[]*graph.Block	
}

type txCache struct {
	mu				sync.RWMutex
	maxSize		int
	cacheMap		map[string]struct{}
	cacheList	[]*graph.Tx	
}

type walletCache struct {
	mu				sync.RWMutex
	maxSize		int
	cacheMap		map[string]struct{}
	cacheList	[]*graph.Wallet
}

// CDBGraph implements a graph that persists its transactions and wallets to 
// a cockroachdb instance.
type CDBGraph struct {
	db				*sql.DB
	usesCache	bool // Is true if CDBGraph uses caching.
	blockC		blockCache
	txC			txCache
	walletC		walletCache
}

// NewCDBGraph returns a CDBGraph instance that connects to the cockroachdb
// instance specified by dsn.
// To optimise performance and the load on the DB it may cache items before inserts.
// Upserting from the block cache always flushes other caches to ensure that if a block
// gets marked as processed in the DB, all its wallets and txs also get written to disk.
func NewCDBGraph(dsn string, usesCache bool) (*CDBGraph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if usesCache {
		blockCacheSize 	:= 6000
		txCacheSize 		:= 6000
		walletCacheSize	:= 1500
		return &CDBGraph{
			db:		db,
			usesCache:	usesCache,
			blockC:	blockCache{
				maxSize:		blockCacheSize, 
				cacheMap: 	make(map[int]struct{}, blockCacheSize),
				cacheList:	make([]*graph.Block, 0, blockCacheSize),	
			},
			txC:	txCache{
				maxSize:		txCacheSize, 
				cacheMap: 	make(map[string]struct{}, txCacheSize),
				cacheList:	make([]*graph.Tx, 0, txCacheSize),	
			},
			walletC:	walletCache{
				maxSize:		walletCacheSize, 
				cacheMap: 	make(map[string]struct{}, walletCacheSize),
				cacheList:	make([]*graph.Wallet, 0, walletCacheSize),	
			},
		}, nil
	} else {
		return &CDBGraph{ db:db }, nil
	}
}

// Close terminates the connection to the backing cockroachdb instance.
func (g *CDBGraph) Close() error {
	return g.db.Close()
}

// Returns a BlockSubscriber connected to a stream of unprocessed blocks.
func (g *CDBGraph) Blocks() (graph.BlockIterator, error) {
	blocks, err := g.getUnprocessedBlocks()
	if err != nil {
		return nil, err
	}
	return &blockIterator{g: g, blocks: blocks}, nil
}

// Returns an iterator for the set of wallets
// whose address is in the [fromAddr, toAddr) range.
func (g *CDBGraph) Wallets(fromAddr, toAddr string) (graph.WalletIterator, error) {
	rows, err := g.db.Query(walletsInPartitionQuery, fromAddr, toAddr)
	if err != nil {
		return nil, xerrors.Errorf("wallets: %w", err)
	}
	return &walletIterator{rows: rows}, nil
}

// Returns an iterator for the set of transactions originating from a wallet address.
func (g *CDBGraph) WalletTxs(address string) (graph.TxIterator, error) {
	rows, err := g.db.Query(walletTxsQuery, address)
	if err != nil {
		return nil, xerrors.Errorf("walletTxs: %w", err)
	}
	return &txIterator{rows: rows}, nil
}

func (g *CDBGraph) UpsertBlocks(blocks []*graph.Block) error {
	if g.usesCache {
		g.blockC.mu.Lock()
		defer g.blockC.mu.Unlock()
		return g.cacheItems(blocks)
	} else {
		return g.batchUpsert(blocks)
	}
}

// InsertTxs inserts new transactions.
func (g *CDBGraph) InsertTxs(txs []*graph.Tx) error {
	if g.usesCache {
		g.txC.mu.Lock()
		defer g.txC.mu.Unlock()
		return g.cacheItems(txs)
	} else {
		return g.batchUpsert(txs)
	}
}

func (g *CDBGraph) UpsertWallets(wallets []*graph.Wallet) error {
	if g.usesCache {
		g.walletC.mu.Lock()
		defer g.walletC.mu.Unlock()
		return g.cacheItems(wallets)
	} else {
		return g.batchUpsert(wallets)
	}
}


// TODO figure out how to avoid this repetition.
func (g *CDBGraph) cacheItems(xs interface{}) error {
	switch items := xs.(type) {
	case []*graph.Block:
		for _,block := range(items) {
			if _, exists := g.blockC.cacheMap[block.Number]; !exists {
				c := new(graph.Block)
				*c = *block
				g.blockC.cacheList = append(g.blockC.cacheList, c)
				g.blockC.cacheMap[block.Number] = struct{}{}
			}
		}
		if len(g.blockC.cacheList) >= g.blockC.maxSize {
			// Upsert other caches before upserting blocks.
			g.txC.mu.Lock()
			g.walletC.mu.Lock()
			defer g.txC.mu.Unlock()
			defer g.walletC.mu.Unlock()
			var err error
			//fmt.Println("upserting wallets because of blocks, with len", len(g.walletC.cacheList))
			if err = g.upsertCache("wallet"); err != nil {
				return err
			}
			//fmt.Println("upserting txs because of blocks, with len", len(g.txC.cacheList))
			if err = g.upsertCache("tx"); err != nil {
				return err
			}
			//fmt.Println("upserting blocks with len", len(g.blockC.cacheList))
			return g.upsertCache("block")
		}
		return nil
	case []*graph.Tx:
		for _,tx := range(items) {
			if _, exists := g.txC.cacheMap[tx.Hash]; !exists {
				c := new(graph.Tx)
				*c = *tx
				g.txC.cacheList = append(g.txC.cacheList, c)
				g.txC.cacheMap[tx.Hash] = struct{}{}
			}
		}
		if len(g.txC.cacheList) >= g.txC.maxSize {
			// Upsert wallets before txs to avoid wallet address foreign key violation.
			g.walletC.mu.Lock()
			defer g.walletC.mu.Unlock()
			//fmt.Println("upserting wallets because of txs, with len", len(g.walletC.cacheList))
			if err := g.upsertCache("wallet"); err != nil {
				return err
			}
			//fmt.Println("upserting txs with len", len(g.txC.cacheList))
			return g.upsertCache("tx")
		}
		return nil
	case []*graph.Wallet:
		for _,wallet := range(items) {
			if _, exists := g.walletC.cacheMap[wallet.Address]; !exists {
				c := new(graph.Wallet)
				*c = *wallet
				g.walletC.cacheList = append(g.walletC.cacheList, c)
				g.walletC.cacheMap[wallet.Address] = struct{}{}
			}
		}
		if len(g.walletC.cacheList) >= g.walletC.maxSize {
			//fmt.Println("upserting wallets with len", len(g.walletC.cacheList))
			return g.upsertCache("wallet")
		}
		return nil
	default:
		return xerrors.Errorf("Upserting unknown type: %#+T", items)
	}
}

// Upsert items in the cache and reset the cache.
func (g *CDBGraph) upsertCache(cacheType string) error {
	switch cacheType {
	case "block":
		if err := g.batchUpsert(g.blockC.cacheList); err != nil {
			return err
		}
		g.blockC.cacheList = g.blockC.cacheList[:0] // reset without decreasing cap.
		for m := range(g.blockC.cacheMap) {
			delete(g.blockC.cacheMap, m)
		}
		return nil
	case "tx":
		if err := g.batchUpsert(g.txC.cacheList); err != nil {
			return err
		}
		g.txC.cacheList = g.txC.cacheList[:0] // reset without decreasing cap.
		for m := range(g.txC.cacheMap) {
			delete(g.txC.cacheMap, m)
		}
		return nil
	case "wallet":
		if err := g.batchUpsert(g.walletC.cacheList); err != nil {
			return err
		}
		g.walletC.cacheList = g.walletC.cacheList[:0] // reset without decreasing cap.
		for m := range(g.walletC.cacheMap) {
			delete(g.walletC.cacheMap, m)
		}
		return nil
	default:
		return xerrors.Errorf("Upserting unknown cache type: ", cacheType)
	}
}

func (g *CDBGraph) batchUpsert(xs interface{}) error {
	type slice struct {
		items interface{}
		startIx, endIx int
	}
	// Let <numWorkers> routines upsert items in batches.
	numWorkers := 6
	sliceCh := make(chan slice, 1)
	doneCh := make(chan struct{}, numWorkers)
	errCh := make(chan error, 1)

	// Function that will upsert.
	inserter := func() {
		defer func(){ doneCh <- struct{}{} }()
		for {
			slice, open := <- sliceCh
			if !open {
				// Channel was closed.
				return
			}
			switch items := slice.items.(type) {
			case []*graph.Block:
				if err := g.bulkUpsertBlocks(items[slice.startIx:slice.endIx]); err != nil {
					maybeEmitError(err, errCh)
					return
				}
			case []*graph.Tx:
				if err := g.bulkUpsertTxs(items[slice.startIx:slice.endIx]); err != nil {
					maybeEmitError(err, errCh)
					return
				}
			case []*graph.Wallet:
				if err := g.bulkUpsertWallets(items[slice.startIx:slice.endIx]); err != nil {
					maybeEmitError(err, errCh)
					return
				}
			}
		}
	}

	for i:=0; i < numWorkers; i++ {
		go inserter()
	}

	switch items := xs.(type) {
	case []*graph.Block:
		batchSize := 750
		for i:=0; i<len(items); i+=batchSize {
			batchSize = int(math.Min(float64(batchSize), float64(len(items)-i)))
			select {
			case err := <- errCh:
				return err
			case sliceCh <- slice{items: items, startIx: i, endIx: i+batchSize}:
			}
		}
	case []*graph.Tx:
		batchSize := 125
		for i:=0; i<len(items); i+=batchSize {
			batchSize = int(math.Min(float64(batchSize), float64(len(items)-i)))
			select {
			case err := <- errCh:
				return err
			case sliceCh <- slice{items: items, startIx: i, endIx: i+batchSize}:
			}
		}
	case []*graph.Wallet:
		batchSize := 250
		for i:=0; i<len(items); i+=batchSize {
			batchSize = int(math.Min(float64(batchSize), float64(len(items)-i)))
			select {
			case err := <- errCh:
				return err
			case sliceCh <- slice{items: items, startIx: i, endIx: i+batchSize}:
			}
		}
	default:
		return xerrors.Errorf("Upserting unknown type: %#+T", items)
	}

	close(sliceCh)
	for i:=0; i < numWorkers; i++ {
		select {
		 case <- doneCh:
		 case err := <- errCh:
			return err
		}
	}
	return nil
}

//Bulk insert blocks.
func (g *CDBGraph) bulkUpsertBlocks(blocks []*graph.Block) error {
	numArgs := 2 // Number of columns in the block table.
	valueStrings := make([]string, 0, len(blocks))
	valueArgs := make([]interface{}, 0, numArgs * len(blocks))
	for i,block := range blocks {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d)", i*numArgs+1, i*numArgs+2))
		valueArgs = append(valueArgs, block.Number)
		valueArgs = append(valueArgs, block.Processed)
	}
	stmt := fmt.Sprintf(`upsert INTO block("number", processed) VALUES %s;`, strings.Join(valueStrings, ","))
	_, err := g.db.Exec(stmt, valueArgs...)
	return err
}

func (g *CDBGraph) bulkUpsertTxs(txs []*graph.Tx) error {
	if len(txs) == 0 {
		return nil
	}
	numArgs := 9 // Number of columns in the tx table.
	valueArgs := make([]interface{}, 0, numArgs * len(txs))
	valueStrings := make([]string, 0, len(txs))

	for i,tx := range txs {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", i*numArgs+1, i*numArgs+2, i*numArgs+3, i*numArgs+4, i*numArgs+5, i*numArgs+6, i*numArgs+7, i*numArgs+8, i*numArgs+9))
		valueArgs = append(valueArgs, tx.Hash)
		valueArgs = append(valueArgs, tx.Status)
		valueArgs = append(valueArgs, tx.Block.String())
		valueArgs = append(valueArgs, tx.Timestamp.UTC())
		valueArgs = append(valueArgs, tx.From)
		valueArgs = append(valueArgs, tx.To)
		valueArgs = append(valueArgs, tx.Value.String())
		valueArgs = append(valueArgs, tx.TransactionFee.String())
		valueArgs = append(valueArgs, tx.Data)
	}
	stmt := fmt.Sprintf(`upsert INTO tx(hash, status, block, timestamp, "from", "to", value, transaction_fee, data) VALUES %s`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		if isForeignKeyViolationError(err) {
			err = graph.ErrUnknownAddress
		}
		return xerrors.Errorf("insert txs: %w", err)
	}
	return nil
}

func (g *CDBGraph) bulkUpsertWallets(wallets []*graph.Wallet) error {
	if len(wallets) == 0 {
		return nil
	}
	numArgs := 1 // Number of columns in the wallet table.
	valueArgs := make([]interface{}, 0, numArgs * len(wallets))
	valueStrings := make([]string, 0, len(wallets))

	for i,wallet := range wallets {
		if len(wallet.Address) != 40 {
			return xerrors.Errorf("upsert wallet: %w", graph.ErrInvalidAddress)
		}
		valueArgs = append(valueArgs, wallet.Address)
		valueStrings = append(valueStrings, fmt.Sprintf("($%d)", i*numArgs+1))
	}

	stmt := fmt.Sprintf(`upsert INTO wallet(address) VALUES %s`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		return xerrors.Errorf("insert wallets: %w", err)
	}
	return nil
}


// Looks up a wallet by its address.
func (g *CDBGraph) FindWallet(address string) (*graph.Wallet, error) {
	row := g.db.QueryRow(findWalletQuery, address)
	wallet := &graph.Wallet{Address: address}
	if err := row.Scan(&wallet.Address); err != nil {
		if err == sql.ErrNoRows {
			return nil, xerrors.Errorf("find wallet: %w", graph.ErrNotFound)
		}

		return nil, xerrors.Errorf("find wallet: %w", err)
	}

	return wallet, nil
}

// Returns a list of unprocessed blocks.
func (g *CDBGraph) getUnprocessedBlocks() ([]*graph.Block, error) {
	// First refresh the blocks.
	if err := g.refreshBlocks(); err != nil {
		return nil, err
	}

	rows, err := g.db.Query(unprocessedBlocksQuery)
	defer rows.Close()
	if err != nil {
		return nil, xerrors.Errorf("get unprocessed blocks: %w", err)
	}

	var list []*graph.Block
	for rows.Next() {
		block := new(graph.Block)
		if err := rows.Scan(&block.Number); err != nil {
			return nil, xerrors.Errorf("get unprocessed blocks, scanning: %w", err)
		}
		list = append(list, block)
	}

	return list, nil
}

// Checks for missing blocks in the graph and inserts all missing blocks, 
// so that every block from 1 to the largest found block are in the graph.
func (g *CDBGraph) refreshBlocks() error {
	var currentBlockNumber, maxBlockNumber int

	// Get all block numbers
	rows, err := g.db.Query(allBlockNumbersQuery)
	if err != nil {
		return xerrors.Errorf("refreshing blocks query: %w", err)
	}

	// Mark all returned block numbers & the max block number.
	seen := make(map[int]bool)
	maxBlockNumber = 0
	for rows.Next() {
		if err := rows.Scan(&currentBlockNumber); err != nil {
			return xerrors.Errorf("refreshing blocks scan: %w", err)
		}
		seen[currentBlockNumber] = true
		if currentBlockNumber > maxBlockNumber {
			maxBlockNumber = currentBlockNumber
		}
	}

	// Construct list of missing blocks.
	var blocks []*graph.Block
	for i:=1; i<maxBlockNumber; i++ {
		_, keyExists := seen[i]
		if ! keyExists {
			blocks = append(blocks, &graph.Block{Number: i, Processed: false})
		}
	}
	err = g.batchUpsert(blocks)
	fmt.Println("Bulk upserted blocks: ", len(blocks))
	return err
}

// Returns true if err indicates a foreign key constraint violation.
func isForeignKeyViolationError(err error) bool {
	pqErr, valid := err.(*pq.Error)
	if !valid {
		return false
	}
	return pqErr.Code.Name() == "foreign_key_violation"
}

func maybeEmitError(err error, errCh chan<- error) {
	select {
		case errCh <- err: // error emitted.
		default: // error channel is full with other errors.
	}
}
