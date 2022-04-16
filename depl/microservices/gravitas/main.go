package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	"github.com/moratsam/etherscan/depl/monolith/partition"
	"github.com/moratsam/etherscan/depl/monolith/service/gravitas"
	ssapi "github.com/moratsam/etherscan/scorestoreapi"
	protossapi "github.com/moratsam/etherscan/scorestoreapi/proto"
	"github.com/moratsam/etherscan/txgraphapi"
	prototxgraphapi "github.com/moratsam/etherscan/txgraphapi/proto"
)

var (
	appName = "etherscan-gravitas"
	appSha  = "populated-at-link-time"
	logger  *logrus.Entry
)

func main() {
	host, _ := os.Hostname()
	rootLogger := logrus.New()
	rootLogger.SetFormatter(new(logrus.JSONFormatter))
	logger = rootLogger.WithFields(logrus.Fields{
		"app":  appName,
		"sha":  appSha,
		"host": host,
	})

	if err := makeApp().Run(os.Args); err != nil {
		logger.WithField("err", err).Error("shutting down due to error")
		_ = os.Stderr.Sync()
		os.Exit(1)
	}
}

func makeApp() *cli.App {
	app := cli.NewApp()
	app.Name = appName
	app.Version = appSha
	app.Flags = []cli.Flag{
		cli.DurationFlag{
			Name:   "gravitas-update-interval",
			Value:  5 * time.Minute,
			EnvVar: "GRAVITAS_UPDATE_INTERVAL",
			Usage:  "The time between subsequent gravitas score updates",
		},
		cli.IntFlag{
			Name:   "gravitas-num-workers",
			Value:  runtime.NumCPU(),
			EnvVar: "GRAVITAS_NUM_WORKERS",
			Usage:  "The number of workers to use for calculating gravitas scores (defaults to number of CPUs)",
		},
		cli.StringFlag{
			Name:   "partition-detection-mode",
			Value:  "single",
			EnvVar: "PARTITION_DETECTION_MODE",
			Usage:  "The partition detection mode to use. Supported values are 'dns=HEADLESS_SERVICE_NAME' (k8s) and 'single' (local dev mode)",
		},
		cli.StringFlag{
			Name:   "score-store-api",
			EnvVar: "SCORE_STORE_API",
			Usage:  "The gRPC endpoint for connecting to the score store",
		},
		cli.StringFlag{
			Name:   "tx-graph-api",
			EnvVar: "TX_GRAPH_API",
			Usage:  "The gRPC endpoint for connecting to the tx graph",
		},
		cli.IntFlag{
			Name:   "pprof-port",
			Value:  6060,
			EnvVar: "PPROF_PORT",
			Usage:  "The port for exposing pprof endpoints",
		},
	}
	app.Action = runMain
	return app
}

func runMain(appCtx *cli.Context) error {
	var wg sync.WaitGroup
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	partDet, err := getPartitionDetector(appCtx.String("partition-detection-mode"))
	if err != nil {
		return err
	}

	scoreStoreAPI, txGraphAPI, err := getAPIs(ctx, appCtx.String("score-store-api"), appCtx.String("tx-graph-api"))
	if err != nil {
		return err
	}

	var gravitasCfg gravitas.Config
	gravitasCfg.ComputeWorkers = appCtx.Int("gravitas-num-workers")
	gravitasCfg.UpdateInterval = appCtx.Duration("gravitas-update-interval")
	gravitasCfg.GraphAPI = txGraphAPI
	gravitasCfg.ScoreStoreAPI = scoreStoreAPI
	gravitasCfg.PartitionDetector = partDet
	gravitasCfg.Logger = logger
	svc, err := gravitas.NewService(gravitasCfg)
	if err != nil {
		return err
	}

	// Start gravitas.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := svc.Run(ctx); err != nil {
			logger.WithField("err", err).Error("gravitas service exited with error")
			cancelFn()
		}
	}()

	// Start pprof server
	pprofListener, err := net.Listen("tcp", fmt.Sprintf(":%d", appCtx.Int("pprof-port")))
	if err != nil {
		return err
	}
	defer func() { _ = pprofListener.Close() }()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.WithField("port", appCtx.Int("pprof-port")).Info("listening for pprof requests")
		srv := new(http.Server)
		_ = srv.Serve(pprofListener)
	}()

	// Start signal watcher
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGHUP)
		select {
		case s := <-sigCh:
			logger.WithField("signal", s.String()).Infof("shutting down due to signal")
			_ = pprofListener.Close()
			cancelFn()
		case <-ctx.Done():
		}
	}()

	// Keep running until we receive a signal
	wg.Wait()
	return nil
}

func getAPIs(ctx context.Context, scoreStoreAPI, txGraphAPI string) (*ssapi.ScoreStoreClient, *txgraphapi.TxGraphClient, error) {
	if scoreStoreAPI == "" {
		return nil, nil, xerrors.Errorf("score store API must be specified with --score-store-api")
	}
	if txGraphAPI == "" {
		return nil, nil, xerrors.Errorf("tx graph API must be specified with --tx-graph-api")
	}

	dialCtx, cancelFn := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFn()
	scoreStoreConn, err := grpc.DialContext(dialCtx, scoreStoreAPI, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, xerrors.Errorf("could not connect to score store API: %w", err)
	}
	scoreStoreCli := ssapi.NewScoreStoreClient(ctx, protossapi.NewScoreStoreClient(scoreStoreConn))

	dialCtx, cancelFn = context.WithTimeout(ctx, 5*time.Second)
	defer cancelFn()
	txGraphConn, err := grpc.DialContext(dialCtx, txGraphAPI, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, xerrors.Errorf("could not connect to tx graph API: %w", err)
	}
	txGraphCli := txgraphapi.NewTxGraphClient(ctx, prototxgraphapi.NewTxGraphClient(txGraphConn))

	return scoreStoreCli, txGraphCli, nil
}

func getPartitionDetector(mode string) (partition.Detector, error) {
	switch {
	case mode == "single":
		return partition.Fixed{Partition: 0, NumPartitions: 1}, nil
	case strings.HasPrefix(mode, "dns="):
		tokens := strings.Split(mode, "=")
		return partition.DetectFromSRVRecords(tokens[1]), nil
	default:
		return nil, xerrors.Errorf("unsupported partition detection mode: %q", mode)
	}
}
