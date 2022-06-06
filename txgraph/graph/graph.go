package graph

import (
	"math/big"
	"time"
)

// Is implemented by graph objects that can be iterated.
type Iterator interface {
	// Advances the iterator. If no more items are available or an error occurs,
	// calls to Next() return false.
	Next() bool
	// Returns the last error encountered by the iterator.
	Error() error
	// Releases any resources associated with the iterator.
	Close() error
}

type BlockIterator interface {
	Iterator

	// Returns the currently fetched block.
	Block() *Block
}

// Is implemented by objects that can iterate the graph transactions.
type TxIterator interface {
	Iterator

	// Returns the currently fetched transaction.
	Tx() *Tx
}

// Is implemented by objects that can iterate the graph wallets.
type WalletIterator interface {
	Iterator

	// Returns the currently fetched wallet.
	Wallet() *Wallet
}

// Encapsulates all information about a block.
type Block struct {
	// The eth block number.
	Number int

	// Turns true after all transactions in the block have been processed by the pipeline
	// and stored in the graph.
	Processed bool
}

// Describes the different transaction statuses
type TxStatus uint8
const (
	Fail TxStatus = iota
	Success
	Unknown
)

// Encapsulates all information about a transaction processed by the etherscan pipeline.
type Tx struct {
	// Unique hash.
	Hash string

	Status TxStatus

	// Number of block when this transaction was mined
	Block *big.Int

	// Time when transaction got finished (either by a failure or success)
	Timestamp time.Time

	// Sender of transaction
	From string

	// Receiver of transaction
	To string

	// Amount eth sent in transaction, given in gwei
	Value *big.Int

	// Given in gwei
	TransactionFee *big.Int

	// Transaction data
	Data []byte
}

// Encapsulates all information about a wallet.
type Wallet struct {
	// Unique address
	Address string
}

// Graph is implemented by objects that can mutate or query a tx graph.
type Graph interface {
	// Returns an iterator for unprocessed blocks (only about 1/2 * 10^6 at a time.
	Blocks() (BlockIterator, error)

	// Creates new blocks or updates existing ones.
	UpsertBlocks(blocks []*Block) error

	// Inserts new transactions.
	InsertTxs(tx []*Tx) error

	// Creates new wallets or updates existing ones.
	UpsertWallets(wallets []*Wallet) error

	// Upsert any items (blocks, wallets, txs).
	Upsert(items []interface{}) error

	// Looks up a wallet by its address.
	// Note: Currently not being used.
	FindWallet(address string) (*Wallet, error)

	// Returns an iterator for the set of wallets that belong in the
	// [fromAddress, toAddress) range.
	Wallets(fromAddress, toAddress string) (WalletIterator, error)

	// Returns an iterator for the set of transactions originating from a wallet.
	WalletTxs(address string) (TxIterator, error)
}
