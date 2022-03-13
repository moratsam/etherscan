package memory

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/scorestore"
)

// Compile-time check for ensuring InMemoryScoreStore implements ScoreStore.
var _ scorestore.ScoreStore = (*InMemoryScoreStore)(nil)

// [<scorer name>] --> <a score belonging to the scorer>
type scorerScoreMap map[string]*scorestore.Score

// InMemoryScoreStore implements an in-memory scorestore that can be concurrently
// accessed by multiple clients.
type InMemoryScoreStore struct {
	mu sync.RWMutex

	// [<scorer name>] --> Scorer
	scorers map[string]*scorestore.Scorer

	// [<wallet address>] --> scorerScoreMap
	walletScores map[string]scorerScoreMap

	// [<scorer name>] --> <list of wallets that have scores from the scorer>
	scorerWallets	map[string][]string
}

// NewInMemoryScoreStore returns an in-memory implementation of the scorestore.
func NewInMemoryScoreStore() (*InMemoryScoreStore) {
	return &InMemoryScoreStore{
		scorers:			make(map[string]*scorestore.Scorer),
		walletScores:	make(map[string]scorerScoreMap),
		scorerWallets:	make(map[string][]string),
	}
}

// Upserts a Score.
// On conflict of (wallet, scorer), the value will be updated.
func (ss *InMemoryScoreStore) UpsertScore(score *scorestore.Score) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Create a copy of the score.
	sCopy := new(scorestore.Score)
	*sCopy = *score

	// Check that the scorer exists.
	if _, keyExists := ss.scorers[sCopy.Scorer]; !keyExists {
		return xerrors.Errorf("upsert score: %w, %s", scorestore.ErrUnknownScorer, sCopy.Scorer)
	}

	// Check if the score wallet already exists in scorerWallets.
	var exists bool
	for _, wallet := range ss.scorerWallets[sCopy.Scorer] {
		if wallet == sCopy.Wallet {
			exists = true
			break
		}
	}
	// If it doesn't exist, add it.
	if !exists {
		ss.scorerWallets[sCopy.Scorer] = append(ss.scorerWallets[sCopy.Scorer], sCopy.Wallet)
	}

	// If the score wallet does not yet exists in walletScores, add it.
	if _, keyExists := ss.walletScores[sCopy.Wallet]; !keyExists {
		ss.walletScores[sCopy.Wallet] = make(map[string]*scorestore.Score)
	}

	// Update the walletScores with the score.
	ss.walletScores[sCopy.Wallet][sCopy.Scorer] = sCopy

	return nil
}

// Upserts a Scorer.
func (ss *InMemoryScoreStore) UpsertScorer(scorer *scorestore.Scorer) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Create a copy of the scorer.
	sCopy := new(scorestore.Scorer)
	*sCopy = *scorer

	// If scorer exists, return.
	if _, keyExists := ss.scorers[sCopy.Name]; keyExists {
		return nil
	}

	// Update scorers.
	ss.scorers[sCopy.Name] = sCopy

	// Create empty list in scorerWallets
	ss.scorerWallets[sCopy.Name] = make([]string, 3)

	return nil
}

// Returns an iterator for all scorers.
func (ss *InMemoryScoreStore) Scorers() (scorestore.ScorerIterator, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var list []*scorestore.Scorer
	for _, scorer := range ss.scorers {
		list = append(list, scorer)
	}
	return &scorerIterator{ss: ss, scorers: list}, nil
}


// Search the ScoreStore by a particular query and return a result iterator.
func (ss *InMemoryScoreStore) Search(query scorestore.Query) (scorestore.ScoreIterator, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var list []*scorestore.Score

	switch query.Type {
	case scorestore.QueryTypeScorer:
		scorerName := query.Expression
		// Iterate over all wallets and append the scores.
		for _, currentScorerScoresMap := range ss.walletScores {
			// If the wallet has a score pertaining to the scorer name, append it to the list.
			if _, keyExists := currentScorerScoresMap[scorerName]; keyExists {
				list = append(list, currentScorerScoresMap[scorerName])
			}
		}
	default:
		return nil, xerrors.Errorf("search: %w, %s", scorestore.ErrUnknownQueryType, query.Type)
	}

	return &scoreIterator{ss: ss, scores: list}, nil
}
