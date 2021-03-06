package memory

import (
	"sync"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// blockIterator is a graph.BlockIterator implementation for the in-memory graph.
type blockIterator struct {
	g *InMemoryGraph

	mu sync.RWMutex

	blocks []*graph.Block
	curIndex int

	lastErr error
}

func (i *blockIterator) Next() bool {
	if i.lastErr != nil {
		return false
	}

	//Wait for new blocks to come in.
	if i.curIndex >= len(i.blocks) {
			return false
	}

	i.curIndex++
	return true
}

func (i *blockIterator) Error() error {
	return i.lastErr
}

func (i *blockIterator) Close() error {
	return nil
}

func (i *blockIterator) Block() *graph.Block {
	// The block pointer contents may be overwritten by a graph update; to 
	// avoid data-races, acquire the read lock and clone the block.
	i.mu.RLock()
	defer i.mu.RUnlock()
	block := new(graph.Block)
	*block = *i.blocks[i.curIndex-1]
	return block
}


// txIterator is a graph.TxIterator implementation for the in-memory graph.
type txIterator struct {
	g *InMemoryGraph

	txs	[]*graph.Tx
	curIndex int
}

func (i *txIterator) Next() bool {
	if i.curIndex >= len(i.txs) {
		return false
	}
	i.curIndex++
	return true
}

func (i *txIterator) Error() error {
	return nil
}

func (i *txIterator) Close() error {
	return nil
}

func (i *txIterator) Tx() *graph.Tx {
	// The transactions are insert-only, so a read lock is not necessary.
	tx := new(graph.Tx)
	*tx = *i.txs[i.curIndex-1]
	return tx
}


// walletIterator is a graph.WalletIterator implementation for the in-memory graph.
type walletIterator struct {
	g *InMemoryGraph

	wallets	[]*graph.Wallet
	curIndex int
}

func (i *walletIterator) Next() bool {
	if i.curIndex >= len(i.wallets) {
		return false
	}
	i.curIndex++
	return true
}

func (i *walletIterator) Error() error {
	return nil
}

func (i *walletIterator) Close() error {
	return nil
}

func (i *walletIterator) Wallet() *graph.Wallet {
	// The wallet pointer contents may be overwritten by a graph update; to 
	// avoid data-races, acquire the read lock and clone the wallet.
	i.g.mu.RLock()
	defer i.g.mu.RUnlock()
	wallet := new(graph.Wallet)
	*wallet = *i.wallets[i.curIndex-1]
	return wallet
}
