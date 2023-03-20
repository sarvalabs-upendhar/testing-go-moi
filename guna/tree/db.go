package tree

import (
	"sync"

	db "github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/types"
)

// persistentDB defines all methods that need to be implemented by persistent DB to handle tree data
type persistentDB interface {
	GetMerkleTreeEntry(address types.Address, prefix db.Prefix, key []byte) ([]byte, error)
	SetMerkleTreeEntry(address types.Address, prefix db.Prefix, key, value []byte) error
	SetMerkleTreeEntries(address types.Address, prefix db.Prefix, entries map[string][]byte) error
	WritePreImages(address types.Address, entries map[types.Hash][]byte) error
	GetPreImage(address types.Address, hash types.Hash) ([]byte, error)
}

// TreeDB implements the DB interface
// all modified entries of trie will be stored in memory and flushed to persistent storage on calling the commit
type TreeDB struct {
	address  types.Address
	dataType db.Prefix
	mtx      sync.RWMutex
	db       persistentDB
	dirty    map[string][]byte
}

func NewTreeDB(address types.Address, dataType db.Prefix, db persistentDB) *TreeDB {
	return &TreeDB{
		address:  address,
		dataType: dataType,
		db:       db,
		dirty:    make(map[string][]byte),
	}
}

// Set adds the given key-value entry to dirty storage
func (tdb *TreeDB) Set(key, value []byte) error {
	tdb.mtx.Lock()
	defer tdb.mtx.Unlock()

	tdb.dirty[string(key)] = value

	return nil
}

// Get returns the value associated with the key from dirty storage
// If no entry is found in dirty storage, data will be fetched from persistent storage
func (tdb *TreeDB) Get(key []byte) ([]byte, error) {
	if tdb.dirty != nil {
		tdb.mtx.RLock()
		defer tdb.mtx.RUnlock()

		data, ok := tdb.dirty[string(key)]
		if ok {
			return data, nil
		}
	}

	return tdb.db.GetMerkleTreeEntry(tdb.address, tdb.dataType, key)
}

// Delete removes the give key from dirty storage
func (tdb *TreeDB) Delete(key []byte) error {
	tdb.mtx.Lock()
	defer tdb.mtx.Unlock()

	delete(tdb.dirty, string(key))

	return nil
}

// Flush writes all the modified entries to persistent storage
func (tdb *TreeDB) Flush() error {
	tdb.mtx.Lock()
	defer func() {
		tdb.mtx.Unlock()
		tdb.dirty = nil
	}()

	return tdb.db.SetMerkleTreeEntries(tdb.address, tdb.dataType, tdb.dirty)
}

// WritePreImages writes all pre-images to persistent storage
func (tdb *TreeDB) WritePreImages(entries map[types.Hash][]byte) error {
	return tdb.db.WritePreImages(tdb.address, entries)
}

// GetPreImage returns the pre-image of the given hash
func (tdb *TreeDB) GetPreImage(hash types.Hash) ([]byte, error) {
	return tdb.db.GetPreImage(tdb.address, hash)
}

// IsDirty returns true if the treeDB has dirty entries/nodes
func (tdb *TreeDB) IsDirty() bool {
	tdb.mtx.RLock()
	defer tdb.mtx.RUnlock()

	return tdb.dirty != nil && len(tdb.dirty) > 0
}

// Copy returns a copy of DB
func (tdb *TreeDB) Copy() DB {
	tdb.mtx.RLock()
	defer tdb.mtx.RUnlock()

	newTreeDB := &TreeDB{
		address:  tdb.address,
		dataType: tdb.dataType,
		db:       tdb.db,
		dirty:    make(map[string][]byte, len(tdb.dirty)),
	}

	for k, v := range tdb.dirty {
		newTreeDB.dirty[k] = v
	}

	return newTreeDB
}
