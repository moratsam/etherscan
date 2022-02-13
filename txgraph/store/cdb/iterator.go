package cdb

import (
	"database/sql"
	"database/sql/driver"
	"math/big"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// blockIterator is a graph.BlockIterator implementation for the cdb graph.
type blockIterator struct {
	g *CockroachDbGraph

	mu sync.RWMutex

	blocks []*graph.Block
	curIndex int

	lastErr error
}

func (i *blockIterator) Next() bool {
	if i.lastErr != nil {
		return false
	}

	//Wait for new blocks to come in.
	if i.curIndex >= len(i.blocks) {
		if ok := i.refresh(); !ok {
			return false
		}
	}

	i.curIndex++
	return true
}

func (i *blockIterator) Error() error {
	return i.lastErr
}

// Wait for new blocks to come in.
// Return false if an error occurred while getting unprocessed blocks.
func (i *blockIterator) refresh() bool{
	i.mu.Lock()
	defer i.mu.Unlock()
	for ; len(i.blocks) < 1; {
		blocks, err := i.g.getUnprocessedBlocks()
		if err != nil {
			i.lastErr = err
			return false
		}
		i.blocks = blocks
	}
	i.curIndex = 0
	return true
}

func (i *blockIterator) Close() error {
	return nil
}

func (i *blockIterator) Block() *graph.Block {
	i.mu.RLock()
	defer i.mu.RUnlock()
	block := new(graph.Block)
	*block = *i.blocks[i.curIndex-1]
	return block
}


// txIterator is a graph.TxIterator implementation for the cbd graph.
type txIterator struct {
	rows			*sql.Rows
	lastErr		error
	latchedTx	*graph.Tx
}

func (i *txIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	// Scan data into a helper Transaction with custom BigInt handling.
	helper := new(helperTx)
	i.lastErr = i.rows.Scan(
		&helper.Hash,
		&helper.Status,
		&helper.Block,
		&helper.Timestamp,
		&helper.From,
		&helper.To,
		&helper.Value,
		&helper.TransactionFee,
		&helper.Data,
	)

	if i.lastErr != nil {
		return false
	}

	// Move data from the helper tx into the graph.Tx and return that.
	// TODO figure out if there's a better way of doing this.
	t := new(graph.Tx)
	t.Hash = helper.Hash
	t.Status = helper.Status
	t.Block = (*big.Int)(helper.Block)
	t.Timestamp = helper.Timestamp.UTC()
	t.From = helper.From
	t.To = helper.To
	t.Value = (*big.Int)(helper.Value)
	t.TransactionFee = (*big.Int)(helper.TransactionFee)
	t.Data = helper.Data

	i.latchedTx = t
	return true
}

func (i *txIterator) Error() error {
	return i.lastErr
}

func (i *txIterator) Close() error {
	err := i.rows.Close()
	if err != nil {
		return xerrors.Errorf("tx iterator: %w", err)
	}
	return nil
}

func (i *txIterator) Tx() *graph.Tx {
	return i.latchedTx
}


// walletIterator is a graph.WalletIterator implementation for the cbd graph.
type walletIterator struct {
	rows				*sql.Rows
	lastErr			error
	latchedWallet	*graph.Wallet
}

func (i *walletIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	w := new(graph.Wallet)
	i.lastErr = i.rows.Scan(&w.Address)
	if i.lastErr != nil {
		return false
	}

	i.latchedWallet = w
	return true
}

func (i *walletIterator) Error() error {
	return i.lastErr
}

func (i *walletIterator) Close() error {
	err := i.rows.Close()
	if err != nil {
		return xerrors.Errorf("wallet iterator: %w", err)
	}
	return nil
}

func (i *walletIterator) Wallet() *graph.Wallet {
	return i.latchedWallet
}

// This is a helper type used to define the custom Scan method required to retrieve
// Big.Int data from the database.
type BigInt big.Int

// Value implements the Valuer interface for BigInt
func (b *BigInt) Value() (driver.Value, error) {
   if b != nil {
      return (*big.Int)(b).String(), nil
   }
   return nil, nil
}

// Scan implements the Scanner interface for BigInt
func (b *BigInt) Scan(value interface{}) error {
	if value == nil {
		b = nil
	}
	switch t := value.(type) {
	case int64:
	 	(*big.Int)(b).SetInt64(value.(int64))
	case []uint8:
		_, ok := (*big.Int)(b).SetString(string(value.([]uint8)), 10)
		if !ok {
			return xerrors.Errorf("failed to load value to []uint8: %v", value)
		}
	default:
		return xerrors.Errorf("could not scan type %T into BigInt", t)
	 }
	return nil
}

// This is equivalent to the graph.Tx, except it contains BigInt instead of big.Int
// Data from the database is scanned into this before being moved into an actual graph.Tx.
type helperTx struct {
	Hash string
	Status graph.TxStatus
	Block *BigInt
	Timestamp time.Time
	From string
	To string
	Value *BigInt
	TransactionFee *BigInt
	Data []byte
}

