package scorestoreapi_test 

import (
	"context"
	_"fmt"
	"io"
	"math/big"
	"net"

	"github.com/golang/protobuf/ptypes/empty"
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
	cli			proto.ScoreStoreClient
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
	s.cli = proto.NewScoreStoreClient(s.cliConn)
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

	score := &proto.Score{
		Wallet:	wallet,
		Scorer:	scorer,
		Value:	original_value.String(),
	}

	// Insert scorer.
	_, err := s.cli.UpsertScorer(context.TODO(), &proto.Scorer{Name: scorer})
	c.Assert(err, gc.IsNil)

	// Insert score.
	_, err = s.cli.UpsertScore(context.TODO(), score)
	c.Assert(err, gc.IsNil)

	// Upsert score.
	score.Value = updated_value.String()
	_, err = s.cli.UpsertScore(context.TODO(), score)
	c.Assert(err, gc.IsNil)

	// Get stream of search results.
	stream, err := s.cli.Search(context.TODO(), &proto.Query{
		Type:	proto.Query_SCORER,
		Expression: "some_scorer",
	})
	c.Assert(err, gc.IsNil)

	// Consume scores from stream.
	var scoreCount int
	for {
		score, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.Fatal(err)
		}
		
		c.Assert(score.Wallet, gc.Equals, wallet)
		c.Assert(score.Scorer, gc.Equals, scorer)
		c.Assert(score.Value, gc.Equals, updated_value.String())
		scoreCount++
	}
	
	c.Assert(scoreCount, gc.Equals, 1)
}

func (s *ServerTestSuite) TestUpsertScorer(c *gc.C) {
	name := "some_name"

	scorer := &proto.Scorer{
		Name: name,
	}

	// Insert scorer.
	_, err := s.cli.UpsertScorer(context.TODO(), scorer)
	c.Assert(err, gc.IsNil)

	// Get stream of scorers.
	stream, err := s.cli.Scorers(context.TODO(), new(empty.Empty))
	c.Assert(err, gc.IsNil)

	// Consume scorers from stream.
	var scorerCount int
	for {
		scorer, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.Fatal(err)
		}
		
		c.Assert(scorer.Name, gc.Equals, name)
		scorerCount++
	}
	
	c.Assert(scorerCount, gc.Equals, 1)
}
