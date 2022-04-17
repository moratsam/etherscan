package service

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/juju/clock"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/bspgraph"
	"github.com/moratsam/etherscan/dbspgraph"
	"github.com/moratsam/etherscan/dbspgraph/job"
	"github.com/moratsam/etherscan/gravitas"
)

// MasterConfig encapsulates the settings for configuring the master node for
// the Gravitas calculator service.
type MasterConfig struct {
	// The address to listen for incoming worker connections.
	ListenAddress string

	// The minimum required number of connected workers for starting a new
	// Gravitas pass. If not specified, a new pass will start when at least
	// one worker has connected.
	MinWorkers int

	// The timeout for the required number of workers to connect before
	// aborting a new pass attempt. If not specified, the master will wait
	// indefinitely.
	WorkerAcquireTimeout time.Duration

	// A clock instance for generating time-related events. If not specified,
	// the default wall-clock will be used instead.
	Clock clock.Clock

	// The time between subsequent gravitas updates.
	UpdateInterval time.Duration

	// The logger to use. If not defined an output-discarding logger will
	// be used instead.
	Logger *logrus.Entry
}

func (cfg *MasterConfig) validate() error {
	var err error
	if cfg.ListenAddress == "" {
		err = multierror.Append(err, xerrors.Errorf("invalid value for listen address"))
	}
	if cfg.Clock == nil {
		cfg.Clock = clock.WallClock
	}
	if cfg.UpdateInterval == 0 {
		err = multierror.Append(err, xerrors.Errorf("invalid value for update interval"))
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(&logrus.Logger{Out: ioutil.Discard})
	}
	return err
}

// MasterNode implements a master node for calculating Gravitas scores in a
// distributed fashion.
type MasterNode struct {
	cfg        MasterConfig
	calculator *gravitas.Calculator

	masterFacade *dbspgraph.Master

	// Stats
	jobStartedAt time.Time
}

// NewMasterNode creates a new master node for the Gravitas calculator service.
func NewMasterNode(cfg MasterConfig) (*MasterNode, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("gravitas service: config validation failed: %w", err)
	}

	calculator, err := gravitas.NewCalculator(gravitas.Config{ComputeWorkers: 1})
	if err != nil {
		return nil, xerrors.Errorf("gravitas service: config validation failed: %w", err)
	}

	masterNode := &MasterNode{
		cfg:        cfg,
		calculator: calculator,
	}

	if masterNode.masterFacade, err = dbspgraph.NewMaster(dbspgraph.MasterConfig{
		ListenAddress: cfg.ListenAddress,
		JobRunner:     masterNode,
		Serializer:    serializer{},
		Logger:        cfg.Logger,
	}); err != nil {
		_ = calculator.Close()
		return nil, err
	}

	if err = masterNode.masterFacade.Start(); err != nil {
		_ = calculator.Close()
		return nil, err
	}

	return masterNode, nil
}

// Run implements the main loop of the master node for the distributed Gravitas
// calculator. It periodically wakes up and orchestrates the execution of a new
// Gravitas update pass across all connected workers.
//
// Run blocks until the provided context expires.
func (n *MasterNode) Run(ctx context.Context) error {
	n.cfg.Logger.WithField("update_interval", n.cfg.UpdateInterval.String()).Info("starting service")
	defer func() {
		_ = n.masterFacade.Close()
		_ = n.calculator.Close()
		n.cfg.Logger.Info("stopped service")
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-n.cfg.Clock.After(n.cfg.UpdateInterval):
			if err := n.masterFacade.RunJob(ctx, n.cfg.MinWorkers, n.cfg.WorkerAcquireTimeout); err != nil {
				if err == dbspgraph.ErrUnableToReserveWorkers {
					n.cfg.Logger.WithFields(logrus.Fields{
						"min_workers":     n.cfg.MinWorkers,
						"acquire_timeout": n.cfg.WorkerAcquireTimeout.String(),
					}).Error("unable to acquire the requested number of workers")
					continue
				}
				n.cfg.Logger.WithField("err", err).Error("Gravitas update job failed")
			}
		}
	}
}

// StartJob implements job.Runner. It initializes the underlying bspgraph.Graph
// instance and invokes the provided ExecutorFactory to create an executor for
// the graph supersteps.
func (n *MasterNode) StartJob(_ job.Details, execFactory bspgraph.ExecutorFactory) (*bspgraph.Executor, error) {
	if err := n.calculator.Graph().Reset(); err != nil {
		return nil, err
	}

	n.jobStartedAt = n.cfg.Clock.Now()
	n.calculator.SetExecutorFactory(execFactory)
	return n.calculator.Executor(), nil
}

// CompleteJob implements job.Runner.
func (n *MasterNode) CompleteJob(_ job.Details) error {
	n.cfg.Logger.WithFields(logrus.Fields{
		"processed_wallets":      len(n.calculator.Graph().Vertices()),
		"total_pass_time":        n.cfg.Clock.Now().Sub(n.jobStartedAt).String(),
	}).Info("completed Gravitas update pass")
	return nil
}

// AbortJob implements job.Runner.
func (n *MasterNode) AbortJob(_ job.Details) {}
