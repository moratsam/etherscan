package txgraphapi

import (
	"context"
	"math/big"

	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
	"github.com/moratsam/etherscan/txgraphapi/proto"
)

var _ proto.TxGraphServer = (*TxGraphServer)(nil)

// TxGraphServer provides a gRPC layer for accessing a txgraph.
type TxGraphServer struct {
	g graph.Graph
}

// NewTxGraphServer returns a new server instance that uses the provided
// graph as its backing store.
func NewTxGraphServer(g graph.Graph) *TxGraphServer {
	return &TxGraphServer{g: g}
}

func (s *TxGraphServer) Blocks(_ *empty.Empty, w proto.TxGraph_BlocksServer) error {
	it, err := s.g.Blocks()
	if err != nil {
		return err
	}
	defer func() { _ = it.Close() }()

	for it.Next() {
		block := it.Block()
		msg := &proto.Block{
			Number:		int32(block.Number),
			Processed:	block.Processed,
		}
		if err := w.Send(msg); err != nil {
			return err
		}
	}

	if err := it.Error(); err != nil {
		return err
	}

	return nil
}

func (s *TxGraphServer) UpsertBlock(_ context.Context, req *proto.Block) (*empty.Empty, error) {
	block := &graph.Block{
		Number:		int(req.Number),
		Processed:	req.Processed,
	}
	err := s.g.UpsertBlock(block)
	return new(empty.Empty), err
}

func (s *TxGraphServer) InsertTxs(_ context.Context, req *proto.Txs) (*empty.Empty, error) {
	txs := make([]*graph.Tx, len(req.Txs))
	for i, reqTx := range req.Txs {
		// Make necessary conversions from proto formats.
		timestamp, err := types.TimestampFromProto(reqTx.Timestamp)
		if err != nil {
			return new(empty.Empty), err
		}

		block := new(big.Int)
		block, ok := block.SetString(reqTx.Block, 10)
		if !ok {
			return new(empty.Empty), xerrors.New("invalid bigint SetString")
		}

		value := new(big.Int)
		value, ok = value.SetString(reqTx.Value, 10)
		if !ok {
			return new(empty.Empty), xerrors.New("invalid bigint SetString")
		}

		transactionFee := new(big.Int)
		transactionFee, ok = transactionFee.SetString(reqTx.TransactionFee, 10)
		if !ok {
			return new(empty.Empty), xerrors.New("invalid bigint SetString")
		}

		txs[i] = &graph.Tx{
			Hash: 				reqTx.Hash,
			Status: 				graph.TxStatus(reqTx.Status),
			Block: 				block,
			Timestamp:			timestamp,
			From: 				reqTx.From,
			To: 					reqTx.To,
			Value:				value,
			TransactionFee:	transactionFee,
			Data:					reqTx.Data,
		}
	}

	err := s.g.InsertTxs(txs)
	return new(empty.Empty), err
}

func (s *TxGraphServer) UpsertWallet(_ context.Context, req *proto.Wallet) (*empty.Empty, error) {
	wallet := &graph.Wallet{
		Address: req.Address,
	}
	err := s.g.UpsertWallet(wallet)
	return new(empty.Empty), err
}

func (s *TxGraphServer) WalletTxs(req *proto.Wallet, w proto.TxGraph_WalletTxsServer) error {
	unmarshalledWallet := &graph.Wallet{
		Address: req.Address,
	}

	it, err := s.g.WalletTxs(unmarshalledWallet.Address)
	if err != nil {
		return err
	}
	defer func() { _ = it.Close() }()

	for it.Next() {
		tx := it.Tx()
		timestamp, err := types.TimestampProto(tx.Timestamp)
		if err != nil {
			return err
		}
		msg := &proto.Tx {
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
		if err := w.Send(msg); err != nil {
			return err
		}
	}

	if err := it.Error(); err != nil {
		return err
	}

	return nil
}

// Streams the set of edges whose addresses belong to the specified partition range.
func (s *TxGraphServer) Wallets(addressRange *proto.Range, w proto.TxGraph_WalletsServer) error {
	it, err := s.g.Wallets(addressRange.FromAddress, addressRange.ToAddress)
	if err != nil {
		return err
	}
	defer func() { _ = it.Close() }()

	for it.Next() {
		wallet := it.Wallet()
		msg := &proto.Wallet{
			Address: wallet.Address,
		}
		if err := w.Send(msg); err != nil {
			return err
		}
	}

	if err := it.Error(); err != nil {
		return err
	}

	return nil
}
