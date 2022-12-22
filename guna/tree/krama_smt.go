package tree

import (
	"hash"
	"sync"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/munna0908/smt"
	"github.com/pkg/errors"
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
	Flush() error
	Copy() DB
}

// KramaHashTree wraps sparse merkle tree with key hashing and keeps track of newly added leaf nodes using a hash table
// RootNode of KramaHashTree consists of two hashes
// - Hash of the SMT root node
// - Hash of the hash table
// This hash table maintains the list of newly added leaf nodes
type KramaHashTree struct {
	root      *types.RootNode
	mtx       sync.RWMutex
	tree      *smt.SparseMerkleTree
	preImages map[types.Hash][]byte
	db        DB
}

func NewKramaHashTree(
	address types.Address,
	root types.Hash,
	db persistentDB,
	hasher hash.Hash,
	dataType dhruva.Prefix,
) (*KramaHashTree, error) {
	kht := &KramaHashTree{
		db: NewTreeDB(address, dataType, db),
		root: &types.RootNode{
			MerkleRoot: types.NilHash,
			HashTable:  make(map[string][]byte),
		},
		preImages: make(map[types.Hash][]byte),
	}

	if root != types.NilHash {
		rawData, err := kht.db.Get(root.Bytes())
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch root node from db")
		}

		if err = kht.root.FromBytes(rawData); err != nil {
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

// RootHash returns the hash of root node
func (kht *KramaHashTree) RootHash() (types.Hash, error) {
	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

	return kht.root.Hash()
}

// Get traversals the tree and returns the value of the key
func (kht *KramaHashTree) Get(key []byte) ([]byte, error) {
	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

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
	kht.mtx.Lock()
	defer kht.mtx.Unlock()

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

	kht.preImages[hashKey] = key
	kht.root.HashTable[string(key)] = value

	return nil
}

// Delete removes the key-value from tree and deletes the pre-images from cache
func (kht *KramaHashTree) Delete(key []byte) error {
	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	hashKey := kht.hashKey(key)

	if _, err := kht.tree.Delete(hashKey.Bytes()); err != nil {
		return err
	}

	delete(kht.preImages, hashKey)
	delete(kht.root.HashTable, string(key))

	return nil
}

// Commit will update the root node of KramaHashTree
func (kht *KramaHashTree) Commit() error {
	if !kht.IsDirty() {
		return nil
	}

	kht.mtx.Lock()
	defer kht.mtx.Unlock()

	kht.root.MerkleRoot = types.BytesToHash(kht.tree.Root())

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
	kht.mtx.RLock()
	defer kht.mtx.RUnlock()

	newSMT := &KramaHashTree{
		root:      &types.RootNode{MerkleRoot: kht.root.MerkleRoot, HashTable: kht.root.HashTable},
		db:        kht.db.Copy(),
		preImages: make(map[types.Hash][]byte, len(kht.preImages)),
	}

	newSMT.tree = smt.ImportSparseMerkleTree(newSMT.db, newSMT.db, blake256.New(), kht.root.MerkleRoot.Bytes())

	for k, v := range kht.preImages {
		newSMT.preImages[k] = v
	}

	return newSMT
}

// IsDirty returns true if the tree is modified
func (kht *KramaHashTree) IsDirty() bool {
	return kht.db.IsDirty()
}

func (kht *KramaHashTree) hashKey(key []byte) types.Hash {
	hasher := blake256.New()

	hasher.Write(key)
	sum := hasher.Sum(nil)
	hasher.Reset()

	return types.BytesToHash(sum)
}
