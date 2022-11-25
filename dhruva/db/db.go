package db

import "github.com/sarvalabs/moichain/types"

// DB defines a common interface implemented by all key-value database
type DB interface {
	Insert(key []byte, value []byte) error
	Update(key []byte, value []byte) error
	Delete(key []byte) error
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	NewIterator() (Iterator, error)
	CleanUp() error
	Close() error
	NewBatchWriter() BatchWriter
}

// BatchWriter is a common interface to write bulk entries
type BatchWriter interface {
	Set(key, value []byte) error
	Flush() error
}

// Iterator is a common interface to iterate over  key-value entries in database
type Iterator interface {
	Close()
	Seek(key []byte)
	Next()
	ValidForPrefix(prefix []byte) bool
	GetNext() (*types.DBEntry, error)
}
