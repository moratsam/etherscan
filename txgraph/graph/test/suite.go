package test

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	gc "gopkg.in/check.v1"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// SuiteBase defines a re-usable set of graph-related tests that can be executed
// against any type that implements graph.Graph.
type SuiteBase struct {
	g graph.Graph
}

func (s *SuiteBase) SetGraph(g graph.Graph) {
	s.g = g
}

func (s *SuiteBase) TestRefreshOfBlockIterator(c *gc.C) {
	testBlock := &graph.Block{Number: 2}

	blockIterator, err := s.g.Blocks()
	c.Assert(err, gc.IsNil)

	// Make a function that will insert a block.
	go func(block *graph.Block){
		s.g.UpsertBlock(block)
	}(testBlock)

	time.Sleep(1 * time.Second)
	c.Assert(blockIterator.Next(), gc.Equals, true, gc.Commentf("block iterator returned false"))
	receivedBlock1 := blockIterator.Block()
	receivedBlock2 := blockIterator.Block()
	if receivedBlock1.Number == 2 {
		c.Assert(receivedBlock1, gc.DeepEquals, testBlock, gc.Commentf("Received block should equal testBlock"))
	} else if receivedBlock2.Number == 2 {
		c.Assert(receivedBlock2, gc.DeepEquals, testBlock, gc.Commentf("Received block should equal testBlock"))
	}
}

func (s *SuiteBase) TestRefreshBlocks(c *gc.C) {
	// Insert block with high number.
	err := s.g.UpsertBlock(&graph.Block{Number: 300})
	c.Assert(err, gc.IsNil)

	blockIterator, err := s.g.Blocks()
	c.Assert(err, gc.IsNil)

	seen := make(map[int]bool)
	for i:=1; i<=300; i++ {
		c.Assert(blockIterator.Next(), gc.Equals, true, gc.Commentf("block iterator returned false"))
		receivedBlock := blockIterator.Block()
		c.Assert(seen[receivedBlock.Number], gc.Equals, false, gc.Commentf("Block %d was seen twice", receivedBlock.Number))
		seen[receivedBlock.Number] = true
	}
	for i,wasSeen := range seen {
		c.Assert(wasSeen, gc.Equals, true, gc.Commentf("Block %d was not seen", i))
	}
}

func (s *SuiteBase) TestBlockIterator(c *gc.C) {
	var err error
	// Insert 5 blocks
	// Set Processed=true for blocks 1 and 3
	for i:=1; i<=5; i++ {
		if i==1 || i==3 {
			err = s.g.UpsertBlock(&graph.Block{Number: i, Processed: true})
		} else {
			err = s.g.UpsertBlock(&graph.Block{Number: i})
		}
		c.Assert(err, gc.IsNil)
	}

	// Create 2 blockIterators
	blockIterator1, err := s.g.Blocks()
	c.Assert(err, gc.IsNil)
	blockIterator2, err := s.g.Blocks()
	c.Assert(err, gc.IsNil)

	// Receive 3 blocks from each iterator 
	seen1 := make(map[int]bool)
	seen2 := make(map[int]bool)
	for i:=0; i<3; i++ {
		c.Assert(blockIterator1.Next(), gc.Equals, true, gc.Commentf("block iterator 1 returned false"))
		receivedBlock := blockIterator1.Block()
		c.Assert(seen1[receivedBlock.Number], gc.Equals, false, gc.Commentf("Block %d was seen twice", receivedBlock.Number))
		seen1[receivedBlock.Number] = true

		c.Assert(blockIterator2.Next(), gc.Equals, true, gc.Commentf("block iterator 2 returned false"))
		receivedBlock = blockIterator2.Block()
		c.Assert(err, gc.IsNil)
		c.Assert(seen2[receivedBlock.Number], gc.Equals, false, gc.Commentf("Block %d was seen twice", receivedBlock.Number))
		seen2[receivedBlock.Number] = true
	}

	// Assert unprocessed blocks were seen.
	for _, blockNumber := range []int{2,4,5} {
		c.Assert(seen1[blockNumber], gc.Equals, true, gc.Commentf("Block %d was not seen1", blockNumber))
		c.Assert(seen2[blockNumber], gc.Equals, true, gc.Commentf("Block %d was not seen2", blockNumber))
	}
}

func (s *SuiteBase) TestUpsertBlock(c *gc.C) {
	testBlockNumber1 := 1
	testBlockNumber2 := 2

	// Create and insert a new block
	original := &graph.Block{Number: testBlockNumber1}
	err := s.g.UpsertBlock(original)
	c.Assert(err, gc.IsNil)
	
	// Update existing block, set Processed to true
	updated := &graph.Block{
		Number: 		testBlockNumber1,
		Processed:	true,
	}
	err = s.g.UpsertBlock(updated)
	c.Assert(err, gc.IsNil)

	// Create and insert a new block
	secondBlock := &graph.Block{Number: testBlockNumber2}
	err = s.g.UpsertBlock(secondBlock)
	c.Assert(err, gc.IsNil)

	// Subscribe to blocks and receive a block and verify it's the secondBlock
	blockIterator, err := s.g.Blocks()
	c.Assert(err, gc.IsNil)
	c.Assert(blockIterator.Next(), gc.Equals, true, gc.Commentf("block iterator returned false"))
	receivedBlock := blockIterator.Block()
	c.Assert(err, gc.IsNil)
	c.Assert(receivedBlock, gc.DeepEquals, secondBlock, gc.Commentf("Original block should not get returned because it's already processed"))
}

func (s *SuiteBase) TestInsertTxs(c *gc.C) {
	testHash := "47d8"
	fromAddr := createAddressFromInt(1);
	toAddr := createAddressFromInt(2);
	initValue := big.NewInt(3)

	tx := &graph.Tx{
		Hash:					testHash,
		Status: 				graph.Success,
		Block: 				big.NewInt(111),
		Timestamp:			time.Now().UTC(),
		From: 				fromAddr,
		To: 					toAddr,
		Value: 				initValue,
		TransactionFee:	big.NewInt(323),
		Data: 				make([]byte, 10),
	}

	// Try to insert a tx without prior inserting the wallets
	err := s.g.InsertTxs([]*graph.Tx{tx})
	c.Assert(xerrors.Is(err, graph.ErrUnknownAddress), gc.Equals, true)
	
	// Insert wallets
	wallets := make([]*graph.Wallet, 2)
	wallets[0] = &graph.Wallet{Address: fromAddr}
	wallets[1] = &graph.Wallet{Address: toAddr}
	err = s.g.UpsertWallets(wallets)
	c.Assert(err, gc.IsNil)

	//find the inserted wallets
	wallet, err := s.g.FindWallet(fromAddr)
	c.Assert(err, gc.IsNil)
	c.Assert(wallet.Address, gc.Equals, fromAddr, gc.Commentf("find wallet returned wrong address"))
	wallet, err = s.g.FindWallet(toAddr)
	c.Assert(err, gc.IsNil)
	c.Assert(wallet.Address, gc.Equals, toAddr, gc.Commentf("find wallet returned wrong address"))


	// Insert transaction
	err = s.g.InsertTxs([]*graph.Tx{tx})
	c.Assert(err, gc.IsNil)

	// Retrieve it from WalletTxs iterators of both wallets
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
	c.Assert(itFrom.Error(), gc.IsNil)
	c.Assert(itFrom.Close(), gc.IsNil)
	c.Assert(i, gc.Equals, 1, gc.Commentf("wrong number of txs for a wallet"))

	for i=0; itTo.Next(); i++ {
		txTo = itTo.Tx()
		txHash := tx.Hash
		c.Assert(txHash, gc.Equals, testHash, gc.Commentf("iterator returned wrong tx"))
		c.Assert(tx.Value, gc.Equals, initValue, gc.Commentf("tx Value got overwritten"))
	}
	c.Assert(i, gc.Equals, 1, gc.Commentf("wrong number of txs for a wallet"))
	c.Assert(itTo.Error(), gc.IsNil)
	c.Assert(itTo.Close(), gc.IsNil)

	// Assert their equivalence
	tx.Timestamp = tx.Timestamp.Truncate(time.Millisecond)
	txFrom.Timestamp = txFrom.Timestamp.Truncate(time.Millisecond)
	txTo.Timestamp = txTo.Timestamp.Truncate(time.Millisecond)
	c.Assert(txFrom, gc.DeepEquals, tx)
	c.Assert(txTo, gc.DeepEquals, tx)
}

func (s *SuiteBase) TestUpsertWallet(c *gc.C) {
	testAddr := createAddressFromInt(123093432)

	// Try to create a wallet with an invalid address
	err := s.g.UpsertWallets([]*graph.Wallet{&graph.Wallet{Address: "abc"}})
	c.Assert(xerrors.Is(err, graph.ErrInvalidAddress), gc.Equals, true)

	// Create a new wallet
	original := &graph.Wallet{
		Address: testAddr,
	}
	err = s.g.UpsertWallets([]*graph.Wallet{original})
	c.Assert(err, gc.IsNil)
	
	// Retrieve original wallet and verify it's equal.
	retrieved, err := s.g.FindWallet(testAddr)
	c.Assert(err, gc.IsNil)
	c.Assert(retrieved, gc.DeepEquals, original, gc.Commentf("lookup by Address returned wrong wallet"))
}

func (s *SuiteBase) TestFindWallet(c *gc.C) {
	testAddr := createAddressFromInt(123093432)
	// Create a new wallet
	original := &graph.Wallet{
		Address: testAddr,
	}
	err := s.g.UpsertWallets([]*graph.Wallet{original})
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
func (s *SuiteBase) TestConcurrentTxIterators(c *gc.C) {
	var (
		wg				 sync.WaitGroup
		numIterators = 10
		numTxs       = 100
		fromAddr 	 = createAddressFromInt(1);
		toAddr 		 = createAddressFromInt(2);
	)

	// Insert two wallets
	wallets := make([]*graph.Wallet, 2)
	wallets[0] = &graph.Wallet{Address: fromAddr}
	wallets[1] = &graph.Wallet{Address: toAddr}
	err := s.g.UpsertWallets(wallets[0:1])
	err = s.g.UpsertWallets(wallets)
	c.Assert(err, gc.IsNil)

	// Insert transactions
	var txs []*graph.Tx
	for i:=0; i<numTxs; i++ {
		tx := &graph.Tx{
			Hash:					fmt.Sprint(i),
			Status: 				graph.Success,
			Block: 				big.NewInt(111),
			Timestamp:			time.Now().UTC(),
			From: 				fromAddr,
			To: 					toAddr,
			Value: 				big.NewInt(1),
			TransactionFee:	big.NewInt(323),
			Data: 				make([]byte, 10),
		}
		txs = append(txs, tx)
	}
	err = s.g.InsertTxs(txs)
	c.Assert(err, gc.IsNil)

	wg.Add(numIterators)
	for i:=0; i<numIterators; i++ {
		go func(id int) {
			defer wg.Done()

			itTagComment := gc.Commentf("iterator %d", id)
			seen := make(map[string]bool)
			it, err := s.g.WalletTxs(fromAddr)
			c.Assert(err, gc.IsNil, itTagComment)
			defer func() {
				c.Assert(it.Close(), gc.IsNil, itTagComment)
			}()

			for i:=0; it.Next(); i++ {
				tx := it.Tx()
				txHash := tx.Hash
				c.Assert(seen[txHash], gc.Equals, false, gc.Commentf("iterator %d saw same tx twice", id))
				seen[txHash] = true
			}

			c.Assert(seen, gc.HasLen, numTxs, itTagComment)
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

// Verifies that multiple clients can concurrently access the store.
func (s *SuiteBase) TestConcurrentWalletIterators(c *gc.C) {
	var (
		wg				 sync.WaitGroup
		numIterators = 10
		numTxs		 = 100
	)

	for i:=0; i<numTxs; i++ {
		wallet := &graph.Wallet{Address: createAddressFromInt(i)}
		c.Assert(s.g.UpsertWallets([]*graph.Wallet{wallet}), gc.IsNil)
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

			c.Assert(seen, gc.HasLen, numTxs, itTagComment)
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
		wallet := &graph.Wallet{Address: createAddressFromInt(i)}
		c.Assert(s.g.UpsertWallets([]*graph.Wallet{wallet}), gc.IsNil)
	}

	// Check with both odd and even partition counts to check for rounding-related bugs.
	c.Assert(s.iteratePartitionedWallets(c, numPartitions), gc.Equals, numWallets)
	c.Assert(s.iteratePartitionedWallets(c, numPartitions+1), gc.Equals, numWallets)
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
	partSize := new(big.Int)
	partSize.SetString(maxAddr, 16)
	partSize = partSize.Div(partSize, big.NewInt(int64(numPartitions)))

	// We model the partitions as a segment that begins at minAddr and ends at maxAddr.
	// By setting the end range for the last partition to maxAddr, we ensure that we always
	// cover the full range of addresses, even if the range itself is not evenly divisible
	// by numPartitions.
	tokenRange := new(big.Int)
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
