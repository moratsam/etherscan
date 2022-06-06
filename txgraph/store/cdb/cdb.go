package cdb

import (
	"database/sql"
	"fmt"
	"strings"

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

	db *sql.DB
)

// CDBGraph implements a graph that persists its transactions and wallets to 
// a cockroachdb instance.
type CDBGraph struct {
	c				multiCache
}

// NewCDBGraph returns a CDBGraph instance that connects to the cockroachdb
// instance specified by dsn.
// To optimise performance and the load on the DB it may cache items before inserts.
// Upserting from the block cache always flushes other caches to ensure that if a block
// gets marked as processed in the DB, all its wallets and txs also get written to disk.
func NewCDBGraph(dsn string, usesCache bool) (*CDBGraph, error) {
	fmt.Println("Caching enabled:", usesCache)
	var err error  
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if usesCache {
		return &CDBGraph{
			c: newMultiCache(3, newDefaultCacheCfg()),
		}, nil
	} else {
		return &CDBGraph{
			c: newMultiCache(1, newNoCachingCfg()),
		}, nil
	}
}

// Close terminates the connection to the backing cockroachdb instance.
func (g *CDBGraph) Close() error {
	return db.Close()
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
	rows, err := db.Query(walletsInPartitionQuery, fromAddr, toAddr)
	if err != nil {
		return nil, xerrors.Errorf("wallets: %w", err)
	}
	return &walletIterator{rows: rows}, nil
}

// Returns an iterator for the set of transactions originating from a wallet address.
func (g *CDBGraph) WalletTxs(address string) (graph.TxIterator, error) {
	rows, err := db.Query(walletTxsQuery, address)
	if err != nil {
		return nil, xerrors.Errorf("walletTxs: %w", err)
	}
	return &txIterator{rows: rows}, nil
}

func (g *CDBGraph) Upsert(items []interface{}) error {
	return g.c.insert(toListOfCacheItems(items))
}

func (g *CDBGraph) UpsertBlocks(list []*graph.Block) error {
	return g.Upsert([]interface{}{list})
}

// InsertTxs inserts new transactions.
func (g *CDBGraph) InsertTxs(list []*graph.Tx) error {
	return g.Upsert([]interface{}{list})
}

func (g *CDBGraph) UpsertWallets(list []*graph.Wallet) error {
	return g.Upsert([]interface{}{list})
}

//Bulk insert blocks.
func bulkUpsertBlocks(blocks []*block) error {
	numArgs := 2 // Number of columns in the block table.
	valueStrings := make([]string, 0, len(blocks))
	valueArgs := make([]interface{}, 0, numArgs * len(blocks))
	for i,block := range blocks {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d)", i*numArgs+1, i*numArgs+2))
		valueArgs = append(valueArgs, block.Number)
		valueArgs = append(valueArgs, block.Processed)
	}
	stmt := fmt.Sprintf(`upsert INTO block("number", processed) VALUES %s;`, strings.Join(valueStrings, ","))
	_, err := db.Exec(stmt, valueArgs...)
	return err
}

func bulkUpsertTxs(txs []*tx) error {
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
	if _, err := db.Exec(stmt, valueArgs...); err != nil {
		if isForeignKeyViolationError(err) {
			err = graph.ErrUnknownAddress
		}
		return xerrors.Errorf("insert txs: %w", err)
	}
	return nil
}

func bulkUpsertWallets(wallets []*wallet) error {
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
	if _, err := db.Exec(stmt, valueArgs...); err != nil {
		return xerrors.Errorf("insert wallets: %w", err)
	}
	return nil
}

// Looks up a wallet by its address.
func (g *CDBGraph) FindWallet(address string) (*graph.Wallet, error) {
	row := db.QueryRow(findWalletQuery, address)
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

	rows, err := db.Query(unprocessedBlocksQuery)
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
	rows, err := db.Query(allBlockNumbersQuery)
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

	// Upsert.
	err = g.Upsert([]interface{}{blocks})
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
