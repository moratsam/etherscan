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
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/depl/partition"
	"github.com/moratsam/etherscan/depl/service"
	"github.com/moratsam/etherscan/depl/service/blockinserter"
	"github.com/moratsam/etherscan/depl/service/gravitas"
	"github.com/moratsam/etherscan/depl/service/scanner"
	"github.com/moratsam/etherscan/ethclient"
	ss "github.com/moratsam/etherscan/scorestore"
	cdbss "github.com/moratsam/etherscan/scorestore/cdb"
	memss "github.com/moratsam/etherscan/scorestore/memory"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
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
		gravitasCfg			gravitas.Config
		scannerCfg			scanner.Config
	)

	flag.IntVar(&gravitasCfg.ComputeWorkers, "gravitas-num-workers", runtime.NumCPU(), "The number of workers to use for calculating gravitas scores (defaults to number of CPUs")
	flag.DurationVar(&gravitasCfg.UpdateInterval, "gravitas-update-interval", 1*time.Minute, "The time between subsequent gravitas score updates (defaults to 3 hours)")

	partitionDetMode := flag.String("partition-detection-mode", "single", "The partition detection mode to use. Supported values are 'dns=HEADLESS_SERVICE_NAME' (k8s) and 'single' (local dev mode)")

	flag.IntVar(&scannerCfg.FetchWorkers, "scanner-num-workers", runtime.NumCPU(), "The maximum number of workers to use for scanning eth blocks (defaults to number of CPUs)")

	scoreStoreURI := flag.String("score-store-uri", "in-memory://", "The URI for connecting to the scorestore (supported URIs: in-memory://, postgresql://user@host:26257/etherscan?sslmode=disable) (defaults to in-memory)")

	txGraphURI := flag.String("tx-graph-uri", "in-memory://", "The URI for connecting to the txgraph (supported URIs: in-memory://, postgresql://user@host:26257/etherscan?sslmode=disable) Defaults to in-memory")

	flag.Parse()

	// Retrieve an ethclient.
	ethClient, err := ethclient.NewETHClient()
	if err != nil {
		logger.WithField("err", err).Error("new eth client")
		return nil, err
	}

	// Create a helper for detecting the partition assigned to this instance.
	partDet, err := getPartitionDetector(*partitionDetMode)
	if err != nil {
		logger.WithField("err", err).Error("get partition detector")
		return nil, err
	}

	// Retrieve a suitable scorestore implementation and plug it to the service configurations.
	scoreStore, err := getScoreStore(*scoreStoreURI, logger)
	if err != nil {
		logger.WithField("err", err).Error("get score store")
		return nil, err
	}

	// Retrieve a suitable txgraph implementation and plug it into the service configurations.
	txGraph, err := getTxGraph(*txGraphURI, logger)
	if err != nil {
		logger.WithField("err", err).Error("get tx graph")
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

	gravitasCfg.GraphAPI 			= txGraph
	gravitasCfg.ScoreScoreAPI		= scoreStore
	gravitasCfg.PartitionDetector	= partDet
	gravitasCfg.Logger				= logger.WithField("service", "gravitas-calculator")
	if svc, err = gravitas.NewService(gravitasCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	scannerCfg.ETHClient = ethClient
	scannerCfg.GraphAPI	= txGraph
	scannerCfg.Logger		= logger.WithField("service", "scanner")
	/*
	logger.Warn("SKIPPING scanner service")
	_ = scannerCfg
	*/
	if svc, err = scanner.NewService(scannerCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	return svcGroup, nil
}

type scoreStore interface {
	UpsertScore(score *ss.Score) error
	UpsertScorer(scorer *ss.Scorer) error
}

func getScoreStore(scoreStoreURI string, logger *logrus.Entry) (scoreStore, error) {
	if scoreStoreURI == "" {
		return nil, xerrors.Errorf("score store URI must be specified with --score-store-uri")
	}

	uri, err := url.Parse(scoreStoreURI)
	if err != nil {
		return nil, xerrors.Errorf("could not parse score store URI: %w", err)
	}

	switch uri.Scheme{
	case "in-memory":
		logger.Info("using in-memory score store")
		return memss.NewInMemoryScoreStore(), nil
	case "postgresql":
		logger.Info("using CDB score store")
		return cdbss.NewCDBScoreStore(scoreStoreURI)
	default:
		return nil, xerrors.Errorf("unsupported score store URI scheme: %q", uri.Scheme)
	}
}

type txGraph interface {
	Blocks() (txgraph.BlockIterator, error)
	InsertTxs(txs []*txgraph.Tx) error
	UpsertBlock(block *txgraph.Block) error
	UpsertWallets(wallets []*txgraph.Wallet) error
	Wallets(fromAddress, toAddress string) (txgraph.WalletIterator, error)
	WalletTxs(address string) (txgraph.TxIterator, error)
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
		logger.Info("using in-memory graph")
		return memgraph.NewInMemoryGraph(), nil
	case "postgresql":
		logger.Info("using CDB graph")
		return cdbgraph.NewCDBGraph(txGraphURI)
	default:
		return nil, xerrors.Errorf("unsupported tx graph URI scheme: %q", uri.Scheme)
	}
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
