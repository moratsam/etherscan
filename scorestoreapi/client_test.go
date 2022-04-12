package scorestoreapi_test 

import (
	"context"
	"fmt"
	"io"
	"math/big"

	_"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/mock/gomock"
	_"golang.org/x/xerrors"
	gc "gopkg.in/check.v1"

	ss "github.com/moratsam/etherscan/scorestore"
	ssapi "github.com/moratsam/etherscan/scorestoreapi"
	"github.com/moratsam/etherscan/scorestoreapi/mocks"
	"github.com/moratsam/etherscan/scorestoreapi/proto"
)

var _ = gc.Suite(new(ClientTestSuite))

type ClientTestSuite struct{}

func (s *ClientTestSuite) TestUpsertScore(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	rpcCli := mocks.NewMockScoreStoreClient(ctrl)

	wallet := "some_address"
	scorer := "some_scorer"
	value := big.NewFloat(3.8)

	score := &ss.Score{
		Wallet:	wallet,
		Scorer:	scorer,
		Value:	value,
	}

	rpcCli.EXPECT().UpsertScore(
		gomock.AssignableToTypeOf(context.TODO()),
		&proto.Score{
			Wallet:	score.Wallet,
			Scorer:	score.Scorer,
			Value:	score.Value.String(),
		},
	).Return(
		nil,
		nil,
	)

	cli := ssapi.NewScoreStoreClient(context.TODO(), rpcCli)
	err := cli.UpsertScore(score)
	c.Assert(err, gc.IsNil)
}

func (s *ClientTestSuite) TestUpsertScorer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	rpcCli := mocks.NewMockScoreStoreClient(ctrl)

	name := "some_name"

	scorer := &ss.Scorer{
		Name: name,
	}

	rpcCli.EXPECT().UpsertScorer(
		gomock.AssignableToTypeOf(context.TODO()),
		&proto.Scorer{
			Name:	scorer.Name,
		},
	).Return(
		nil,
		nil,
	)

	cli := ssapi.NewScoreStoreClient(context.TODO(), rpcCli)
	err := cli.UpsertScorer(scorer)
	c.Assert(err, gc.IsNil)
}

func (s *ClientTestSuite) TestSearch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	rpcCli := mocks.NewMockScoreStoreClient(ctrl)
	resultStream := mocks.NewMockScoreStore_SearchClient(ctrl)

	ctxWithCancel, cancelFn := context.WithCancel(context.TODO())
	defer cancelFn()

	rpcCli.EXPECT().Search(
		gomock.AssignableToTypeOf(ctxWithCancel),
		&proto.Query{Type: proto.Query_SCORER, Expression: "some_expression"},
	).Return(resultStream, nil)

	returns := [][]interface{}{
		{&proto.Score{
			Wallet:	"wallet-0",
			Scorer:	"scorer-0",
			Value:	"0",
		}, nil},
		{&proto.Score{
			Wallet:	"wallet-1",
			Scorer:	"scorer-1",
			Value:	"1",
		}, nil},
		{nil, io.EOF},
	}
	resultStream.EXPECT().Recv().DoAndReturn(
		func() (interface{}, interface{}) {
			next := returns[0]
			returns = returns[1:]
			return next[0], next[1]
		},
	).Times(len(returns))

	cli := ssapi.NewScoreStoreClient(context.TODO(), rpcCli)
	it, err := cli.Search(ss.Query{Type: ss.QueryTypeScorer, Expression: "some_expression"})
	c.Assert(err, gc.IsNil)

	var scoreCount int
	for it.Next() {
		score := it.Score()
		c.Assert(score.Wallet, gc.Equals, fmt.Sprintf("wallet-%d", scoreCount))
		c.Assert(score.Scorer, gc.Equals, fmt.Sprintf("scorer-%d", scoreCount))
		c.Assert(score.Value.String(), gc.Equals, fmt.Sprintf("%d", scoreCount))
		scoreCount++
	}
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)
	c.Assert(scoreCount, gc.Equals, 2)
}
