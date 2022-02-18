package scanner 

import (
	_"context"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/scanner/mocks"
)

var _ = gc.Suite(new(TxParserTestSuite))

type TxParserTestSuite struct {
	graph *mocks.MockGraph
}

func (s *TxParserTestSuite) TestTxParser(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.graph = mocks.NewMockGraph(ctrl)

	/* TODO
	payload := &scannerPayload{
		BlockNumber: 1,
		Txs: nil,
	}
	*/
}
