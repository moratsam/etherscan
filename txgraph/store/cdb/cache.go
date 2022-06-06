package cdb

import (
	"fmt"
	"math"
	"math/big"
	"time"

	cl "github.com/juju/clock"

	"github.com/moratsam/etherscan/txgraph/graph"
)

var clock = cl.WallClock

func maybeEmitError(err error, errCh chan<- error) {
	select {
		case errCh <- err: // error emitted.
		default: // error channel is full with other errors.
	}
}

func newBlock(b *graph.Block) *block {
	return &block {
		Number: 		b.Number,
		Processed:	b.Processed,
	}

}
type block struct {
	Number int
	Processed bool
}
func (b *block) key() interface{} {
	return b.Number
}
func toDBBlocks(items []cacheItem) error {
	blocks := make([]*block, len(items))
	for i,item := range items {
		blocks[i] = item.(*block)
	}
	return bulkUpsertBlocks(blocks)
}

func newTx(t *graph.Tx) *tx {
	return &tx{
		Hash: 				t.Hash,
		Status: 				graph.TxStatus(t.Status),
		Block: 				t.Block,
		Timestamp:			t.Timestamp,
		From: 				t.From,
		To: 					t.To,
		Value:				t.Value,
		TransactionFee:	t.TransactionFee,
		Data:					t.Data,
	}
}
type tx struct {
	Hash string
	Status graph.TxStatus
	Block *big.Int
	Timestamp time.Time
	From string
	To string
	Value *big.Int
	TransactionFee *big.Int
	Data []byte
}
func (t *tx) key() interface{} {
	return t.Hash
}
func toDBTxs(items []cacheItem) error {
	txs := make([]*tx, len(items))
	for i,item := range items {
		txs[i] = item.(*tx)
	}
	return bulkUpsertTxs(txs)
}

func newWallet(w *graph.Wallet) *wallet {
	return &wallet{
		Address: w.Address,
	}
}
type wallet struct {
	Address string
}
func (w *wallet) key() interface{} {
	return w.Address
}
func toDBWallets(items []cacheItem) error {
	wallets := make([]*wallet, len(items))
	for i,item := range items {
		wallets[i] = item.(*wallet)
	}
	return bulkUpsertWallets(wallets)
}

type cacheItem interface {
	key() interface{}
}
func toListOfCacheItems(item interface{}) []cacheItem{
	var cacheItems []cacheItem
	switch x := item.(type) {
	case *graph.Block:
		cacheItems = append(cacheItems, newBlock(x))
	case *graph.Tx:
		cacheItems = append(cacheItems, newTx(x))
	case *graph.Wallet:
		cacheItems = append(cacheItems, newWallet(x))
	case []*graph.Block:
		for _, y := range x {
			cacheItems = append(cacheItems, toListOfCacheItems(y)...)	
		}
	case []*graph.Tx:
		for _, y := range x {
			cacheItems = append(cacheItems, toListOfCacheItems(y)...)	
		}
	case []*graph.Wallet:
		for _, y := range x {
			cacheItems = append(cacheItems, toListOfCacheItems(y)...)	
		}
	case []interface{}:
		for _, y := range x {
			cacheItems = append(cacheItems, toListOfCacheItems(y)...)	
		}
	default:
		fmt.Printf("wat dis: %#+T\n", x)
	}
	return cacheItems
}
	

type cacheMap[A comparable] struct {
	m map[A]cacheItem
	batchSize, numUpserters int
}
func (cm cacheMap[A]) insert(item cacheItem) {
	k := item.key().(A)
	cm.m[k] = item
}
func (cm cacheMap[_]) purge() {
	for k := range cm.m{
		delete(cm.m, k)
	}
}
func (cm cacheMap[_]) toList() []cacheItem {
	list := make([]cacheItem, 0, len(cm.m))
	for _, item := range cm.m {
		list = append(list, item)
	}
	return list
}
func (cm cacheMap[_]) toDB() error {
	defer cm.purge()
	// Let <numUpserters> routines upsert list in batches.
	batchCh := make(chan []cacheItem, 1)
	doneCh := make(chan struct{}, cm.numUpserters)
	errCh := make(chan error, 1)

	// Function that will upsert.
	inserter := func() {
		defer func(){ doneCh <- struct{}{} }()
		for {
			batch, open := <- batchCh
			if !open {
				// Channel was closed.
				return
			}

			var err error
			switch batch[0].(type){
			case *block:
				err = toDBBlocks(batch)
			case *tx:
				err = toDBTxs(batch)
			case *wallet:
				err = toDBWallets(batch)
			}
			if err != nil {
				maybeEmitError(err, errCh)
			}
		}
	}

	for i:=0; i < cm.numUpserters; i++ {
		go inserter()
	}

	list := cm.toList()
	batchSize := cm.batchSize
	for i:=0; i<len(list); i+=batchSize {
		batchSize = int(math.Min(float64(batchSize), float64(len(list)-i)))
		select {
		case err := <- errCh:
			return err
		case batchCh <- list[i:i+batchSize]:
		}
	}

	close(batchCh)
	for i:=0; i < cm.numUpserters; i++ {
		select {
		 case <- doneCh:
		 case err := <- errCh:
			return err
		}
	}
	return nil
}

type cacheCfg struct {
	maxBlocks, blockBatchSize, txBatchSize, walletBatchSize, numUpserters int
}
func newDefaultCacheCfg() cacheCfg{
	return cacheCfg {
		maxBlocks:			100,
		blockBatchSize:	35,
		txBatchSize:		1500,
		walletBatchSize:	1000,
		numUpserters:		3,
	}
}
func newNoCachingCfg() cacheCfg{
	return cacheCfg {
		maxBlocks:			0,
		blockBatchSize:	100,
		txBatchSize:		125,
		walletBatchSize:	250,
		numUpserters:		1,
	}
}

func newCache(tokenCh chan<-int, ix int, cfg cacheCfg) cache[int, string] {
	c := cache[int, string]{
		tokenCh: 	tokenCh,
		ix:			ix,
		maxBlocks:	cfg.maxBlocks,
		blocks:		cacheMap[int]{
			m: 				make(map[int]cacheItem, cfg.maxBlocks), // Declare cap for keks.
			batchSize:		cfg.blockBatchSize,
			numUpserters:	cfg.walletBatchSize,
		},
		txs:			cacheMap[string]{
			m: 				make(map[string]cacheItem, 50*cfg.maxBlocks), // Declare cap for keks.
			batchSize:		cfg.txBatchSize,
			numUpserters:	cfg.numUpserters,
		},
		wallets:		cacheMap[string]{
			m: 				make(map[string]cacheItem, 30*cfg.maxBlocks), // Declare cap for keks.
			batchSize:		cfg.walletBatchSize,
			numUpserters:	1,
		},
	}

	// At initialisation, make the cache signal it is free.
	go func(){
		time.Sleep(time.Duration(261*ix) * time.Second)
		fmt.Printf("cache %d online\n", ix)
		c.signalFree()
	}()
	return c
}
type cache[I, S comparable] struct {
	tokenCh		chan<-int
	ix				int
	maxBlocks	int

	blocks	cacheMap[I]
	wallets	cacheMap[S]
	txs		cacheMap[S]
}
func (c cache[_,_]) insert(items []cacheItem) error {
	for _,item := range items {
		switch item.(type) {
		case *block:
			c.blocks.insert(item)
		case *tx:
			c.txs.insert(item)
		case *wallet:
			c.wallets.insert(item)
		}
	}
	if len(c.blocks.m) >= c.maxBlocks {
		fmt.Printf("%d:\tb: %d, t: %d, w: %d\n", c.ix, len(c.blocks.m), len(c.wallets.m), len(c.txs.m))
		return c.toDB()	
	}
	return nil
}
func (c cache[_,_]) toDB() error {
	t1 := clock.Now()
	if err := c.wallets.toDB(); err != nil {
		return err
	}
	t2 := clock.Now()
	if err := c.txs.toDB(); err != nil {
		return err
	}
	t3 := clock.Now()
	//return c.blocks.toDB()
	if err := c.blocks.toDB(); err != nil {
		return err
	}
	t4 := clock.Now()
	fmt.Printf("time\tb: %s, t: %s, w: %s\n", t2.Sub(t1).String(), t3.Sub(t2).String(), t4.Sub(t3).String())
	return nil
}
func (c cache[_,_]) signalFree() {
	c.tokenCh <- c.ix
}

func newMultiCache(dim int, cfg cacheCfg) multiCache {
	tokenCh := make(chan int, dim)
	caches := make([]cache[int, string], dim)
	for i:=0; i<dim; i++ {
		caches[i] = newCache(tokenCh, i, cfg)
	}

	return multiCache{
		dim:		dim,
		tokenCh:	tokenCh,
		caches:	caches,
	}
}
type multiCache struct {
	dim		int
	tokenCh	chan int
	caches	[]cache[int, string]
}

func (mc multiCache) insert(items []cacheItem) error {
	//tick := clock.Now()
	ix := <- mc.tokenCh 					// Get the index of a free cache.
	//waitTime := clock.Now().Sub(tick)
	//fmt.Printf("assigned cache %d after %s\n", ix, waitTime.String())
	defer mc.caches[ix].signalFree() // At the end, signal it is again free.

	return mc.caches[ix].insert(items)
}
