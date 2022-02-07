package cdb

import (
	"database/sql"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

var (
	insertTxQuery =`
insert into tx(hash, status, block, timestamp, "from", "to", value, transaction_fee, 
data) values ($1, $2, $3, $4, $5, $6, $7, $8, $9) on conflict(hash) do update set hash=$1 
returning hash`
  
  upsertWalletQuery = `
insert into wallet(address, crawled) values ($1, $2) on conflict (address) do 
update set crawled=$2 returning crawled`

  findWalletQuery = "select crawled from wallet where address=$1"

  walletsInPartitionQuery = `
select address, crawled from wallet where address >= $1 and address < $2`

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
func NewCockroachDBGraph(dsn string) (*CockroachDbGraph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &CockroachDbGraph{db: db}, nil
}

// Close terminates the connection to the backing cockroachdb instance.
func (c *CockroachDbGraph) Close() error {
	return c.db.Close()
}

//InsertTx inserts a new transaction
func (c *CockroachDbGraph) InsertTx(tx *graph.Tx) error {
	row := c.db.QueryRow(
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
func (c *CockroachDbGraph) UpsertWallet(wallet *graph.Wallet) error {
	if len(wallet.Address) != 40 {
		return xerrors.Errorf("upsert wallet: %w", graph.ErrInvalidAddress)
	}
	// In case wallet already exists in the graph and has Crawled field set to true,
	// make sure this is not overwritten by this upsert.
	crawled := wallet.Crawled
	existing, _ := c.FindWallet(wallet.Address)
	if existing != nil && existing.Crawled {
		crawled = true	
	}
	row := c.db.QueryRow(upsertWalletQuery, wallet.Address, crawled)
	if err := row.Scan(&wallet.Crawled); err != nil {
		return xerrors.Errorf("upsert wallet: %w", err)
	}
	return nil
}

// Looks up a wallet by its address.
func (c *CockroachDbGraph) FindWallet(address string) (*graph.Wallet, error) {
	row := c.db.QueryRow(findWalletQuery, address)
	wallet := &graph.Wallet{Address: address}
	if err := row.Scan(&wallet.Crawled); err != nil {
		if err == sql.ErrNoRows {
			return nil, xerrors.Errorf("find wallet: %w", graph.ErrNotFound)
		}

		return nil, xerrors.Errorf("find wallet: %w", err)
	}

	return wallet, nil
}

// Returns an iterator for the set of wallets
// whose address is in the [fromAddr, toAddr) range.
func (c *CockroachDbGraph) Wallets(fromAddr, toAddr string) (graph.WalletIterator, error) {
	rows, err := c.db.Query(walletsInPartitionQuery, fromAddr, toAddr)
	if err != nil {
		return nil, xerrors.Errorf("wallets: %w", err)
	}
	return &walletIterator{rows: rows}, nil
}

// Returns an iterator for the set of transactions whose from or to field equals to address.
func (c *CockroachDbGraph) WalletTxs(address string) (graph.TxIterator, error) {
	rows, err := c.db.Query(walletTxsQuery, address)
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
