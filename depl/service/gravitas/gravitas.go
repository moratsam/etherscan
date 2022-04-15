package gravitas

import (
	"context"
	"math/big"
	"io/ioutil"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/juju/clock"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/depl/partition"
	"github.com/moratsam/etherscan/gravitas"
	ss "github.com/moratsam/etherscan/scorestore"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
)

//go:generate mockgen -package mocks -destination mocks/mocks.go github.com/moratsam/etherscan/depl/service/gravitas GraphAPI,ScoreStoreAPI
//go:generate mockgen -package mocks -destination mocks/mock_iterator.go github.com/moratsam/etherscan/txgraph/graph TxIterator,WalletIterator

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

// Config encapsulates the settings for configuring the Gravitas calculator service.
type Config struct {
	// An API for interating wallets and their transactions from the txgraph.
	GraphAPI GraphAPI

	// An API for updating the Gravitas scores of wallets.
	ScoreStoreAPI ScoreStoreAPI

	// An API for detecting the partition assignments for this service.
	PartitionDetector partition.Detector

	// A clock instance for generating time-related events. If not specified,
	// the default wall-clock will be used instead.
	Clock clock.Clock

	// The number of workers to spin up for computing Gravitas scores. If
	// not specified, a default value of 1 will be used instead.
	ComputeWorkers int

	// The time between subsequent gravitas calculation passes.
	UpdateInterval time.Duration

	// The logger to use. If not defined an output-discarding logger will
	// be used instead.
	Logger *logrus.Entry
}

func (cfg *Config) validate() error {
	var err error
	if cfg.GraphAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("graph API has not been provided"))
	}
	if cfg.ScoreStoreAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("scorestore API has not been provided"))
	}
	if cfg.PartitionDetector == nil {
		err = multierror.Append(err, xerrors.Errorf("partition detector has not been provided"))
	}
	if cfg.Clock == nil {
		cfg.Clock = clock.WallClock
	}
	if cfg.ComputeWorkers <= 0 {
		err = multierror.Append(err, xerrors.Errorf("invalid value for compute workers"))
	}
	if cfg.UpdateInterval == 0 {
		err = multierror.Append(err, xerrors.Errorf("invalid value for update interval"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// Service implements the Gravitas calculator component for the Links 'R' Us project.
type Service struct {
	cfg        Config
	calculator *gravitas.Calculator
}

// NewService creates a new Gravitas calculator service instance with the specified config.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("gravitas service: config validation failed: %w", err)
	}

	calculator, err := gravitas.NewCalculator(gravitas.Config{ComputeWorkers: cfg.ComputeWorkers})
	if err != nil {
		return nil, xerrors.Errorf("gravitas service: new calculator creation failed: %w", err)
	}

	return &Service{
		cfg:        cfg,
		calculator: calculator,
	}, nil
}

// Name implements service.Service
func (svc *Service) Name() string { return "Gravitas calculator" }

// Run implements service.Service
func (svc *Service) Run(ctx context.Context) error {
	svc.cfg.Logger.WithField("update_interval", svc.cfg.UpdateInterval.String()).Info("starting gravitas calculator")
	defer svc.cfg.Logger.Info("stopped service")

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-svc.cfg.Clock.After(svc.cfg.UpdateInterval):
			curPartition, _, err := svc.cfg.PartitionDetector.PartitionInfo()
			if err != nil {
				if xerrors.Is(err, partition.ErrNoPartitionDataAvailableYet) {
					svc.cfg.Logger.Warn("deferring Gravitas update pass: partition data not yet available")
					continue
				}
				return err
			}

			if curPartition != 0 {
				svc.cfg.Logger.Info("service can only run on the leader of the application cluster")
				return nil
			}

			if err := svc.updateGraphScores(ctx); err != nil {
				return err
			}
		}
	}
}

func (svc *Service) updateGraphScores(ctx context.Context) error {
	svc.cfg.Logger.Info("starting gravitas update pass")
	startAt := svc.cfg.Clock.Now()
	tick := startAt

	fromAddr := "0000000000000000000000000000000000000000"
	toAddr := "ffffffffffffffffffffffffffffffffffffffff"
	if err := svc.calculator.Graph().Reset(); err != nil {
		return err
	} else if err := svc.loadWallets(fromAddr, toAddr); err != nil {
		return err
	}
	graphPopulateTime := svc.cfg.Clock.Now().Sub(tick)

	tick = svc.cfg.Clock.Now()
	if err := svc.calculator.Executor().RunToCompletion(ctx); err != nil {
		return err
	}
	scoreCalculationTime := svc.cfg.Clock.Now().Sub(tick)

	tick = svc.cfg.Clock.Now()
	if err := svc.calculator.Scores(svc.persistScore); err != nil {
		return err
	}
	scorePersistTime := svc.cfg.Clock.Now().Sub(tick)

	svc.cfg.Logger.WithFields(logrus.Fields{
		"processed_wallets":        len(svc.calculator.Graph().Vertices()),
		"graph_populate_time":    graphPopulateTime.String(),
		"score_calculation_time": scoreCalculationTime.String(),
		"score_persist_time":     scorePersistTime.String(),
		"total_pass_time":        svc.cfg.Clock.Now().Sub(startAt).String(),
	}).Info("completed Gravitas update pass")
	return nil
}

func (svc *Service) persistScore(wallet string, value *big.Float) error {
	score := &ss.Score{
		Wallet:	wallet,
		Scorer:	"balance_eth",
		Value:	value,	
	}

	return svc.cfg.ScoreStoreAPI.UpsertScore(score)
}

func (svc *Service) loadWallets(fromAddr, toAddr string) error {
	walletIt, err := svc.cfg.GraphAPI.Wallets(fromAddr, toAddr)
	if err != nil {
		return err
	}

	for walletIt.Next() {
		wallet := walletIt.Wallet()
		
		// Retrieve wallet's transactions.
		txIt, err := svc.cfg.GraphAPI.WalletTxs(wallet.Address)
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
		svc.calculator.AddVertex(wallet.Address, txs)
	}
	if err = walletIt.Error(); err != nil {
		_ = walletIt.Close()
		return err
	}

	return walletIt.Close()
}
