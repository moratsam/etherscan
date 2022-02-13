package cdb

import (
	"golang.org/x/xerrors"

	"github.com/moratsam/etherscan/txgraph/graph"
)

// blockSubscriber is a graph.BlockSubscriber implementation for the in-memory graph.
type blockSubscriber struct {
	blockCh	<-chan *graph.Block
	doneCh	chan<- struct{}
	closed	bool
}

func (s *blockSubscriber) Next() (*graph.Block, error) {
	if s.closed {
		return nil, xerrors.Errorf("receiving next block: %w", graph.ErrChannelClosed)
	}

	return <-s.blockCh, nil
}

func (s *blockSubscriber) Close() error {
	s.doneCh <- struct{}{}
	close(s.doneCh)

	s.closed = true
	return nil
}

//returns a blockSubscriber connected to a stream of unprocessed blocks.
func newBlockSubscriber(g *CockroachDbGraph) *blockSubscriber {
	blockCh := make(chan *graph.Block)
	doneCh := make(chan struct{})

	pub := &blockPublisher{
		g:		 	g,
		blockCh:	blockCh,
		doneCh:	doneCh,
	}

	// blockPublisher will keep publishing unprocessed blocks
	// until the blockSubscriber has been closed.
	go pub.stream()

	sub := &blockSubscriber{
		blockCh: blockCh,
		doneCh: doneCh,
	}

	return sub
}

// blockPublisher is responsible for streaming unprocessed blocks to a blockSubscriber.
type blockPublisher struct {
	g *CockroachDbGraph

	blockCh	chan<- *graph.Block
	doneCh	<-chan struct{}
}

func (p *blockPublisher) stream() {
	for {
		blocks := p.g.getUnprocessedBlocks() // Get unprocessed blocks.
		for _, block := range blocks {
			select {
			case p.blockCh <- block: // Send a block.
			case <- p.doneCh: // Subscriber closed the subscription.
				p.handleClose()
				return
			}
		}
	}
}


// After a blockSubscriber closes the subscription close the block channel.
func (p *blockPublisher) handleClose() {
	close(p.blockCh)
}
