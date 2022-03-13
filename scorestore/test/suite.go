package test

import (
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/scorestore"
)

// SuiteBase defines a re-usable set of scorestore related tests that can be executed
// against any type that implements scorestore.ScoreStore.
type SuiteBase struct {
	ss scorestore.ScoreStore
}

func (s *SuiteBase) SetScoreStore(ss scorestore.ScoreStore) {
	s.ss = ss
}

func (s *SuiteBase) TestUpsertScore(c *gc.C) {
	wallet := createAddressFromInt(1)
	scorer := "test_scorer"
	original_value := float64(3.7)
	updated_value := original_value+ 0.1

	original := &scorestore.Score{
		Wallet: wallet,
		Scorer: scorer,
		Value: original_value,
	}

	// Attempt to upsert score without existing scorer.
	err := s.ss.UpsertScore(original)
	c.Assert(err, gc.ErrorMatches, ".*unknown scorer.*")

	// Insert scorer.
	err = s.ss.UpsertScorer(&scorestore.Scorer{Name: scorer})
	c.Assert(err, gc.IsNil)
	
	// insert a new score.
	err = s.ss.UpsertScore(original)
	c.Assert(err, gc.IsNil)

	// Update value.
	original.Value = updated_value
	err = s.ss.UpsertScore(original)
	c.Assert(err, gc.IsNil)

	// Get a ScoreIterator.
	query := scorestore.Query{
		Type:			scorestore.QueryTypeScorer,
		Expression:	scorer,
	}
	scoreIterator, err := s.ss.Search(query)
	c.Assert(err, gc.IsNil)

	// Retrieve the score.
	c.Assert(scoreIterator.Next(), gc.Equals, true, gc.Commentf("score iterator returned false"))
	err = scoreIterator.Error()
	c.Assert(err, gc.IsNil)
	retrievedScore := scoreIterator.Score()
	c.Assert(scoreIterator.Next(), gc.Equals, false, gc.Commentf("score iterator should have only 1 score"))
	err = scoreIterator.Error()
	c.Assert(err, gc.IsNil)
	err = scoreIterator.Close()
	c.Assert(err, gc.IsNil)

	//Assert the scores' equivalence.
	c.Assert(retrievedScore, gc.DeepEquals, original, gc.Commentf("Retrieved score does not equal original"))
}

func (s *SuiteBase) TestUpsertScorer(c *gc.C) {
	scorer1 := &scorestore.Scorer{Name: "test_scorer1"}
	scorer2 := &scorestore.Scorer{Name: "test_scorer2"}

	// Insert scorers
	err := s.ss.UpsertScorer(scorer1)
	c.Assert(err, gc.IsNil)
	err = s.ss.UpsertScorer(scorer2)
	c.Assert(err, gc.IsNil)

	// Get a ScorerIterator.
	scorerIterator, err := s.ss.Scorers()
	c.Assert(err, gc.IsNil)

	// Retrieve the scorers.
	c.Assert(scorerIterator.Next(), gc.Equals, true, gc.Commentf("scorer iterator returned false"))
	retrievedScorer1 := scorerIterator.Scorer()
	c.Assert(scorerIterator.Next(), gc.Equals, true, gc.Commentf("scorer iterator returned false"))
	retrievedScorer2 := scorerIterator.Scorer()
	c.Assert(scorerIterator.Next(), gc.Equals, false, gc.Commentf("scorer iterator should have only 2 scorers"))
	err = scorerIterator.Error()
	c.Assert(err, gc.IsNil)
	err = scorerIterator.Close()
	c.Assert(err, gc.IsNil)

	if retrievedScorer1.Name == scorer1.Name {
		c.Assert(retrievedScorer1, gc.DeepEquals, scorer1, gc.Commentf("Retrieved scorer does not equal original"))
		c.Assert(retrievedScorer2, gc.DeepEquals, scorer2, gc.Commentf("Retrieved scorer does not equal original"))
	} else {
		c.Assert(retrievedScorer1, gc.DeepEquals, scorer2, gc.Commentf("Retrieved scorer does not equal original"))
		c.Assert(retrievedScorer2, gc.DeepEquals, scorer1, gc.Commentf("Retrieved scorer does not equal original"))
	}
}

func (s *SuiteBase) TestSearch(c *gc.C) {
	// Attempt to get a ScoreIterator with an invalid query type.
	query := scorestore.Query{
		Type:			73,
		Expression:	"some expression",
	}
	scoreIterator, err := s.ss.Search(query)
	c.Assert(scoreIterator, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*unknown query type.*")

}

// If address is not 40 chars long, string comparisons will not work as expected.
// The following is loop far from efficient, but it's only for tests so who cares.
func createAddressFromInt(addressInt int) string {
	x := fmt.Sprintf("%x", addressInt) // convert to hex string
	padding := 40-len(x)
	for i:=0; i<padding; i++ {
		x = "0" + x
	}
	return x
}
