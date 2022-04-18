package service

import (
	"context"
	"io/ioutil"
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
	UpsertScores(scores []*ss.Score) error
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

// CompleteJob implements job.Runner. It persists the locally computed Gravitas
// scores after a successful execution of a distributed Gravitas run.
func (n *WorkerNode) CompleteJob(_ job.Details) error {
	scoreCalculationTime := time.Since(n.scoreCalculationStartedAt)

	tick := time.Now()
	if err := n.calculator.ScoresAll(n.persistScores); err != nil {
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

// AbortJob implements job.Runner.
func (n *WorkerNode) AbortJob(_ job.Details) {}

func (n *WorkerNode) persistScores(vertices []*bspgraph.Vertex) error {
	scores := make([]*ss.Score, len(vertices))
	for i, vertex := range vertices {
		scores[i] = &ss.Score{
			Wallet: vertex.ID(),
			Scorer: "balance_eth",
			Value: vertex.Value().(gravitas.VertexData).Value,
		}
	}

	return n.cfg.ScoreStoreAPI.UpsertScores(scores)
}

type vertex struct {
	address string
	txs []*txgraph.Tx
}

// Loads wallets and their transactions into the graph.
// To speed things up, <numWorkers> routines are simultaneously fetching txs.
func (n *WorkerNode) loadWallets(fromAddr, toAddr string) error {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	numWorkers := 10
	vertexCh := make(chan vertex, numWorkers)
	walletNumCh := make(chan int, 1)
	doneCh := make(chan struct{}, 1)
	addrCh := make(chan string, numWorkers)
	errCh := make(chan error, 1)
	for i:=0; i<numWorkers; i++ {
		go n.fetchWalletTxs(ctx, addrCh, vertexCh, errCh)
	}

	go n.addVertices(ctx, vertexCh, walletNumCh, doneCh)

	// Iterate over all the wallets and send their addresses to the addrCh.
	// If an error occurs in any of the fetchWalletTxs subroutines, return it.
	walletIt, err := n.cfg.GraphAPI.Wallets(fromAddr, toAddr)
	if err != nil {
		return err
	}
	walletNum := 0
	for walletIt.Next() {
		walletNum++
		wallet := walletIt.Wallet()
		select {
		case err := <-errCh:
			_ = walletIt.Close()
			return err
		case addrCh <- wallet.Address:
		}
	}

	if err = walletIt.Error(); err != nil {
		_ = walletIt.Close()
		return err
	}
	if err = walletIt.Close(); err != nil {
		return err
	}

	// Send the total number of wallets to the addVertices routine, so it knows when it's done.
	walletNumCh <- walletNum

	// Wait for the addVertices routine to finish or an error to occur.
	select {
	case <- doneCh:
		return nil
	case err := <-errCh:
		return err
	}
}

// Reads wallet addresses from the addrCh, creates a iterator for a wallets transactions,
// sends the address + txs data as a vertex to the addVertices.
// If it encounters an error, it sends it to the errCh.
func (n *WorkerNode) fetchWalletTxs(ctx context.Context, addrCh <-chan string, vertexCh chan<- vertex, errCh chan<- error) {
	var addr string
	for {
		select {
		case <-ctx.Done():
			return
		case addr = <-addrCh:
			// Retrieve wallet's transactions.
			txIt, err := n.cfg.GraphAPI.WalletTxs(addr)
			if err != nil {
				maybeEmitError(err, errCh)
				return
			}
			var txs []*txgraph.Tx
			for txIt.Next() {
				txs = append(txs, txIt.Tx())	
			}
			if err = txIt.Error(); err != nil {
				_ = txIt.Close()
				maybeEmitError(err, errCh)
				return
			}
			if err = txIt.Close(); err != nil {
				maybeEmitError(err, errCh)
				return
			}
			
			// Send the vertex data to the vertexCh.
			vertexCh <- vertex{address: addr, txs: txs}
		}
	}
}

// Reads vertices from the vertexCh and adds them to the graph.
// Receives the total number of vertices via the walletNumCh.
// Signals via the doneCh when it's done.
func (n *WorkerNode) addVertices(ctx context.Context, vertexCh <-chan vertex, walletNumCh <-chan int, doneCh chan<- struct{}) {
	walletNum := -1
	walletNumSeen := 0
	var v vertex
	for {
		if walletNumSeen == walletNum {
			doneCh <- struct{}{}
			return
		}
		select {
		case <-ctx.Done():
			return
		case walletNum = <-walletNumCh:
		case v = <-vertexCh:
			walletNumSeen++
			// Add vertex
			n.calculator.AddVertex(v.address, v.txs)
		}
	}
}

func maybeEmitError(err error, errCh chan<- error) {
	select {
		case errCh <- err: // error emitted.
		default: // error channel is full with other errors.
	}
}
