package datastore

import (
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
)

// CaveatReader offers read operations for caveats
type CaveatReader interface {
	// ReadCaveatByName returns a caveat with the provided name
	ReadCaveatByName(name string) (*core.Caveat, error)
}

// CaveatStorer offers both read and write operations for Caveats
type CaveatStorer interface {
	CaveatReader

	// WriteCaveats stores the provided caveats, and returns the assigned IDs
	// Each element o the returning slice corresponds by possition to the input slice
	WriteCaveats([]*core.Caveat) error

	// DeleteCaveats deletes the provided caveats
	DeleteCaveats([]*core.Caveat) error
}