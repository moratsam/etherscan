package ethclient

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/xerrors"
)

type ETHClient interface {
	BlockByNumber(number int) (*types.Block, error)
}

// Implements ETHClient
type ethClient struct {
	client *ethclient.Client
}

func (e *ethClient) BlockByNumber(number int) (*types.Block, error) {
		block, err := e.client.BlockByNumber(context.Background(), big.NewInt(int64(number)))	
		if err != nil {
			return nil, xerrors.Errorf("block by number: %w", err)
		}
		return block, nil
}

func NewETHClient() (ETHClient, error){
	endpoint := "wss://mainnet.infura.io/ws/v3/a1e9ac80b1d048139ceec8e3e783b80b"

	client, err := ethclient.Dial(endpoint)
	if err != nil{
		return nil, xerrors.Errorf("dialing ethclient: %w", err)
	}

	return &ethClient{client: client}, nil
}
