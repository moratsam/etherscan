package cdb

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/scorestore"
)

var (
	upsertScoreQuery = `insert into score(wallet, scorer, value, timestamp) values ($1, $2, $3, $4) on conflict (wallet, scorer) do update set value=$3, timestamp=$4`

	upsertScorerQuery = `insert into scorer(name) values ($1) on conflict (name) do nothing`

	scoreQuery = `select * from score where scorer=$1`

	scorerQuery = `select * from scorer`

	// Compile-time check for ensuring CDBScoreStore implements ScoreStore.
	_ graph.ScoreStore = (*CDBScoreStore)(nil)
)

// CDBScoreStore implements a score store that persists the scores and scorers to 
// a cockroachdb instance.
type CDBScoreStore struct {
	db *sql.DB
}

// NewCDBScoreStore returns a CDBScoreStore instance that connects to
// the cockroachdb instance specified by dsn.
func NewCDBScoreStore(dsn string) (*CDBScoreStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &CDBScoreStore{db: db}, nil
}

// Close terminates the connection to the backing cockroachdb instance.
func (ss *CDBScoreStore) Close() error {
	return ss.db.Close()
}

// Upserts a Score.
// On conflict of (wallet, scorer), the timestamp and value will be updated.
func (ss *CDBScoreStore) UpsertScore(score *scorestore.Score) error {
	if _, err := ss.db.Exec(
		upsertScoreQuery, 
		score.Wallet, 
		score.Scorer, 
		score.Value, 
		score.Timestamp
	); err != nil {
		return xerrors.Errorf("upsert score: %w", err)
	}
	return nil
}

// Upserts a Scorer.
func (ss *CDBScoreStore) UpsertScorer(scorer *scorestore.Scorer) error {
	if _,err := ss.db.Exec(upsertScorerQuery, scorer.Name); err != nil {
		return xerrors.Errorf("upsert scorer: %w", err)
	}
	return nil
}

// Returns an iterator for all scorers.
func (ss *CDBScoreStore) Scorers() (scorestore.ScorerIterator, error) {
	rows, err := ss.db.Query(stmt, query.Expression)
	if err != nil {
		return nil, xerrors.Errorf("scorers: %w", err)
	}
	return &scorerIterator{rows: rows}, nil
}


// Search the ScoreStore by a particular query and return a result iterator.
func (ss *CDBScoreStore) Search(query scorestore.Query) (scorestore.Iterator, error) {
	var stmt string
	switch t := query.Type {
	case scorestore.QueryTypeScorer:
		stmt = scoreQuery	
	default:
		return nil, xerrors.New("search: unknown query type: ", t.(type))
	}

	rows, err := ss.db.Query(stmt, query.Expression)
	if err != nil {
		return nil, xerrors.Errorf("search with expression: %s : %w", query.Expression, err)
	}
	return &scoreIterator{rows: rows}, nil
}
