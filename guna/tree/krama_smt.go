package tree

import (
	"hash"
	"sync"

	"github.com/munna0908/smt"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/types"
)

// DB wraps access to trie data
type DB interface {
	WritePreImages(map[types.Hash][]byte) error
	GetPreImage(hash types.Hash) ([]byte, error)
	Get(key []byte) ([]byte, error) // Get gets the value for a key.
	Set(key, value []byte) error    // Set updates the value for a key.
	Delete(key []byte) error        // Delete deletes a key.
	IsDirty() bool
	Commit() error
	Copy() DB
}

// KramaHashTree wraps sparse merkle tree with key hashing and keeps track of newly added leaf nodes using a hash table
// RootNode of KramaHashTree consists of two hashes
// - Hash of the SMT root node
// - Hash of the hash table
// This hash table maintains the list of newly added leaf nodes
type KramaHashTree struct {
	root       *rootNode
	mtx        sync.RWMutex
	tree       *smt.SparseMerkleTree
	preImages  map[types.Hash][]byte
	deltaNodes map[string][]byte
	db         DB
	hasher     hash.Hash
}

func NewKramaHashTree(
	address types.Address,
	root types.Hash,
	db persistentDB,
	hasher hash.Hash,
) (*KramaHashTree, error) {
	kht := &KramaHashTree{
		db:     NewTreeDB(address, dhruva.Storage, db),
		hasher: hasher,
		root: &rootNode{
			MerkleRoot: types.NilHash,
			HashTable:  types.NilHash,
		},
		preImages:  make(map[types.Hash][]byte),
		deltaNodes: make(map[string][]byte),
	}

	if root != types.NilHash {
		rawData, err := kht.db.Get(root.Bytes())
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch root node from db")
		}

		if err := polo.Depolorize(kht.root, rawData); err != nil {
			return nil, errors.Wrap(err, "failed to depolarise root node")
		}
	}

	if kht.root.MerkleRoot == types.NilHash {
		kht.tree = smt.NewSparseMerkleTree(kht.db, kht.db, hasher)
	} else {
		kht.tree = smt.ImportSparseMerkleTree(kht.db, kht.db, hasher, kht.root.MerkleRoot.Bytes())
	}

	return kht, nil
}

// Root returns the hash of root node
func (kht *KramaHashTree) Root() (types.Hash, error) {
	return kht.root.Hash()
}

// Get traversals the tree and returns the value of the key
func (kht *KramaHashTree) Get(key []byte) ([]byte, error) {
	value, err := kht.tree.GetDescend(kht.hashKey(key).Bytes())
	if err != nil {
		return nil, err
	}

	if len(value) == 0 {
		return nil, types.ErrKeyNotFound
	}

	return value, nil
}

// Set adds the give key-value to the merkle tree and stores the preimage
func (kht *KramaHashTree) Set(key, value []byte) error {
	if len(key) == 0 {
		return types.ErrInvalidKey
	}

	if len(value) == 0 {
		return types.ErrInvalidValue
	}

	hashKey := kht.hashKey(key)

	if _, err := kht.tree.Update(hashKey.Bytes(), value); err != nil {
		return err
	}

	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	kht.preImages[hashKey] = key
	kht.deltaNodes[string(key)] = value

	return nil
}

// Delete removes the key-value from tree and deletes the pre-images from cache
func (kht *KramaHashTree) Delete(key []byte) error {
	hashKey := kht.hashKey(key)

	if _, err := kht.tree.Delete(hashKey.Bytes()); err != nil {
		return err
	}

	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	delete(kht.preImages, hashKey)
	delete(kht.deltaNodes, string(key))

	return nil
}

// Commit will update the root node of KramaHashTree
func (kht *KramaHashTree) Commit() error {
	if !kht.IsDirty() {
		return nil
	}

	deltaInfo, err := polo.Polorize(kht.deltaNodes)
	if err != nil {
		return errors.Wrap(err, "failed to polorize delta nodes")
	}

	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	kht.root = &rootNode{
		HashTable:  types.GetHash(deltaInfo),
		MerkleRoot: types.BytesToHash(kht.tree.Root()),
	}

	if kht.root.HashTable != types.NilHash {
		if err := kht.db.Set(kht.root.HashTable.Bytes(), deltaInfo); err != nil {
			return errors.Wrap(err, "failed to write delta info to db")
		}
	}

	rootHash, err := kht.root.Hash()
	if err != nil {
		return err
	}

	rawData, err := kht.root.Bytes()
	if err != nil {
		return err
	}

	return kht.db.Set(rootHash.Bytes(), rawData)
}

// Flush commits the merkle tree changes and writes the preimages to db
func (kht *KramaHashTree) Flush() error {
	if !kht.IsDirty() {
		return nil
	}

	// flush the tree nodes
	if err := kht.db.Commit(); err != nil {
		return err
	}

	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	// flush the preimage keys
	return kht.db.WritePreImages(kht.preImages)
}

// NewIterator returns a tree iterator
func (kht *KramaHashTree) NewIterator() smt.Iterator {
	return kht.tree.NewIterator()
}

// GetPreImageKey returns the preimage of the hashed key
func (kht *KramaHashTree) GetPreImageKey(hashKey types.Hash) ([]byte, error) {
	kht.mtx.RLock()
	if value, ok := kht.preImages[hashKey]; ok {
		return value, nil
	}
	kht.mtx.RUnlock()

	return kht.db.GetPreImage(hashKey)
}

// Copy returns the copy of krama hash tree
func (kht *KramaHashTree) Copy() MerkleTree {
	newSMT := &KramaHashTree{
		root:       &rootNode{MerkleRoot: kht.root.MerkleRoot, HashTable: kht.root.HashTable},
		hasher:     kht.hasher,
		db:         kht.db.Copy(),
		preImages:  make(map[types.Hash][]byte, len(kht.preImages)),
		deltaNodes: make(map[string][]byte, len(kht.deltaNodes)),
	}

	newSMT.tree = smt.ImportSparseMerkleTree(newSMT.db, newSMT.db, newSMT.hasher, kht.root.MerkleRoot.Bytes())

	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

	for k, v := range kht.preImages {
		newSMT.preImages[k] = v
	}

	for k, v := range kht.deltaNodes {
		newSMT.deltaNodes[k] = v
	}

	return newSMT
}

// IsDirty returns true if the tree is modified
func (kht *KramaHashTree) IsDirty() bool {
	return kht.db.IsDirty()
}

func (kht *KramaHashTree) hashKey(key []byte) types.Hash {
	kht.hasher.Write(key)
	sum := kht.hasher.Sum(nil)
	kht.hasher.Reset()

	return types.BytesToHash(sum)
}

func FetchDeltaNodes(rawData []byte) (map[string][]byte, error) {
	var entries map[string][]byte

	if err := polo.Depolorize(&entries, rawData); err != nil {
		return nil, err
	}

	return entries, nil
}
