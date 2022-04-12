package scorestore

import "math/big"

type ScoreStore interface {
	// Upsert a score.
	// On conflict of (wallet, scorer), the value will be updated.
	UpsertScore(score *Score) error

	// Upsert a scorer.
	UpsertScorer(scorer *Scorer) error

	// Returns an iterator for all the scorers.
	Scorers() (ScorerIterator, error)

	// Search the ScoreStore by a particular query and return a result iterator.
	// This is intentionally vague, to allow for easy expansion in the future.
	Search(query Query) (ScoreIterator, error)
}

// The value that a scorer outputs for a wallet at a given time.
type Score struct {
	Wallet string
	Scorer string
	Value *big.Float
}

type Scorer struct {
	Name string
}

// QueryType describes the types of queries supported by the ScoreStore implementation.
type QueryType uint8

const (
	//QueryTypeScorer requests from the score store all scores pertaining to a specified scorer.
	QueryTypeScorer QueryType = iota
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

// ScoreIterator is implemented by objects that can paginate search results.
type ScoreIterator interface {
	// Close the iterator and release any allocated resources.
	Close() error

	// Next loads the next Score matching the search query.
	// It returns false if no more scores are available.
	Next() bool

	// Error returns the last error encountered by the iterator.
	Error() error

	// Score returns the current score from the resuls set.
	Score() *Score
}

// ScorerIterator is implemented by objects that can iterate scorers.
type ScorerIterator interface {
	// Close the iterator and release any allocated resources.
	Close() error

	// Next loads the next Scorer.
	// It returns false if no more scorers are available.
	Next() bool

	// Error returns the last error encountered by the iterator.
	Error() error

	// Scorer returns the current scorer.
	Scorer() *Scorer
}
