package scorestore

import (
	"time"
)

type ScoreStore interface {
	// Upsert a score.
	UpsertScore(score *Score) error

	// Insert a new scorer. The scorer's field Id will be populated by the function call.
	UpsertScorer(scorer *Scorer) error

	// Get all scorers from the store.
	Scorers() ([]*Scorer, error)

	// Search the ScoreStore for a particular and return a result iterator.
	// This is intentionally vague, to allow for easy expansion in the future.
	Search(query Query) (Iterator, error)
}

// The value that a scorer outputs for a wallet at a given time.
type Score struct {
	Wallet string
	Scorer int
	Value float64
	Timestamp time.Time
}

type Scorer struct {
	Id int
	Name string
}

// QueryType describes the types of queries supported by the ScoreStore implementation.
type QueryType uint8

const (
	//QueryScorer requests from the score store all scores pertaining to a specified scorer.
	QueryScorer QueryType = iota
)

// Query encapsulates a set of parameters to use when searching the score store.
// This is intentionally vague, to allow for easy expansion in the future.
type Query struct {
	// The way that the ScoreStore should interpret the search expression.
	Type QueryType

	// The search expression.
	Expression string

	// The number of search results to skip.
	Offset uint64
}

// Iterator is implemented by objects that can paginate search results.
type Iterator interface {
	// Close the iterator and release any allocated resources.
	Close() error

	// Next loads the next Score matching the search query.
	// It returns false if no more scores are available.
	Next() bool

	// Error returns the last error encountered by the iterator.
	Error() error

	// Score returns the current score from the resuls set.
	Score() *Score

	// TotalCount returns the approximate number of search results.
	TotalCount() uint64
}

