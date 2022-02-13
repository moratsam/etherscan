package scanner

import (
	"context"

	"github.com/moratsam/etherscan/txgraph/graph"
	"github.com/moratsam/etherscan/pipeline"
)

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
