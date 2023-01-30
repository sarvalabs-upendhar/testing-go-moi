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
	// Create a new Polorizer
	polorizer := polo.NewPolorizer()
	// Polorize the address
	if err := polorizer.Polorize(store.address); err != nil {
		return nil, err
	}
	// Polorize the storage
	if err := polorizer.Polorize(store.storage); err != nil {
		return nil, err
	}
	// Polorize the balances
	if err := polorizer.Polorize(store.balances); err != nil {
		return nil, err
	}
	// Polorize the approvals
	if err := polorizer.Polorize(store.approvals); err != nil {
		return nil, err
	}

	// Return packed bytes
	return polorizer.Bytes(), nil
}

func (store *MemStorage) DecodePOLO(bytes []byte) error {
	// Create a new Depolorizer
	depolorizer, err := polo.NewDepolorizer(bytes)
	if err != nil {
		return err
	}
	// Depolorize address
	if err = depolorizer.Depolorize(&store.address); err != nil {
		return err
	}
	// Depolorize storage
	if err = depolorizer.Depolorize(&store.storage); err != nil {
		return err
	}
	// Depolorize balances
	if err = depolorizer.Depolorize(&store.balances); err != nil {
		return err
	}
	// Depolorize approvals
	if err = depolorizer.Depolorize(&store.approvals); err != nil {
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

func (store *MemStorage) Copy() *MemStorage {
	copied := &MemStorage{
		address:   store.address,
		balances:  store.balances.Copy(),
		storage:   make(map[string]map[string][]byte, len(store.storage)),
		approvals: make(map[types.Address]types.AssetMap, len(store.approvals)),
	}

	for owner, assets := range store.approvals {
		copied.approvals[owner] = assets.Copy()
	}

	for namespace, storage := range store.storage {
		copied.storage[namespace] = make(map[string][]byte, len(storage))
		for key, val := range storage {
			copied.storage[namespace][key] = val
		}
	}

	return copied
}
