package test

import (
	"fmt"
	"math/big"

	gc "gopkg.in/check.v1"

	ss "github.com/moratsam/etherscan/scorestore"
)

// SuiteBase defines a re-usable set of scorestore related tests that can be executed
// against any type that implements ss.ScoreStore.
type SuiteBase struct {
	s ss.ScoreStore
}

func (s *SuiteBase) SetScoreStore(scorestore ss.ScoreStore) {
	s.s = scorestore
}

func (s *SuiteBase) TestUpsertScores(c *gc.C) {
	wallet := createAddressFromInt(1)
	wallet2 := createAddressFromInt(2)
	scorer := "test_scorer"
	original_value := big.NewFloat(3.8)
	updated_value := big.NewFloat(-3.0157)
	another_value := big.NewFloat(66)

	scores := make([]*ss.Score, 1)
	original := &ss.Score{
		Wallet: wallet,
		Scorer: scorer,
		Value: original_value,
	}
	scores[0] = original

	// Attempt to upsert score without existing scorer.
	err := s.s.UpsertScores(scores)
	c.Assert(err, gc.ErrorMatches, ".*unknown scorer.*")

	// Insert scorer.
	err = s.s.UpsertScorer(&ss.Scorer{Name: scorer})
	c.Assert(err, gc.IsNil)
	
	// insert a new score.
	err = s.s.UpsertScores(scores)
	c.Assert(err, gc.IsNil)

	// Update value and add another score.
	original.Value = updated_value
	another_score := &ss.Score{
		Wallet: wallet2,
		Scorer: scorer,
		Value: another_value,
	}
	scores = append(scores, another_score)
	err = s.s.UpsertScores(scores)
	c.Assert(err, gc.IsNil)

	// Get a ScoreIterator.
	query := ss.Query{
		Type:			ss.QueryTypeScorer,
		Expression:	scorer,
	}
	scoreIterator, err := s.s.Search(query)
	c.Assert(err, gc.IsNil)

	// Retrieve the score.
	c.Assert(scoreIterator.TotalCount(), gc.Equals, uint64(2), gc.Commentf("total count should equal 2"))
	for scoreIterator.Next() {
		retrievedScore := scoreIterator.Score()
		if retrievedScore.Wallet == wallet {
			//Assert the scores' equivalence.
			c.Assert(retrievedScore.Wallet, gc.DeepEquals, original.Wallet, gc.Commentf("Retrieved score does not equal original"))
			c.Assert(retrievedScore.Scorer, gc.DeepEquals, original.Scorer, gc.Commentf("Retrieved score does not equal original"))
			c.Assert(retrievedScore.Value.String(), gc.DeepEquals, updated_value.String(), gc.Commentf("Retrieved score does not equal original"))
		} else {
			//Assert the scores' equivalence.
			c.Assert(retrievedScore.Wallet, gc.DeepEquals, another_score.Wallet, gc.Commentf("Retrieved score does not equal another"))
			c.Assert(retrievedScore.Scorer, gc.DeepEquals, another_score.Scorer, gc.Commentf("Retrieved score does not equal another"))
			c.Assert(retrievedScore.Value.String(), gc.DeepEquals, another_value.String(), gc.Commentf("Retrieved score does not equal another"))
		}
	}
	err = scoreIterator.Error()
	c.Assert(err, gc.IsNil)
	c.Assert(scoreIterator.Next(), gc.Equals, false, gc.Commentf("score iterator should have only 2 scores"))
	err = scoreIterator.Error()
	c.Assert(err, gc.IsNil)
	err = scoreIterator.Close()
	c.Assert(err, gc.IsNil)
}

func (s *SuiteBase) TestUpsertScorer(c *gc.C) {
	scorer1 := &ss.Scorer{Name: "test_scorer1"}
	scorer2 := &ss.Scorer{Name: "test_scorer2"}

	// Insert scorers
	err := s.s.UpsertScorer(scorer1)
	c.Assert(err, gc.IsNil)
	err = s.s.UpsertScorer(scorer2)
	c.Assert(err, gc.IsNil)

	// Get a ScorerIterator.
	scorerIterator, err := s.s.Scorers()
	c.Assert(err, gc.IsNil)

	// Retrieve the scorers.
	c.Assert(scorerIterator.TotalCount(), gc.Equals, uint64(2), gc.Commentf("total count should equal 2"))
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
	query := ss.Query{
		Type:			73,
		Expression:	"some expression",
	}
	scoreIterator, err := s.s.Search(query)
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
