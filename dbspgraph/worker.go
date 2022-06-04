package dbspgraph

import (
	"context"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	"github.com/moratsam/etherscan/dbspgraph/job"
	"github.com/moratsam/etherscan/dbspgraph/proto"
)

// Worker coordinates the execution of a distributed graph-based algorithm
// built on top of the bspgraph framework with a remote master node.
type Worker struct {
	cfg WorkerConfig

	masterConn *grpc.ClientConn
	masterCli  proto.JobQueueClient
}

// NewWorker creates a new Worker instance with the specified configuration.
func NewWorker(cfg WorkerConfig) (*Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, xerrors.Errorf("worker config validation failed: %w", err)
	}

	return &Worker{cfg: cfg}, nil
}

// Dial establishes a connection to the master node.
func (w *Worker) Dial(masterEndpoint string, dialTimeout time.Duration) error {
	var dialCtx context.Context
	if dialTimeout != 0 {
		var cancelFn func()
		dialCtx, cancelFn = context.WithTimeout(context.Background(), dialTimeout)
		defer cancelFn()
	}

	conn, err := grpc.DialContext(dialCtx, masterEndpoint, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*20), grpc.MaxCallSendMsgSize(1024*1024*20)))
	if err != nil {
		return xerrors.Errorf("unable to dial master: %w", err)
	}

	w.masterConn = conn
	w.masterCli = proto.NewJobQueueClient(conn)
	return nil
}

// Close shuts down the worker.
func (w *Worker) Close() error {
	var err error
	if w.masterConn != nil {
		err = w.masterConn.Close()
		w.masterConn = nil
	}
	return err
}

// RunJob waits for a new job announcement from the master and coordinates its
// execution with the master until it either completes or is aborted due to a
// context expiration or a local/remote error.
func (w *Worker) RunJob(ctx context.Context) error {
	stream, err := w.masterCli.JobStream(ctx)
	if err != nil {
		return err
	}

	w.cfg.Logger.Info("waiting for next job")
	jobDetails, err := w.waitForJob(stream)
	if err != nil {
		return err
	}

	masterStream := newRemoteMasterStream(stream)
	jobLogger := w.cfg.Logger.WithField("job_id", jobDetails.JobID)
	coordinator := newWorkerJobCoordinator(ctx, workerJobCoordinatorConfig{
		jobDetails:   jobDetails,
		masterStream: masterStream,
		jobRunner:    w.cfg.JobRunner,
		serializer:   w.cfg.Serializer,
		logger:       jobLogger,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := masterStream.HandleSendRecv(); err != nil {
			coordinator.cancelJobCtx()
		}
	}()
	jobLogger.WithFields(logrus.Fields{
		"created_at":          jobDetails.CreatedAt,
		"partition_from_addr": jobDetails.PartitionFromAddr,
		"partition_to_addr":   jobDetails.PartitionToAddr,
	}).Info("starting new job")

	if err = coordinator.RunJob(); err != nil {
		jobLogger.WithField("err", err).Error("job execution failed")
	} else {
		jobLogger.Info("job completed successfully")
	}
	masterStream.Close()
	wg.Wait()
	return err
}

// waitForJob blocks waiting for a job announcement forom the master node.
func (w *Worker) waitForJob(jobStream proto.JobQueue_JobStreamClient) (job.Details, error) {
	var jobDetails job.Details

	mMsg, err := jobStream.Recv()
	if err != nil {
		return jobDetails, xerrors.Errorf("unable to read job details from master: %w", err)
	}

	jobDetailsMsg := mMsg.GetJobDetails()
	if jobDetailsMsg == nil {
		return jobDetails, xerrors.Errorf("expected master to send a JobDetails message")
	}

	jobDetails.JobID = jobDetailsMsg.JobId
	if jobDetails.CreatedAt, err = types.TimestampFromProto(jobDetailsMsg.CreatedAt); err != nil {
		return jobDetails, xerrors.Errorf("unable to parse job creation time: %w", err)
	}
	jobDetails.PartitionFromAddr = jobDetailsMsg.PartitionFromAddr
	jobDetails.PartitionToAddr = jobDetailsMsg.PartitionToAddr

	return jobDetails, nil
}
