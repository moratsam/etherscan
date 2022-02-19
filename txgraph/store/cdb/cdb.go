package cdb

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

var (
	allBlockNumbersQuery = `select "number" from block order by "number" desc`

	unprocessedBlocksQuery = `select "number" from block where processed=false`

	findBlockQuery = `select "number", processed from block where "number"=$1`

	upsertBlockQuery = `
insert into block("number", processed) values ($1, $2) on conflict ("number") do
update set processed=$2 returning processed`

	insertTxQuery = `
insert into tx(hash, status, block, timestamp, "from", "to", value, transaction_fee, 
data) values ($1, $2, $3, $4, $5, $6, $7, $8, $9) on conflict(hash) do update set hash=$1 
returning hash`
  
  upsertWalletQuery = `insert into wallet(address) values ($1)
on conflict (address) do nothing returning address`

  findWalletQuery = "select address from wallet where address=$1"

  walletsInPartitionQuery = `select address from wallet where address >= $1 and address < $2`

  walletTxsQuery = `
select hash, status, block, timestamp, "from", "to", value, transaction_fee, data 
from tx where "from"=$1 or "to"=$1`

	// Compile-time check for ensuring CockroachDbGraph implements Graph.
	_ graph.Graph = (*CockroachDbGraph)(nil)
)

// CockroachDbGraph implements a graph that persists its transactions and wallets to 
// a cockroachdb instance.
type CockroachDbGraph struct {
	db *sql.DB
}

// NewCockroachDBGraph returns a CockroachDbGraph instance that connects to the cockroachdb
// instance specified by dsn.
// It contains all blocks from block 1 to the current largest block that was inserted.
func NewCockroachDBGraph(dsn string) (*CockroachDbGraph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &CockroachDbGraph{db: db}, nil
}

// Close terminates the connection to the backing cockroachdb instance.
func (g *CockroachDbGraph) Close() error {
	return g.db.Close()
}

// Checks for missing blocks in the graph and inserts all missing blocks, 
// so that every block from 1 to the largest found block are in the graph.
func (g *CockroachDbGraph) refreshBlocks() error {
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

	// Insert missing blocks
	for i:=1; i<maxBlockNumber; i++ {
		_, keyExists := seen[i]
		if ! keyExists {
			if err := g.UpsertBlock(&graph.Block{Number: i}); err != nil {
				return xerrors.Errorf("refreshing blocks upsert: %w", err)
			}
		}
	}
	return nil
}

// Returns a list of unprocessed blocks.
func (g *CockroachDbGraph) getUnprocessedBlocks() ([]*graph.Block, error) {
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
func (g *CockroachDbGraph) Blocks() (graph.BlockIterator, error) {
	blocks, err := g.getUnprocessedBlocks()
	if err != nil {
		return nil, err
	}
	return &blockIterator{g: g, blocks: blocks}, nil
}

// Upserts a Block.
// Once the Processed field of a block equals true, it cannot be changed to false.
func (g *CockroachDbGraph) UpsertBlock(block *graph.Block) error {
	// In case block already exists in the graph and has Processed field set to true,
	// make sure this is not overwritten by this upsert.
	var processed bool
	row := g.db.QueryRow(findBlockQuery, fmt.Sprintf("%x", block.Number))
	row.Scan(&processed)
	if !processed {
		processed = block.Processed
	}

	fmt.Println("Upserted block", block.Number)

	row = g.db.QueryRow(upsertBlockQuery, block.Number, processed)
	if err := row.Scan(&block.Processed); err != nil {
		return xerrors.Errorf("upsert block: %w", err)
	}
	return nil
}


//InsertTx inserts a new transaction.
func (g *CockroachDbGraph) InsertTx(tx *graph.Tx) error {
	row := g.db.QueryRow(
		insertTxQuery,
		tx.Hash,
		tx.Status,
		tx.Block.String(),
		tx.Timestamp.UTC(),
		tx.From,
		tx.To,
		tx.Value.String(),
		tx.TransactionFee.String(),
		tx.Data,
	)
	if err := row.Scan(&tx.Hash); err != nil {
		if isForeignKeyViolationError(err) {
			err = graph.ErrUnknownAddress
		}
		return xerrors.Errorf("insert tx: %w", err)
	}
	return nil
}

// Upserts a wallet.
func (g *CockroachDbGraph) UpsertWallet(wallet *graph.Wallet) error {
	if len(wallet.Address) != 40 {
		return xerrors.Errorf("upsert wallet: %w", graph.ErrInvalidAddress)
	}
	row := g.db.QueryRow(upsertWalletQuery, wallet.Address)
	if err := row.Scan(&wallet.Address); err != nil {
		return xerrors.Errorf("upsert wallet: %w", err)
	}
	return nil
}

// Looks up a wallet by its address.
func (g *CockroachDbGraph) FindWallet(address string) (*graph.Wallet, error) {
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
func (g *CockroachDbGraph) Wallets(fromAddr, toAddr string) (graph.WalletIterator, error) {
	rows, err := g.db.Query(walletsInPartitionQuery, fromAddr, toAddr)
	if err != nil {
		return nil, xerrors.Errorf("wallets: %w", err)
	}
	return &walletIterator{rows: rows}, nil
}

// Returns an iterator for the set of transactions whose from or to field equals to address.
func (g *CockroachDbGraph) WalletTxs(address string) (graph.TxIterator, error) {
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
