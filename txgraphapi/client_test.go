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

func (s *ClientTestSuite) TestUpsertBlocks(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	rpcCli := mocks.NewMockTxGraphClient(ctrl)

	blocks := []*graph.Block{&graph.Block{Number: 1}}

	rpcCli.EXPECT().UpsertBlocks(
		gomock.AssignableToTypeOf(context.TODO()),
		&proto.BlockBatch{
			Blocks: []*proto.Block{
				&proto.Block{
					Number: 		int32(blocks[0].Number),
					Processed:	blocks[0].Processed,
				},
			},
		},
	).Return(
		nil,
		nil,
	)

	cli := txgraphapi.NewTxGraphClient(context.TODO(), rpcCli)
	err := cli.UpsertBlocks(blocks)
	c.Assert(err, gc.IsNil)
}
