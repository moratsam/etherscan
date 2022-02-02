package graphtest

import (

	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// SuiteBase defines a re-usable set of graph-relates tests that can be executed
// against any type that implements graph.Graph.
type SuiteBase struct {
	g graph.Graph
}

func (s *SuiteBase) SetGraph(g graph.Graph) {
	s.g = g
}

func (s *SuiteBase) TestUpsertWallet(c *gc.C) {
	test_addr := "0xabc"
	// Create a new wallet
	original := &graph.Wallet{
		Address: test_addr,
		Crawled: false,
	}
	err := s.g.UpsertWallet(original)
	c.Assert(err, gc.IsNil)
	
	// Update existing wallet, set Crawled to true
	updated := &graph.Wallet{
		Address: test_addr,
		Crawled: true,
	}
	err = s.g.UpsertWallet(updated)
	c.Assert(err, gc.IsNil)

	// Retrieve original wallet and verify Crawled field is true
	stored, err := s.g.FindWallet("0xabc")
	c.Assert(err, gc.IsNil)
	c.Assert(stored.Crawled, gc.Equals, true, gc.Commentf("update Crawled field to true"))

	// Update existing wallet, try to set Crawled to false
	updated = &graph.Wallet{
		Address: test_addr,
		Crawled: false,
	}
	err = s.g.UpsertWallet(updated)
	c.Assert(err, gc.IsNil)

	// Retrieve original wallet and verify Crawled field is still true
	stored, err = s.g.FindWallet("0xabc")
	c.Assert(err, gc.IsNil)
	c.Assert(stored.Crawled, gc.Equals, true, gc.Commentf("Crawled field not updated back to false"))
}
/*
func (s *SuiteBase) (c *gc.C) {
	
}
func (s *SuiteBase) (c *gc.C) {
	
}
func (s *SuiteBase) (c *gc.C) {
	
}
func (s *SuiteBase) (c *gc.C) {
	
}
func (s *SuiteBase) (c *gc.C) {
	
}
*/
