package badger

import (
	"github.com/dgraph-io/badger/v3"
)

// BatchWriter is used to perform batch writes to badger database, It implements db.BatchWriter interface
type BatchWriter struct {
	bw *badger.WriteBatch
}

func (b *BatchWriter) Set(key, value []byte) error {
	return b.bw.Set(key, value)
}

// Flush commits all the entries to database
func (b *BatchWriter) Flush() error {
	return b.bw.Flush()
}
