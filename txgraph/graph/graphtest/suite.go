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

func (s *SuiteBase) TestInsertTx(c *gc.C) {
	testHash := "47d8"
	fromAddr := s.createAddressFromInt(c, 1);
	toAddr := s.createAddressFromInt(c, 2);
	initValue := big.NewInt(3)
	changedValue := big.NewInt(333)

	tx := &graph.Tx{
		Hash:					testHash,
		Status: 				graph.Success,
		Block: 				big.NewInt(111),
		Timestamp:			time.Now(),
		From: 				fromAddr,
		To: 					toAddr,
		Value: 				initValue,
		TransactionFee:	big.NewInt(323),
		Data: 				make([]byte, 10),
	}

	// Try to insert a tx without prior inserting the wallets
	err := s.g.InsertTx(tx)
	c.Assert(xerrors.Is(err, graph.ErrUnknownAddress), gc.Equals, true)
	
	// Insert wallets
	err = s.g.UpsertWallet(&graph.Wallet{Address: fromAddr, Crawled: false})
	c.Assert(err, gc.IsNil)
	err = s.g.UpsertWallet(&graph.Wallet{Address: toAddr, Crawled: false})
	c.Assert(err, gc.IsNil)

	// Insert transaction
	err = s.g.InsertTx(tx)
	c.Assert(err, gc.IsNil)

	// Change a field and attempt to re-insert the tx, the change it back to state.
	tx.Value = changedValue
	err = s.g.InsertTx(tx)
	c.Assert(err, gc.IsNil)
	tx.Value = initValue

	// Retrieve it from WalletTxs iterator
	itFrom, err := s.g.WalletTxs(fromAddr)
	c.Assert(err, gc.IsNil)
	itTo, err := s.g.WalletTxs(toAddr)
	c.Assert(err, gc.IsNil)

	i := 0
	var txFrom, txTo *graph.Tx
	for i=0; itFrom.Next(); i++ {
		txFrom = itFrom.Tx()
		txHash := tx.Hash
		c.Assert(txHash, gc.Equals, testHash, gc.Commentf("iterator returned wrong tx"))
		c.Assert(tx.Value, gc.Equals, initValue, gc.Commentf("tx Value got overwritten"))
	}
	c.Assert(i, gc.Equals, 1, gc.Commentf("wrong number of txs for a wallet"))
	c.Assert(itFrom.Error(), gc.IsNil)
	c.Assert(itFrom.Close(), gc.IsNil)

	for i=0; itTo.Next(); i++ {
		txTo = itTo.Tx()
		txHash := tx.Hash
		c.Assert(txHash, gc.Equals, testHash, gc.Commentf("iterator returned wrong tx"))
		c.Assert(tx.Value, gc.Equals, initValue, gc.Commentf("tx Value got overwritten"))
	}
	c.Assert(i, gc.Equals, 1, gc.Commentf("wrong number of txs for a wallet"))
	c.Assert(itTo.Error(), gc.IsNil)
	c.Assert(itTo.Close(), gc.IsNil)

	c.Assert(txFrom, gc.DeepEquals, tx)
	c.Assert(txTo, gc.DeepEquals, tx)
}

func (s *SuiteBase) TestUpsertWallet(c *gc.C) {
	testAddr := s.createAddressFromInt(c, 123093432)

	// Try to create a wallet with an invalid address
	err := s.g.UpsertWallet(&graph.Wallet{Address: "abc", Crawled: false})
	c.Assert(xerrors.Is(err, graph.ErrInvalidAddress), gc.Equals, true)

	// Create a new wallet
	original := &graph.Wallet{
		Address: testAddr,
		Crawled: false,
	}
	err = s.g.UpsertWallet(original)
	c.Assert(err, gc.IsNil)
	
	// Update existing wallet, set Crawled to true
	updated := &graph.Wallet{
		Address: testAddr,
		Crawled: true,
	}
	err = s.g.UpsertWallet(updated)
	c.Assert(err, gc.IsNil)

	// Retrieve original wallet and verify Crawled field is true
	stored, err := s.g.FindWallet(testAddr)
	c.Assert(err, gc.IsNil)
	c.Assert(stored.Crawled, gc.Equals, true, gc.Commentf("Crawled field not updated to true"))

	// Update existing wallet, try to set Crawled to false
	updated = &graph.Wallet{
		Address: testAddr,
		Crawled: false,
	}
	err = s.g.UpsertWallet(updated)
	c.Assert(err, gc.IsNil)

	// Retrieve original wallet and verify Crawled field is still true
	stored, err = s.g.FindWallet(testAddr)
	c.Assert(err, gc.IsNil)
	c.Assert(stored.Crawled, gc.Equals, true, gc.Commentf("Crawled field updated back to false"))
}

func (s *SuiteBase) TestFindWallet(c *gc.C) {
	testAddr := s.createAddressFromInt(c, 123093432)
	// Create a new wallet
	original := &graph.Wallet{
		Address: testAddr,
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

func (s *SuiteBase) iteratePartitionedWallets(c *gc.C, numPartitions int) int {
	seen := make(map[string]bool)
	for partition := 0; partition < numPartitions; partition++ {
		it, err := s.partitionedWalletIterator(c, partition, numPartitions)
		c.Assert(err, gc.IsNil)
		defer func() {
			c.Assert(it.Close(), gc.IsNil)
		}()

		for it.Next() {
			wallet := it.Wallet()
			walletAddr := wallet.Address
			c.Assert(seen[walletAddr], gc.Equals, false, gc.Commentf("iterator returned same wallet in different partitions"))
			seen[walletAddr] = true
		}

		c.Assert(it.Error(), gc.IsNil)
		c.Assert(it.Close(), gc.IsNil)
	}
	
	return len(seen)
}

// TestPartitionedWalletIterators verifies that the graph partitioning logic
// works as expected even when partitions contain an uneven number of items.
func (s *SuiteBase) TestPartitionedWalletIterators(c *gc.C) {
	numPartitions := 10
	numWallets	 := 100
	for i:=0; i<numWallets; i++ {
		wallet := &graph.Wallet{Address: s.createAddressFromInt(c, i)}
		c.Assert(s.g.UpsertWallet(wallet), gc.IsNil)
	}

	// Check with both odd and even partition counts to check for rounding-related bugs.
	c.Assert(s.iteratePartitionedWallets(c, numPartitions), gc.Equals, numWallets)
	c.Assert(s.iteratePartitionedWallets(c, numPartitions+1), gc.Equals, numWallets)
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
