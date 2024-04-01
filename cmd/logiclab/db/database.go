package db

import "github.com/pkg/errors"

// Database defines a common interface implemented by all key-value database
type Database interface {
	Has([]byte) (bool, error)
	Get([]byte) ([]byte, error)
	Set([]byte, []byte) error
	Del([]byte) error

	PrefixDelete([]byte) error
	PrefixCollect([]byte) (map[string][]byte, error)
	DropAll() error

	Root() string
	Close() error
}

var (
	ErrPrefixOpFail = "database operation failed"
	ErrKeyNotFound  = errors.New("key not found")
)
