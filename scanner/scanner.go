package scanner

import (
	"context"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/moratsam/etherscan/txgraph/graph"
	"github.com/moratsam/etherscan/pipeline"
)

//ETHClient is implemented by objects that can fetch an eth block by its number.
type ETHClient interface {
	BlockByNumber(number int) (*types.Block, error)
}

// Graph is implemented by objects that can insert transactions and upsert wallets
// into a tx graph instance.
type Graph interface {
	// Creates a new tx.
	InsertTx(tx *graph.Tx) error

	// Creates a new wallet or updates an existing one.
	UpsertWallet(wallet *graph.Wallet) error
}

// Config encapsulates the configuration options for creating a new Scanner.
type Config struct {
	// An ETHClient instance for fetching blocks.
	ETHClient ETHClient

	// A Graph instance for adding transactions and wallets to the transaction graph.
	Graph Graph

	// The maximum number of concurrent workers used for fetching blocks.
	FetchWorkers int
}

// Scanner implements a eth blockchain scanning pipeline consisting of the following stages:
// - Given a block number, retrieve the block and some additional block-related data
// - Parse all transactions in a block and insert the data into transaction graph.
type Scanner struct {
	p *pipeline.Pipeline
}

// Returns a new Scanner instance.
func NewScanner(cfg Config) *Scanner {
	return &Scanner{
		p: assembleScannerPipeline(cfg),
	}
}

// Creates the stages of a scanner pipeline using the options in cfg
// and assembles them into a pipeline instance.
func assembleScannerPipeline(cfg Config) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.DynamicWorkerPool(
			newBlockFetcher(cfg.ETHClient),
			cfg.FetchWorkers,
		),
		pipeline.FIFO(
			newTxParser(cfg.Graph),
		),
	)
}

// Scan iterates blockIt and sends each block through the scanner pipeline
// returning the total count of blocks that went through the pipeline.
// Calls to Scan block until the block iterator is exhausted (which never happens),
// or an error occurs or the context is cancelled.
func (s *Scanner) Scan(ctx context.Context, blockIt graph.BlockIterator) (int, error) {
	sink := new(countingSink)
	err := s.p.Process(ctx, &blockSource{blockIt: blockIt}, sink)
	return sink.getCount(), err
}

// Implements the pipeline.Source.
// The source for the scanner pipeline is large part just a wrapper for the
// graph.BlockIterator
type blockSource struct {
	blockIt graph.BlockIterator
}

func (so *blockSource) Error() error 					{ return so.blockIt.Error() }
func (so *blockSource) Next(context.Context) bool	{ return so.blockIt.Next() }

// Fetch a Payload instance from the pool, populate it with a Block fetched from the blockIt.
func (so *blockSource) Payload() pipeline.Payload {
	block := so.blockIt.Block()
	payload := payloadPool.Get().(*scannerPayload)
	payload.BlockNumber = block.Number
	return payload
}

// Implements the pipeline.Sink.
type countingSink struct {
	count int	
}

// Consume can be empty because the scanner pipeline inserts to the graph in a previous stage.
// Once the Consume returns, the pipeline worker automatically invokes MarkAsProcessed
// on the payload, which ensures the payload is returned to the payloadPool.
func (si *countingSink) Consume(_ context.Context, p pipeline.Payload) error {
	si.count++
	return nil
}

func (si *countingSink) getCount() int {
	return si.count
}
