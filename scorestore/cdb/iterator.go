package cdb

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math/big"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/scorestore"
)

// scoreIterator is a scorestore.ScoreIterator implementation for the cbd scorestore.
type scoreIterator struct {
	rows				*sql.Rows
	totalRows		uint64
	lastErr			error
	latchedScore	*scorestore.Score
}

func (i *scoreIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	// Scan Value into the custom type with big.Float handling.
	helperValue := new(bigFloat)
	score := new(scorestore.Score)
	i.lastErr = i.rows.Scan(
		&score.Wallet,
		&score.Scorer,
		&helperValue,
	)

	if i.lastErr != nil {
		return false
	}
	score.Value = (*big.Float)(helperValue)


	i.latchedScore = score
	return true
}

func (i *scoreIterator) Error() error {
	return i.lastErr
}

func (i *scoreIterator) Close() error {
	err := i.rows.Close()
	if err != nil {
		return xerrors.Errorf("score iterator: %w", err)
	}
	return nil
}

func (i *scoreIterator) Score() *scorestore.Score {
	return i.latchedScore
}

func (i *scoreIterator) TotalCount() uint64 {
	return i.totalRows
}


// scorerIterator is a scorestore.ScorerIterator implementation for the cbd scorestore.
type scorerIterator struct {
	rows				*sql.Rows
	totalRows		uint64
	lastErr			error
	latchedScorer	*scorestore.Scorer
}

func (i *scorerIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	scorer := new(scorestore.Scorer)
	i.lastErr = i.rows.Scan(&scorer.Name)
	if i.lastErr != nil {
		return false
	}

	i.latchedScorer = scorer
	return true
}

func (i *scorerIterator) Error() error {
	return i.lastErr
}

func (i *scorerIterator) Close() error {
	err := i.rows.Close()
	if err != nil {
		return xerrors.Errorf("scorer iterator: %w", err)
	}
	return nil
}

func (i *scorerIterator) Scorer() *scorestore.Scorer {
	return i.latchedScorer
}

func (i *scorerIterator) TotalCount() uint64 {
	return i.totalRows
}


// This is a helper type used to define the custom Scan method required to retrieve
// big.Float data from the database.
type bigFloat big.Float

// Value implements the Valuer interface for bigFloat
func (b *bigFloat) Value() (driver.Value, error) {
   if b != nil {
      return (*big.Float)(b).String(), nil
   }
   return nil, nil
}

// Scan implements the Scanner interface for bigFloat
func (b *bigFloat) Scan(value interface{}) error {
	if value == nil {
		b = nil
	}
	switch t := value.(type) {
	case float64:
	 	(*big.Float)(b).SetFloat64(value.(float64))
	case []byte:
		_, err := fmt.Sscan(string(value.([]byte)), (*big.Float)(b))
		if err != nil {
			fmt.Println("error scanning value:", err)
		}
	default:
		return xerrors.Errorf("could not scan type %T into bigFloat", t)
	 }
	return nil
}
