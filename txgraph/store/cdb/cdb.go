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

	unprocessedBlocksQuery = `select "number" from block where processed=false limit 500000`

	walletsInPartitionQuery = `select address from wallet where address >= $1 and address < $2`

	walletTxsQuery = `select * from tx where "from"=$1`

	// Compile-time check for ensuring CDBGraph implements Graph.
	_ graph.Graph = (*CDBGraph)(nil)
)

// CDBGraph implements a graph that persists its transactions and wallets to 
// a cockroachdb instance.
type CDBGraph struct {
	db *sql.DB
	mu sync.RWMutex
	walletCache map[string]bool
}

// NewCDBGraph returns a CDBGraph instance that connects to the cockroachdb
// instance specified by dsn.
// It contains all blocks from block 1 to the current largest block that was inserted.
func NewCDBGraph(dsn string) (*CDBGraph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	g := &CDBGraph{
		db:				db,
		walletCache:	make(map[string]bool),
	}

	return g, g.fillWalletCache()
}

// Clears wallet cache (Used for testing and before Closing the CDBGraph).
func (g *CDBGraph) ClearWalletCache() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.walletCache = make(map[string]bool)
}

// Close terminates the connection to the backing cockroachdb instance.
func (g *CDBGraph) Close() error {
	g.ClearWalletCache()
	return g.db.Close()
}

func (g *CDBGraph) fillWalletCache() error {
	rows, err := g.db.Query(allWalletAddressesQuery)
	if err != nil {
		return xerrors.Errorf("allWalletAddressesQuery: %w", err)
	}
	defer rows.Close()

	var addr string
	for rows.Next() {
		if err := rows.Scan(&addr); err != nil {
			return xerrors.Errorf("fillWalletCache scan: %w", err)
		}
		g.walletCache[addr] = true
	}
	fmt.Println("Wallet cache size: ", len(g.walletCache))
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

func (g *CDBGraph) UpsertBlocks(blocks []*graph.Block) error {
	// Insert blocks in batches.
	batchSize := 10000
	for i:=0; i<len(blocks); i+=batchSize {
		batchSize = int(math.Min(float64(batchSize), float64(len(blocks)-i)))
		if err := g.bulkUpsertBlocks(blocks[i:i+batchSize]); err != nil {
			return err
		}
	}
	return nil
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

	//Mark all returned block numbers & the max block number.
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

	var blocks []*graph.Block
	for i:=1; i<maxBlockNumber; i++ {
		_, keyExists := seen[i]
		if ! keyExists {
			blocks = append(blocks, &graph.Block{Number: i, Processed: false})
		}
	}
	err = g.UpsertBlocks(blocks)
	fmt.Println("Bulk upserted blocks: ", len(blocks))
	return err
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

// Returns a BlockSubscriber connected to a stream of unprocessed blocks.
func (g *CDBGraph) Blocks() (graph.BlockIterator, error) {
	blocks, err := g.getUnprocessedBlocks()
	if err != nil {
		return nil, err
	}
	return &blockIterator{g: g, blocks: blocks}, nil
}

func (g *CDBGraph) bulkInsertTxs(txs []*graph.Tx) error {
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
	stmt := fmt.Sprintf(`insert INTO tx(hash, status, block, timestamp, "from", "to", value, transaction_fee, data) VALUES %s`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		stmt = fmt.Sprintf(`upsert INTO tx(hash, status, block, timestamp, "from", "to", value, transaction_fee, data) VALUES %s`, strings.Join(valueStrings, ","))
		if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
			return xerrors.Errorf("insert txs: %w", err)
		}
	}
	return nil
}

// InsertTxs inserts new transactions.
// Not the nicest code, constructing a raw bulk insert SQL statement.
func (g *CDBGraph) InsertTxs(txs []*graph.Tx) error {
	// Insert transactions in batches.
	batchSize := 500
	for i:=0; i<len(txs); i+=batchSize {
		batchSize = int(math.Min(float64(batchSize), float64(len(txs)-i)))
		if err := g.bulkInsertTxs(txs[i:i+batchSize]); err != nil {
			return err
		}
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

	stmt := fmt.Sprintf(`insert INTO wallet(address) VALUES %s`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		stmt = fmt.Sprintf(`upsert INTO wallet(address) VALUES %s`, strings.Join(valueStrings, ","))
		if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
			return xerrors.Errorf("insert wallets: %w", err)
		}
	}
	return nil
}


// Upserts wallets.
func (g *CDBGraph) UpsertWallets(wallets []*graph.Wallet) error {
	var newWallets []*graph.Wallet
	// Find new wallet addresses.
	g.mu.RLock()
	for _, wallet := range wallets {
		if _, exists := g.walletCache[wallet.Address]; !exists {
			newWallets = append(newWallets, wallet)
		}
	}
	g.mu.RUnlock()

	// Upsert new wallets in batches.
	batchSize := 500
	for i:=0; i<len(newWallets); i+=batchSize {
		batchSize = int(math.Min(float64(batchSize), float64(len(newWallets)-i)))
		if err := g.bulkUpsertWallets(newWallets[i:i+batchSize]); err != nil {
			return err
		}
	}

	//Add new wallet addresses to the cache.
	g.mu.Lock()
	for _, wallet := range newWallets {
		g.walletCache[wallet.Address] = true
	}
	g.mu.Unlock()

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

// Returns true if err indicates a foreign key constraint violation.
func isForeignKeyViolationError(err error) bool {
	pqErr, valid := err.(*pq.Error)
	if !valid {
		return false
	}
	return pqErr.Code.Name() == "foreign_key_violation"
}
