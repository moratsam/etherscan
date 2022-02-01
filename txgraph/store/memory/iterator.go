package memory

import "github.com/moratsam/etherscan/txgraph/graph"

// txIterator is a graph.TxIterator implementation for the in-memory graph.
type txIterator struct {
	g *InMemoryGraph

	txs	[]*graph.Tx
	curIndex int
}

type (i *txIterator) Next() bool {
	if i.curIndex >= len(i.txs) {
		return false
	}
	i.curIndex++
	return true
}

type (i *txIterator) Error() error {
	return nil
}

type (i *txIterator) Close() error {
	return nil
}

type (i *txIterator) Tx() *graph.Tx {
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

type (i *walletIterator) Next() bool {
	if i.curIndex >= len(i.wallets) {
		return false
	}
	i.curIndex++
	return true
}

type (i *walletIterator) Error() error {
	return nil
}

type (i *walletIterator) Close() error {
	return nil
}

type (i *walletIterator) Wallet() *graph.Wallet {
	// The transactions are insert-only, so a read lock is necessary.
	i.g.mu.RLock()
	defer i.g.mu.RUnlock()
	wallet := new(graph.Wallet)
	*wallet = *i.wallets[i.curIndex-1]
	return wallet
}
