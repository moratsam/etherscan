package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	txgraph "github.com/moratsam/etherscan/txgraph/graph"
	cdbgraph "github.com/moratsam/etherscan/txgraph/store/cdb"
	memgraph "github.com/moratsam/etherscan/txgraph/store/memory"
	"github.com/moratsam/etherscan/txgraphapi"
	prototxgraphapi "github.com/moratsam/etherscan/txgraphapi/proto"
)

var (
	appName = "etherscan-txgraph"
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
		cli.BoolTFlag{
			Name:		"tx-graph-cache",
			EnvVar:	"TX_GRAPH_CACHE",
			Usage: 	"true - cache inserts in-memory to reduce load on DB",
		},
		cli.StringFlag{
			Name:   "tx-graph-uri",
			Value:  "in-memory://",
			EnvVar: "TX_GRAPH_URI",
			Usage:  "The URI for connecting to the txgraph (supported URIs: in-memory://, postgresql://user@host:26257/etherscan?sslmode=disable) Defaults to in-memory",
		},
		cli.IntFlag{
			Name:   "grpc-port",
			Value:  8080,
			EnvVar: "GRPC_PORT",
			Usage:  "The port for exposing the gRPC endpoints for accessing the tx graph",
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

	graph, err := getTxGraph(appCtx.String("tx-graph-uri"), appCtx.Bool("tx-graph-cache"))
	if err != nil {
		return err
	}

	// Start gRPC server
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", appCtx.Int("grpc-port")))
	if err != nil {
		return err
	}
	defer func() { _ = grpcListener.Close() }()

	wg.Add(1)
	go func() {
		defer wg.Done()
		srv := grpc.NewServer()
		prototxgraphapi.RegisterTxGraphServer(srv, txgraphapi.NewTxGraphServer(graph))
		logger.WithField("port", appCtx.Int("grpc-port")).Info("listening for gRPC connections")
		_ = srv.Serve(grpcListener)
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
			_ = grpcListener.Close()
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

func getTxGraph(txGraphURI string, cache bool) (txgraph.Graph, error) {
	if txGraphURI == "" {
		return nil, xerrors.Errorf("tx graph URI must be specified with --tx-graph-uri")
	}

	uri, err := url.Parse(txGraphURI)
	if err != nil {
		return nil, xerrors.Errorf("could not parse tx graph URI: %w", err)
	}

	switch uri.Scheme {
	case "in-memory":
		logger.Info("using in-memory graph")
		return memgraph.NewInMemoryGraph(), nil
	case "postgresql":
		logger.Info("using CDB graph")
		return cdbgraph.NewCDBGraph(txGraphURI, cache)
	default:
		return nil, xerrors.Errorf("unsupported tx graph URI scheme: %q", uri.Scheme)
	}
}
