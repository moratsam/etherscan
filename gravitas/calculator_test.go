package gravitas_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	gc "gopkg.in/check.v1"
	
	"github.com/moratsam/etherscan/gravitas"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
)

var _ = gc.Suite(new(CalculatorTestSuite))

func Test(t *testing.T) {
	// Run all gocheck test-suites
	gc.TestingT(t)
}

type spec struct {
	descr     string
	vertices_ids  []string
	vertices_data [][]*txgraph.Tx
	expScores map[string]*big.Int
}

type CalculatorTestSuite struct {}

func (s *CalculatorTestSuite) TestSimple1(c *gc.C) {
	address1 := createAddressFromInt(1)
	address2 := createAddressFromInt(2)
	tx := createTx(address1, address2, 1, 0)

	spec := spec{
		descr: `1 -> 2 sends 1 eth, transaction fee is 0`,
		vertices_ids: []string{ address1, address2 },
		vertices_data: [][]*txgraph.Tx{ []*txgraph.Tx{tx}, []*txgraph.Tx{tx} },
		expScores: map[string]*big.Int{
			address1: big.NewInt(-1),
			address2: big.NewInt(1),
		},
	}

	s.assertGravitasScores(c, spec)
}


func (s *CalculatorTestSuite) TestSimple2(c *gc.C) {
	address1 := createAddressFromInt(1)
	address2 := createAddressFromInt(2)
	address3 := createAddressFromInt(3)
	txs := []*txgraph.Tx{
		createTx(address1, address2, 1, 0),	
		createTx(address1, address2, 100, 12),	
		createTx(address1, address3, 2, 2),	
		createTx(address2, address2, 4, 11),	
		createTx(address3, address1, 5, 13),	
		createTx(address3, address2, 6, 17),	
	}

	spec := spec{
		descr: `1 -> 2 sends 1 eth, transaction fee is 0
1 -> 2 sends 100 eth, transaction fee is 12
1 -> 3 sends 2 eth, transaction fee is 2
2 -> 2 sends 4 eth, transaction fee is 11
3 -> 1 sends 5 eth, transaction fee is 13
3 -> 2 sends 6 eth, transaction fee is 17`,

		vertices_ids: []string{ address1, address2, address3 },
		vertices_data: [][]*txgraph.Tx{
			[]*txgraph.Tx{txs[0], txs[1], txs[2], txs[4]},
			[]*txgraph.Tx{txs[0], txs[1], txs[3], txs[5]},
			[]*txgraph.Tx{txs[2], txs[4], txs[5]},
		},
		expScores: map[string]*big.Int{
			address1: big.NewInt(-112),
			address2: big.NewInt(96),
			address3: big.NewInt(-39),
		},
	}

	s.assertGravitasScores(c, spec)
}

func (s *CalculatorTestSuite) assertGravitasScores(c *gc.C, spec spec) {
	c.Log(spec.descr)

	calc, err := gravitas.NewCalculator(gravitas.Config{
		ComputeWorkers: 1,
	})
	c.Assert(err, gc.IsNil)
	defer func() { _ = calc.Close() }()

	for i, id := range spec.vertices_ids {
		calc.AddVertex(id, spec.vertices_data[i])
	}

	ex := calc.Executor()
	err = ex.RunToCompletion(context.TODO())
	c.Assert(err, gc.IsNil)
	c.Logf("converged after %d steps", ex.Superstep())

	err = calc.Scores(func(id string, score *big.Int) error {
		c.Assert(score.String(), gc.Equals, spec.expScores[id].String(), gc.Commentf("expected score for %v to be %f;", id, spec.expScores[id].String(), score.String()))
		return nil
	})
	c.Assert(err, gc.IsNil)
}

func createTx(from, to string, value, transactionFee int64) *txgraph.Tx {
	return &txgraph.Tx{
		From: 				from,
		To: 					to,
		Value: 				big.NewInt(value),
		TransactionFee:	big.NewInt(transactionFee),
	}
}

// If address is not 40 chars long, string comparisons will not work as expected.
// The following is loop far from efficient, but it's only for tests so who cares.
func createAddressFromInt(addressInt int) string {
	x := fmt.Sprintf("%x", addressInt) // convert to hex string
	padding := 40-len(x)
	for i:=0; i<padding; i++ {
		x = "0" + x
	}
	return x
}
