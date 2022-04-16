package frontend

import (
	"context"
	"io/ioutil"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/frontend"
	ss "github.com/moratsam/etherscan/scorestore"
)

const defaultResultsPerPage = 30

// ScoreStoreAPI defines a set of API methods for searching the score store
// for calculated gravitas scores.
type ScoreStoreAPI interface {
	Search(query ss.Query) (ss.ScoreIterator, error)
}

// Config encapsulates the settings for configuring the front-end service.
type Config struct {
	// An API for executing queries against the score store.
	ScoreStoreAPI ScoreStoreAPI

	// The port to listen for incoming requests.
	ListenAddr string

	// The number of results to display per page. If not specified, a default
	// value of 30 results per page will be used instead.
	ResultsPerPage int

	// The logger to use. If not defined an output-discarding logger will
	// be used instead.
	Logger *logrus.Entry
}

func (cfg *Config) validate() error {
	var err error
	if cfg.ListenAddr == "" {
		err = multierror.Append(err, xerrors.Errorf("listen address has not been specified"))
	}
	if cfg.ResultsPerPage <= 0 {
		cfg.ResultsPerPage = defaultResultsPerPage
	}
	if cfg.ScoreStoreAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("scorestore API has not been provided"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// Service implements the front-end component for the etherscan project.
type Service struct {
	cfg   	Config
	frontend	*frontend.Frontend
}

// NewService creates a new front-end service instance with the specified config.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("front-end service: config validation failed: %w", err)
	}

	frontend, err := frontend.NewFrontend(frontend.Config{
		ScoreStoreAPI:		cfg.ScoreStoreAPI,
		ListenAddr:			cfg.ListenAddr,
		ResultsPerPage:	cfg.ResultsPerPage,
	})
	if err != nil {
		return nil, xerrors.Errorf("front-end service: new frontend creation failed: %w", err)
	}

	svc := &Service{
		cfg:    		cfg,
		frontend:	frontend,
	}

	return svc, nil
}

// Name implements service.Service
func (svc *Service) Name() string { return "front-end" }

// Run implements service.Service
func (svc *Service) Run(ctx context.Context) error {
	svc.cfg.Logger.WithField("addr", svc.cfg.ListenAddr).Info("starting front-end server")
	err := svc.frontend.Serve(ctx)
	return err
}
