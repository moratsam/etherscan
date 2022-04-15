package scorestoreapi

import (
	"context"
	"math/big"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/xerrors"

	ss "github.com/moratsam/etherscan/scorestore"
	"github.com/moratsam/etherscan/scorestoreapi/proto"
)

var _ proto.ScoreStoreServer = (*ScoreStoreServer)(nil)

// ScoreStoreServer provides a gRPC layer for accessing a scorestore.
type ScoreStoreServer struct {
	s ss.ScoreStore
}

// NewScoreStoreServer returns a new server instance that uses the provided
// scorestore as its backing store.
func NewScoreStoreServer(s ss.ScoreStore) *ScoreStoreServer {
	return &ScoreStoreServer{s: s}
}

func (s *ScoreStoreServer) UpsertScore(_ context.Context, req *proto.Score) (*empty.Empty, error) {
	// Make necessary conversions from proto formats.
	value := new(big.Float)
	value, ok := value.SetString(req.Value)
	if !ok {
		return new(empty.Empty), xerrors.New("invalid bigint SetString")
	}

	score := &ss.Score{
		Wallet:	req.Wallet,
		Scorer:	req.Scorer,
		Value:	value,
	}
	err := s.s.UpsertScore(score)
	return new(empty.Empty), err
}

func (s *ScoreStoreServer) UpsertScorer(_ context.Context, req *proto.Scorer) (*empty.Empty, error) {
	scorer := &ss.Scorer{
		Name: req.Name,
	}
	err := s.s.UpsertScorer(scorer)
	return new(empty.Empty), err
}

func (s *ScoreStoreServer) Scorers(_ *empty.Empty, w proto.ScoreStore_ScorersServer) error {
	it, err := s.s.Scorers()
	if err != nil {
		return err
	}
	defer func() { _ = it.Close() }()



	// Send back the total result count.
	countRes := &proto.ScorersResponse{
		Result: &proto.ScorersResponse_ScorerCount{ScorerCount: it.TotalCount()},
	}
	if err = w.Send(countRes); err != nil {
		return err
	}

	// Start streaming.
	for it.Next() {
		scorer := it.Scorer()
		msg := &proto.ScorersResponse{
			Result: &proto.ScorersResponse_Scorer{
				Scorer: &proto.Scorer{
					Name: scorer.Name,
				},
			},
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


func (s *ScoreStoreServer) Search(req *proto.Query, w proto.ScoreStore_SearchServer) error {
	query := ss.Query{
		Type:			ss.QueryType(req.Type),
		Expression:	req.Expression,
		Offset:		req.Offset,
	}

	it, err := s.s.Search(query)
	if err != nil {
		return err
	}
	defer func() { _ = it.Close() }()

	// Send back the total result count.
	countRes := &proto.SearchResponse{
		Result: &proto.SearchResponse_ScoreCount{ScoreCount: it.TotalCount()},
	}
	if err = w.Send(countRes); err != nil {
		return err
	}

	// Start streaming.
	for it.Next() {
		score := it.Score()
		msg := &proto.SearchResponse{
			Result: &proto.SearchResponse_Score{
				Score: &proto.Score{
					Wallet:	score.Wallet,
					Scorer:	score.Scorer,
					Value:	score.Value.String(),
				},
			},
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

