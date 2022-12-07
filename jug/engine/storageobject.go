package engine

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// StorageObject is an interface that defines a storage driver for an ExecutionEngine
type StorageObject interface {
	Get([]byte) ([]byte, error)
	Set([]byte, []byte) error
}

// MemStore is simple in-memory storage object that implements the StorageObject interface
type MemStore map[string][]byte

// Get retrieves some []byte data for a given key from the MemStore.
// Returns error if there is no data for the key.
// Implements the StorageObject interface for MemStore.
func (store MemStore) Get(key []byte) ([]byte, error) {
	val, ok := store[string(key)]
	if !ok {
		return nil, errors.New("key not found")
	}

	return val, nil
}

// Set inserts a key-value pair of data into the MemStore.
// If data already exists for the key, it is overwritten.
// Implements the StorageObject interface for MemStore.
func (store MemStore) Set(key, val []byte) error {
	store[string(key)] = val

	return nil
}

// String implements the Stringer interface for MemStore
func (store MemStore) String() string {
	var (
		idx int
		str strings.Builder
	)

	str.WriteString("memstore[")

	for k, v := range store {
		str.WriteString(fmt.Sprintf("0x%x:%v", k, v))

		idx++
		if idx != len(store) {
			str.WriteString(" ")
		}
	}

	str.WriteString("]")

	return str.String()
}
