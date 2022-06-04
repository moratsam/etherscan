package cdb

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/scorestore"
)

var (
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

func (g *CDBScoreStore) bulkUpsertScores(scores []*scorestore.Score) error {
	if len(scores) == 0 {
		return nil
	}
	numArgs := 3 // Number of columns in the score table.
	valueArgs := make([]interface{}, 0, numArgs * len(scores))
	valueStrings := make([]string, 0, len(scores))

	for i,score := range scores {
		valueArgs = append(valueArgs, score.Wallet)
		valueArgs = append(valueArgs, score.Scorer)
		valueArgs = append(valueArgs, score.Value.String())
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d)", i*numArgs+1, i*numArgs+2, i*numArgs+3))
	}

	stmt := fmt.Sprintf(`INSERT INTO score(wallet, scorer, value) VALUES %s ON CONFLICT (wallet, scorer) DO UPDATE SET value=EXCLUDED.value;`, strings.Join(valueStrings, ","))
	if _, err := g.db.Exec(stmt, valueArgs...); err != nil {
		if isForeignKeyViolationError(err) {
			return xerrors.Errorf("upsert score: %w, %s", scorestore.ErrUnknownScorer, scores[0].Scorer)
		}
		return xerrors.Errorf("upsert scores: %w", err)
	}
	return nil
}

// Upserts Scores.
// On conflict of (wallet, scorer), the value will be updated.
func (ss *CDBScoreStore) UpsertScores(scores []*scorestore.Score) error {
	// Upsert scores in batches.
	batchSize := 1500
	numWorkers := 3
	type slice struct {
		startIx, endIx int
	}
	sliceCh := make(chan slice, 1)
	doneCh := make(chan struct{}, numWorkers)
	errCh := make(chan error, 1)

	inserter := func() {
		defer func(){ doneCh <- struct{}{} }()
		for {
			select {
			case slice, open := <- sliceCh:
				if !open {
					// Channel was closed.
					return
				}
				if err := ss.bulkUpsertScores(scores[slice.startIx:slice.endIx]); err != nil {
					maybeEmitError(err, errCh)
					return
				}
			}
		}
	}

	for i:=0; i < numWorkers; i++ {
		go inserter()
	}

	for i:=0; i<len(scores); i+=batchSize {
		batchSize = int(math.Min(float64(batchSize), float64(len(scores)-i)))
		select {
		case err := <- errCh:
			return err
		case sliceCh <- slice{startIx: i, endIx: i+batchSize}:
		}
	}

	close(sliceCh)
	for i:=0; i < numWorkers; i++ {
		select {
		 case <- doneCh:
		 case err := <- errCh:
			return err
		}
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


func maybeEmitError(err error, errCh chan<- error) {
	select {
		case errCh <- err: // error emitted.
		default: // error channel is full with other errors.
	}
}
