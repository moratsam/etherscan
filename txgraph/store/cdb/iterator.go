package cdb

import (
	"database/sql"
	"database/sql/driver"
	"math/big"
	"time"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// txIterator is a graph.TxIterator implementation for the cbd graph.
type txIterator struct {
	rows			*sql.Rows
	lastErr		error
	latchedTx	*graph.Tx
}

type BigInt big.Int

type xx struct {
	// Unique hash.
	Hash string

	Status graph.TxStatus

	// Number of block when this transaction was mined
	Block *BigInt

	// Time when transaction got finished (either by a failure or success)
	Timestamp time.Time

	// Sender of transaction
	From string

	// Receiver of transaction
	To string

	// Amount eth sent in transaction, given in gwei
	Value *BigInt

	// Given in gwei
	TransactionFee *BigInt

	// Transaction data
	Data string
}


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

func (i *txIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	t := new(xx)
	i.lastErr = i.rows.Scan(
		&t.Hash,
		&t.Status,
		&t.Block,
		&t.Timestamp,
		&t.From,
		&t.To,
		&t.Value,
		&t.TransactionFee,
		&t.Data,
	)

	if i.lastErr != nil {
		return false
	}

	x := new(graph.Tx)
	x.Hash = t.Hash
	x.Status = t.Status
	x.Block = (*big.Int)(t.Block)
	x.Timestamp = t.Timestamp
	x.From = t.From
	x.To = t.To
	x.Value = (*big.Int)(t.Value)
	x.TransactionFee = (*big.Int)(t.TransactionFee)
	x.Data = ([]byte)(t.Data)
	i.latchedTx = x
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
	i.lastErr = i.rows.Scan(&w.Address, &w.Crawled)
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
