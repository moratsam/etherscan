package gravitas

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/depl/monolith/partition"
	"github.com/moratsam/etherscan/depl/monolith/service/gravitas/mocks"
	ss "github.com/moratsam/etherscan/scorestore"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
)

var _ = gc.Suite(new(ConfigTestSuite))
var _ = gc.Suite(new(GravitasTestSuite))

type ConfigTestSuite struct{}
type GravitasTestSuite struct{}

func (s *ConfigTestSuite) TestConfigValidation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	origCfg := Config{
		GraphAPI:				mocks.NewMockGraphAPI(ctrl),
		ScoreStoreAPI:			mocks.NewMockScoreStoreAPI(ctrl),
		PartitionDetector:	partition.Fixed{},
		ComputeWorkers:		4,
		TxFetchers:				1,
		UpdateInterval:		time.Minute,
	}

	cfg := origCfg
	c.Assert(cfg.validate(), gc.IsNil)
	c.Assert(cfg.Clock, gc.Not(gc.IsNil), gc.Commentf("default clock was not assigned"))
	c.Assert(cfg.Logger, gc.Not(gc.IsNil), gc.Commentf("default logger was not assigned"))

	cfg = origCfg
	cfg.GraphAPI = nil
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*graph API has not been provided.*")

	cfg = origCfg
	cfg.ScoreStoreAPI = nil
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*scorestore API has not been provided.*")

	cfg = origCfg
	cfg.PartitionDetector = nil
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*partition detector has not been provided.*")

	cfg = origCfg
	cfg.ComputeWorkers = 0
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*invalid value for compute workers.*")

	cfg = origCfg
	cfg.UpdateInterval = 0
	c.Assert(cfg.validate(), gc.ErrorMatches, "(?ms).*invalid value for update interval.*")
}

func (s *GravitasTestSuite) TestFullRun(c *gc.C) {
	// TODO fixme
	fmt.Println("gravitas TestFullRun In case of panics/errors: call to Reset() was moved from the beginning of the gravitas.updateGraphScores() to the end, so that memory is free during the wait during gravitas.Run() invocations. The downside of this is that it causes panics in this test.")
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockGraph := mocks.NewMockGraphAPI(ctrl)
	mockScoreStore := mocks.NewMockScoreStoreAPI(ctrl)
	clk := testclock.NewClock(time.Now())

	cfg := Config{
		GraphAPI:        		mockGraph,
		ScoreStoreAPI:			mockScoreStore,
		PartitionDetector:	partition.Fixed{Partition: 0, NumPartitions: 1},
		Clock:           		clk,
		ComputeWorkers:  		1,
		TxFetchers:				1,
		UpdateInterval:  		time.Minute,
	}
	svc, err := NewService(cfg)
	c.Assert(err, gc.IsNil)

	ctx, cancelFn := context.WithCancel(context.TODO())
	defer cancelFn()

	addr1, addr2 := createAddressFromInt(1), createAddressFromInt(2)
	tx := createTx(addr1, addr2, 1, 1)

	mockWalletIt := mocks.NewMockWalletIterator(ctrl)
	gomock.InOrder(
		mockWalletIt.EXPECT().Next().Return(true),
		mockWalletIt.EXPECT().Wallet().Return(&txgraph.Wallet{Address: addr1}),
		mockWalletIt.EXPECT().Next().Return(true),
		mockWalletIt.EXPECT().Wallet().Return(&txgraph.Wallet{Address: addr2}),
		mockWalletIt.EXPECT().Next().Return(false),
	)
	mockWalletIt.EXPECT().Error().Return(nil)
	mockWalletIt.EXPECT().Close().Return(nil)

	mockTxIt := mocks.NewMockTxIterator(ctrl)
	gomock.InOrder(
		mockTxIt.EXPECT().Next().Return(true),
		mockTxIt.EXPECT().Tx().Return(tx),
		mockTxIt.EXPECT().Next().Return(false),
	)
	mockTxIt.EXPECT().Error().Return(nil)
	mockTxIt.EXPECT().Close().Return(nil)

	mockTxIt2 := mocks.NewMockTxIterator(ctrl)
	gomock.InOrder(
		mockTxIt2.EXPECT().Next().Return(false),
	)
	mockTxIt2.EXPECT().Error().Return(nil)
	mockTxIt2.EXPECT().Close().Return(nil)

	minAddress := "0000000000000000000000000000000000000000"
	maxAddress := "ffffffffffffffffffffffffffffffffffffffff"
	mockGraph.EXPECT().Wallets(minAddress, maxAddress).Return(mockWalletIt, nil)
	mockGraph.EXPECT().WalletTxs(addr1).Return(mockTxIt, nil)
	mockGraph.EXPECT().WalletTxs(addr2).Return(mockTxIt2, nil)

	scores := []*ss.Score{
		&ss.Score{
			Wallet:	addr1,
			Scorer:	"balance_eth",
			Value:	big.NewFloat(-2),
		},
		&ss.Score{
			Wallet:	addr2,
			Scorer:	"balance_eth",
			Value:	big.NewFloat(1),
		},
	}
	// TODO this sometimes fails because the two scores are reversed.
	mockScoreStore.EXPECT().UpsertScores(scores)

	go func() {
		// Wait until the main loop calls time.After (or timeout if
		// 10 sec elapse) and advance the time to trigger a new gravitas
		// pass.
		c.Assert(clk.WaitAdvance(time.Minute, 10*time.Second, 1), gc.IsNil)

		// Wait until the main loop calls time.After again and cancel
		// the context.
		time.Sleep(1*time.Second)
		c.Assert(clk.WaitAdvance(100*time.Millisecond, 10*time.Second, 1), gc.IsNil)
		cancelFn()
	}()

	// Enter the blocking main loop
	err = svc.Run(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(svc.calculator.Graph().Aggregator("wallet_count").Get(), gc.Equals, 2)
	c.Assert(svc.calculator.Graph().Aggregator("tx_count").Get().(int), gc.Equals, 1)
}

func (s *GravitasTestSuite) TestRunWhileInNonZeroPartition(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clk := testclock.NewClock(time.Now())

	cfg := Config{
		GraphAPI:          	mocks.NewMockGraphAPI(ctrl),
		ScoreStoreAPI:     	mocks.NewMockScoreStoreAPI(ctrl),
		PartitionDetector:	partition.Fixed{Partition: 1, NumPartitions: 2},
		Clock:            	clk,
		ComputeWorkers:    	1,
		TxFetchers:				1,
		UpdateInterval:    	time.Minute,
	}
	svc, err := NewService(cfg)
	c.Assert(err, gc.IsNil)

	go func() {
		// Wait until the main loop calls time.After and advance the time.
		// The service will check the partition information, see that
		// it is not assigned to partition 0 and exit the main loop.
		c.Assert(clk.WaitAdvance(time.Minute, 10*time.Second, 1), gc.IsNil)
	}()

	// Enter the blocking main loop
	err = svc.Run(context.TODO())
	c.Assert(err, gc.IsNil)
}

func Test(t *testing.T) {
	// Run all gocheck test-suites
	gc.TestingT(t)
}

// If address is not 40 chars long, string comparisons will not work as expected.
// The following is loop far from efficient, but it's only for tests so who cares.
func createAddressFromInt(addressInt int) string {
	x := fmt.Sprintf("%x", addressInt) // convert to hex string
	padding := 40-len(x)
	for i:=0; i<padding; i++ {
		x = "0" + x
	}
	return x
}

func createTx(from, to string, value, transactionFee int64) *txgraph.Tx {
	return &txgraph.Tx{
		From: 				from,
		To: 					to,
		Value: 				big.NewInt(value),
		TransactionFee:	big.NewInt(transactionFee),
	}
}

