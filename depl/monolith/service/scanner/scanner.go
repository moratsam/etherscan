package scanner

import (
	"context"
	"io/ioutil"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
	scanner_pipeline "github.com/moratsam/etherscan/scanner"
)

//go:generate mockgen -package mocks -destination mocks/mocks.go github.com/moratsam/etherscan/depl/monolith/service/scanner ETHClient,GraphAPI
//go:generate mockgen -package mocks -destination mocks/mock_iterator.go github.com/moratsam/etherscan/txgraph/graph BlockIterator

//ETHClient is implemented by objects that can fetch an eth block by its number.
type ETHClient interface {
	BlockByNumber(number int) (*types.Block, error)
}

// GraphAPI defines as set of API methods for accessing the tx graph.
type GraphAPI interface {
	Blocks() (graph.BlockIterator, error)
	UpsertBlocks(blocks []*graph.Block) error
	InsertTxs(txs []*graph.Tx) error
	UpsertWallets(wallets []*graph.Wallet) error
}

// Config encapsulates the settings for configuring the eth-scanner service.
type Config struct {
	// An API for managing and interating blocks, transactions, wallets in the txgraph.
	GraphAPI GraphAPI

	// An API for fetching blocks.
	ETHClient ETHClient

	// The maximum number of concurrent workers used for fetching blocks.
	FetchWorkers int

	// The logger to use. If not defined an output-discarding logger will
	// be used instead.
	Logger *logrus.Entry
}

func (cfg *Config) validate() error {
	var err error
	if cfg.ETHClient == nil {
		err = multierror.Append(err, xerrors.Errorf("eth client API has not been provided"))
	}
	if cfg.GraphAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("graph API has not been provided"))
	}
	if cfg.FetchWorkers <= 0 {
		err = multierror.Append(err, xerrors.Errorf("invalid value for fetch workers"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// Service implements the eth-scanner component for the etherscan project.
type Service struct {
	cfg     Config
	scanner *scanner_pipeline.Scanner
}

// NewService creates a new scanner service instance with the specified config.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("scanner service: config validation failed: %w", err)
	}

	return &Service{
		cfg: cfg,
		scanner: scanner_pipeline.NewScanner(scanner_pipeline.Config{
			ETHClient: 		cfg.ETHClient,
			Graph:			cfg.GraphAPI,
			FetchWorkers:	cfg.FetchWorkers,
		}),
	}, nil
}

// Name implements service.Service
func (svc *Service) Name() string { return "scanner" }

// Run implements service.Service
func (svc *Service) Run(ctx context.Context) error {
	svc.cfg.Logger.Info("starting scanner")
	defer svc.cfg.Logger.Info("stopped service")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := svc.scan(ctx); err != nil {
				return err
			}
		}
	}
}

func (svc *Service) scan(ctx context.Context) error {
	svc.cfg.Logger.Info("starting new scan pass")

	blockIt, err := svc.cfg.GraphAPI.Blocks()
	if err != nil {
		return xerrors.Errorf("scanner: unable to retrieve block iterator: %w", err)
	}

	processed, err := svc.scanner.Scan(ctx, blockIt, svc.cfg.GraphAPI)
	svc.cfg.Logger.WithFields(logrus.Fields{
		"processed_blocks_count": processed,
	}).Info("exited scan")
	if err != nil {
		return xerrors.Errorf("scanner: unable to complete scanning: %w", err)
	} else if err = blockIt.Close(); err != nil {
		return xerrors.Errorf("scanner: unable to close the block iterator: %w", err)
	}
	return nil
}
