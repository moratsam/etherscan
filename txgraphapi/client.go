package txgraphapi

import (
	"context"
	"io"
	"math/big"

	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
	"github.com/moratsam/etherscan/txgraphapi/proto"
)

//go:generate mockgen -package mocks -destination mocks/mock.go github.com/moratsam/etherscan/txgraphapi/proto TxGraphClient,TxGraph_BlocksClient,TxGraph_WalletTxsClient,TxGraph_WalletsClient

// TxGraphClient provides an API compatible with the graph.Graph interface
// for accessing graph instances exposed by a remote gRPC server.
type TxGraphClient struct {
	ctx context.Context
	cli proto.TxGraphClient
}

// NewTxGraphClient returns a new client instance that implements a subset of the graph.Graph
// interface by delegating methods to a graph instance exposed by a remote gRPC server.
func NewTxGraphClient(ctx context.Context, rpcClient proto.TxGraphClient) *TxGraphClient {
	return &TxGraphClient{ctx: ctx, cli: rpcClient}
}

func (c *TxGraphClient) Blocks() (graph.BlockIterator, error) {
	req := new(empty.Empty)

	ctx, cancelFn := context.WithCancel(c.ctx)
	stream, err := c.cli.Blocks(ctx, req)
	if err != nil {
		cancelFn()
		return nil, err
	}

	return &blockIterator{stream: stream, cancelFn: cancelFn}, nil
	
}
func (c *TxGraphClient) UpsertBlock(block *graph.Block) error {
	req := &proto.Block{
		Number:		int32(block.Number),
		Processed:	block.Processed,
	}
	_, err := c.cli.UpsertBlock(c.ctx, req)
	return err
}

func (c *TxGraphClient) InsertTxs(txs []*graph.Tx) error {
	reqTxs := make([]*proto.Tx, len(txs))
	for i,tx := range txs {
		timestamp, err := types.TimestampProto(tx.Timestamp)
		if err != nil {
			return err
		}

		reqTxs[i] = &proto.Tx {
			Hash: 				tx.Hash,
			Status: 				proto.Tx_TxStatus(tx.Status),
			Block: 				tx.Block.String(),
			Timestamp:			timestamp,
			From: 				tx.From,
			To: 					tx.To,
			Value:				tx.Value.String(),
			TransactionFee:	tx.TransactionFee.String(),
			Data:					tx.Data,
		}
	}
	req := &proto.Txs{Txs: reqTxs}
	_, err := c.cli.InsertTxs(c.ctx, req)
	return err
}

func (c *TxGraphClient) UpsertWallet(wallet *graph.Wallet) error {
	req := &proto.Wallet{
		Address: wallet.Address,
	}
	_, err := c.cli.UpsertWallet(c.ctx, req)
	return err
	
}

func (c *TxGraphClient) WalletTxs(wallet *graph.Wallet) (graph.TxIterator, error) {
	req := &proto.Wallet{
		Address: wallet.Address,
	}

	ctx, cancelFn := context.WithCancel(c.ctx)
	stream, err := c.cli.WalletTxs(ctx, req)
	if err != nil {
		cancelFn()
		return nil, err
	}

	return &txIterator{stream: stream, cancelFn: cancelFn}, nil
}

func (c *TxGraphClient) Wallets(fromAddress, toAddress string) (graph.WalletIterator, error) {
	req := &proto.Range{
		FromAddress:	fromAddress,
		ToAddress:		toAddress,
	}

	ctx, cancelFn := context.WithCancel(c.ctx)
	stream, err := c.cli.Wallets(ctx, req)
	if err != nil {
		cancelFn()
		return nil, err
	}

	return &walletIterator{stream: stream, cancelFn: cancelFn}, nil
}

type blockIterator struct {
	stream 	proto.TxGraph_BlocksClient
	next		*graph.Block
	lastErr	error

	// A function to cancel the context used to perform the streaming RPC.
	// It allows us to abort server-streaming calls from the client side.
	cancelFn func()
}

// Next advances the iterator. If no more items are available or an error occurs, 
// calls to Next() return false
func (it *blockIterator) Next() bool {
	res, err := it.stream.Recv()
	if err != nil {
		if err != io.EOF {
			it.lastErr = err
		}
		it.cancelFn()
		return false
	}

	it.next = &graph.Block{
		Number:		int(res.Number),
		Processed:	res.Processed,
	}
	return true
}

// Returns the last error encountered by the iterator.
func (it *blockIterator) Error() error { return it.lastErr }

// Returns the currently fetched block.
func (it *blockIterator) Block() *graph.Block{ return it.next }

// Releases any objects associated with the iterator.
func (it *blockIterator) Close() error {
	it.cancelFn()
	return nil
}


type txIterator struct {
	stream 	proto.TxGraph_WalletTxsClient
	next		*graph.Tx
	lastErr	error

	// A function to cancel the context used to perform the streaming RPC.
	// It allows us to abort server-streaming calls from the client side.
	cancelFn func()
}

// Next advances the iterator. If no more items are available or an error occurs, 
// calls to Next() return false
func (it *txIterator) Next() bool {
	res, err := it.stream.Recv()
	if err != nil {
		if err != io.EOF {
			it.lastErr = err
		}
		it.cancelFn()
		return false
	}

	// Make necessary conversions from proto formats.
	timestamp, err := types.TimestampFromProto(res.Timestamp)
	if err != nil {
		it.lastErr = err
		it.cancelFn()
		return false
	}

	block := new(big.Int)
	block, ok := block.SetString(res.Block, 10)
	if !ok {
		it.lastErr = xerrors.New("invalid bigint SetString")
		it.cancelFn()
		return false
	}

	value := new(big.Int)
	value, ok = value.SetString(res.Value, 10)
	if !ok {
		it.lastErr = xerrors.New("invalid bigint SetString")
		it.cancelFn()
		return false
	}

	transactionFee := new(big.Int)
	transactionFee, ok = transactionFee.SetString(res.TransactionFee, 10)
	if !ok {
		it.lastErr = xerrors.New("invalid bigint SetString")
		it.cancelFn()
		return false
	}

	it.next = &graph.Tx{
		Hash: 				res.Hash,
		Status: 				graph.TxStatus(res.Status),
		Block: 				block,
		Timestamp:			timestamp,
		From: 				res.From,
		To: 					res.To,
		Value:				value,
		TransactionFee:	transactionFee,
		Data:					res.Data,
	}
	return true
}

// Returns the last error encountered by the iterator.
func (it *txIterator) Error() error { return it.lastErr }

// Returns the currently fetched tx.
func (it *txIterator) Tx() *graph.Tx { return it.next }

// Releases any objects associated with the iterator.
func (it *txIterator) Close() error {
	it.cancelFn()
	return nil
}

type walletIterator struct {
	stream 	proto.TxGraph_WalletsClient
	next		*graph.Wallet
	lastErr	error

	// A function to cancel the context used to perform the streaming RPC.
	// It allows us to abort server-streaming calls from the client side.
	cancelFn func()
}

// Next advances the iterator. If no more items are available or an error occurs, 
// calls to Next() return false
func (it *walletIterator) Next() bool {
	res, err := it.stream.Recv()
	if err != nil {
		if err != io.EOF {
			it.lastErr = err
		}
		it.cancelFn()
		return false
	}

	it.next = &graph.Wallet{
		Address:	res.Address,
	}
	return true
}

// Returns the last error encountered by the iterator.
func (it *walletIterator) Error() error { return it.lastErr }

// Returns the currently fetched wallet.
func (it *walletIterator) Wallet() *graph.Wallet { return it.next }

// Releases any objects associated with the iterator.
func (it *walletIterator) Close() error {
	it.cancelFn()
	return nil
}
