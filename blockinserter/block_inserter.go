package blockinserter

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// Graph is implemented by objects that can insert transactions into a tx graph instance.
type Graph interface {
	UpsertBlock(block *graph.Block) error
}

//ETHClient is implemented by objects that can fetch an eth block by its number.
type ETHClient interface {
	SubscribeNewHead(ctx context.Context) (<-chan *types.Header, ethereum.Subscription, error)
}

type BlockInserter interface {
	// Start inserting blocks into the tx graph.
	Start(ctx context.Context) error	
}

type blockInserter struct {
	client ETHClient
	graph Graph
}

func NewBlockInserter(client ETHClient, graph Graph) BlockInserter {
	return &blockInserter{
		client: client,
		graph: graph,
	}
}

func (i *blockInserter) Start(ctx context.Context) error {
	headerCh, sub, err := i.client.SubscribeNewHead(ctx)
	if err != nil {
		return err
	}

	// Insert every new block number into the graph.
	go func() {
		for {
			select {
			case <-sub.Err():
				panic("receiving header")
			case header := <-headerCh:
				i.graph.UpsertBlock(&graph.Block{Number: int(header.Number.Int64())})
			}
		}
	}()

	return nil
}
