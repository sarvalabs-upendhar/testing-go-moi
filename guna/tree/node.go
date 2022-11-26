package tree

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/types"
)

type rootNode struct {
	MerkleRoot types.Hash
	HashTable  types.Hash
}

// Hash returns the blake256 hash of the rootNode
func (r *rootNode) Hash() (types.Hash, error) {
	if r == nil {
		return types.NilHash, errors.New("invalid root node")
	}

	rawData, err := r.Bytes()
	if err != nil {
		return types.NilHash, err
	}

	return types.GetHash(rawData), nil
}

// Bytes serialises the root node
func (r *rootNode) Bytes() ([]byte, error) {
	if r == nil {
		return nil, errors.New("invalid root node")
	}

	rawData, err := polo.Polorize(r)
	if err != nil {
		return nil, err
	}

	return rawData, nil
}
