package graph

import "golang.org/x/xerrors"

var (
	// ErrNotFound is returned when a wallet lookup fails.
	ErrNotFound = xerrors.New("not found")

	// ErrInvalidAddress is returned when upserting a wallet
	// whose address is not 40 chars long.
	ErrInvalidAddress = xerrors.New("invalid wallet address.")

	// ErrUnknownAddress is returned when attempting to insert a transaction whose 
	// to or from wallet addresses have not yet been inserted into the graph.
	ErrUnknownAddress = xerrors.New("unknown wallet address.")
)
