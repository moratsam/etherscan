package txgraphapi_test

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"time"

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
	cli			*txgraphapi.TxGraphClient
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
	proto_cli := proto.NewTxGraphClient(s.cliConn)
	s.cli = txgraphapi.NewTxGraphClient(context.TODO(), proto_cli)
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
	err := s.cli.UpsertBlock(&graph.Block{Number: testBlockNumber1})
	c.Assert(err, gc.IsNil)
	
	// Update existing block, set Processed to true
	updated := &graph.Block{
		Number: 		testBlockNumber1,
		Processed:	true,
	}
	err = s.cli.UpsertBlock(updated)
	c.Assert(err, gc.IsNil)

	// Create and insert the second block
	secondBlock := &graph.Block{Number: testBlockNumber2}
	err = s.cli.UpsertBlock(secondBlock)
	c.Assert(err, gc.IsNil)

	// Subscribe to blocks and receive a block and verify it's the secondBlock
	it, err := s.cli.Blocks()
	c.Assert(err, gc.IsNil)
	var cnt int
	for it.Next() {
		next := it.Block()
		if err != nil {
			c.Assert(err, gc.IsNil)
		}
		c.Assert(next, gc.DeepEquals, secondBlock, gc.Commentf("Original block should not get returned because it's already processed"))
		cnt++;
		break;
	}
	
	c.Assert(cnt, gc.Equals, 1)
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)
}


func (s *ServerTestSuite) TestInsertTxs(c *gc.C) {
	testHash := "47d8"
	fromAddr := createAddressFromInt(1);
	toAddr := createAddressFromInt(2);
	initValue := big.NewInt(3)

	// Create and insert wallets.
	wallets := []*graph.Wallet{
		&graph.Wallet{Address: fromAddr},
		&graph.Wallet{Address: toAddr},
	}
	err := s.cli.UpsertWallets(wallets)
	c.Assert(err, gc.IsNil)

	// Create and insert transaction.
	txs := []*graph.Tx{
		&graph.Tx{
			Hash:					testHash,
			Status: 				graph.Success,
			Block: 				big.NewInt(111),
			Timestamp:			time.Now().UTC(),
			From: 				fromAddr,
			To: 					toAddr,
			Value: 				initValue,
			TransactionFee:	big.NewInt(323),
			Data: 				make([]byte, 10),
		},
	}
	err = s.cli.InsertTxs(txs)
	c.Assert(err, gc.IsNil)

	// Retrieve transaction from WalletTxs iterators of both wallets.
	it, err := s.cli.WalletTxs(wallets[0].Address)
	c.Assert(err, gc.IsNil)
	var cnt int
	for it.Next() {
		next := it.Tx()
		if err != nil {
			c.Assert(err, gc.IsNil)
		}
		c.Assert(next, gc.DeepEquals, txs[0], gc.Commentf("tx should equal only tx"))
		cnt++;
	}
	c.Assert(cnt, gc.Equals, 1)
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)

	it, err = s.cli.WalletTxs(wallets[1].Address)
	c.Assert(err, gc.IsNil)
	cnt = 0
	for it.Next() {
		next := it.Tx()
		if err != nil {
			c.Assert(err, gc.IsNil)
		}
		c.Assert(next, gc.DeepEquals, txs[0], gc.Commentf("tx should equal only tx"))
		cnt++;
	}
	c.Assert(cnt, gc.Equals, 1)
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)
}

func (s *ServerTestSuite) TestUpsertWallets(c *gc.C) {
	fromAddr := createAddressFromInt(1);
	toAddr := createAddressFromInt(2);

	// Create and insert wallets.
	wallets := []*graph.Wallet{
		&graph.Wallet{Address: fromAddr},
		&graph.Wallet{Address: toAddr},
	}
	err := s.cli.UpsertWallets(wallets)
	c.Assert(err, gc.IsNil)

	// Consume wallets.
	it, err := s.cli.Wallets(fromAddr, createAddressFromInt(3))
	c.Assert(err, gc.IsNil)
	cnt := 0
	for it.Next() {
		next := it.Wallet()
		if err != nil {
			c.Assert(err, gc.IsNil)
		}
		c.Assert(next, gc.DeepEquals, wallets[cnt], gc.Commentf("wallet should equal"))
		cnt++;
	}
	c.Assert(cnt, gc.Equals, 2)
	c.Assert(it.Error(), gc.IsNil)
	c.Assert(it.Close(), gc.IsNil)
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

