package tree

import (
	"github.com/munna0908/smt"
	"github.com/sarvalabs/moichain/types"
)

type MerkleTree interface {
	Root() types.Hash
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	NewIterator() smt.Iterator
	GetPreImageKey(hashKey types.Hash) ([]byte, error)
	Copy() MerkleTree
	Flush() error
	IsDirty() bool
}
