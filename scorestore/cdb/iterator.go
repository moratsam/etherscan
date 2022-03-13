package cdb

import (
	"database/sql"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/scorestore"
)

// scoreIterator is a scorestore.ScoreIterator implementation for the cbd scorestore.
type scoreIterator struct {
	rows				*sql.Rows
	lastErr			error
	latchedScore	*scorestore.Score
}

func (i *scoreIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	var id int64
	score := new(scorestore.Score)
	i.lastErr = i.rows.Scan(
		&id,
		&score.Wallet,
		&score.Scorer,
		&score.Value,
	)

	if i.lastErr != nil {
		return false
	}

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


// scorerIterator is a scorestore.ScorerIterator implementation for the cbd scorestore.
type scorerIterator struct {
	rows				*sql.Rows
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
