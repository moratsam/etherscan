package gravitas

import (
	"math/big"


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

// A TxMessage is sent from the sender vertex of a transaction to it's recipient vertex.
// With them, the value of transactions is propagated through the graph.
type TxMessage struct {
	Value *big.Float
}

// Type returns the type of this message.
func (m TxMessage) Type() string { return "eth_value" }

// makeComputeFunc returns a ComputeFunc that executes the Gravitas calculation algorhitm.
func makeComputeFunc() bspgraph.ComputeFunc {
	return func(g *bspgraph.Graph, v *bspgraph.Vertex, msgIt message.Iterator) error {
		superstep := g.Superstep()
		vData := v.Value().(VertexData)

		switch superstep {
		case 0:
			// Use an aggregator to count the number of vertices and txs in the graph.
			walletCountAgg := g.Aggregator("wallet_count")
			walletCountAgg.Aggregate(1)
			txCountAgg := g.Aggregator("tx_count")
			txCountAgg.Aggregate(len(vData.Txs))
			return nil
		case 1:
			// Subtract from the vertex value the sum cost of all outgoing txs (value + fees).
			// For every tx, send its value to the recipient.
			sum := big.NewFloat(0)
			for _, tx := range vData.Txs {
				sum.Add(sum, new(big.Float).SetInt(tx.Value))
				sum.Add(sum, new(big.Float).SetInt(tx.TransactionFee))

				msg := TxMessage{Value: new(big.Float).SetInt(tx.Value)}
				if err := g.SendMessage(tx.To, msg); err != nil {
					return err
				}
			}
			vData.Value.Sub(vData.Value, sum)
			v.SetValue(vData)
		case 2:
			// Add to the vertex value the sum of all incoming tx values (sent in superstep 1).
			sum := big.NewFloat(0).Set(vData.Value)
			for msgIt.Next() {
				msgValue := msgIt.Message().(TxMessage).Value
				sum.Add(sum, msgValue)
			}
			vData.Value = sum
			v.SetValue(vData)
		}

		// Calculator will stop when all vertices are frozen (see PostStepKeepRunning).
		// A frozen vertex gets unfrozen upon receival of a message.
		v.Freeze()
		return nil
	}
}
