package scanner

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/moratsam/etherscan/pipeline"
	"github.com/moratsam/etherscan/txgraph/graph"
)

const zeroAddress string = "0000000000000000000000000000000000000000"

var _ pipeline.Processor = (*txParser)(nil)

type txParser struct {
	txGraph Graph
}

func newTxParser(txGraph Graph) *txParser {
	return &txParser{txGraph: txGraph}
}

func (tp *txParser) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*scannerPayload)
	
	var txs []*graph.Tx
	walletsMap := make(map[string]bool)
	for _, tx := range payload.Txs {
		// First insert From and To wallet addresses to the batch of wallet addresses.
		from := tp.parseFrom(tx)
		to := tp.parseTo(tx)
		walletsMap[from] = true
		walletsMap[to] = true

		//Create transaction and append it.
		graphTx := &graph.Tx{
			Hash: 				tx.Hash().String()[2:],
			Status: 				tp.parseStatus(tx),
			Block: 				big.NewInt(int64(payload.BlockNumber)),
			Timestamp:			time.Now(), //TODO
			From: 				from,
			To:					to, 
			Value: 				tx.Value(),
			TransactionFee:	tp.parseCost(tx),
			Data: 				tx.Data(),
		}
		txs = append(txs, graphTx)
	}
	if len(txs) > 0 {
		// First upsert the From/To wallets.
		var wallets []*graph.Wallet
		for addr := range walletsMap {
			wallet := &graph.Wallet{Address: addr}
			wallets = append(wallets, wallet)
		}
		if err := tp.txGraph.UpsertWallets(wallets); err != nil {
			return nil, err
		}

		// Then insert the transactions.
		if err := tp.txGraph.InsertTxs(txs); err != nil {
			return nil, err
		}
	}
	
	return p, nil
}

// TODO parse from signature?
func (tp *txParser) parseFrom(tx *types.Transaction) string {
	return zeroAddress
}

// From docs:
//		To returns the recipient address of the transaction.
//		For contract-creation transactions, To returns nil. 
func (tp *txParser) parseTo(tx *types.Transaction) string {
	addr := tx.To()
	if addr == nil {
		return zeroAddress
	} else {
		return addr.String()[2:]
	}
}

// TODO parse from receipt?
func (tp *txParser) parseStatus(tx *types.Transaction) graph.TxStatus {
	return graph.Success
}

// From docs: Cost returns gas * gasPrice + value. 
func (tp *txParser) parseCost(tx *types.Transaction) *big.Int {
	return big.NewInt(0).Sub(tx.Cost(), tx.Value())
}
