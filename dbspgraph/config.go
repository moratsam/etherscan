package dbspgraph

import (
	"io/ioutil"

	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/dbspgraph/job"
)

//go:generate mockgen -package mocks -destination mocks/mocks_serializer.go github.com/moratsam/etherscan/dbspgraph Serializer
//go:generate mockgen -package mocks -destination mocks/mocks_job.go github.com/moratsam/etherscan/dbspgraph/job Runner

// Serializer is implemented by types that can serialize aggregator and
// graph messages from and to an types.Any value.
type Serializer interface {
	// Serialize encodes the given value into an types.Any protobuf message.
	Serialize(interface{}) (*types.Any, error)

	// Unserialize decodes the given types.Any protobuf value.
	Unserialize(*types.Any) (interface{}, error)
}

// MasterConfig encapsulates the configuration options for a master node.
type MasterConfig struct {
	// The address where the master will listen for incoming gRPC
	// connections from workers.
	ListenAddress string

	// JobRunner
	JobRunner job.Runner

	// A helper for serializing and unserializing aggregator values.
	Serializer Serializer

	// A logger instance to use. If not specified, a null logger will be
	// used instead.
	Logger *logrus.Entry
}

// Validate the config options.
func (cfg *MasterConfig) Validate() error {
	var err error
	if cfg.ListenAddress == "" {
		err = multierror.Append(err, xerrors.Errorf("listen address not specified"))
	}
	if cfg.JobRunner == nil {
		err = multierror.Append(err, xerrors.Errorf("job runner not specified"))
	}
	if cfg.Serializer == nil {
		err = multierror.Append(err, xerrors.Errorf("aggregator serializer not specified"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// WorkerConfig encapsulates the configuration options for a worker node.
type WorkerConfig struct {
	// JobRunner
	JobRunner job.Runner

	// A helper for serializing and unserializing aggregator values and
	// vertex messages to/from protocol buffer messages.
	Serializer Serializer

	// A logger instance to use. If not specified, a null logger will be
	// used instead.
	Logger *logrus.Entry
}

// Validate the config options.
func (cfg *WorkerConfig) Validate() error {
	var err error
	if cfg.JobRunner == nil {
		err = multierror.Append(err, xerrors.Errorf("job runner not specified"))
	}
	if cfg.Serializer == nil {
		err = multierror.Append(err, xerrors.Errorf("message/aggregator serializer not specified"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}
