package ethclient

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/xerrors"
)

type ETHClient interface {
	BlockByNumber(number int) (*types.Block, error)
	SubscribeNewHead(ctx context.Context) (<-chan *types.Header, ethereum.Subscription, error)
}

// Config encapsulates the confifuration options for creating a new ETHClient.
type Config struct {
	// If true, ETHClient will attempt to establish a connection to the local geth client.
	// If false, ETHClient will attempt to establish a connection to an infura endpoint.
	Local bool
}

func NewETHClient(cfg Config) (ETHClient, error){
	var endpoint string
	if cfg.Local {
		endpoint = "/run/media/o/cigla/.ethereum/geth.ipc"
	} else {
		endpoint = "wss://mainnet.infura.io/ws/v3/a1e9ac80b1d048139ceec8e3e783b80b"
	}

	client, err := ethclient.Dial(endpoint)
	if err != nil{
		return nil, xerrors.Errorf("dialling ethclient: %w", err)
	}

	return &ethClient{client: client}, nil
}

// Implements ETHClient
type ethClient struct {
	client *ethclient.Client
}

// Given a block number, fetches the pertaining block.
func (e *ethClient) BlockByNumber(number int) (*types.Block, error) {
		block, err := e.client.BlockByNumber(context.Background(), big.NewInt(int64(number)))	
		if err != nil {
			return nil, xerrors.Errorf("block by number: %w", err)
		}
		return block, nil
}

// Subscribes to receive the latest block headers. The Subscription can be queryed foe errors.
func (e *ethClient) SubscribeNewHead(ctx context.Context) (<-chan *types.Header, ethereum.Subscription, error) {
	// Make a channel that will be receiving the latest block headers.
	headerCh := make(chan *types.Header)

	// Subscribe to head.
	sub, err := e.client.SubscribeNewHead(ctx, headerCh)
	return headerCh, sub, err
}
