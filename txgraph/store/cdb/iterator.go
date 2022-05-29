package cdb

import (
	"database/sql"
	"database/sql/driver"
	"math/big"
	"sync"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// blockIterator is a graph.BlockIterator implementation for the cdb graph.
type blockIterator struct {
	g *CDBGraph

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
		return false
	}

	i.curIndex++
	return true
}

func (i *blockIterator) Error() error {
	return i.lastErr
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

	// Scan some data into a helper type with custom bigInt handling.
	helperBlock := new(bigInt)
	helperValue := new(bigInt)
	helperTransactionFee := new(bigInt)
	t := new(graph.Tx)
	i.lastErr = i.rows.Scan(
		&t.Hash,
		&t.Status,
		&helperBlock,
		&t.Timestamp,
		&t.From,
		&t.To,
		&helperValue,
		&helperTransactionFee,
		&t.Data,
	)

	if i.lastErr != nil {
		return false
	}

	t.Timestamp = t.Timestamp.UTC()
	// Move data from the helpers into the graph.Tx and return that.
	t.Block = (*big.Int)(helperBlock)
	t.Value = (*big.Int)(helperValue)
	t.TransactionFee = (*big.Int)(helperTransactionFee)

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
type bigInt big.Int

// Value implements the Valuer interface for bigInt
func (b *bigInt) Value() (driver.Value, error) {
   if b != nil {
      return (*big.Int)(b).String(), nil
   }
   return nil, nil
}

// Scan implements the Scanner interface for bigInt
func (b *bigInt) Scan(value interface{}) error {
	if value == nil {
		b = nil
	}
	switch t := value.(type) {
	case int64:
	 	(*big.Int)(b).SetInt64(value.(int64))
	case []byte:
		_, ok := (*big.Int)(b).SetString(string(value.([]byte)), 10)
		if !ok {
			return xerrors.Errorf("failed to load value to []byte: %v", value)
		}
	default:
		return xerrors.Errorf("could not scan type %T into bigInt", t)
	 }
	return nil
}
