package cdb

import (
	"database/sql"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/scorestore"
)

var (
	upsertScoreQuery = `insert into score(wallet, scorer, value) values ($1, $2, $3) on conflict (wallet, scorer) do update set value=$3`

	upsertScorerQuery = `insert into scorer(name) values ($1) on conflict (name) do nothing`

	scoreQuery = `select wallet,scorer,value from score where scorer=$1`

	scorerQuery = `select * from scorer`

	totalScoresQuery = `select count(*) from score where scorer=$1`

	totalScorersQuery = `select count(*) from scorer`

	// Compile-time check for ensuring CDBScoreStore implements ScoreStore.
	_ scorestore.ScoreStore = (*CDBScoreStore)(nil)
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
// On conflict of (wallet, scorer), the value will be updated.
func (ss *CDBScoreStore) UpsertScore(score *scorestore.Score) error {
	if _, err := ss.db.Exec(
		upsertScoreQuery, 
		score.Wallet, 
		score.Scorer, 
		score.Value.String(), 
	); err != nil {
		if isForeignKeyViolationError(err) {
			return xerrors.Errorf("upsert score: %w, %s", scorestore.ErrUnknownScorer, score.Scorer)
		}
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
	var totalRows uint64
	row := ss.db.QueryRow(totalScorersQuery)
	if err := row.Scan(&totalRows); err != nil {
		return nil, xerrors.Errorf("scorers: %w", err)
	}

	rows, err := ss.db.Query(scorerQuery)
	if err != nil {
		return nil, xerrors.Errorf("scorers: %w", err)
	}
	return &scorerIterator{rows: rows, totalRows: totalRows}, nil
}


// Search the ScoreStore by a particular query and return a result iterator.
func (ss *CDBScoreStore) Search(query scorestore.Query) (scorestore.ScoreIterator, error) {
	var stmt string
	var totalRows uint64

	// Get query type and total number of rows.
	switch query.Type {
	case scorestore.QueryTypeScorer:
		stmt = scoreQuery	

		// Get total number of rows.
		row := ss.db.QueryRow(totalScoresQuery, query.Expression)
		if err := row.Scan(&totalRows); err != nil {
			return nil, xerrors.Errorf("total rows with expression: %s : %w", query.Expression, err)
		}
	default:
		return nil, xerrors.Errorf("search: %w, %s", scorestore.ErrUnknownQueryType, query.Type)
	}

	// Get rows.
	rows, err := ss.db.Query(stmt, query.Expression)
	if err != nil {
		return nil, xerrors.Errorf("search with expression: %s : %w", query.Expression, err)
	}
	return &scoreIterator{rows: rows, totalRows: totalRows}, nil
}

// Returns true if err indicates a foreign key constraint violation.
func isForeignKeyViolationError(err error) bool {
	pqErr, valid := err.(*pq.Error)
	if !valid {
		return false
	}
	return pqErr.Code.Name() == "foreign_key_violation"
}
