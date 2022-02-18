package scanner 

import (
	_"context"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/scanner/mocks"
)

var _ = gc.Suite(new(BlockFetcherTestSuite))

type BlockFetcherTestSuite struct {
	client *mocks.MockETHClient
}

func (s *BlockFetcherTestSuite) SetUpTest(c *gc.C) {}

func (s *BlockFetcherTestSuite) TestBlockFetcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = mocks.NewMockETHClient(ctrl)
	
	//TODO
}
