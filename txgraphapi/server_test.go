package txgraphapi_test

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/txgraph/graph"
	memgraph "github.com/moratsam/etherscan/txgraph/store/memory"
	"github.com/moratsam/etherscan/txgraphapi"
	"github.com/moratsam/etherscan/txgraphapi/proto"
)

var _ = gc.Suite(new(ServerTestSuite))

type ServerTestSuite struct{
	g	graph.Graph

	netListener *bufconn.Listener
	grpcSrv		*grpc.Server

	cliConn		*grpc.ClientConn
	cli			proto.TxGraphClient
}

func (s *ServerTestSuite) SetUpTest(c *gc.C) {
	var err error
	s.g = memgraph.NewInMemoryGraph()

	s.netListener = bufconn.Listen(1024)
	s.grpcSrv = grpc.NewServer()
	proto.RegisterTxGraphServer(s.grpcSrv, txgraphapi.NewTxGraphServer(s.g))
	go func() {
		err := s.grpcSrv.Serve(s.netListener)
		c.Assert(err, gc.IsNil)
	}()

	s.cliConn, err = grpc.Dial(
		"bufnet",
		grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) { return s.netListener.Dial() },
		),
		grpc.WithInsecure(),
	)
	c.Assert(err, gc.IsNil)
	s.cli = proto.NewTxGraphClient(s.cliConn)
}

func (s *ServerTestSuite) TearDownTest(c *gc.C) {
	_ = s.cliConn.Close()
	s.grpcSrv.Stop()
	_ = s.netListener.Close()
}

func (s *ServerTestSuite) TestUpsertBlock(c *gc.C) {
	testBlockNumber1 := 1
	testBlockNumber2 := 2

	// Create and insert the first block
	_, err := s.cli.UpsertBlock(context.TODO(), &proto.Block{Number: int32(testBlockNumber1)})
	c.Assert(err, gc.IsNil)
	
	// Update existing block, set Processed to true
	updated := &proto.Block{
		Number: 		int32(testBlockNumber1),
		Processed:	true,
	}
	_, err = s.cli.UpsertBlock(context.TODO(), updated)
	c.Assert(err, gc.IsNil)

	// Create and insert the second block
	secondBlock := &proto.Block{
		Number: 		int32(testBlockNumber2),
	}
	_, err = s.cli.UpsertBlock(context.TODO(), secondBlock)
	c.Assert(err, gc.IsNil)

	// Subscribe to blocks and receive a block and verify it's the secondBlock
	stream, err := s.cli.Blocks(context.TODO(), new(empty.Empty))
	c.Assert(err, gc.IsNil)
	var cnt int
	for {
		next, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.Fatal(err)
		}

		c.Assert(next, gc.DeepEquals, secondBlock, gc.Commentf("Original block should not get returned because it's already processed"))
		cnt++
		break
	}

	c.Assert(cnt, gc.Equals, 1)
}


func (s *ServerTestSuite) TestInsertTxs(c *gc.C) {
	testHash := "47d8"
	fromAddr := createAddressFromInt(1);
	toAddr := createAddressFromInt(2);
	initValue := big.NewInt(3)

	// Create and insert wallets.
	reqWallets := []*proto.Wallet{
		&proto.Wallet{Address: fromAddr},
		&proto.Wallet{Address: toAddr},
	}
	req := &proto.WalletBatch{Wallets: reqWallets}
	_, err := s.cli.UpsertWallets(context.TODO(), req)
	c.Assert(err, gc.IsNil)

	// Create and insert transaction.
	timestamp, err := types.TimestampProto(time.Now().UTC())
	c.Assert(err, gc.IsNil)

	reqTxs := []*proto.Tx{
		&proto.Tx{
			Hash:					testHash,
			Status: 				proto.Tx_TxStatus(graph.Success),
			Block: 				big.NewInt(111).String(),
			Timestamp:			timestamp,
			From: 				fromAddr,
			To: 					toAddr,
			Value: 				initValue.String(),
			TransactionFee:	big.NewInt(323).String(),
			Data: 				make([]byte, 10),
		},
	}
	req2 := &proto.TxBatch{Txs: reqTxs}

	_, err = s.cli.InsertTxs(context.TODO(), req2)
	c.Assert(err, gc.IsNil)

	// Retrieve transaction from WalletTxs iterators of both wallets.
	stream, err := s.cli.WalletTxs(context.TODO(), reqWallets[0])
	c.Assert(err, gc.IsNil)
	var cnt int
	for {
		next, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.Fatal(err)
		}

		c.Assert(next, gc.DeepEquals, reqTxs[0], gc.Commentf("tx should equal only tx"))
		cnt++
	}
	c.Assert(cnt, gc.Equals, 1)

	stream, err = s.cli.WalletTxs(context.TODO(), reqWallets[1])
	c.Assert(err, gc.IsNil)
	cnt = 0
	for {
		next, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.Fatal(err)
		}

		c.Assert(next, gc.DeepEquals, reqTxs[0], gc.Commentf("tx should equal only tx"))
		cnt++
	}
	c.Assert(cnt, gc.Equals, 1)
}

func (s *ServerTestSuite) TestUpsertWallets(c *gc.C) {
	fromAddr := createAddressFromInt(1);
	toAddr := createAddressFromInt(2);

	// Create and insert wallets.
	reqWallets := []*proto.Wallet{
		&proto.Wallet{Address: fromAddr},
		&proto.Wallet{Address: toAddr},
	}
	req := &proto.WalletBatch{Wallets: reqWallets}
	_, err := s.cli.UpsertWallets(context.TODO(), req)
	c.Assert(err, gc.IsNil)

	// Open stream and consume wallets.
	req2 := &proto.Range{
		FromAddress:	fromAddr,
		ToAddress:		createAddressFromInt(3),
	}
	stream, err := s.cli.Wallets(context.TODO(), req2)
	c.Assert(err, gc.IsNil)
	var cnt int
	for {
		next, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.Fatal(err)
		}

		c.Assert(next, gc.DeepEquals, reqWallets[cnt], gc.Commentf("wallet should equal"))
		cnt++
	}
	c.Assert(cnt, gc.Equals, 2)
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

