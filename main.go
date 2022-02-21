package main

import (
	"context"
	"fmt"
	"os"

	"github.com/moratsam/etherscan/blockinserter"
	"github.com/moratsam/etherscan/ethclient"
	"github.com/moratsam/etherscan/scanner"
	cdbgraph "github.com/moratsam/etherscan/txgraph/store/cdb"
	//memgraph "github.com/moratsam/etherscan/txgraph/store/memory"
)

func main() {
	//txGraph := memgraph.NewInMemoryGraph()
	txGraph, err := cdbgraph.NewCockroachDBGraph(os.Getenv("CDB_DSN"))
	if err != nil {
		fmt.Println("new cdb graph", err)
		return
	}
	ethClient, err := ethclient.NewETHClient()
	if err != nil {
		fmt.Println("new eth client", err)
		return
	}
	blockInserter := blockinserter.NewBlockInserter(ethClient, txGraph)
	if err := blockInserter.Start(context.Background()); err != nil {
		fmt.Println("block inserter start", err)
	}

	cfg := scanner.Config{
		ETHClient: ethClient,
		Graph: txGraph,
		FetchWorkers: 100,
	}

	blockIterator, err := txGraph.Blocks()
	if err != nil {
		fmt.Println("block iterator", err)
	}
	_, err = scanner.NewScanner(cfg).Scan(context.Background(), blockIterator, txGraph)
	if err != nil {
		fmt.Println("scan", err)
	}
}
