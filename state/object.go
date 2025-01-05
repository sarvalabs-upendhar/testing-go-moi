package state

import (
	"encoding/hex"
	"math/big"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/decred/dcrd/crypto/blake256"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
)

// StorageTree: Holds key-value pairs associated with logic (identified by logicID) for storage.
// LogicTree: Manages logicObject for each logic.
// MetaStorageTree: Keeps track of the root of the storage tree for each logic.

type Object struct {
	cache     *lru.Cache
	treeCache *fastcache.Cache

	address   identifiers.Address
	accType   common.AccountType
	data      common.Account
	isGenesis bool // used by transition objects in execution

	db Store

	deeds *Deeds

	assetTree       tree.MerkleTree
	logicTree       tree.MerkleTree
	metaStorageTree tree.MerkleTree
	storageTrees    map[identifiers.LogicID]tree.MerkleTree
	fileTree        tree.MerkleTree //nolint:unused

	dirtyEntries Storage
	receipts     common.Receipts

	storageTreeTxns map[identifiers.LogicID]*iradix.Txn
	assetTreeTxn    *iradix.Txn
	logicTreeTxn    *iradix.Txn

	files   map[common.Hash][]byte
	context *ContextObject
	metrics *Metrics
}

func NewStateObject(
	id identifiers.Address,
	cache *lru.Cache,
	treeCache *fastcache.Cache,
	db Store,
	account common.Account,
	metrics *Metrics,
	isGenesis bool,
) *Object {
	return &Object{
		accType:         account.AccType,
		cache:           cache,
		treeCache:       treeCache,
		db:              db,
		data:            account,
		address:         id,
		deeds:           nil,
		files:           make(map[common.Hash][]byte),
		dirtyEntries:    make(Storage),
		receipts:        make(common.Receipts),
		storageTreeTxns: make(map[identifiers.LogicID]*iradix.Txn),
		storageTrees:    make(map[identifiers.LogicID]tree.MerkleTree),
		metrics:         metrics,
		isGenesis:       isGenesis,
	}
}

// Address returns the participant's address associated with the object.
func (object *Object) Address() identifiers.Address {
	return object.address
}

// IsGenesis indicates whether the object is a genesis object.
func (object *Object) IsGenesis() bool {
	return object.isGenesis
}

// Data returns the account info associated with the object.
func (object *Object) Data() *common.Account {
	return &object.data
}

// AccountType returns the type of the account.
func (object *Object) AccountType() common.AccountType {
	return object.accType
}

// AccountState returns the current state of the account.
func (object *Object) AccountState() common.Account {
	return object.data
}

// Deeds returns the deeds associated. If the deeds is not already loaded,
// it attempts to load it from the store. An error is returned if loading fails.
func (object *Object) Deeds() (*Deeds, error) {
	if object.deeds == nil {
		if err := object.loadDeeds(); err != nil {
			return nil, errors.Wrap(err, "failed to load deeds")
		}
	}

	return object.deeds, nil
}

// CreateDeedsEntry creates a new entry in the deeds with the specified key and value.
// If an entry with the same key already exists, an error is returned.
func (object *Object) CreateDeedsEntry(key string) error {
	deeds, err := object.Deeds()
	if err != nil {
		return err
	}

	if _, ok := deeds.Entries[key]; ok {
		return common.ErrAssetAlreadyRegistered
	}

	deeds.Entries[key] = struct{}{}

	return nil
}

// updateAssetTree ensures the asset transaction tree is initialized and inserts the given asset object.
func (object *Object) updateAssetTree(assetID identifiers.AssetID, assetObject *AssetObject) {
	// Initialize assetTreeTxn if not already done
	if object.assetTreeTxn == nil {
		object.assetTreeTxn = iradix.New().Txn()
	}

	// Insert the updated asset object into the txn tree
	object.assetTreeTxn.Insert(assetID.Bytes(), assetObject)
}

// updateLogicTree ensures the logic transaction tree is initialized and inserts the given logic object.
func (object *Object) updateLogicTree(logicID identifiers.LogicID, logicObject *LogicObject) {
	// Initialize logicTreeTxn if not already done
	if object.logicTreeTxn == nil {
		object.logicTreeTxn = iradix.New().Txn()
	}

	// Insert the updated logic object into the txn tree
	object.logicTreeTxn.Insert(logicID.Bytes(), logicObject)
}

// Balances retrieves and returns the balances of all the assets held by the participant.
func (object *Object) Balances() (map[identifiers.AssetID]*big.Int, error) {
	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	it := assetTree.NewIterator()
	balances := make(map[identifiers.AssetID]*big.Int)

	for it.Next() {
		if it.Leaf() {
			key, err := assetTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			assetID := identifiers.AssetID(hex.EncodeToString(key))

			assetObject, err := object.getAssetObject(assetID, false)
			if err != nil {
				return nil, err
			}

			balances[assetID] = assetObject.Balance
		}
	}

	return balances, nil
}

// BalanceOf returns the balance of a specific asset, identified by its asset id.
func (object *Object) BalanceOf(id identifiers.AssetID) (*big.Int, error) {
	assetObject, err := object.getAssetObject(id, false)
	if err != nil {
		return big.NewInt(0), common.ErrAssetNotFound
	}

	return assetObject.Balance, nil
}

// AddBalance increments the balance of the specified asset based on the given amount.
func (object *Object) AddBalance(assetID identifiers.AssetID, amount *big.Int) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	assetObject.Balance.Add(assetObject.Balance, amount)

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// SubBalance decrements the balance of the specified asset based the given amount.
func (object *Object) SubBalance(assetID identifiers.AssetID, amount *big.Int) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	assetObject.Balance.Sub(assetObject.Balance, amount)

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// CreateLockup transfers a specified amount from the participant's asset balance into a lockup
// associated with a specified address. The lockup represents funds reserved for a specific purpose,
// reducing the available balance for the participant.
func (object *Object) CreateLockup(assetID identifiers.AssetID, address identifiers.Address, amount *big.Int) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// Deduct the amount from asset balance
	assetObject.Balance.Sub(assetObject.Balance, amount)

	// Create a new lockup for the specified address
	assetObject.Lockup[address] = amount

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// ReleaseLockup reduces the lockup amount from a specified address for the given asset.
// If the lockup amount becomes zero, the lockup entry is deleted from the asset object.
func (object *Object) ReleaseLockup(assetID identifiers.AssetID, address identifiers.Address, amount *big.Int) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	lockupAmount, ok := assetObject.Lockup[address]
	if !ok {
		return common.ErrLockupNotFound
	}

	lockupAmount.Sub(lockupAmount, amount)

	if lockupAmount.Cmp(big.NewInt(0)) == 0 {
		delete(assetObject.Lockup, address)
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// Lockups retrieves all active lockups across all assets in the AssetTree.
// It iterates through the AssetTree, collects lockup information, and returns it as a slice.
func (object *Object) Lockups() ([]common.AssetMandateOrLockup, error) {
	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	it := assetTree.NewIterator()
	lockups := make([]common.AssetMandateOrLockup, 0)

	for it.Next() {
		if it.Leaf() {
			key, err := assetTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			assetID := identifiers.AssetID(hex.EncodeToString(key))

			assetObject, err := object.getAssetObject(assetID, false)
			if err != nil {
				return nil, err
			}

			for address, amount := range assetObject.Lockup {
				lockups = append(lockups, common.AssetMandateOrLockup{
					AssetID: assetID,
					Address: address,
					Amount:  amount,
				})
			}
		}
	}

	return lockups, nil
}

// GetLockup retrieves the lockup amount for the given logic and asset id.
func (object *Object) GetLockup(assetID identifiers.AssetID, address identifiers.Address) (*big.Int, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	if amount, ok := assetObject.Lockup[address]; ok {
		return amount, nil
	}

	return nil, common.ErrLockupNotFound
}

// CreateMandate assigns a spending mandate to an address for the specified asset.
// The mandate grants the recipient the authorization to spend the specified amount on behalf of the participant.
func (object *Object) CreateMandate(
	assetID identifiers.AssetID,
	address identifiers.Address,
	amount *big.Int,
	expiresAt uint64,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if mandate, ok := assetObject.Mandate[address]; ok {
		// Increment the mandate amount if the mandate already exist
		assetObject.Mandate[address] = &Mandate{
			Amount:    mandate.Amount.Add(mandate.Amount, amount),
			ExpiresAt: expiresAt,
		}
	} else {
		// Create a new mandate if it doesn't exist
		assetObject.Mandate[address] = &Mandate{
			Amount:    amount,
			ExpiresAt: expiresAt,
		}
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// SubMandateBalance decrements the mandate balance of the specified asset by the given amount
// for the specified address.
func (object *Object) SubMandateBalance(
	assetID identifiers.AssetID, address identifiers.Address, amount *big.Int,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	mandate, ok := assetObject.Mandate[address]
	if !ok {
		return common.ErrMandateNotFound
	}

	// Decrement the mandate balance by the given amount
	mandate.Amount = mandate.Amount.Sub(mandate.Amount, amount)

	// If the mandate amount is zero, remove the mandate for the specified address.
	if mandate.Amount.Cmp(big.NewInt(0)) == 0 {
		delete(assetObject.Mandate, address)
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// ConsumeMandate decrements the benefactor's mandate balance and asset balance
func (object *Object) ConsumeMandate(assetID identifiers.AssetID, address identifiers.Address, amount *big.Int) error {
	// Deduct the mandate amount from the sender's mandate balance
	if err := object.SubMandateBalance(assetID, address, amount); err != nil {
		return err
	}

	// Deduct the transfer amount from the sender's asset balance
	if err := object.SubBalance(assetID, amount); err != nil {
		return err
	}

	return nil
}

// DeleteMandate revokes a granted spending authorization from a specified address for the given asset id.
func (object *Object) DeleteMandate(assetID identifiers.AssetID, address identifiers.Address) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	delete(assetObject.Mandate, address)

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// Mandates retrieves and returns all the asset mandates with their corresponding asset IDs, addresses, and amounts.
func (object *Object) Mandates() ([]common.AssetMandateOrLockup, error) {
	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	it := assetTree.NewIterator()
	mandates := make([]common.AssetMandateOrLockup, 0)

	for it.Next() {
		if it.Leaf() {
			key, err := assetTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			assetID := identifiers.AssetID(hex.EncodeToString(key))

			assetObject, err := object.getAssetObject(assetID, false)
			if err != nil {
				return nil, err
			}

			for address, mandate := range assetObject.Mandate {
				mandates = append(mandates, common.AssetMandateOrLockup{
					AssetID: assetID,
					Address: address,
					Amount:  mandate.Amount,
				})
			}
		}
	}

	return mandates, nil
}

// GetMandate retrieves the mandate amount for the given address and asset id.
func (object *Object) GetMandate(assetID identifiers.AssetID, address identifiers.Address) (*Mandate, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	if mandate, ok := assetObject.Mandate[address]; ok {
		return mandate, nil
	}

	return nil, common.ErrMandateNotFound
}

// GetState retrieves the current properties of the specified asset,
// such as its symbol and supply details.
func (object *Object) GetState(assetID identifiers.AssetID) (*common.AssetDescriptor, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	return assetObject.Properties, nil
}

// SetState updates the properties of the specified asset, including details
// like its symbol or supply. This modifies the asset's metadata.
func (object *Object) SetState(assetID identifiers.AssetID, properties *common.AssetDescriptor) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	assetObject.Properties = properties

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// Copy creates and returns a new object that replicates the state and all associated data of the original state object.
func (object *Object) Copy() *Object {
	sObj := NewStateObject(object.address, object.cache, object.treeCache, object.db,
		object.data, object.metrics, object.isGenesis)

	sObj.dirtyEntries = object.dirtyEntries.Copy()

	if object.assetTreeTxn != nil {
		sObj.assetTreeTxn = object.assetTreeTxn.CommitOnly().Txn()
	}

	if object.assetTree != nil {
		sObj.assetTree = object.assetTree.Copy()
	}

	if object.deeds != nil {
		sObj.deeds = object.deeds.Copy()
	}

	for logicID, sTree := range object.storageTreeTxns {
		if sTree != nil {
			sObj.storageTreeTxns[logicID] = sTree.CommitOnly().Txn()
		}
	}

	if object.logicTreeTxn != nil {
		sObj.logicTreeTxn = object.logicTreeTxn.CommitOnly().Txn()
	}

	if object.logicTree != nil {
		sObj.logicTree = object.logicTree.Copy()
	}

	for id, sTree := range object.storageTrees {
		sObj.storageTrees[id] = sTree.Copy()
	}

	if object.metaStorageTree != nil {
		sObj.metaStorageTree = object.metaStorageTree.Copy()
	}

	for key, value := range object.files {
		v := make([]byte, len(value))

		copy(v, value)

		sObj.files[key] = v
	}

	return sObj
}

// SetDirtyEntry marks a specific key-value pair as dirty in the object’s dirty entries.
func (object *Object) SetDirtyEntry(key string, value []byte) {
	object.dirtyEntries[key] = value
}

// GetDirtyEntry retrieves the value associated with a given key from the dirty entries.
// Returns an error if the key is not found.
func (object *Object) GetDirtyEntry(key string) ([]byte, error) {
	val, ok := object.dirtyEntries[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

// IncrementNonce increases the nonce value by the specified count.
func (object *Object) IncrementNonce(count uint64) {
	object.data.Nonce += count
}

// Commit finalizes all changes to the object by committing the deeds, assets, logics, and
// storage trees to the database.
func (object *Object) Commit() (common.Hash, error) {
	if _, err := object.commitDeeds(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit deeds ")
	}

	if _, err := object.commitAssets(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit asset tree")
	}

	if _, err := object.commitLogics(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	if _, err := object.commitStorage(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit storage tree")
	}

	accCid, err := object.commitAccount()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit account")
	}

	return accCid, nil
}

// commitDeeds stores the current deeds to the dirty entries.
func (object *Object) commitDeeds() (common.Hash, error) {
	if object.deeds == nil || len(object.deeds.Entries) == 0 {
		return common.NilHash, nil
	}

	data, err := object.deeds.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)

	object.SetDirtyEntry(
		common.BytesToHex(storage.DeedsKey(object.address, hash)),
		data,
	)

	object.data.AssetDeeds = hash

	return hash, nil
}

// commitAccount stores the current account data to the dirty entries and returns its hash.
func (object *Object) commitAccount() (common.Hash, error) {
	data, err := object.data.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)
	key := common.BytesToHex(storage.AccountKey(object.address, hash))

	object.SetDirtyEntry(key, data)

	return hash, nil
}

// commitContextObject serializes and stores the Context object to the dirty entries.
func (object *Object) commitContextObject(obj Context) (common.Hash, error) {
	// Add type checks here
	rawData, err := obj.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(rawData)
	key := common.BytesToHex(storage.ContextObjectKey(object.address, hash))

	object.SetDirtyEntry(key, rawData)

	return hash, nil
}

// commitActiveStorageTrees commits all active storage trees and updates the meta storage tree with their root hashes.
func (object *Object) commitActiveStorageTrees() error {
	for logicID, txn := range object.storageTreeTxns {
		sTree, err := object.GetStorageTree(logicID)
		if err != nil {
			return err
		}

		txn.Root().Walk(func(k []byte, v interface{}) bool {
			if err = sTree.Set(k, v.([]byte)); err != nil { //nolint
				return true
			}

			return false
		})

		if err = sTree.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit storage tree")
		}

		rootHash, err := sTree.RootHash()
		if err != nil {
			return err
		}

		// Add the updated logic-id <=> storage-root in master storage merkleTree
		if err = object.metaStorageTree.Set(logicID.Bytes(), rootHash.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

// commitMetaStorageTree commits the meta storage tree transactions.
func (object *Object) commitMetaStorageTree() (common.Hash, error) {
	if object.metaStorageTree == nil || !object.metaStorageTree.IsDirty() {
		return object.data.StorageRoot, nil
	}

	if err := object.metaStorageTree.Commit(); err != nil {
		return common.NilHash, err
	}

	rootHash, err := object.metaStorageTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	object.data.StorageRoot = rootHash

	return rootHash, nil
}

// commitStorage commits active storage tree and the meta storage tree transactions.
func (object *Object) commitStorage() (common.Hash, error) {
	err := object.commitActiveStorageTrees()
	if err != nil {
		return common.NilHash, err
	}

	return object.commitMetaStorageTree()
}

// commitAssets commits the asset tree transactions.
func (object *Object) commitAssets() (common.Hash, error) {
	if object.assetTreeTxn == nil {
		return object.data.AssetRoot, nil
	}

	assetTree, err := object.getAssetTree()
	if err != nil {
		return common.NilHash, err
	}

	objects := make(map[identifiers.AssetID]*AssetObject)

	object.assetTreeTxn.Root().Walk(func(k []byte, v interface{}) bool {
		obj, _ := v.(*AssetObject)
		assetID := identifiers.AssetID(hex.EncodeToString(k))
		objects[assetID] = obj

		return false
	})

	for assetID, obj := range objects {
		rawData, err := obj.Bytes()
		if err != nil {
			return common.NilHash, err
		}

		err = assetTree.Set(assetID.Bytes(), rawData)
		if err != nil {
			return common.NilHash, err
		}
	}

	err = assetTree.Commit()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit asset tree")
	}

	object.data.AssetRoot, err = assetTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	return object.data.AssetRoot, nil
}

// commitLogics commits the logic tree transactions
func (object *Object) commitLogics() (common.Hash, error) {
	if object.logicTreeTxn == nil {
		return object.data.LogicRoot, nil
	}

	logicTree, err := object.getLogicTree()
	if err != nil {
		return common.NilHash, err
	}

	objects := make([]*LogicObject, 0)

	object.logicTreeTxn.Root().Walk(func(k []byte, v interface{}) bool {
		obj, _ := v.(*LogicObject)
		objects = append(objects, obj)

		return false
	})

	for _, obj := range objects {
		rawBytes, err := obj.Bytes()
		if err != nil {
			return common.NilHash, err
		}

		if err = logicTree.Set(obj.ID.Bytes(), rawBytes); err != nil {
			return common.NilHash, err
		}
	}

	err = logicTree.Commit()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	object.data.LogicRoot, err = object.logicTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	return object.data.LogicRoot, nil
}

// flush will write all dirty entries to the database
func (object *Object) flush() error {
	if err := object.flushAssetTree(); err != nil {
		return errors.Wrap(err, "failed to flush asset tree")
	}

	if err := object.flushLogicTree(); err != nil {
		return errors.Wrap(err, "failed to flush logic tree")
	}

	if err := object.flushStorageTrees(); err != nil {
		return errors.Wrap(err, "failed to flush active storage trees")
	}

	for k, v := range object.GetDirtyStorage() {
		if err := object.db.CreateEntry(common.FromHex(k), v); err != nil {
			return errors.Wrap(err, "failed to write dirty entries")
		}
	}

	return nil
}

// Flushes the asset tree if it exists.
func (object *Object) flushAssetTree() error {
	if object.assetTree == nil {
		return nil
	}

	return object.assetTree.Flush()
}

// Flushes the logic tree if it exists.
func (object *Object) flushLogicTree() error {
	if object.logicTree == nil {
		return nil
	}

	return object.logicTree.Flush()
}

// Flushes all storage trees, including the master storage tree.
func (object *Object) flushStorageTrees() error {
	if object.metaStorageTree == nil {
		return nil
	}
	// flush active storage trees
	for _, storageTree := range object.storageTrees {
		if err := storageTree.Flush(); err != nil {
			return errors.Wrap(err, "failed to commit modified storage tree entries to store")
		}
	}
	// flush master storage trees
	return object.metaStorageTree.Flush()
}

// CreateStorageTreeForLogic creates a storage tree for the given logic ID.
func (object *Object) CreateStorageTreeForLogic(logicID identifiers.LogicID) error {
	_, err := object.createStorageTreeForLogic(logicID)

	return err
}

// CreateAsset creates an asset and returns its asset ID.
func (object *Object) CreateAsset(
	addr identifiers.Address,
	descriptor *common.AssetDescriptor,
) (identifiers.AssetID, error) {
	assetID := identifiers.NewAssetIDv0(
		descriptor.IsLogical,
		descriptor.IsStateFul,
		descriptor.Dimension,
		uint16(descriptor.Standard),
		addr,
	)

	assetObject := NewAssetObject(big.NewInt(0), descriptor)

	if err := object.InsertNewAssetObject(assetID, assetObject); err != nil {
		return "", err
	}

	return assetID, nil
}

// MintAsset increases the supply of the specified asset by the given amount.
func (object *Object) MintAsset(assetID identifiers.AssetID, amount *big.Int) (big.Int, error) {
	assetObject, err := object.getAssetObject(assetID, false)
	if err != nil {
		return *big.NewInt(0), common.ErrAssetNotFound
	}

	assetObject.Properties.Supply.Add(assetObject.Properties.Supply, amount)

	return *assetObject.Properties.Supply, nil
}

// BurnAsset decreases the supply of the specified asset by the given amount.
func (object *Object) BurnAsset(assetID identifiers.AssetID, amount *big.Int) (big.Int, error) {
	assetObject, err := object.getAssetObject(assetID, false)
	if err != nil {
		return *big.NewInt(0), common.ErrAssetNotFound
	}

	assetObject.Properties.Supply.Sub(assetObject.Properties.Supply, amount)

	return *assetObject.Properties.Supply, nil
}

// CreateLogic creates a new logic object and returns its logic ID.
func (object *Object) CreateLogic(descriptor engineio.LogicDescriptor) (identifiers.LogicID, error) {
	// Generate the key for the LogicManifest from its hash
	key := common.BytesToHex(storage.LogicManifestKey(object.Address(), descriptor.ManifestHash))
	// Write the manifest into the dirty entries
	object.SetDirtyEntry(key, descriptor.ManifestData)

	// Create a new LogicObject from the LogicDescriptor
	logicObject := NewLogicObject(object.Address(), descriptor)
	// Insert the LogicObject into the state object
	if err := object.InsertNewLogicObject(logicObject.ID, logicObject); err != nil {
		return "", errors.Wrap(err, "could not insert logic object into state object")
	}

	// Initialise the logic for itself
	if err := object.InitLogicStorage(logicObject.LogicID()); err != nil {
		return "", err
	}

	return logicObject.ID, nil
}

// InitLogicStorage initializes the storage for a given logic ID.
func (object *Object) InitLogicStorage(logicID identifiers.LogicID) error {
	// Initialize a storage tree for the LogicID on the state object
	if _, err := object.createStorageTreeForLogic(logicID); err != nil {
		return err
	}

	object.storageTreeTxns[logicID] = iradix.New().Txn()

	return nil
}

// AddAccountGenesisInfo adds genesis information for an account.
func (object *Object) AddAccountGenesisInfo(address identifiers.Address, ixHash common.Hash) error {
	accInfo := common.AccountGenesisInfo{
		IxHash: ixHash,
	}

	rawData, err := accInfo.Bytes()
	if err != nil {
		return err
	}

	return object.SetStorageEntry(common.SargaLogicID, address.Bytes(), rawData)
}

func (object *Object) IsAccountRegistered(address identifiers.Address) (bool, error) {
	_, err := object.GetStorageEntry(common.SargaLogicID, address.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateContext creates a context object with given nodes and returns its hash.
func (object *Object) CreateContext(behaviouralNodes, randomNodes []kramaid.KramaID) (common.Hash, error) {
	if len(behaviouralNodes)+len(randomNodes) < MinimumContextSize {
		return common.NilHash, errors.New("liveliness size not met")
	}

	var (
		behaviouralContextObject = new(ContextObject)
		randomContextObject      = new(ContextObject)
		metaContextObject        = new(MetaContextObject)
	)

	behaviouralContextObject.Ids = append(behaviouralContextObject.Ids, behaviouralNodes...)
	randomContextObject.Ids = append(randomContextObject.Ids, randomNodes...)

	bHash, err := object.commitContextObject(behaviouralContextObject)
	if err != nil {
		return common.NilHash, errors.Wrap(common.ErrContextCreation, err.Error())
	}

	rHash, err := object.commitContextObject(randomContextObject)
	if err != nil {
		return common.NilHash, errors.Wrap(common.ErrContextCreation, err.Error())
	}

	metaContextObject.BehaviouralContext = bHash
	metaContextObject.RandomContext = rHash
	metaContextObject.PreviousHash = common.NilHash

	mHash, err := object.commitContextObject(metaContextObject)
	if err != nil {
		return common.NilHash, errors.Wrap(common.ErrContextCreation, err.Error())
	}

	// TODO:journal this
	object.cache.Add(bHash, behaviouralContextObject)
	object.cache.Add(rHash, randomContextObject)
	object.cache.Add(mHash, metaContextObject)

	// we set this object for temporary retrieval
	object.context = behaviouralContextObject

	object.data.ContextHash = mHash

	return mHash, nil
}

// UpdateContext updates the context with new nodes and returns the new context hash.
func (object *Object) UpdateContext(behaviouralNodes, randomNodes []kramaid.KramaID) (common.Hash, error) {
	if len(behaviouralNodes) == 0 && len(randomNodes) == 0 {
		return object.data.ContextHash, nil
	}

	var (
		err                 error
		behaviourObjectHash common.Hash
		randomObjectHash    common.Hash
	)

	metaObj, err := object.getMetaContextObjectCopy()
	if err != nil {
		return common.NilHash, err
	}

	// Set the previous Hash
	metaObj.PreviousHash = object.ContextHash()

	if len(behaviouralNodes) > 0 {
		behaviouralObj, err := object.getContextObjectCopy(metaObj.BehaviouralContext)
		if err != nil {
			return common.NilHash, err
		}

		behaviouralObj.AddNodes(behaviouralNodes, common.BehaviouralContextSize)

		behaviourObjectHash, err = object.commitContextObject(behaviouralObj)
		if err != nil {
			return common.NilHash, err
		}
	}

	if len(randomNodes) > 0 {
		randomObj, err := object.getContextObjectCopy(metaObj.RandomContext)
		if err != nil {
			return common.NilHash, err
		}

		randomObj.AddNodes(randomNodes, common.StochasticSetSize)

		// TODO:Sort based on the stake of the nodes

		randomObjectHash, err = object.commitContextObject(randomObj)
		if err != nil {
			return common.NilHash, err
		}
	}

	// TODO:Sort based on the stake of the nodes

	if !behaviourObjectHash.IsNil() {
		metaObj.BehaviouralContext = behaviourObjectHash
	}

	if !randomObjectHash.IsNil() {
		metaObj.RandomContext = randomObjectHash
	}

	contextHash, err := object.commitContextObject(metaObj)
	if err != nil {
		return common.NilHash, err
	}

	object.data.ContextHash = contextHash

	return contextHash, nil
}

// ContextHash returns the current context hash.
func (object *Object) ContextHash() common.Hash {
	return object.data.ContextHash
}

// getMetaContextObjectCopy retrieves a copy of the meta context object from cache or database.
func (object *Object) getMetaContextObjectCopy() (*MetaContextObject, error) {
	data, isAvailable := object.cache.Get(object.ContextHash())
	if isAvailable {
		metaContextObject, ok := data.(*MetaContextObject)
		if !ok {
			return nil, common.ErrInterfaceConversion
		}

		return metaContextObject.Copy(), nil
	}

	rawData, err := object.db.GetContext(object.address, object.ContextHash())
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch meta context object")
	}

	obj := new(MetaContextObject)
	if err := obj.FromBytes(rawData); err != nil {
		return nil, err
	}

	object.cache.Add(object.ContextHash(), obj)

	return obj.Copy(), nil
}

// getContextObjectCopy retrieves a copy of a context object from cache or database.
func (object *Object) getContextObjectCopy(hash common.Hash) (*ContextObject, error) {
	data, isAvailable := object.cache.Get(hash)
	if !isAvailable {
		rawData, err := object.db.GetContext(object.address, hash)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch context object")
		}

		obj := new(ContextObject)
		if err := obj.FromBytes(rawData); err != nil {
			return nil, err
		}

		object.cache.Add(hash, obj)

		return obj.Copy(), nil
	}

	contextObject, ok := data.(*ContextObject)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	return contextObject.Copy(), nil
}

// loads the asset deeds from the database or initializes a new one.
func (object *Object) loadDeeds() error {
	if object.data.AssetDeeds.IsNil() {
		object.deeds = &Deeds{
			Entries: make(map[string]struct{}),
		}

		return nil
	}

	data, err := object.db.GetDeeds(object.address, object.data.AssetDeeds)
	if err != nil {
		return err
	}

	object.deeds = new(Deeds)

	if err = object.deeds.FromBytes(data); err != nil {
		return err
	}

	return nil
}

// HasStorageTree checks if a storage tree for the given logic ID exists.
func (object *Object) HasStorageTree(logicID identifiers.LogicID) (bool, error) {
	if _, ok := object.storageTrees[logicID]; ok {
		return true, nil
	}

	metaStorageTree, err := object.getMetaStorageTree()
	if err != nil {
		return false, err
	}

	if _, err := metaStorageTree.Get(logicID.Bytes()); err != nil {
		return false, nil
	}

	return true, nil
}

// GetStorageTree retrieves and returns the Merkle tree based on the specified logic ID.
// If the tree is not cached, it loads it from the meta storage tree and initializes it.
// Returns an error if loading or initialization fails.
func (object *Object) GetStorageTree(logicID identifiers.LogicID) (tree.MerkleTree, error) {
	storageTree, ok := object.storageTrees[logicID]
	if ok {
		return storageTree, nil
	}

	metaStorageTree, err := object.getMetaStorageTree()
	if err != nil {
		return nil, err
	}

	root, err := metaStorageTree.Get(logicID.Bytes())
	if err != nil {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	storageTree, err = tree.NewKramaHashTree(
		object.address,
		common.BytesToHash(root),
		object.db, blake256.New(),
		storage.Storage,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic storage tree")
	}

	object.storageTrees[logicID] = storageTree

	return storageTree, nil
}

// SetStorageEntry inserts a key-value pair in the storage tree for the given logic ID.
// Returns an error if any issues arise during the process.
func (object *Object) SetStorageEntry(logicID identifiers.LogicID, key, value []byte) error {
	_, ok := object.storageTreeTxns[logicID]
	if !ok {
		if _, err := object.GetStorageTree(logicID); err != nil {
			return err
		}

		object.storageTreeTxns[logicID] = iradix.New().Txn()
	}

	// If the value has zero length, we treat it as a
	// delete operation instead of a write operation
	if len(value) == 0 {
		object.storageTreeTxns[logicID].Delete(key)

		return nil
	}

	object.storageTreeTxns[logicID].Insert(key, value)

	return nil
}

// GetStorageEntry retrieves the value associated a specific key from the storage tree for the given logic ID.
func (object *Object) GetStorageEntry(logicID identifiers.LogicID, key []byte) (value []byte, err error) {
	activeStorageTree, ok := object.storageTreeTxns[logicID]
	if ok {
		v, ok := activeStorageTree.Get(key)
		if ok {
			return v.([]byte), nil //nolint
		}
	}

	merkleTree, err := object.GetStorageTree(logicID)
	if err != nil {
		return nil, err
	}

	return merkleTree.Get(key)
}

// GetDirtyStorage returns the collection of storage entries that have been modified
// but not yet committed to the database.
func (object *Object) GetDirtyStorage() Storage {
	return object.dirtyEntries
}

// getMetaStorageTree retrieves the meta storage tree. If it's not initialized, it creates a new instance.
// Returns the meta storage tree or an error if initialization fails.
func (object *Object) getMetaStorageTree() (tree.MerkleTree, error) {
	if object.metaStorageTree != nil {
		return object.metaStorageTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.address,
		object.data.StorageRoot,
		object.db,
		blake256.New(),
		storage.Storage,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate storage tree")
	}

	object.metaStorageTree = merkleTree

	return object.metaStorageTree, nil
}

// createStorageTreeForLogic initializes a new Merkle tree for the specified logic ID and updates the meta storage tree.
// Returns the Merkle tree or an error if the creation or update fails.
func (object *Object) createStorageTreeForLogic(logicID identifiers.LogicID) (tree.MerkleTree, error) {
	if _, err := object.getMetaStorageTree(); err != nil {
		return nil, err
	}

	newStorageTree, err := tree.NewKramaHashTree(
		object.address,
		common.NilHash,
		object.db,
		blake256.New(),
		storage.Storage,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, err
	}

	object.storageTrees[logicID] = newStorageTree

	return newStorageTree, object.metaStorageTree.Set(logicID.Bytes(), common.NilHash.Bytes())
}

// isAssetRegistered checks if the given asset ID is registered.
func (object *Object) isAssetRegistered(assetID identifiers.AssetID) error {
	_, err := object.getAssetObject(assetID, true)
	if err != nil {
		return err
	}

	return nil
}

// isAssetRegistered checks if the given logic ID is registered.
func (object *Object) isLogicRegistered(logicID identifiers.LogicID) error {
	_, err := object.getLogicObject(logicID)
	if err != nil {
		return err
	}

	return nil
}

// getAssetTree retrieves the Merkle tree used for managing assets. If the tree is not initialized,
// it creates a new instance. Returns the asset Merkle tree or an error if the initialization fails.
func (object *Object) getAssetTree() (tree.MerkleTree, error) {
	if object.assetTree != nil {
		return object.assetTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.address,
		object.data.AssetRoot,
		object.db,
		blake256.New(),
		storage.Asset,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate asset tree")
	}

	object.assetTree = merkleTree

	return object.assetTree, nil
}

// getLogicTree retrieves the Merkle tree used for managing logic. If the tree is not initialized,
// it creates a new instance. Returns the logic Merkle tree or an error if the initialization fails.
func (object *Object) getLogicTree() (tree.MerkleTree, error) {
	if object.logicTree != nil {
		return object.logicTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.address,
		object.data.LogicRoot,
		object.db,
		blake256.New(),
		storage.Logic,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic tree")
	}

	object.logicTree = merkleTree

	return object.logicTree, nil
}

// getAssetObject retrieves the asset object for the specified asset ID.
func (object *Object) getAssetObject(assetID identifiers.AssetID, checkTxn bool) (*AssetObject, error) {
	if checkTxn && object.assetTreeTxn != nil {
		if v, ok := object.assetTreeTxn.Get(assetID.Bytes()); ok {
			if assetObject, ok := v.(*AssetObject); ok {
				return assetObject, nil
			}
		}
	}

	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	rawObject, err := assetTree.Get(assetID.Bytes())
	if err != nil {
		return nil, err
	}

	assetObject := new(AssetObject)
	if err = assetObject.FromBytes(rawObject); err != nil {
		return nil, err
	}

	return assetObject, nil
}

// getLogicObject retrieves the logic object for the specified logic ID.
func (object *Object) getLogicObject(logicID identifiers.LogicID) (*LogicObject, error) {
	if object.logicTreeTxn != nil {
		v, ok := object.logicTreeTxn.Get(logicID.Bytes())
		if ok {
			return v.(*LogicObject), nil //nolint
		}
	}

	logicTree, err := object.getLogicTree()
	if err != nil {
		return nil, err
	}

	rawObject, err := logicTree.Get(logicID.Bytes())
	if err != nil {
		return nil, err
	}

	logicObject := new(LogicObject)
	if err = logicObject.FromBytes(rawObject); err != nil {
		return nil, err
	}

	return logicObject, nil
}

// InsertNewAssetObject inserts the assetID and assetObject into the assetTree
// If the assetID is registered, this returns an error
func (object *Object) InsertNewAssetObject(assetID identifiers.AssetID, assetObject *AssetObject) error {
	if err := object.isAssetRegistered(assetID); err == nil {
		return common.ErrAssetAlreadyRegistered
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// InsertNewLogicObject inserts the logicID and logicObject into the logicsTree
// If the logicID is registered, this returns an error
func (object *Object) InsertNewLogicObject(logicID identifiers.LogicID, logicObject *LogicObject) error {
	if err := object.isLogicRegistered(logicID); err == nil {
		return errors.New("logic already registered")
	}

	object.updateLogicTree(logicID, logicObject)

	return nil
}

// FetchAssetObject returns the AssetObject associated with the given assetID,
// This returns an error if the assetID is not registered
func (object *Object) FetchAssetObject(assetID identifiers.AssetID, fromTxn bool) (*AssetObject, error) {
	return object.getAssetObject(assetID, fromTxn)
}

// FetchLogicObject returns the LogicObject associated with the given logicID,
// This returns an error if the logicID is not registered
func (object *Object) FetchLogicObject(logicID identifiers.LogicID) (*LogicObject, error) {
	return object.getLogicObject(logicID)
}

// GenerateLogicStorageObject returns a LogicStorageObject scoped to a given types.LogicID
func (object *Object) GenerateLogicStorageObject(logicID identifiers.LogicID) *LogicStorageObject {
	return NewLogicStorageObject(logicID, object)
}

func (object *Object) HasSufficientFuel(amount *big.Int) (bool, error) {
	if amount.Sign() == -1 {
		return false, errors.New("invalid transfer amount")
	}

	// Fetch sender balance object
	balance, _ := object.BalanceOf(common.KMOITokenAssetID)

	// Check if sender has sufficient balance
	if balance.Cmp(amount) == -1 {
		return false, nil
	}

	return true, nil
}

func (object *Object) DeductFuel(amount *big.Int) {
	// Remove amount from sender balance for asset
	_ = object.SubBalance(common.KMOITokenAssetID, amount)
}

func (object *Object) BehaviourContextObj() *ContextObject {
	return object.context
}
