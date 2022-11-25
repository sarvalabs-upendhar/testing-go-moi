package badger

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/sarvalabs/moichain/types"
)

// Iterator is a prefix enable badger key-value iterator
type Iterator struct {
	it  *badger.Iterator
	txn *badger.Txn
}

// Close closes the iterator and discards the transaction
func (b *Iterator) Close() {
	b.it.Close()
	b.txn.Discard()
}

// Seek move's the iterator to the given key
func (b *Iterator) Seek(key []byte) {
	b.it.Seek(key)
}

// Next moves the iterator to the next entry
func (b *Iterator) Next() {
	b.it.Next()
}

// ValidForPrefix checks if the current key is valid for the given prefix
func (b *Iterator) ValidForPrefix(prefix []byte) bool {
	return b.it.ValidForPrefix(prefix)
}

// GetNext returns the next entry
func (b *Iterator) GetNext() (*types.DBEntry, error) {
	var entry *types.DBEntry

	item := b.it.Item()
	err := item.Value(func(v []byte) error {
		if v != nil {
			entry = &types.DBEntry{Key: item.Key(), Value: v}
		}

		return nil
	})

	return entry, err
}
