package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

// MemStorage is simple in-memory storage object that implements the Storage interface.
// It can be used for debugging functionality for all engine implementations
type MemStorage struct {
	address   types.Address
	balances  types.AssetMap
	storage   map[string]map[string][]byte
	approvals map[types.Address]types.AssetMap
}

// NewMemStorage generates a new MemStorage for a given Address
func NewMemStorage(addr types.Address) *MemStorage {
	return &MemStorage{
		address:   addr,
		balances:  make(types.AssetMap),
		storage:   make(map[string]map[string][]byte),
		approvals: make(map[types.Address]types.AssetMap),
	}
}

func (store MemStorage) EncodePOLO() ([]byte, error) {
	return polo.Polorize(store)
}

func (store *MemStorage) DecodePOLO(bytes []byte) error {
	return polo.Depolorize(store, bytes)
}

// Address returns the address of the MemStorage account
// Implements the Storage interface for MemStorage.
func (store MemStorage) Address() types.Address {
	return store.address
}

// GetStorageEntry retrieves some []byte data for a given key from the MemStorage.
// Returns error if there is no data for the key.
// Implements the Storage interface for MemStorage.
func (store MemStorage) GetStorageEntry(namespace string, key []byte) ([]byte, error) {
	tree, ok := store.storage[namespace]
	if !ok {
		return nil, errors.New("no such namespace")
	}

	val, ok := tree[string(key)]
	if !ok {
		return nil, errors.New("no data for key")
	}

	return val, nil
}

// SetStorageEntry inserts a key-value pair of data into the MemStorage.
// If data already exists for the key, it is overwritten.
// Implements the Storage interface for MemStorage.
func (store MemStorage) SetStorageEntry(namespace string, key, val []byte) error {
	tree, ok := store.storage[namespace]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[string(key)] = val

	return nil
}
