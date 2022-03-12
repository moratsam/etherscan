package scorestore

import "golang.org/x/xerrors"

var (
	// ErrUnknownScorer is returned when the scorer is not known to the score store.
	ErrUnknownScorer = xerrors.New("unknown scorer.")

	// ErrUnknownQueryType is returned when Search is called with an unknown query type.
	ErrUnknownQueryType = xerrors.New("unknown query type.")
)
