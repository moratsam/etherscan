package service

import (
	"context"
	"io/ioutil"
	"math/big"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/bspgraph"
	"github.com/moratsam/etherscan/dbspgraph"
	"github.com/moratsam/etherscan/dbspgraph/job"
	"github.com/moratsam/etherscan/gravitas"
	ss "github.com/moratsam/etherscan/scorestore"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
)

// GraphAPI defines as set of API methods for fetching the wallets and their transactions
// from the wallet graph.
type GraphAPI interface {
	WalletTxs(address string) (txgraph.TxIterator, error)
	Wallets(fromAddress, toAddress string) (txgraph.WalletIterator, error)
}

// ScoreStoreAPI defines a set of API methods for updating the Gravitas scores of wallets.
type ScoreStoreAPI interface {
	UpsertScore(score *ss.Score) error
	UpsertScorer(scorer *ss.Scorer) error
}

// WorkerConfig encapsulates the settings for configuring a worker node for the
// Gravitas calculator service.
type WorkerConfig struct {
	// The master node endpoint.
	MasterEndpoint string

	// The timeout for establishing a connection to the master node.
	MasterDialTimeout time.Duration

	// An API for interating wallets and their transactions from the txgraph.
	GraphAPI GraphAPI

	// An API for updating the Gravitas scores of wallets.
	ScoreStoreAPI ScoreStoreAPI

	// The number of workers to spin up for computing Gravitas scores. If
	// not specified, a default value of 1 will be used instead.
	ComputeWorkers int

	// The logger to use. If not defined an output-discarding logger will
	// be used instead.
	Logger *logrus.Entry
}

func (cfg *WorkerConfig) validate() error {
	var err error
	if cfg.MasterEndpoint == "" {
		err = multierror.Append(err, xerrors.Errorf("invalid value for master endpoint"))
	}
	if cfg.GraphAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("graph API has not been provided"))
	}
	if cfg.ScoreStoreAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("scorestore API has not been provided"))
	}
	if cfg.ComputeWorkers <= 0 {
		err = multierror.Append(err, xerrors.Errorf("invalid value for compute workers"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// WorkerNode implements a master node for calculating Gravitas scores in a
// distributed fashion.
type WorkerNode struct {
	cfg          WorkerConfig
	calculator   *gravitas.Calculator
	workerFacade *dbspgraph.Worker

	// Stats
	jobStartedAt              time.Time
	graphPopulateTime         time.Duration
	scoreCalculationStartedAt time.Time
}

// NewWorkerNode creates a new worker node for the Gravitas calculator service.
func NewWorkerNode(cfg WorkerConfig) (*WorkerNode, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("gravitas service: config validation failed: %w", err)
	}
	calculator, err := gravitas.NewCalculator(gravitas.Config{ComputeWorkers: cfg.ComputeWorkers})
	if err != nil {
		return nil, xerrors.Errorf("gravitas service: config validation failed: %w", err)
	}

	workerNode := &WorkerNode{
		cfg:        cfg,
		calculator: calculator,
	}

	if workerNode.workerFacade, err = dbspgraph.NewWorker(dbspgraph.WorkerConfig{
		JobRunner:  workerNode,
		Serializer: serializer{},
		Logger:     cfg.Logger,
	}); err != nil {
		_ = calculator.Close()
		return nil, err
	}

	if err = workerNode.workerFacade.Dial(cfg.MasterEndpoint, cfg.MasterDialTimeout); err != nil {
		_ = calculator.Close()
		return nil, err
	}

	return workerNode, nil
}

// Run implements the main loop of a worker that executes the Gravitas
// algorithm on a subset of the txgraph. The worker waits for the master
// node to publish a new Gravitas job and then begins the algorithm execution
// constrained to the assigned partition range.
//
// Run blocks until the provided context expires.
func (n *WorkerNode) Run(ctx context.Context) error {
	n.cfg.Logger.Info("starting service")
	defer func() {
		_ = n.workerFacade.Close()
		_ = n.calculator.Close()
		n.cfg.Logger.Info("stopped service")
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if err := n.workerFacade.RunJob(ctx); err != nil {
			n.cfg.Logger.WithField("err", err).Error("Gravitas update job failed")
		}
	}
}

// StartJob implements job.Runner. It initializes the underlying bspgraph.Graph
// instance and invokes the provided ExecutorFactory to create an executor for
// the graph supersteps.
func (n *WorkerNode) StartJob(jobDetails job.Details, execFactory bspgraph.ExecutorFactory) (*bspgraph.Executor, error) {
	n.jobStartedAt = time.Now()
	if err := n.calculator.Graph().Reset(); err != nil {
		return nil, err
	} else if err := n.loadWallets(jobDetails.PartitionFromAddr, jobDetails.PartitionToAddr); err != nil {
		return nil, err
	}
	n.graphPopulateTime = time.Since(n.jobStartedAt)

	n.scoreCalculationStartedAt = time.Now()
	n.calculator.SetExecutorFactory(execFactory)
	return n.calculator.Executor(), nil
}

func (n *WorkerNode) loadWallets(fromAddr, toAddr string) error {
	walletIt, err := n.cfg.GraphAPI.Wallets(fromAddr, toAddr)
	if err != nil {
		return err
	}

	for walletIt.Next() {
		wallet := walletIt.Wallet()
		
		// Retrieve wallet's transactions.
		txIt, err := n.cfg.GraphAPI.WalletTxs(wallet.Address)
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
		n.calculator.AddVertex(wallet.Address, txs)
	}
	if err = walletIt.Error(); err != nil {
		_ = walletIt.Close()
		return err
	}

	return walletIt.Close()
}

// CompleteJob implements job.Runner. It persists the locally computed Gravitas
// scores after a successful execution of a distributed Gravitas run.
func (n *WorkerNode) CompleteJob(_ job.Details) error {
	scoreCalculationTime := time.Since(n.scoreCalculationStartedAt)

	tick := time.Now()
	if err := n.calculator.Scores(n.persistScore); err != nil {
		return err
	}
	scorePersistTime := time.Since(tick)

	n.cfg.Logger.WithFields(logrus.Fields{
		"processed_wallets":			len(n.calculator.Graph().Vertices()),
		"graph_populate_time":		n.graphPopulateTime.String(),
		"score_calculation_time":	scoreCalculationTime.String(),
		"score_persist_time":		scorePersistTime.String(),
		"total_pass_time":			time.Since(n.jobStartedAt).String(),
	}).Info("completed Gravitas update pass")
	return nil
}

func (n *WorkerNode) persistScore(wallet string, value *big.Float) error {
	score := &ss.Score{
		Wallet:	wallet,
		Scorer:	"balance_eth",
		Value:	value,	
	}

	return n.cfg.ScoreStoreAPI.UpsertScore(score)
}

// AbortJob implements job.Runner.
func (n *WorkerNode) AbortJob(_ job.Details) {}
