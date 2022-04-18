package gravitas

import (
	"context"
	"math/big"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/bspgraph"
	txgraph "github.com/moratsam/etherscan/txgraph/graph"
)

// Calculator executes the iterative version of the Gravitas algorithm on a graph.
type Calculator struct {
	g   *bspgraph.Graph
	cfg Config

	executorFactory bspgraph.ExecutorFactory
}

// NewCalculator returns a new Calculator instance using the provided config options.
func NewCalculator(cfg Config) (*Calculator, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("Gravitas calculator config validation failed: %w", err)
	}

	g, err := bspgraph.NewGraph(bspgraph.GraphConfig{
		ComputeWorkers: cfg.ComputeWorkers,
		ComputeFn:      makeComputeFunc(),
	})
	if err != nil {
		return nil, err
	}

	return &Calculator{
		cfg:             cfg,
		g:               g,
		executorFactory: bspgraph.NewExecutor,
	}, nil
}

// Close releases any resources allocated by this Gravitas calculator instance.
func (c *Calculator) Close() error {
	return c.g.Close()
}

// SetExecutorFactory configures the calculator to use the a custom executor
// factory when the Executor method is invoked.
func (c *Calculator) SetExecutorFactory(factory bspgraph.ExecutorFactory) {
	c.executorFactory = factory
}

// AddVertex inserts a new vertex to the graph with the given id.
func (c *Calculator) AddVertex(id string, txs []*txgraph.Tx) {
	vData := VertexData{
		Value:	big.NewFloat(0),
		Txs:		txs,
	}
	c.g.AddVertex(id, vData)
}

// AddEdge inserts a directed edge from src to dst. If both src and dst refer
// to the same vertex then this is a no-op.
func (c *Calculator) AddEdge(src, dst string) error {
	// Don't allow self-links
	if src == dst {
		return nil
	}
	return c.g.AddEdge(src, dst, nil)
}

// Graph returns the underlying bspgraph.Graph instance.
func (c *Calculator) Graph() *bspgraph.Graph {
	return c.g
}

// Executor creates and return a bspgraph.Executor for running the Gravitas
// algorithm once the graph layout has been properly set up.
func (c *Calculator) Executor() *bspgraph.Executor {
	c.registerAggregators()
	cb := bspgraph.ExecutorCallbacks{
		PreStep: func(_ context.Context, g *bspgraph.Graph) error {
			return nil
		},
		PostStepKeepRunning: func(_ context.Context, g *bspgraph.Graph, _ int) (bool, error) {
			// Everything should be done in superstep 0.
			return g.Superstep() < 1, nil
		},
	}

	return c.executorFactory(c.g, cb)
}

// registerAggregators creates and registers the aggregator instances that we
// need to run the Gravitas calculation algorithm.
func (c *Calculator) registerAggregators() {}

// Scores invokes the provided visitor function for each vertex in the graph.
func (c *Calculator) Scores(visitFn func(id string, score *big.Float) error) error {
	for id, v := range c.g.Vertices() {
		if err := visitFn(id, v.Value().(VertexData).Value); err != nil {
			return err
		}
	}

	return nil
}

// ScoresAll invokes the provided visitor function with all the vertices in the graph.
func (c *Calculator) ScoresAll(visitFn func([]*bspgraph.Vertex) error) error {
	vertices := make([]*bspgraph.Vertex, len(c.g.Vertices()))
	i := 0
	for _, vertex := range c.g.Vertices() {
		vertices[i] = vertex
		i++
	}
	return visitFn(vertices)
}
