package tree

import (
	"sync"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	db "github.com/sarvalabs/go-moi/storage"
)

// persistentDB defines all methods that need to be implemented by persistent DB to handle tree data
type persistentDB interface {
	GetMerkleTreeEntry(id identifiers.Identifier, prefix db.PrefixTag, key []byte) ([]byte, error)
	SetMerkleTreeEntry(id identifiers.Identifier, prefix db.PrefixTag, key, value []byte) error
	SetMerkleTreeEntries(id identifiers.Identifier, prefix db.PrefixTag, entries map[string][]byte) error
	WritePreImages(id identifiers.Identifier, entries map[common.Hash][]byte) error
	GetPreImage(id identifiers.Identifier, hash common.Hash) ([]byte, error)
}

// TreeDB implements the DB interface
// all modified entries of trie will be stored in memory and flushed to persistent storage on calling the commit
type TreeDB struct {
	id        identifiers.Identifier
	dataType  db.PrefixTag
	mtx       sync.RWMutex
	db        persistentDB
	dirty     map[string][]byte
	treeCache *fastcache.Cache
	metrics   *Metrics
}

func NewTreeDB(id identifiers.Identifier, dataType db.PrefixTag,
	db persistentDB, treeCache *fastcache.Cache, metrics *Metrics,
) *TreeDB {
	return &TreeDB{
		id:        id,
		dataType:  dataType,
		db:        db,
		dirty:     make(map[string][]byte),
		treeCache: treeCache,
		metrics:   metrics,
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

	if val := tdb.treeCache.Get(nil, key); val != nil {
		tdb.metrics.AddTreeCacheHitCount(1)

		return val, nil
	}

	val, err := tdb.db.GetMerkleTreeEntry(tdb.id, tdb.dataType, key)
	if err != nil {
		return nil, err
	}

	tdb.treeCache.Set(key, val)
	tdb.metrics.AddTreeCacheMissCount(1)

	return val, nil
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

	return tdb.db.SetMerkleTreeEntries(tdb.id, tdb.dataType, tdb.dirty)
}

// WritePreImages writes all pre-images to persistent storage
func (tdb *TreeDB) WritePreImages(entries map[common.Hash][]byte) error {
	return tdb.db.WritePreImages(tdb.id, entries)
}

// GetPreImage returns the pre-image of the given hash
func (tdb *TreeDB) GetPreImage(hash common.Hash) ([]byte, error) {
	return tdb.db.GetPreImage(tdb.id, hash)
}

// IsDirty returns true if the treeDB has dirty entries/nodes
func (tdb *TreeDB) IsDirty() bool {
	tdb.mtx.RLock()
	defer tdb.mtx.RUnlock()

	return len(tdb.dirty) > 0
}

// Copy returns a copy of DB
func (tdb *TreeDB) Copy() DB {
	tdb.mtx.RLock()
	defer tdb.mtx.RUnlock()

	newTreeDB := &TreeDB{
		id:        tdb.id,
		dataType:  tdb.dataType,
		db:        tdb.db,
		dirty:     make(map[string][]byte, len(tdb.dirty)),
		treeCache: tdb.treeCache,
		metrics:   tdb.metrics,
	}

	for key, value := range tdb.dirty {
		v := make([]byte, len(value))

		copy(v, value)

		newTreeDB.dirty[key] = v
	}

	return newTreeDB
}
