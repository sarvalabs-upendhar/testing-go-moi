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
	// Create a new Packer
	packer := polo.NewPacker()
	// Pack the address
	if err := packer.Pack(store.address); err != nil {
		return nil, err
	}
	// Pack the storage
	if err := packer.Pack(store.storage); err != nil {
		return nil, err
	}
	// Pack the balances
	if err := packer.Pack(store.balances); err != nil {
		return nil, err
	}
	// Pack the approvals
	if err := packer.Pack(store.approvals); err != nil {
		return nil, err
	}

	// Return packed bytes
	return packer.Bytes(), nil
}

func (store *MemStorage) DecodePOLO(bytes []byte) error {
	// Create a new Unpacker
	unpacker, err := polo.NewUnpacker(bytes)
	if err != nil {
		return err
	}
	// Unpack address
	if err = unpacker.Unpack(&store.address); err != nil {
		return err
	}
	// Unpack storage
	if err = unpacker.Unpack(&store.storage); err != nil {
		return err
	}
	// Unpack balances
	if err = unpacker.Unpack(&store.balances); err != nil {
		return err
	}
	// Unpack approvals
	if err = unpacker.Unpack(&store.approvals); err != nil {
		return err
	}

	return nil
}

// Address returns the address of the MemStorage account
// Implements the Storage interface for MemStorage.
func (store MemStorage) Address() types.Address {
	return store.address
}

// GetStorageEntry retrieves some []byte data for a given key from the MemStorage.
// Returns error if there is no data for the key.
// Implements the Storage interface for MemStorage.
func (store MemStorage) GetStorageEntry(logicID types.LogicID, key []byte) ([]byte, error) {
	tree, ok := store.storage[string(logicID)]
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
func (store *MemStorage) SetStorageEntry(logicID types.LogicID, key, val []byte) error {
	tree, ok := store.storage[string(logicID)]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[string(key)] = val
	store.storage[string(logicID)] = tree

	return nil
}
