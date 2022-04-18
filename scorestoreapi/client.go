package scorestoreapi

import (
	"context"
	"io"
	"math/big"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/xerrors"

	ss "github.com/moratsam/etherscan/scorestore"
	"github.com/moratsam/etherscan/scorestoreapi/proto"
)

// ScoreStoreClient provides an API compatible with the scorestore.ScoreStore interface
// for interacting with scorestore instances exposed by a remote gRPC server.
type ScoreStoreClient struct {
	ctx context.Context
	cli proto.ScoreStoreClient
}

// NewScoreStoreClient returns a new client instance that implements a subset of the  
// scorestore.ScoreStore interface by delegating methods to a graph instance
// exposed by a remote gRPC server.
func NewScoreStoreClient(ctx context.Context, rpcClient proto.ScoreStoreClient) *ScoreStoreClient {
	return &ScoreStoreClient{ctx: ctx, cli: rpcClient}
}

func (c *ScoreStoreClient) UpsertScores(scores []*ss.Score) error {
	reqScores := make([]*proto.Score, len(scores))
	for i,score := range scores {
		reqScores[i] = &proto.Score{
			Wallet:	score.Wallet,
			Scorer:	score.Scorer,
			Value:	score.Value.String(),
		}
	}
	req := &proto.ScoreBatch{Scores: reqScores}
	_, err := c.cli.UpsertScores(c.ctx, req)
	return err
}

func (c *ScoreStoreClient) UpsertScorer(scorer *ss.Scorer) error {
	req := &proto.Scorer{
		Name:	scorer.Name,
	}
	_, err := c.cli.UpsertScorer(c.ctx, req)
	return err
}

func (c *ScoreStoreClient) Scorers() (ss.ScorerIterator, error) {
	ctx, cancelFn := context.WithCancel(c.ctx)
	req := new(empty.Empty)
	stream, err := c.cli.Scorers(ctx, req)
	if err != nil {
		cancelFn()
		return nil, err
	}

	// Read the result count.
	res, err := stream.Recv()
	if err != nil {
		cancelFn()
		return nil, err
	} else if res.GetScorer() != nil {
		cancelFn()
		return nil, xerrors.Errorf("expected server to report the result count before sending any scorers")
	}

	return &scorerIterator{stream: stream, totalCount: res.GetScorerCount(), cancelFn: cancelFn}, nil
}

func (c *ScoreStoreClient) Search(query ss.Query) (ss.ScoreIterator, error) {
	ctx, cancelFn := context.WithCancel(c.ctx)
	req := &proto.Query{
		Type:			proto.Query_QueryType(query.Type),
		Expression:	query.Expression,
		Offset:		query.Offset,
	}
	stream, err := c.cli.Search(ctx, req)
	if err != nil {
		cancelFn()
		return nil, err
	}

	// Read the result count.
	res, err := stream.Recv()
	if err != nil {
		cancelFn()
		return nil, err
	} else if res.GetScore() != nil {
		cancelFn()
		return nil, xerrors.Errorf("expected server to report the result count before sending any scores")
	}

	return &scoreIterator{stream: stream, totalCount: res.GetScoreCount(), cancelFn: cancelFn}, nil
}

type scoreIterator struct {
	stream 		proto.ScoreStore_SearchClient
	totalCount	uint64
	next			*ss.Score
	lastErr		error

	// A function to cancel the context used to perform the streaming RPC.
	// It allows us to abort server-streaming calls from the client side.
	cancelFn func()
}

// Next advances the iterator. If no more items are available or an error occurs, 
// calls to Next() return false
func (it *scoreIterator) Next() bool {
	res, err := it.stream.Recv()
	if err != nil {
		if err != io.EOF {
			it.lastErr = err
		}
		it.cancelFn()
		return false
	}

	score := res.GetScore()
	if score == nil {
		it.cancelFn()
		it.lastErr = xerrors.Errorf("received nil score in search result list")
		return false
	}

	// Make necessary conversions from proto formats.
	value := new(big.Float)
	value, ok := value.SetString(score.Value)
	if !ok {
		it.lastErr = xerrors.New("invalid bigint SetString")
		it.cancelFn()
		return false
	}

	it.next = &ss.Score{
		Wallet:	score.Wallet,
		Scorer:	score.Scorer,
		Value:	value,
	}
	return true
}

// Returns the last error encountered by the iterator.
func (it *scoreIterator) Error() error { return it.lastErr }

// Returns the currently fetched score.
func (it *scoreIterator) Score() *ss.Score { return it.next }

// Returns the total count of results.
func (it *scoreIterator) TotalCount() uint64 { return it.totalCount }

// Releases any objects associated with the iterator.
func (it *scoreIterator) Close() error {
	it.cancelFn()
	return nil
}

type scorerIterator struct {
	stream 	proto.ScoreStore_ScorersClient
	totalCount	uint64
	next		*ss.Scorer
	lastErr	error

	// A function to cancel the context used to perform the streaming RPC.
	// It allows us to abort server-streaming calls from the client side.
	cancelFn func()
}

// Next advances the iterator. If no more items are available or an error occurs, 
// calls to Next() return false
func (it *scorerIterator) Next() bool {
	res, err := it.stream.Recv()
	if err != nil {
		if err != io.EOF {
			it.lastErr = err
		}
		it.cancelFn()
		return false
	}

	scorer := res.GetScorer()
	if scorer == nil {
		it.cancelFn()
		it.lastErr = xerrors.Errorf("received nil scorer in scorers response list")
		return false
	}

	it.next = &ss.Scorer{
		Name: scorer.Name,
	}
	return true
}

// Returns the last error encountered by the iterator.
func (it *scorerIterator) Error() error { return it.lastErr }

// Returns the currently fetched scorer.
func (it *scorerIterator) Scorer() *ss.Scorer { return it.next }

// Returns the total count of results.
func (it *scorerIterator) TotalCount() uint64 { return it.totalCount }

// Releases any objects associated with the iterator.
func (it *scorerIterator) Close() error {
	it.cancelFn()
	return nil
}
