package eth_client

import "github.com/ethereum/go-ethereum/ethclient"


func NewClient() (*ethclient.Client, error){
	endpoint := "wss://mainnet.infura.io/ws/v3/a1e9ac80b1d048139ceec8e3e783b80b"

	var err error
	var client *ethclient.Client
	client, err = ethclient.Dial(endpoint)
	if err != nil{
		return nil, err
	}
	return client, nil
}
