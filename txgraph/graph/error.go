package graph

import "golang.org/x/xerrors"

var (
	// ErrNotFound is returned when a wallet lookup fails.
	ErrNotFound = xerrors.New("not found")

	// ErrUnknownTxWallets is returned when attempting to create a transactions,
	// with an invalid to or from wallet address.
	ErrUnknownTxWallets = xerrors.New("unknown to or from wallet address for transaction.")
