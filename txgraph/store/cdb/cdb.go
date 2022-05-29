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
	allBlockNumbersQuery = `select "number" from block order by "number" desc`

	unprocessedBlocksQuery = `select "number" from block where processed=false limit 500000`

	findBlockQuery = `select "number", processed from block where "number"=$1`

	upsertBlockQuery = `
upsert into block("number", processed) values ($1, $2) returning processed`

  findWalletQuery = "select address from wallet where address=$1"

  walletsInPartitionQuery = `select address from wallet where address >= $1 and address < $2`

  walletTxsQuery = `select * from tx where "from"=$1 or "to"=$1`

	// Compile-time check for ensuring CDBGraph implements Graph.
	_ graph.Graph = (*CDBGraph)(nil)
)

// CDBGraph implements a graph that persists its transactions and wallets to 
// a cockroachdb instance.
type CDBGraph struct {
	db *sql.DB
}

// NewCDBGraph returns a CDBGraph instance that connects to the cockroachdb
// instance specified by dsn.
// It contains all blocks from block 1 to the current largest block that was inserted.
func NewCDBGraph(dsn string) (*CDBGraph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &CDBGraph{db: db}, nil
}

// Close terminates the connection to the backing cockroachdb instance.
func (g *CDBGraph) Close() error {
	return g.db.Close()
}

//Bulk insert blocks whose numbers are in blockNumbers array.
func (g *CDBGraph) bulkInsertBlocks(blockNumbers []int) error {
	numArgs := 2 // Number of columns in the block table.
	valueStrings := make([]string, 0, len(blockNumbers))
	valueArgs := make([]interface{}, 0, numArgs * len(blockNumbers))
	for i,blockNumber := range blockNumbers {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d)", i*numArgs+1, i*numArgs+2))
		valueArgs = append(valueArgs, blockNumber)
		valueArgs = append(valueArgs, false)
	}
	stmt := fmt.Sprintf(`upsert INTO block("number", processed) VALUES %s;`, strings.Join(valueStrings, ","))
	_, err := g.db.Exec(stmt, valueArgs...)
	return err
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

	// Insert missing blocks in batches.
	blocksInsertedCnt := 0
	batchSize := 10000
	var batch []int
	for i:=1; i<maxBlockNumber; i++ {
		_, keyExists := seen[i]
		if ! keyExists {
			batch = append(batch, i)
			blocksInsertedCnt++
		}
		if len(batch) == batchSize {
			if err := g.bulkInsertBlocks(batch); err != nil {
				return xerrors.Errorf("refreshing blocks bulk insert: %w", err)
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := g.bulkInsertBlocks(batch); err != nil {
			return xerrors.Errorf("refreshing blocks bulk insert: %w", err)
		}
	}

	fmt.Printf("Bulk inserted %d blocks\n", blocksInsertedCnt)
	return nil
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

// Upserts a Block.
// Once the Processed field of a block equals true, it cannot be changed to false.
func (g *CDBGraph) UpsertBlock(block *graph.Block) error {
	// In case block already exists in the graph and has Processed field set to true,
	// make sure this is not overwritten by this upsert.
	var processed bool
	row := g.db.QueryRow(findBlockQuery, fmt.Sprintf("%x", block.Number))
	row.Scan(&processed)
	if !processed {
		processed = block.Processed
	}

	row = g.db.QueryRow(upsertBlockQuery, block.Number, processed)
	if err := row.Scan(&block.Processed); err != nil {
		return xerrors.Errorf("upsert block: %w", err)
	}
	return nil
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
	stmt := fmt.Sprintf(`upsert INTO tx(hash, status, block, timestamp, "from", "to", value, transaction_fee, data) VALUES %s`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		if isForeignKeyViolationError(err) {
			err = graph.ErrUnknownAddress
		}
		return xerrors.Errorf("insert txs: %w", err)
	}
	return nil
}

// InsertTxs inserts new transactions.
// Not the nicest code, constructing a raw bulk insert SQL statement.
func (g *CDBGraph) InsertTxs(txs []*graph.Tx) error {
	// Insert transactions in batches.
	batchSize := 500
	var i int
	for i=0; i+batchSize<len(txs); i+=batchSize {
		if err := g.bulkInsertTxs(txs[i:i+batchSize]); err != nil {
			return err
		}
	}
	if i < len(txs) {
		if err := g.bulkInsertTxs(txs[i:]); err != nil {
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

	stmt := fmt.Sprintf(`upsert INTO wallet(address) VALUES %s`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		return xerrors.Errorf("insert wallets: %w", err)
	}
	return nil
}


// Upserts wallets.
func (g *CDBGraph) UpsertWallets(wallets []*graph.Wallet) error {
	// Upsert wallets in batches.
	batchSize := 1000
	var i int
	for i=0; i+batchSize<len(wallets); i+=batchSize {
		if err := g.bulkUpsertWallets(wallets[i:i+batchSize]); err != nil {
			return err
		}
	}
	if i < len(wallets) {
		if err := g.bulkUpsertWallets(wallets[i:]); err != nil {
			return err
		}
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

// Returns an iterator for the set of wallets
// whose address is in the [fromAddr, toAddr) range.
func (g *CDBGraph) Wallets(fromAddr, toAddr string) (graph.WalletIterator, error) {
	rows, err := g.db.Query(walletsInPartitionQuery, fromAddr, toAddr)
	if err != nil {
		return nil, xerrors.Errorf("wallets: %w", err)
	}
	return &walletIterator{rows: rows}, nil
}

// Returns an iterator for the set of transactions whose from or to field equals to address.
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
