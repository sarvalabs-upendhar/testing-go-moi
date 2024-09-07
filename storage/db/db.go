package db

import (
	"context"

	"github.com/dgraph-io/ristretto/z"

	"github.com/sarvalabs/go-moi/common"
)

type Collector interface {
	Send(buf *z.Buffer) error
}

// Database defines a common interface implemented by all key-value database
type Database interface {
	Insert(key []byte, value []byte) error
	Update(key []byte, value []byte) error
	Delete(key []byte) error
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	NewIterator() (Iterator, error)
	CleanUp() error
	Close() error
	NewBatchWriter() BatchWriter
	Snapshot(ctx context.Context, prefix []byte, sinceTS uint64, collector Collector) error
	DropWithPrefix(prefix []byte) error
	GetLastActiveTimeStamp() uint64
}

// BatchWriter is a common interface to write bulk entries
type BatchWriter interface {
	Set(key, value []byte) error
	Delete(key []byte) error
	WriteBuffer(buf []byte) error // This should contain key value entries
	Flush() error
}

// Iterator is a common interface to iterate over  key-value entries in database
type Iterator interface {
	Close()
	Seek(key []byte)
	Next()
	ValidForPrefix(prefix []byte) bool
	GetNext() (*common.DBEntry, error)
}
