package memory

import (
	"github.com/moratsam/etherscan/scorestore"
)

// scoreIterator is a scorestore.ScoreIterator implementation for the in-memory scorestore.
type scoreIterator struct {
	ss *InMemoryScoreStore

	scores	[]*scorestore.Score
	curIndex int
}

func (i *scoreIterator) Next() bool {
	if i.curIndex >= len(i.scores) {
		return false
	}
	i.curIndex++
	return true
}

func (i *scoreIterator) Error() error {
	return nil
}

func (i *scoreIterator) Close() error {
	return nil
}

func (i *scoreIterator) Score() *scorestore.Score {
	// The score pointer contents may be overwritten by a scorestore update; to 
	// avoid data-races, acquire the read lock and clone the score.
	i.ss.mu.RLock()
	defer i.ss.mu.RUnlock()
	score := new(scorestore.Score)
	*score = *i.scores[i.curIndex-1]
	return score
}


// scorerIterator is a scorestore.ScorerIterator implementation for the in-memory scorestore.
type scorerIterator struct {
	ss *InMemoryScoreStore

	scorers	[]*scorestore.Scorer
	curIndex int
}

func (i *scorerIterator) Next() bool {
	if i.curIndex >= len(i.scorers) {
		return false
	}
	i.curIndex++
	return true
}

func (i *scorerIterator) Error() error {
	return nil
}

func (i *scorerIterator) Close() error {
	return nil
}

func (i *scorerIterator) Scorer() *scorestore.Scorer {
	// The scorer pointer contents may be overwritten by a scorestore update; to 
	// avoid data-races, acquire the read lock and clone the scorer.
	i.ss.mu.RLock()
	defer i.ss.mu.RUnlock()
	scorer := new(scorestore.Scorer)
	*scorer = *i.scorers[i.curIndex-1]
	return scorer
}
