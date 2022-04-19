package gravitas

import (
	"math/big"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/bspgraph"
	"github.com/moratsam/etherscan/bspgraph/message"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
)

// VertexData is the data held by each bspgraph vertex in its gravitas calculations.
type VertexData struct {
	// The placeholder for the scorer value calculated on this vertex.
	Value	*big.Float

	// A list of all the wallet's incoming and outgoing transactions.
	Txs	[]*txgraph.Tx
}

// makeComputeFunc returns a ComputeFunc that executes the Gravitas calculation algorhitm.
func makeComputeFunc() bspgraph.ComputeFunc {
	return func(g *bspgraph.Graph, v *bspgraph.Vertex, _ message.Iterator) error {
		superstep := g.Superstep()

		// Use an aggregator to count the number of vertices in the graph.
		if superstep == 0 {
			walletCountAgg := g.Aggregator("wallet_count")
			walletCountAgg.Aggregate(1)
			return nil
		} else if  superstep > 1 {
			return xerrors.New("everything should be basta in first step")
		}

		vData := v.Value().(VertexData)

		// Calculate value of all incoming transactions,
		// decreased by value of all outgoing transactions and their fees.
		sum := big.NewFloat(0)
		for _, tx := range vData.Txs {
			if tx.From == v.ID() && tx.To != v.ID() {
				sum = sum.Sub(sum, new(big.Float).SetInt(tx.Value))
				sum = sum.Sub(sum, new(big.Float).SetInt(tx.TransactionFee))
			} else if tx.From != v.ID() && tx.To == v.ID() {
				sum = sum.Add(sum, new(big.Float).SetInt(tx.Value))
			} else {
				sum = sum.Sub(sum, new(big.Float).SetInt(tx.TransactionFee))
			}
		}

		txCountAgg := g.Aggregator("tx_count")
		txCountAgg.Aggregate(len(vData.Txs))

		vData.Value = sum
		v.SetValue(vData)

		return nil
	}
}
