package scanner

import (
	"sync"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/moratsam/etherscan/pipeline"
)

var (
	// Compile-time check that scannerPayload implements pipeline.Payload.
	_ pipeline.Payload = (*scannerPayload)(nil)

	// Use a pool to ease the strain on the garbage collector.		
	payloadPool = sync.Pool{
		New: func() interface{} { return new(scannerPayload) },
	}
)

// Implements the pipeline.Payload.
// TODO add timestamp, receives
type scannerPayload struct {
	BlockNumber	int
	Txs			[]*types.Transaction
}

func (p *scannerPayload) Clone() pipeline.Payload {
	newP := payloadPool.Get().(*scannerPayload) // Get a new Payload from the pool.
	newP.BlockNumber = p.BlockNumber
	newP.Txs = append([]*types.Transaction(nil), p.Txs...)

	return newP
}

func (p *scannerPayload) MarkAsProcessed() {
	p.Txs = p.Txs[:0] // Optimisation: set length to zero without modifying the capacity.
	payloadPool.Put(p) // Return the Payload to the pool.
}
