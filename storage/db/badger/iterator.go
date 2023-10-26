package badger

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/sarvalabs/go-moi/storage/db"

	"github.com/sarvalabs/go-moi/common"
)

// Iterator is a prefix enable badger key-value iterator
type Iterator struct {
	it      *badger.Iterator
	txn     *badger.Txn
	metrics *db.Metrics
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
func (b *Iterator) GetNext() (*common.DBEntry, error) {
	b.metrics.CaptureDBReads(1)

	var entry *common.DBEntry

	item := b.it.Item()
	err := item.Value(func(v []byte) error {
		if v != nil {
			entry = &common.DBEntry{Key: item.KeyCopy(nil), Value: append([]byte{}, v...)}
		}

		return nil
	})

	return entry, err
}
