package memory

import (
	"sync"
	"time"

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

	blocks	map[int]*graph.Block			//[block number] --> Block
	txs 		map[string]*graph.Tx 		//[tx hash] --> Tx
	wallets	map[string]*graph.Wallet	//[wallet address] --> Wallet

	// [wallet address] --> list of hashes of the transactions a wallet is connected to.
	// This means the wallet is either the sender or the receiver of the transaction.
	walletTxsMap map[string]txList
}

// NewInMemoryGraph returns an in-memory implementation of the Graph.
// The InMemoryGraph checks every refreshBlocksSeconds that there are no gaps
// in the blocks it contains. If any are found, it inserts the blocks to fill the gaps.
// It has all blocks from block 1 to the current largest block that was inserted.
func NewInMemoryGraph(refreshBlocksSeconds int) *InMemoryGraph {
	g := &InMemoryGraph{
		blocks:			make(map[int]*graph.Block),
		txs:				make(map[string]*graph.Tx),
		wallets:			make(map[string]*graph.Wallet),
		walletTxsMap:	make(map[string]txList),
	}
	go g.refreshBlocks(refreshBlocksSeconds)
	return g

}

// Continually checks for missing blocks in the graph.
// Insert all missing blocks, so that every block from 1 to the largest found block are
// in the graph.
func (g *InMemoryGraph) refreshBlocks(refreshBlocksSeconds int) {
	var maxBlockNumber int
	for {
		maxBlockNumber = 0
		g.mu.RLock()
		// Find largest block.
		for blockNumber, _ := range g.blocks {
			if blockNumber > maxBlockNumber {
				maxBlockNumber = blockNumber
			}
		}
		g.mu.RUnlock()

		// Insert missing blocks
		for i:=1; i<maxBlockNumber; i++ {
			_, keyExists := g.blocks[i]
			if ! keyExists {
				g.UpsertBlock(&graph.Block{Number: i})
			}
		}

		// Sleep for refreshBlocksSeconds seconds.
		time.Sleep(time.Duration(refreshBlocksSeconds) * time.Second)
	}
}

// Returns a list of unprocessed blocks.
func (g *InMemoryGraph) getUnprocessedBlocks() ([]*graph.Block, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var list []*graph.Block
	for _, block := range g.blocks {
		if !block.Processed {
			list = append(list, block)
		}
	}

	return list, nil
}

// Returns a BlockSubscriber connected to a stream of unprocessed blocks.
func (g *InMemoryGraph) Blocks() (graph.BlockIterator, error) {
	blocks, err := g.getUnprocessedBlocks()
	if err != nil {
		return nil, err
	}
	return &blockIterator{g: g, blocks: blocks}, nil
}

// Upserts a Block.
// Once the Processed field of a block equals true, it cannot be changed to false.
func (g *InMemoryGraph) UpsertBlock(block *graph.Block) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check if a block with the same number already exists. 
	// If so, potentially update it's Processed field.
	if existing := g.blocks[block.Number]; existing != nil {
		if !existing.Processed && block.Processed {
			existing.Processed = block.Processed
		}
		return nil
	}

	//Add a copy of the block to the graph.
	bCopy := new(graph.Block)
	*bCopy = *block
	g.blocks[bCopy.Number] = bCopy
	return nil
}

// Inserts a transaction.
func (g *InMemoryGraph) InsertTx(tx *graph.Tx) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If wallets connected to this transaction are unknown, return error.
	_, fromExists := g.wallets[tx.From]
	_, toExists := g.wallets[tx.To]
	if !fromExists || !toExists {
		return xerrors.Errorf("insert tx: %w", graph.ErrUnknownAddress)
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
func (g *InMemoryGraph) UpsertWallet(wallet *graph.Wallet) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(wallet.Address) != 40 {
		return xerrors.Errorf("upsert wallet: %w", graph.ErrInvalidAddress)
	}

	// Check if a wallet with the same address already exists. 
	if existing := g.wallets[wallet.Address]; existing != nil {
		return nil
	}

	//Add a copy of the wallet to the graph.
	wCopy := new(graph.Wallet)
	*wCopy = *wallet
	g.wallets[wCopy.Address] = wCopy
	return nil
}

// Looks up a wallet by its address.
func (g *InMemoryGraph) FindWallet(address string) (*graph.Wallet, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	wallet := g.wallets[address]
	if wallet == nil {
		return nil, xerrors.Errorf("find wallet: %w", graph.ErrNotFound)
	}

	wCopy := new(graph.Wallet)
	*wCopy = *wallet
	return wCopy, nil
}

// Returns an iterator for the set of transactions connected to a wallet.
func (g *InMemoryGraph) WalletTxs(address string) (graph.TxIterator, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var list []*graph.Tx
	for _, tx := range g.walletTxsMap[address] {
		list = append(list, g.txs[tx])
	}
	return &txIterator{g: g, txs: list}, nil
}

// Returns an iterator for the set of wallets that belong in the
// [fromAddress, toAddress) range.
func (g *InMemoryGraph) Wallets(fromAddress, toAddress string) (graph.WalletIterator, error) {
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
