package tree

import (
	"encoding/hex"
	"hash"
	"sync"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/munna0908/smt"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/storage"
)

// DB wraps access to trie data
type DB interface {
	WritePreImages(map[common.Hash][]byte) error
	GetPreImage(hash common.Hash) ([]byte, error)
	Get(key []byte) ([]byte, error) // Get gets the value for a key.
	Set(key, value []byte) error    // Set updates the value for a key.
	Delete(key []byte) error        // Delete deletes a key.
	IsDirty() bool
	Flush() error
	Copy() DB
}

// KramaHashTree wraps sparse merkle tree with key hashing and keeps track of newly added leaf nodes using a hash table
// RootNode of KramaHashTree consists of two hashes
// - Hash of the SMT root node
// - Hash of the hash table
// This hash table maintains the list of newly added leaf nodes
type KramaHashTree struct {
	root      *common.RootNode
	mtx       sync.RWMutex
	tree      *smt.SparseMerkleTree
	preImages map[common.Hash][]byte
	db        DB
}

func NewKramaHashTree(
	address common.Address,
	root common.Hash,
	db persistentDB,
	hasher hash.Hash,
	dataType storage.PrefixTag,
) (*KramaHashTree, error) {
	kht := &KramaHashTree{
		db: NewTreeDB(address, dataType, db),
		root: &common.RootNode{
			MerkleRoot: common.NilHash,
			HashTable:  make(map[string][]byte),
		},
		preImages: make(map[common.Hash][]byte),
	}

	if root != common.NilHash {
		rawData, err := kht.db.Get(root.Bytes())
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch root node from db")
		}

		if err = kht.root.FromBytes(rawData); err != nil {
			return nil, errors.Wrap(err, "failed to depolarise root node")
		}
	}

	if kht.root.MerkleRoot == common.NilHash {
		kht.tree = smt.NewSparseMerkleTree(kht.db, kht.db, hasher)
	} else {
		kht.tree = smt.ImportSparseMerkleTree(kht.db, kht.db, hasher, kht.root.MerkleRoot.Bytes())
	}

	return kht, nil
}

func (kht *KramaHashTree) Root() common.RootNode {
	return *kht.root
}

// RootHash returns the hash of root node
func (kht *KramaHashTree) RootHash() (common.Hash, error) {
	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

	return kht.root.Hash()
}

// Get traversals the tree and returns the value of the key
func (kht *KramaHashTree) Get(key []byte) ([]byte, error) {
	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

	value, err := kht.tree.GetDescend(HashKey(key).Bytes())
	if err != nil {
		return nil, err
	}

	if len(value) == 0 {
		return nil, common.ErrKeyNotFound
	}

	return value, nil
}

// Set adds the give key-value to the merkle tree and stores the preimage
func (kht *KramaHashTree) Set(key, value []byte) error {
	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	if len(key) == 0 {
		return common.ErrInvalidKey
	}

	if len(value) == 0 {
		return common.ErrInvalidValue
	}

	hashKey := HashKey(key)

	if _, err := kht.tree.Update(hashKey.Bytes(), value); err != nil {
		return err
	}

	kht.preImages[hashKey] = key

	kht.root.HashTable[hex.EncodeToString(key)] = value

	return nil
}

// Delete removes the key-value from tree and deletes the pre-images from cache
func (kht *KramaHashTree) Delete(key []byte) error {
	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	hashKey := HashKey(key)

	if _, err := kht.tree.Delete(hashKey.Bytes()); err != nil {
		return err
	}

	delete(kht.preImages, hashKey)
	delete(kht.root.HashTable, hex.EncodeToString(key))

	return nil
}

// Commit will update the root node of KramaHashTree
func (kht *KramaHashTree) Commit() error {
	if !kht.IsDirty() {
		return nil
	}

	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	kht.root.MerkleRoot = common.BytesToHash(kht.tree.Root())

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
	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	if !kht.IsDirty() {
		return nil
	}

	// flush the tree nodes
	if err := kht.db.Flush(); err != nil {
		return err
	}

	// flush the preimage keys
	return kht.db.WritePreImages(kht.preImages)
}

// NewIterator returns a tree iterator
func (kht *KramaHashTree) NewIterator() smt.Iterator {
	return kht.tree.NewIterator()
}

// GetPreImageKey returns the preimage of the hashed key
func (kht *KramaHashTree) GetPreImageKey(hashKey common.Hash) ([]byte, error) {
	kht.mtx.RLock()
	if value, ok := kht.preImages[hashKey]; ok {
		return value, nil
	}
	kht.mtx.RUnlock()

	return kht.db.GetPreImage(hashKey)
}

// Copy returns the copy of krama hash tree
func (kht *KramaHashTree) Copy() MerkleTree {
	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

	newSMT := &KramaHashTree{
		root:      &common.RootNode{MerkleRoot: kht.root.MerkleRoot, HashTable: make(map[string][]byte)},
		db:        kht.db.Copy(),
		preImages: make(map[common.Hash][]byte, len(kht.preImages)),
	}

	newSMT.tree = smt.ImportSparseMerkleTree(newSMT.db, newSMT.db, blake256.New(), kht.root.MerkleRoot.Bytes())

	for key, value := range kht.root.HashTable {
		v := make([]byte, len(value))

		copy(v, value)

		newSMT.root.HashTable[key] = v
	}

	for key, value := range kht.preImages {
		v := make([]byte, len(value))

		copy(v, value)

		newSMT.preImages[key] = v
	}

	return newSMT
}

// IsDirty returns true if the tree is modified
func (kht *KramaHashTree) IsDirty() bool {
	return kht.db.IsDirty()
}

func HashKey(key []byte) common.Hash {
	hasher := blake256.New()

	hasher.Write(key)
	sum := hasher.Sum(nil)
	hasher.Reset()

	return common.BytesToHash(sum)
}
