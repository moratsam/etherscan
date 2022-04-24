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
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	"github.com/moratsam/etherscan/depl/microservices/gravitas/service"
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
		cli.StringFlag{
			Name:	"mode",
			EnvVar: "MODE",
			Usage:  "The operation mode to use (master or worker)",
		},
		cli.StringFlag{
			Name:	"master-endpoint",
			EnvVar: "MASTER_ENDPOINT",
			Usage:  "The endpoint for connecting to the master node (worker mode)",
		},
		cli.DurationFlag{
			Name:	"master-dial-timeout",
			EnvVar: "MASTER_DIAL_TIMEOUT",
			Value:  25 * time.Second,
			Usage:  "The timeout for establishing a connection to the master node (worker mode)",
		},
		cli.IntFlag{
			Name:	"master-port",
			Value:  8080,
			EnvVar: "MASTER_PORT",
			Usage:  "The port where the master listens for incoming connections (master mode)",
		},
		cli.DurationFlag{
			Name:	"gravitas-update-interval",
			Value:  5 * time.Minute,
			EnvVar: "GRAVITAS_UPDATE_INTERVAL",
			Usage:  "The time between subsequent gravitas score updates",
		},
		cli.IntFlag{
			Name:	"min-workers-for-update",
			Value:  1,
			EnvVar: "MIN_WORKERS_FOR_UPDATE",
			Usage:  "The minimum number of workers that must be connected before making a new pass; 0 indicates that at least one worker is required (master mode)",
		},
		cli.DurationFlag{
			Name:	"worker-acquire-timeout",
			Value:  25*time.Second,
			EnvVar: "WORKER_ACQUIRE_TIMEOUT",
			Usage:  "The time that the master waits for the requested number of workers to be connected before skipping a pass (master mode)",
		},
		cli.IntFlag{
			Name:	"gravitas-tx-fetchers",
			Value:  5,
			EnvVar: "GRAVITAS_TX_FETCHERS",
			Usage:  "The number of workers that will concurrently fetch wallet txs and load them into BSP graph",
		},
		cli.IntFlag{
			Name:	"gravitas-num-workers",
			Value:  runtime.NumCPU(),
			EnvVar: "GRAVITAS_NUM_WORKERS",
			Usage:  "The number of workers to use for calculating gravitas scores (defaults to number of CPUs)",
		},
		cli.StringFlag{
			Name:	"score-store-api",
			EnvVar: "SCORE_STORE_API",
			Usage:  "The gRPC endpoint for connecting to the score store",
		},
		cli.StringFlag{
			Name:	"tx-graph-api",
			EnvVar: "TX_GRAPH_API",
			Usage:  "The gRPC endpoint for connecting to the tx graph",
		},
		cli.IntFlag{
			Name:	"pprof-port",
			Value:  6060,
			EnvVar: "PPROF_PORT",
			Usage:  "The port for exposing pprof endpoints",
		},
	}
	app.Action = runMain
	return app
}

func runMain(appCtx *cli.Context) error {
	var (
		serviceRunner interface {
			Run(context.Context) error
		}
		ctx, cancelFn = context.WithCancel(context.Background())
		err			  error
		logger		  = logger.WithField("mode", appCtx.String("mode"))
	)
	defer cancelFn()

	switch appCtx.String("mode") {
	case "master":
		if serviceRunner, err = service.NewMasterNode(service.MasterConfig{
			ListenAddress:				fmt.Sprintf(":%d", appCtx.Int("master-port")),
			UpdateInterval:			appCtx.Duration("gravitas-update-interval"),
			MinWorkers:					appCtx.Int("min-workers-for-update"),
			WorkerAcquireTimeout:	appCtx.Duration("worker-acquire-timeout"),
			Logger:						logger,
		}); err != nil {
			return err
		}
	case "worker":
		scoreStoreAPI, txGraphAPI, err := getAPIs(ctx, appCtx.String("score-store-api"), appCtx.String("tx-graph-api"))
		if err != nil {
			return err
		}

		if serviceRunner, err = service.NewWorkerNode(service.WorkerConfig{
			MasterEndpoint:		appCtx.String("master-endpoint"),
			MasterDialTimeout:	appCtx.Duration("master-dial-timeout"),
			GraphAPI:				txGraphAPI,
			ScoreStoreAPI:			scoreStoreAPI,
			TxFetchers:				appCtx.Int("gravitas-tx-fetchers"),
			ComputeWorkers:	 	appCtx.Int("gravitas-num-workers"),
			Logger:					logger,
		}); err != nil {
			return err
		}
	default:
		return xerrors.Errorf("unsupported mode %q; please specify one of: master, worker", appCtx.String("role"))
	}

	var wg sync.WaitGroup
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := serviceRunner.Run(ctx); err != nil {
			logger.WithField("err", err).Error("gravitas service exited with error")
			cancelFn()
		}
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
			_ = pprofListener.Close()
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

	dialCtx, cancelFn := context.WithTimeout(ctx, 15*time.Second)
	defer cancelFn()
	scoreStoreConn, err := grpc.DialContext(dialCtx, scoreStoreAPI, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, xerrors.Errorf("could not connect to score store API: %w", err)
	}
	scoreStoreCli := ssapi.NewScoreStoreClient(ctx, protossapi.NewScoreStoreClient(scoreStoreConn))

	dialCtx, cancelFn = context.WithTimeout(ctx, 15*time.Second)
	defer cancelFn()
	txGraphConn, err := grpc.DialContext(dialCtx, txGraphAPI, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, xerrors.Errorf("could not connect to tx graph API: %w", err)
	}
	txGraphCli := txgraphapi.NewTxGraphClient(ctx, prototxgraphapi.NewTxGraphClient(txGraphConn))

	return scoreStoreCli, txGraphCli, nil
}
