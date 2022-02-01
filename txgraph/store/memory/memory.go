package memory

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// Compile-time check for ensuring InMemoryGraph implements Graph.
var _ graph.Graph = (*InMemoryGraph)(nil)

// txList contains the slice of hashes of transactions a given wallet is connected to.
// This means the wallet is either the sender or the receiver of the transaction.
type txList []string

// InMemoryGraph implements an in-memory transaction graph that can be concurrently
// accessed by multiple clients.
type InMemoryGraph struct {
	mu sync.RWMutex

	txs 		map[string]*graph.Tx 		//[tx hash] --> Tx
	wallets	map[string]*graph.Wallet	//[wallet address] --> Wallet

	// [wallet address] --> list of hashes of the transactions a wallet is connected to.
	// This means the wallet is either the sender or the receiver of the transaction.
	walletTxsMap map[string]txList
}

func NewInMemoryGraph() *InMemoryGraph {
	return &InMemoryGraph{
		txs:				make(map[string]*graph.Tx),
		wallets			make(map[string]*graph.Wallet),
		walletTxsMap:	make(map[string]txList),
	}
}

// Inserts a transaction.
func (g *InMemoryGraph) InsertTx(tx *Tx) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If wallets connected to this transaction are unknown, return error.
	_, fromExists := g.wallets[tx.From]
	_, toExists := g.wallets[tx.To]
	if !fromExists || !toExists {
		return xerrors.Errorf("insert tx: %w", graph.ErrUnknownTxWallets)
	}

	// If a tx with the given hash already exists, do nothing.
	if existing := g.txs[tx.Hash]; existing != nil {
		return nil
	}

	// Add a copy of the transaction to the graph.
	txCopy := new(graph.Tx)
	*txCopy = *tx
	g.txs[txCopy.Hash] = txCopy

	// Append the transaction hash to txLists for wallets listed in To and From
	g.walletTxsMap[txCopy.From] = append(g.walletTxsMap[txCopy.From], txCopy.Hash)
	g.walletTxsMap[txCopy.To] = append(g.walletTxsMap[txCopy.To], txCopy.Hash)
	return nil
}

// Upserts a Wallet.
func (g *InMemoryGraph) UpsertWallet(wallet *Wallet) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check if a wallet with the same address already exists. 
	// If so, potentially update it's Crawled field.
	if existing := g.wallets[wallet.Address]; existing != nil {
		if ! existing.Crawled && wallet.Crawled {
			existing.Crawled = wallet.Crawled
		}
		return nil
	}

	//Add a copy of the wallet to the graph.
	wCopy = new(graph.Wallet)
	*wCopy = *wallet
	g.wallets[wCopy.Address] = wCopy
	return nil
}

// Looks up a wallet by its address.
func (g *InMemoryGraph) FindWallet(address string) (*Tx, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	wallet := g.wallets[address]
	if wallet == nil {
		return nil, xerror.Errorf("find wallet: %w", graph.ErrNotFound)
	}

	wCopy = new(graph.Wallet)
	*wCopy = *wallet
	return wCopy, nil
}

// Returns an iterator for the set of transactions connected to a wallet.
func (g *InMemoryGraph) WalletTxs(address string) (TxIterator, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var list []*graph.Tx
	for _, tx := range g.walletTxsMap[address] {
		list = append(list, tx)
	}
	return &txIterator{g: g, txs: list}, nil
}

// Returns an iterator for the set of wallets that belong in the
// [fromAddress, toAddress) range.
func (g *InMemoryGraph) Wallets(fromAddress, toAddress string) (WalletIterator, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var list []*graph.Wallet
	for address, wallet := range g.wallets {
		if address >= fromAddress && address < toAddress {
			list = append(list, wallet)
		}
	}
	return &walletIterator{g: g, wallets: list}, nil
}
