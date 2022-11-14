package tree

import (
	"gitlab.com/sarvalabs/moichain/types"
	"gitlab.com/sarvalabs/polo/go-polo"
)

type rootNode struct {
	MerkleRoot types.Hash
	HashTable  types.Hash
}

// Hash returns the blake256 hash of the rootNode
func (r *rootNode) Hash() types.Hash {
	if r == nil {
		return types.NilHash
	}

	return types.GetHash(r.Bytes())
}

// Bytes serialises the root node
func (r *rootNode) Bytes() []byte {
	if r == nil {
		return nil
	}

	return polo.Polorize(r)
}
