package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	"github.com/moratsam/etherscan/depl/monolith/service/frontend"
	ssapi "github.com/moratsam/etherscan/scorestoreapi"
	protossapi "github.com/moratsam/etherscan/scorestoreapi/proto"
)

var (
	appName = "etherscan-frontend"
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
			Name:   "score-store-api",
			EnvVar: "SCORE_STORE_API",
			Usage:  "The gRPC endpoint for connecting to the score store",
		},
		cli.IntFlag{
			Name:   "results-per-page",
			Value:  30,
			EnvVar: "RESULTS_PER_PAGE",
			Usage:  "The number of entries for each result page",
		},
		cli.IntFlag{
			Name:   "fe-port",
			Value:  8080,
			EnvVar: "FE_PORT",
			Usage:  "The port for exposing the front-end",
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

	scoreStoreAPI, err := getScoreStoreAPI(ctx, appCtx.String("score-store-api"))
	if err != nil {
		return err
	}

	var frontendCfg frontend.Config
	frontendCfg.ListenAddr = fmt.Sprintf(":%d", appCtx.Int("fe-port"))
	frontendCfg.ResultsPerPage = appCtx.Int("results-per-page")
	frontendCfg.ScoreStoreAPI = scoreStoreAPI
	frontendCfg.Logger = logger
	feSvc, err := frontend.NewService(frontendCfg)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := feSvc.Run(ctx); err != nil {
			logger.WithField("err", err).Error("front-end service exited with error")
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
			_ = pprofListener.Close()
		}
	}()

	// Keep running until we receive a signal
	wg.Wait()
	return nil
}

func getScoreStoreAPI(ctx context.Context, scoreStoreAPI string) (*ssapi.ScoreStoreClient, error) {
	if scoreStoreAPI == "" {
		return nil, xerrors.Errorf("score store API must be specified with --score-store-api")
	}

	dialCtx, cancelFn := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFn()
	scoreStoreConn, err := grpc.DialContext(dialCtx, scoreStoreAPI, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, xerrors.Errorf("could not connect to score store API: %w", err)
	}
	scoreStoreCli := ssapi.NewScoreStoreClient(ctx, protossapi.NewScoreStoreClient(scoreStoreConn))

	return scoreStoreCli, nil
}
