package scanner

import (
	"context"

	"github.com/moratsam/etherscan/pipeline"
)

var _ pipeline.Processor = (*blockFetcher)(nil)

type blockFetcher struct {
	client ETHClient
}

func newBlockFetcher(client ETHClient) *blockFetcher {
	return &blockFetcher{client: client}
}

func (bf *blockFetcher) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*scannerPayload)

	// Fetch block.
	block, err := bf.client.BlockByNumber(payload.BlockNumber)
	if err != nil {
		return nil, err
	}

	// Extract transactions and save them to the payload.
	payload.Txs = block.Transactions()

	return payload, nil
}
