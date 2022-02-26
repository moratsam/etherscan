package txgraphapi_test

import (
	"context"
	_"io"
	_"math/big"

	_"github.com/gogo/protobuf/types"
	_"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/mock/gomock"
	_"golang.org/x/xerrors"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/txgraph/graph"
	"github.com/moratsam/etherscan/txgraphapi"
	"github.com/moratsam/etherscan/txgraphapi/mocks"
	"github.com/moratsam/etherscan/txgraphapi/proto"
)

var _ = gc.Suite(new(ClientTestSuite))

type ClientTestSuite struct{}

func (s *ClientTestSuite) TestUpsertBlock(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	rpcCli := mocks.NewMockTxGraphClient(ctrl)

	block := &graph.Block{
		Number:		1,
		Processed:	false,
	}

	rpcCli.EXPECT().UpsertBlock(
		gomock.AssignableToTypeOf(context.TODO()),
		&proto.Block{
			Number: 		int32(block.Number),
			Processed:	block.Processed,
		},
	).Return(nil)

	cli := txgraphapi.NewTxGraphClient(context.TODO(), rpcCli)
	err := cli.UpsertBlock(block)
	c.Assert(err, gc.IsNil)
}
