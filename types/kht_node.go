package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type RootNode struct {
	MerkleRoot Hash
	HashTable  map[string][]byte
}

// Hash returns the blake256 hash of the RootNode
func (r *RootNode) Hash() (Hash, error) {
	if r == nil {
		return NilHash, errors.New("invalid root node")
	}

	rawData, err := r.Bytes()
	if err != nil {
		return NilHash, err
	}

	return GetHash(rawData), nil
}

// Bytes serialises the root node
func (r *RootNode) Bytes() ([]byte, error) {
	if r == nil {
		return nil, errors.New("invalid root node")
	}

	rawData, err := polo.Polorize(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize root node")
	}

	return rawData, nil
}

func (r *RootNode) FromBytes(bytes []byte) error {
	if r == nil {
		return errors.New("invalid root node")
	}

	if err := polo.Depolorize(r, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize root node")
	}

	return nil
}
