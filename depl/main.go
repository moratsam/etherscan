package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/depl/service"
	"github.com/moratsam/etherscan/depl/service/blockinserter"
	"github.com/moratsam/etherscan/depl/service/scanner"
	"github.com/moratsam/etherscan/ethclient"
	"github.com/moratsam/etherscan/txgraph/graph"
	cdbgraph "github.com/moratsam/etherscan/txgraph/store/cdb"
	memgraph "github.com/moratsam/etherscan/txgraph/store/memory"
)

var (
	appName 	= "etherscan"
	appSha	= "populated-later"
)

func main() {
	// Expose pprof at localhost:6060/debug/pprof
	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	host, _ := os.Hostname()
	rootLogger := logrus.New()
	logger := rootLogger.WithFields(logrus.Fields{
		"app":	appName,
		"sha":	appSha,
		"host":	host,
	})

	if err := runMain(logger); err != nil {
		logrus.WithField("err", err).Error("shutting down due to error")
		return
	}
	logger.Info("shutdown complete")
}

func runMain(logger *logrus.Entry) error {
	svcGroup, err := setupServices(logger)
	if err != nil {
		return err
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGHUP)
		select {
		case s := <-sigCh:
			logger.WithField("signal", s.String()).Infof("shutting down due to signal")
			cancelFn()
		case <-ctx.Done():
		}
	}()

	return svcGroup.Run(ctx)
}

func setupServices(logger *logrus.Entry) (service.Group, error) {
	var (
		blockInserterCfg	blockinserter.Config
		scannerCfg			scanner.Config
	)

	flag.IntVar(&scannerCfg.FetchWorkers, "scanner-num-workers", runtime.NumCPU(), "The maximum number of workers to use for scanning eth blocks (defaults to number of CPUs)")
	txGraphURI := flag.String("tx-graph-uri", "in-memory://", "The URI for connecting to the txgraph (supported URIs: in-memory://, postgresql://user@host:26257/etherscan?sslmode=disable) Defaults to in-memory")
	flag.Parse()

	// Retrieve a suitable txgraph implementation and plug it into the service configurations.
	txGraph, err := getTxGraph(*txGraphURI, logger)
	if err != nil {
		return nil, err
	}

	// Retrieve an ethclient
	ethClient, err := ethclient.NewETHClient()
	if err != nil {
		logger.WithField("err", err).Error("new eth client")
		return nil, err
	}

	var svc service.Service
	var svcGroup service.Group

	blockInserterCfg.ETHClient = ethClient
	blockInserterCfg.GraphAPI	= txGraph
	blockInserterCfg.Logger		= logger.WithField("service", "block-inserter")
	if svc, err = blockinserter.NewService(blockInserterCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	scannerCfg.ETHClient = ethClient
	scannerCfg.GraphAPI	= txGraph
	scannerCfg.Logger		= logger.WithField("service", "scanner")
	if svc, err = scanner.NewService(scannerCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	return svcGroup, nil
}

type txGraph interface {
	Blocks() (graph.BlockIterator, error)
	InsertTxs(txs []*graph.Tx) error
	UpsertBlock(block *graph.Block) error
	UpsertWallets(wallets []*graph.Wallet) error
	
}

func getTxGraph(txGraphURI string, logger *logrus.Entry) (txGraph, error) {
	if txGraphURI == "" {
		return nil, xerrors.Errorf("tx graph URI must be specified with --tx-graph-uri")
	}

	uri, err := url.Parse(txGraphURI)
	if err != nil {
		return nil, xerrors.Errorf("could not parse tx graph URI: %w", err)
	}

	switch uri.Scheme{
	case "in-memory":
		logger.Info("using in-memory grraph")
		return memgraph.NewInMemoryGraph(), nil
	case "postgresql":
		logger.Info("using CDB grraph")
		return cdbgraph.NewCockroachDBGraph(txGraphURI)
	default:
		return nil, xerrors.Errorf("unsupported tx graph URI scheme: %q", uri.Scheme)
	}
}
