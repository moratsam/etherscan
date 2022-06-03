package blockinserter

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// Graph is implemented by objects that can insert transactions into a tx graph instance.
type Graph interface {
	UpsertBlocks(blocks []*graph.Block) error
}

// ETHClient is implemented by objects that can fetch an eth block by its number.
type ETHClient interface {
	SubscribeNewHead(ctx context.Context) (<-chan *types.Header, ethereum.Subscription, error)
}

// Config encapsulates the configuration options for creating a new BlockInserter.
type Config struct {
	// An ETHClient instance for subscribing to new head.
	ETHClient ETHClient

	// A Graph instance for inserting new blocks.
	Graph Graph
}

type BlockInserter struct {
	client ETHClient
	graph Graph
}

func NewBlockInserter(cfg Config) *BlockInserter {
	return &BlockInserter{
		client: cfg.ETHClient,
		graph: cfg.Graph,
	}
}

func (i *BlockInserter) Start(ctx context.Context) error {
	headerCh, sub, err := i.client.SubscribeNewHead(ctx)
	if err != nil {
		return xerrors.Errorf("subscribing new head: %w", err)
	}

	var header *types.Header
	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return nil
		case err := <-sub.Err():
			return xerrors.Errorf("receiving header: %w", err)
		case header = <-headerCh:
			err := i.graph.UpsertBlocks([]*graph.Block{&graph.Block{Number: int(header.Number.Int64())}})
			if err != nil {
				sub.Unsubscribe()
				return err
			}
		}
	}

	return nil
}
