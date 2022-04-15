package frontend

import (
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
)

// Config encapsulates the settings for configuring a frontend instance.
type Config struct {
	// An API for executing queries against the score store.
	ScoreStoreAPI ScoreStoreAPI

	// The port to listen for incoming requests.
	ListenAddr string

	// The number of results to display per page.
	ResultsPerPage int
}

func (cfg *Config) validate() error {
	var err error
	if cfg.ListenAddr == "" {
		err = multierror.Append(err, xerrors.Errorf("listen address has not been specified"))
	}
	if cfg.ResultsPerPage <= 0 {
		err = multierror.Append(err, xerrors.Errorf("results per page has not been specified"))
	}
	if cfg.ScoreStoreAPI == nil {
		err = multierror.Append(err, xerrors.Errorf("scorestore API has not been provided"))
	}
	return err
}

