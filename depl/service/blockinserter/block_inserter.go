package blockinserter

import (
	"context"
	"io/ioutil"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
	binserter "github.com/moratsam/etherscan/blockinserter"
)

//go:generate mockgen -package mocks -destination mocks/mocks.go github.com/moratsam/etherscan/depl/service/scanner ETHClient,GraphAPI
//go:generate mockgen -package mocks -destination mocks/mock_iterator.go github.com/moratsam/etherscan/txgraph/graph BlockIterator

//ETHClient is implemented by objects that can fetch an eth block by its number.
type ETHClient interface {
	SubscribeNewHead(ctx context.Context) (<-chan *types.Header, ethereum.Subscription, error)
}

// GraphAPI is implemented by objects that can insert transactions into a tx graph instance.
type GraphAPI interface {
	UpsertBlock(block *graph.Block) error
}

// Config encapsulates the settings for configuring the block-inserter service.
type Config struct {
	// An API for managing and interating blocks, transactions, wallets in the txgraph.
	GraphAPI GraphAPI

	// An API for fetching blocks.
	ETHClient ETHClient

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
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// Service implements the block-inserter component for the etherscan project.
type Service struct {
	cfg     			Config
	blockInserter	*binserter.BlockInserter
}

// NewService creates a new block-inserter service instance with the specified config.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("block-inserter service: config validation failed: %w", err)
	}

	return &Service{
		cfg: cfg,
		blockInserter: binserter.NewBlockInserter(binserter.Config{
			ETHClient:	cfg.ETHClient,
			Graph:		cfg.GraphAPI,
		}),
	}, nil
}

// Name implements service.Service
func (svc *Service) Name() string { return "block-inserter" }

// Run implements service.Service
func (svc *Service) Run(ctx context.Context) error {
	defer svc.cfg.Logger.Info("stopped service")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			svc.cfg.Logger.Info("starting service block-inserter")
			if err := svc.blockInserter.Start(ctx); err != nil {
				return err
			}
		}
	}
}
