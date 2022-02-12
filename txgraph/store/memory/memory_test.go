package memory

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/txgraph/graph/graphtest"
)

var _ = gc.Suite(new(InMemoryGraphTestSuite))

func Test(t *testing.T) { gc.TestingT(t) }

type InMemoryGraphTestSuite struct {
	graphtest.SuiteBase
}

func (s *InMemoryGraphTestSuite) SetUpTest(c *gc.C) {
	s.SetGraph(NewInMemoryGraph(1))
}
