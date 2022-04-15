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
	scorer := "some_scorer"
	original_value := big.NewFloat(3.8)
	updated_value := big.NewFloat(-0.0371)

	score := &ss.Score{
		Wallet:	wallet,
		Scorer:	scorer,
		Value:	original_value,
	}

	// Insert scorer.
	err := s.cli.UpsertScorer(&ss.Scorer{Name: scorer})
	c.Assert(err, gc.IsNil)

	// Insert score.
	err = s.cli.UpsertScore(score)
	c.Assert(err, gc.IsNil)

	// Upsert score.
	score.Value = updated_value
	err = s.cli.UpsertScore(score)
	c.Assert(err, gc.IsNil)

	// Get iterator for score search results.
	it, err := s.cli.Search(ss.Query{
		Type:	ss.QueryTypeScorer,
		Expression: "some_scorer",
	})
	c.Assert(err, gc.IsNil)

	// Consume scores.
	var scoreCount int
	for it.Next() {
		current_score := it.Score()
		if err != nil {
			c.Assert(err, gc.IsNil)
		}
		c.Assert(current_score.Wallet, gc.Equals, score.Wallet)
		c.Assert(current_score.Scorer, gc.Equals, score.Scorer)
		c.Assert(current_score.Value.String(), gc.Equals, score.Value.String())
		scoreCount++
	}
	
	c.Assert(scoreCount, gc.Equals, 1)
	c.Assert(uint64(scoreCount), gc.Equals, it.TotalCount())
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)
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
