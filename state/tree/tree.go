package tree

import (
	"github.com/munna0908/smt"

	"github.com/sarvalabs/moichain/common"
)

type MerkleTree interface {
	Root() common.RootNode
	RootHash() (common.Hash, error)
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	NewIterator() smt.Iterator
	GetPreImageKey(hashKey common.Hash) ([]byte, error)
	Copy() MerkleTree
	Flush() error
	IsDirty() bool
}
