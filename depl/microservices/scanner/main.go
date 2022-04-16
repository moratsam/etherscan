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

	"github.com/moratsam/etherscan/depl/monolith/service/scanner"
	"github.com/moratsam/etherscan/ethclient"
	"github.com/moratsam/etherscan/txgraphapi"
	prototxgraphapi "github.com/moratsam/etherscan/txgraphapi/proto"
)

var (
	appName = "etherscan-scanner"
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
			Name:   "tx-graph-api",
			EnvVar: "TX_GRAPH_API",
			Usage:  "The gRPC endpoint for connecting to the tx graph",
		},
		cli.IntFlag{
			Name:   "scanner-num-workers",
			Value:  runtime.NumCPU(),
			EnvVar: "SCANNER_NUM_WORKERS",
			Usage:  "The maximum number of workers to use for scanning eth blocks (defaults to number of CPUs)",
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

	// Retrieve an ethclient.
	ethClient, err := ethclient.NewETHClient()
	if err != nil {
		logger.WithField("err", err).Error("new eth client")
		return nil
	}

	txGraphAPI, err := getTxGraphAPI(ctx, appCtx.String("tx-graph-api"))
	if err != nil {
		return err
	}

	var scannerCfg scanner.Config
	scannerCfg.ETHClient = ethClient
	scannerCfg.GraphAPI = txGraphAPI
	scannerCfg.Logger = logger
	svc, err := scanner.NewService(scannerCfg)
	if err != nil {
		return err
	}

	// Start scanner.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := svc.Run(ctx); err != nil {
			logger.WithField("err", err).Error("scanner service exited with error")
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

func getTxGraphAPI(ctx context.Context, txGraphAPI string) (*txgraphapi.TxGraphClient, error) {
	if txGraphAPI == "" {
		return nil, xerrors.Errorf("tx graph API must be specified with --tx-graph-api")
	}

	dialCtx, cancelFn := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFn()
	txGraphConn, err := grpc.DialContext(dialCtx, txGraphAPI, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, xerrors.Errorf("could not connect to tx graph API: %w", err)
	}
	txGraphCli := txgraphapi.NewTxGraphClient(ctx, prototxgraphapi.NewTxGraphClient(txGraphConn))

	return txGraphCli, nil
}
