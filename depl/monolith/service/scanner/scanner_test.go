package scanner 

import (
	"testing"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/depl/monolith/service/scanner/mocks"
)

var _ = gc.Suite(new(ConfigTestSuite))

type ConfigTestSuite struct{}

func (s *ConfigTestSuite) TestConfigValidation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	origCfg := Config{
		ETHClient:		mocks.NewMockETHClient(ctrl),
		GraphAPI:		mocks.NewMockGraphAPI(ctrl),
		FetchWorkers:	3,
	}

	cfg := origCfg
	c.Assert(cfg.validate(), gc.IsNil)
	c.Assert(cfg.Logger, gc.Not(gc.IsNil), gc.Commentf("default logger was not assigned"))

	cfg = origCfg
	cfg.ETHClient = nil
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*client API has not been provided.*")

	cfg = origCfg
	cfg.GraphAPI = nil
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*graph API has not been provided.*")

	cfg = origCfg
	cfg.FetchWorkers = 0
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*invalid value for fetch workers.*")

}

func Test(t *testing.T) {
	// Run all gocheck test-suites
	gc.TestingT(t)
}
