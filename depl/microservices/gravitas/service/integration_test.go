package service_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/sirupsen/logrus"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/depl/microservices/gravitas/service"
	"github.com/moratsam/etherscan/gravitas"
	ss "github.com/moratsam/etherscan/scorestore"
	memss "github.com/moratsam/etherscan/scorestore/memory"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
	memgraph "github.com/moratsam/etherscan/txgraph/store/memory"
)

var _ = gc.Suite(new(DistributedGravitasTestSuite))

var minAddress = "0000000000000000000000000000000000000000"
var maxAddress = "ffffffffffffffffffffffffffffffffffffffff"

type DistributedGravitasTestSuite struct {
	logger    *logrus.Entry
	logOutput bytes.Buffer
}

func (s *DistributedGravitasTestSuite) SetUpTest(c *gc.C) {
	s.logOutput.Reset()
	rootLogger := logrus.New()
	rootLogger.Level = logrus.DebugLevel
	rootLogger.Out = &s.logOutput

	s.logger = logrus.NewEntry(rootLogger)
}

func (s *DistributedGravitasTestSuite) TearDownTest(c *gc.C) {
	c.Log(s.logOutput.String())
	//fmt.Println(s.logOutput.String())
}

func (s *DistributedGravitasTestSuite) TestVerifyDistributedCalculationsAreCorrect(c *gc.C) {
	graphInstance, scoreStoreInstance := s.generateGraph(c, 2048, 10)

	// Run the calculations on a single instance so we can get a baseline
	// for our comparisons.
	singleResults := s.runStandaloneCalculator(c, graphInstance)

	// Reset the scores and run in distributed mode
	s.resetScores(c, graphInstance, scoreStoreInstance)
	distributedResults := s.runDistributedCalculator(c, graphInstance, scoreStoreInstance, 42)

	// Compare results
	deltaTolerance := 0.0001
	sumTolerance := 0.001
	s.assertResultsMatch(c, singleResults, distributedResults, deltaTolerance, sumTolerance)
}

func (s *DistributedGravitasTestSuite) assertResultsMatch(c *gc.C, singleResults, distributedResults map[string]float64, deltaTolerance, sumTolerance float64) {
	c.Assert(len(singleResults), gc.Equals, len(distributedResults), gc.Commentf("result count mismatch"))
	c.Logf("checking single and distributed run results (count %d)", len(singleResults))

	var singleSum, distributedSum float64
	for vertexID, singleScore := range singleResults {
		distributedScore, found := distributedResults[vertexID]
		c.Assert(found, gc.Equals, true, gc.Commentf("vertex %s not found in distributed result set", vertexID))

		absDelta := math.Abs(singleScore - distributedScore)
		c.Assert(absDelta <= deltaTolerance, gc.Equals, true, gc.Commentf("vertex %s: single score %v, distr. score %v, absDelta %v > %v", vertexID, singleScore, distributedScore, absDelta, deltaTolerance))

		singleSum += singleScore
		distributedSum += distributedScore
	}
}

func (s *DistributedGravitasTestSuite) resetScores(c *gc.C, graphInstance txgraph.Graph, scoreStoreInstance ss.ScoreStore) {
	scoreIt, err := scoreStoreInstance.Search(ss.Query{Type: ss.QueryTypeScorer, Expression: "balance_eth"})
	c.Assert(err, gc.IsNil)
	for scoreIt.Next(){
		score := scoreIt.Score()
		score.Value = big.NewFloat(0)
		err := scoreStoreInstance.UpsertScore(score)
		c.Assert(err, gc.IsNil)
	}
}

// runStandaloneCalculator processes the txgraph using a single calculator
// instance with only one worker and returns back the calculated scores as a
// map.
func (s *DistributedGravitasTestSuite) runStandaloneCalculator(c *gc.C, graphInstance txgraph.Graph) map[string]float64 {
	calc, err := gravitas.NewCalculator(gravitas.Config{ComputeWorkers: 1})
	c.Assert(err, gc.IsNil)

	// Load wallets
	err = loadWallets(calc, graphInstance, minAddress, maxAddress)

	// Execute graph and collect results
	err = calc.Executor().RunToCompletion(context.TODO())
	c.Assert(err, gc.IsNil)

	resMap := make(map[string]float64)
	err = calc.Scores(func(id string, score *big.Float) error {
		s, _ := score.Float64()
		resMap[id] = s
		return nil
	})
	c.Assert(err, gc.IsNil)

	return resMap
}

func (s *DistributedGravitasTestSuite) generateGraph(c *gc.C, numWallets, numTxs int) (txgraph.Graph, ss.ScoreStore) {
	graphInstance := memgraph.NewInMemoryGraph()
	scoreStoreInstance := memss.NewInMemoryScoreStore()

	// Setup wallets.
	wallets := make([]*txgraph.Wallet, 0)
	for i:=0; i<numWallets; i++ {
		wallets = append(wallets, &txgraph.Wallet{Address: createAddressFromInt(i)})
	}

	err := graphInstance.UpsertWallets(wallets)
	c.Assert(err, gc.IsNil)

	// Setup txs.
	rand.Seed(73)
	txs := make([]*txgraph.Tx, 0)
	for i:=0; i<numTxs; i++ {
		txs = append(txs, createTx(
			createAddressFromInt(rand.Intn(numWallets)),
			createAddressFromInt(rand.Intn(numWallets)),
			int64(rand.Intn(10000000)),
			int64(rand.Intn(10000000)),
		))
	}
	err = graphInstance.InsertTxs(txs)
	c.Assert(err, gc.IsNil)

	// Setup scorestore
	scoreStoreInstance.UpsertScorer(&ss.Scorer{Name: "balance_eth"})

	return graphInstance, scoreStoreInstance
}

// runDistributedCalculator processes the txgraph using the distributed calculator
// and returns back the calculated scores as a map.
func (s *DistributedGravitasTestSuite) runDistributedCalculator(c *gc.C, graphInstance txgraph.Graph, scoreStoreInstance ss.ScoreStore, numWorkers int) map[string]float64{
	var (
		ctx, cancelFn = context.WithCancel(context.TODO())
		clk           = testclock.NewClock(time.Now())
		wg            sync.WaitGroup
	)
	defer cancelFn()

	master, err := service.NewMasterNode(service.MasterConfig{
		ListenAddress:  ":9998",
		Clock:          clk,
		UpdateInterval: time.Minute,
		MinWorkers:     numWorkers,
		Logger:         s.logger.WithField("master", true),
	})
	c.Assert(err, gc.IsNil)

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Assert(master.Run(ctx), gc.IsNil)
	}()

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(i int) {
			defer wg.Done()
			worker, err := service.NewWorkerNode(service.WorkerConfig{
				MasterEndpoint:		":9998",
				MasterDialTimeout:	10 * time.Second,
				GraphAPI:				graphInstance,
				ScoreStoreAPI:			scoreStoreInstance,
				ComputeWorkers:		runtime.NumCPU(),
				Logger:					s.logger.WithField("worker_id", i),
			})
			c.Assert(err, gc.IsNil)
			c.Assert(worker.Run(ctx), gc.IsNil)
		}(i)
	}

	// Trigger an update on the master
	c.Assert(clk.WaitAdvance(time.Minute, 60*time.Second, 1), gc.IsNil)

	// Wait till the master goes back to the main loop and stop it
	c.Assert(clk.WaitAdvance(time.Second, 60*time.Second, 1), gc.IsNil)
	cancelFn()

	// Wait for master and workers to shut down.
	wg.Wait()

	return s.extractScores(c, scoreStoreInstance)
}

func (s *DistributedGravitasTestSuite) extractScores(c *gc.C, scoreStoreInstance ss.ScoreStore) map[string]float64{
	resMap := make(map[string]float64)

	scoreIt, err := scoreStoreInstance.Search(ss.Query{Type: ss.QueryTypeScorer, Expression: "balance_eth"})
	c.Assert(err, gc.IsNil)
	for scoreIt.Next(){
		score := scoreIt.Score()
		s, _ := score.Value.Float64()
		resMap[score.Wallet] = s
	}
	c.Assert(scoreIt.Close(), gc.IsNil)

	return resMap
}

func Test(t *testing.T) {
	// Run all gocheck test-suites
	gc.TestingT(t)
}

func loadWallets(calculator *gravitas.Calculator, graphInstance txgraph.Graph, fromAddr, toAddr string) error {
	walletIt, err := graphInstance.Wallets(fromAddr, toAddr)
	if err != nil {
		return err
	}

	for walletIt.Next() {
		wallet := walletIt.Wallet()
		
		// Retrieve wallet's transactions.
		txIt, err := graphInstance.WalletTxs(wallet.Address)
		if err != nil {
			return err
		}
		var txs []*txgraph.Tx
		for txIt.Next() {
			txs = append(txs, txIt.Tx())	
		}
		if err = txIt.Error(); err != nil {
			_ = txIt.Close()
			return err
		}
		if err = txIt.Close(); err != nil {
			return err
		}

		// Add vertex
		calculator.AddVertex(wallet.Address, txs)
	}
	if err = walletIt.Error(); err != nil {
		_ = walletIt.Close()
		return err
	}

	return walletIt.Close()
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
