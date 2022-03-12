package memory

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/scorestore/test"
)

var _ = gc.Suite(new(InMemoryScoreStoreTestSuite))

func Test(t *testing.T) { gc.TestingT(t) }

type InMemoryScoreStoreTestSuite struct {
	test.SuiteBase
}

func (s *InMemoryScoreStoreTestSuite) SetUpTest(c *gc.C) {
	s.SetScoreStore(NewInMemoryScoreStore())
}
