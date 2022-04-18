package scorestoreapi_test 

import (
	"context"
	"math/big"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	ss "github.com/moratsam/etherscan/scorestore"
	memss "github.com/moratsam/etherscan/scorestore/memory"
	ssapi "github.com/moratsam/etherscan/scorestoreapi"
	"github.com/moratsam/etherscan/scorestoreapi/proto"
)

var _ = gc.Suite(new(ServerTestSuite))

type ServerTestSuite struct{
	s	ss.ScoreStore 

	netListener *bufconn.Listener
	grpcSrv		*grpc.Server

	cliConn		*grpc.ClientConn
	cli			*ssapi.ScoreStoreClient
}

func (s *ServerTestSuite) SetUpTest(c *gc.C) {
	var err error
	s.s = memss.NewInMemoryScoreStore()

	s.netListener = bufconn.Listen(1024)
	s.grpcSrv = grpc.NewServer()
	proto.RegisterScoreStoreServer(s.grpcSrv, ssapi.NewScoreStoreServer(s.s))
	go func() {
		err := s.grpcSrv.Serve(s.netListener)
		c.Assert(err, gc.IsNil)
	}()

	s.cliConn, err = grpc.Dial(
		"bufnet",
		grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) { return s.netListener.Dial() },
		),
		grpc.WithInsecure(),
	)
	c.Assert(err, gc.IsNil)
	proto_cli := proto.NewScoreStoreClient(s.cliConn)
	s.cli = ssapi.NewScoreStoreClient(context.TODO(), proto_cli)
}

func (s *ServerTestSuite) TearDownTest(c *gc.C) {
	_ = s.cliConn.Close()
	s.grpcSrv.Stop()
	_ = s.netListener.Close()
}

func (s *ServerTestSuite) TestUpsertScore(c *gc.C) {
	wallet := "some_address"
	wallet2 := "another_address"
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
	err := s.cli.UpsertScores(scores)
	c.Assert(err, gc.ErrorMatches, ".*unknown scorer.*")

	// Insert scorer.
	err = s.cli.UpsertScorer(&ss.Scorer{Name: scorer})
	c.Assert(err, gc.IsNil)
	
	// insert a new score.
	err = s.cli.UpsertScores(scores)
	c.Assert(err, gc.IsNil)

	// Update value and add another score.
	original.Value = updated_value
	another_score := &ss.Score{
		Wallet: wallet2,
		Scorer: scorer,
		Value: another_value,
	}
	scores = append(scores, another_score)
	err = s.cli.UpsertScores(scores)
	c.Assert(err, gc.IsNil)

	// Get a ScoreIterator.
	query := ss.Query{
		Type:			ss.QueryTypeScorer,
		Expression:	scorer,
	}
	scoreIterator, err := s.cli.Search(query)
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

func (s *ServerTestSuite) TestUpsertScorer(c *gc.C) {
	name := "some_name"

	scorer := &ss.Scorer{Name: name}

	// Insert scorer.
	err := s.cli.UpsertScorer(scorer)
	c.Assert(err, gc.IsNil)

	// Get iterator for scorers
	it, err := s.cli.Scorers()
	c.Assert(err, gc.IsNil)

	// Consume scorers.
	var scorerCount int
	for it.Next() {
		current_scorer := it.Scorer()
		if err != nil {
			c.Assert(err, gc.IsNil)
		}
		c.Assert(current_scorer, gc.DeepEquals, scorer)
		scorerCount++
	}
	
	c.Assert(scorerCount, gc.Equals, 1)
	c.Assert(uint64(scorerCount), gc.Equals, it.TotalCount())
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)
}
