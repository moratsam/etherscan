package graphtest

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	gc "gopkg.in/check.v1"
	"golang.org/x/xerrors"

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
	test_addr := "abc"
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
	stored, err := s.g.FindWallet(test_addr)
	c.Assert(err, gc.IsNil)
	c.Assert(stored.Crawled, gc.Equals, true, gc.Commentf("Crawled field not updated to true"))

	// Update existing wallet, try to set Crawled to false
	updated = &graph.Wallet{
		Address: test_addr,
		Crawled: false,
	}
	err = s.g.UpsertWallet(updated)
	c.Assert(err, gc.IsNil)

	// Retrieve original wallet and verify Crawled field is still true
	stored, err = s.g.FindWallet(test_addr)
	c.Assert(err, gc.IsNil)
	c.Assert(stored.Crawled, gc.Equals, true, gc.Commentf("Crawled field updated back to false"))
}

func (s *SuiteBase) TestFindWallet(c *gc.C) {
	test_addr := "abc"
	// Create a new wallet
	original := &graph.Wallet{
		Address: test_addr,
		Crawled: false,
	}
	err := s.g.UpsertWallet(original)
	c.Assert(err, gc.IsNil)

	// Retrieve wallet
	retrieved, err := s.g.FindWallet(original.Address)
	c.Assert(err, gc.IsNil)
	c.Assert(retrieved, gc.DeepEquals, original, gc.Commentf("lookup by Address returned wrong wallet"))

	// Lookup unknown wallet
	_, err = s.g.FindWallet("inexistent")
	c.Assert(xerrors.Is(err, graph.ErrNotFound), gc.Equals, true)
}

// Verifies that multiple clients can concurrently access the store.
func (s *SuiteBase) TestConcurrentWalletIterators(c *gc.C) {
	var (
		wg				 sync.WaitGroup
		numIterators = 10
		numWallets	 = 100
	)

	for i:=0; i<numWallets; i++ {
		wallet := &graph.Wallet{Address: s.createAddressFromInt(c, i)}
		c.Assert(s.g.UpsertWallet(wallet), gc.IsNil)
	}

	wg.Add(numIterators)
	for i:=0; i<numIterators; i++ {
		go func(id int) {
			defer wg.Done()

			itTagComment := gc.Commentf("iterator %d", id)
			seen := make(map[string]bool)
			it, err := s.partitionedWalletIterator(c, 0, 1)
			c.Assert(err, gc.IsNil, itTagComment)
			defer func() {
				c.Assert(it.Close(), gc.IsNil, itTagComment)
			}()

			for i:=0; it.Next(); i++ {
				wallet := it.Wallet()
				walletAddr := wallet.Address
				c.Assert(seen[walletAddr], gc.Equals, false, gc.Commentf("iterator %d saw same wallet twice", id))
				seen[walletAddr] = true
			}

			c.Assert(seen, gc.HasLen, numWallets, itTagComment)
			c.Assert(it.Error(), gc.IsNil, itTagComment)
			c.Assert(it.Close(), gc.IsNil, itTagComment)
		}(i)
	}

	// Wait for the routines to finish
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
		case <-doneCh:
		// test completed successfully
		case <- time.After(10 * time.Second):
			c.Fatal("timed out waiting for test to complete")
	}
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

// If address is not 40 chars long, string comparisons will not work as expected.
// The following loop far from efficient, but it's only for tests so it should be fine.
func (s *SuiteBase) createAddressFromInt(c *gc.C, addressInt int) string {
	x := fmt.Sprintf("%x", addressInt) // convert to hex string
	padding := 40-len(x)
	for i:=0; i<padding; i++ {
		x = "0" + x
	}
	return x

}

func (s *SuiteBase) partitionedWalletIterator(c *gc.C, partition, numPartitions int) (graph.WalletIterator, error) {
	from, to := s.partitionRange(c, partition, numPartitions)
	return s.g.Wallets(from, to)
}

//make a partition of the space of all wallet addresses
func (s *SuiteBase) partitionRange(c *gc.C, partition, numPartitions int) (from, to string) {
	if partition < 0 || partition >= numPartitions {
		c.Fatal("invalid partition")
	}

	var minAddr = "0000000000000000000000000000000000000000"
	var maxAddr = "ffffffffffffffffffffffffffffffffffffffff"

	// Calculate the size of each partition as 2^(4*40) / numPartitions
	tokenRange := new(big.Int)
	partSize := new(big.Int)
	partSize.SetString(maxAddr, 16)
	partSize = partSize.Div(partSize, big.NewInt(int64(numPartitions)))

	// We model the partitions as a segment that begins at minAddr and ends at maxAddr.
	// By setting the end range for the last partition to maxAddr, we ensure that we always
	// cover the full range of addresses, even if the range itself is not evenly divisible
	// by numPartitions.
	if partition == 0 {
		from = minAddr
	} else {
		tokenRange.Mul(partSize, big.NewInt(int64(partition)))
		from = fmt.Sprintf("%x", tokenRange) // convert to hex string
	}

	if partition == numPartitions-1 {
		to = maxAddr
	} else {
		tokenRange.Mul(partSize, big.NewInt(int64(partition+1)))
		to = fmt.Sprintf("%x", tokenRange) // convert to hex string
	}
	
	return from, to
}
