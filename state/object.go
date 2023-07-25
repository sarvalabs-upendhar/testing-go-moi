package state

import (
	"log"
	"math/big"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
)

// var blakeHasher = blake256.New()

type Object struct {
	journal *Journal
	cache   *lru.Cache

	address common.Address
	accType common.AccountType
	data    common.Account

	db Store

	balance   *BalanceObject
	approvals *ApprovalObject
	registry  *RegistryObject

	logicTree       tree.MerkleTree
	metaStorageTree tree.MerkleTree
	fileTree        tree.MerkleTree //nolint:unused

	dirtyEntries LogicStorageObject
	receipts     common.Receipts

	activeStorageTrees map[string]tree.MerkleTree
	files              map[common.Hash][]byte
}

func NewStateObject(
	id common.Address,
	cache *lru.Cache,
	j *Journal,
	db Store,
	account common.Account,
) *Object {
	return &Object{
		journal:  j,
		accType:  account.AccType,
		cache:    cache,
		db:       db,
		data:     account,
		address:  id,
		balance:  nil,
		registry: nil,
		approvals: &ApprovalObject{
			Approvals: make(map[common.Address]common.AssetMap),
			PrvHash:   common.NilHash,
		},
		files:              make(map[common.Hash][]byte),
		dirtyEntries:       make(LogicStorageObject),
		receipts:           make(common.Receipts),
		activeStorageTrees: make(map[string]tree.MerkleTree, 4),
	}
}

func (object *Object) Address() common.Address {
	return object.address
}

func (object *Object) Data() *common.Account {
	return &object.data
}

func (object *Object) AccountType() common.AccountType {
	return object.accType
}

func (object *Object) AccountState() common.Account {
	return object.data
}

func (object *Object) Journal() *Journal {
	return object.journal
}

func (object *Object) Registry() (*RegistryObject, error) {
	if object.registry == nil {
		if err := object.loadRegistryObject(); err != nil {
			return nil, errors.Wrap(err, "failed to load registry object")
		}
	}

	return object.registry, nil
}

func (object *Object) GetRegistryEntry(key string) ([]byte, error) {
	registry, err := object.Registry()
	if err != nil {
		return nil, err
	}

	v, ok := registry.Entries[key]
	if !ok {
		return nil, common.ErrRegistryEntryNotFound
	}

	return v, nil
}

func (object *Object) CreateRegistryEntry(key string, info []byte) error {
	registry, err := object.Registry()
	if err != nil {
		return err
	}

	if _, ok := registry.Entries[key]; ok {
		return common.ErrAssetAlreadyRegistered
	}

	registry.Entries[key] = info

	return err
}

func (object *Object) UpdateRegistryEntry(key string, info []byte) error {
	registry, err := object.Registry()
	if err != nil {
		return err
	}

	registry.Entries[key] = info

	return err
}

func (object *Object) Balances() (*BalanceObject, error) {
	if object.balance == nil {
		if err := object.loadBalanceObject(); err != nil {
			return nil, errors.Wrap(err, "failed to load balance object")
		}
	}

	return object.balance, nil
}

func (object *Object) BalanceOf(id common.AssetID) (*big.Int, error) {
	balObject, err := object.Balances()
	if err != nil {
		return nil, err
	}

	if v, ok := balObject.AssetMap[id]; ok {
		return v, nil
	}

	return nil, common.ErrAssetNotFound
}

func (object *Object) AddBalance(aid common.AssetID, amount *big.Int) {
	if object.balance == nil {
		if err := object.loadBalanceObject(); err != nil {
			panic(err)
		}
	}

	bal, ok := object.balance.AssetMap[aid]
	if ok {
		object.balance.AssetMap[aid] = new(big.Int).Add(amount, bal)
	} else {
		object.balance.AssetMap[aid] = amount
	}
}

func (object *Object) SubBalance(aid common.AssetID, amount *big.Int) {
	if object.balance == nil {
		if err := object.loadBalanceObject(); err != nil {
			panic(err)
		}
	}

	if bal, ok := object.balance.AssetMap[aid]; ok && bal != nil {
		object.balance.AssetMap[aid] = new(big.Int).Sub(bal, amount)
	} else {
		log.Panicln("asset not found")
	}
}

// setBalance is used for test purposes only
func (object *Object) setBalance(assetID common.AssetID, bal *big.Int) {
	object.balance.AssetMap[assetID] = bal
}

func (object *Object) loadBalanceObject() error {
	if object.data.Balance.IsNil() {
		object.balance = &BalanceObject{
			AssetMap: make(map[common.AssetID]*big.Int),
		}

		return nil
	}

	data, err := object.db.GetBalance(object.address, object.data.Balance)
	if err != nil {
		return err
	}

	object.balance = new(BalanceObject)

	if err = object.balance.FromBytes(data); err != nil {
		return err
	}

	return nil
}

func (object *Object) Balance() (*BalanceObject, error) {
	if object.balance == nil {
		if err := object.loadBalanceObject(); err != nil {
			return nil, err
		}
	}

	return object.balance, nil
}

func (object *Object) Copy() *Object {
	j := new(Journal)
	sObj := NewStateObject(object.address, object.cache, j, object.db, object.data)

	sObj.dirtyEntries = object.dirtyEntries.Copy()

	if object.balance != nil {
		sObj.balance = object.balance.Copy()
	}

	if object.approvals != nil {
		sObj.approvals = object.approvals.Copy()
	}

	if object.registry != nil {
		sObj.registry = object.registry.Copy()
	}

	if object.logicTree != nil {
		sObj.logicTree = object.logicTree.Copy()
	}

	if object.metaStorageTree != nil {
		sObj.metaStorageTree = object.metaStorageTree.Copy() // TODO: Check if we require deep copy
	}

	for key, value := range object.files {
		v := make([]byte, len(value))

		copy(v, value)

		sObj.files[key] = v
	}

	return sObj
}

func (object *Object) SetDirtyEntry(key string, value []byte) {
	object.dirtyEntries[key] = value
}

func (object *Object) GetDirtyEntry(key string) ([]byte, error) {
	val, ok := object.dirtyEntries[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

func (object *Object) IncrementNonce(count uint64) {
	object.data.Nonce += count
}

func (object *Object) Commit() (common.Hash, error) {
	if _, err := object.commitBalanceObject(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit balance object")
	}

	if _, err := object.commitRegistryObject(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit registry object ")
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

func (object *Object) commitRegistryObject() (common.Hash, error) {
	if object.registry == nil || len(object.registry.Entries) == 0 {
		return common.NilHash, nil
	}

	data, err := object.registry.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)

	object.journal.append(RegistryUpdation{
		addr: &object.address,
		id:   hash,
	})

	object.SetDirtyEntry(
		common.BytesToHex(storage.RegistryObjectKey(object.address, hash)),
		data,
	)

	object.data.AssetRegistry = hash

	return hash, nil
}

func (object *Object) commitBalanceObject() (common.Hash, error) {
	if object.balance == nil || len(object.balance.AssetMap) == 0 {
		return common.NilHash, nil
	}

	data, err := object.balance.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)

	object.journal.append(BalanceUpdation{
		addr: &object.address,
		id:   hash,
	})

	key := common.BytesToHex(storage.BalanceObjectKey(object.address, hash))
	object.SetDirtyEntry(key, data)
	object.data.Balance = hash

	return hash, nil
}

func (object *Object) commitAccount() (common.Hash, error) {
	data, err := object.data.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)

	object.journal.append(AccountUpdation{
		addr: &object.address,
		id:   hash,
	})

	key := common.BytesToHex(storage.AccountKey(object.address, hash))
	object.SetDirtyEntry(key, data)

	return hash, nil
}

func (object *Object) commitContextObject(obj Context) (common.Hash, error) {
	// Add type checks here
	rawData, err := obj.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(rawData)

	object.journal.append(ContextUpdation{
		addr: &object.address,
		id:   hash,
	})

	key := common.BytesToHex(storage.ContextObjectKey(object.address, hash))
	object.SetDirtyEntry(key, rawData)

	return hash, nil
}

func (object *Object) commitActiveStorageTrees() error {
	// Add the updated logic-id <=> storage-root in master storage merkleTree
	for logicID, merkleTree := range object.activeStorageTrees {
		if !merkleTree.IsDirty() {
			continue
		}

		if err := merkleTree.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit storage tree")
		}

		rootHash, err := merkleTree.RootHash()
		if err != nil {
			return err
		}

		if err = object.metaStorageTree.Set(common.FromHex(logicID), rootHash.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

func (object *Object) commitMetaStorageTree() (common.Hash, error) {
	if !object.metaStorageTree.IsDirty() {
		return object.data.StorageRoot, nil
	}

	if err := object.metaStorageTree.Commit(); err != nil {
		return common.NilHash, err
	}

	rootHash, err := object.metaStorageTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	object.journal.append(StorageUpdation{
		addr: &object.address,
		id:   rootHash,
	})

	object.data.StorageRoot = rootHash

	return rootHash, nil
}

func (object *Object) commitStorage() (common.Hash, error) {
	if object.metaStorageTree == nil {
		return object.data.StorageRoot, nil
	}

	err := object.commitActiveStorageTrees()
	if err != nil {
		return common.NilHash, err
	}

	return object.commitMetaStorageTree()
}

// commitLogics commits the logic tree and flushes the changes to db
func (object *Object) commitLogics() (common.Hash, error) {
	if object.logicTree == nil {
		return object.data.LogicRoot, nil
	}

	err := object.logicTree.Commit()
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
	if err := object.flushLogicTree(); err != nil {
		return errors.Wrap(err, "failed to fetch logic tree")
	}

	if err := object.flushActiveStorageTrees(); err != nil {
		return errors.Wrap(err, "failed to flush active storage trees")
	}

	for k, v := range object.GetDirtyStorage() {
		if err := object.db.CreateEntry(common.FromHex(k), v); err != nil {
			return errors.Wrap(err, "failed to write dirty entries")
		}
	}

	return nil
}

func (object *Object) flushLogicTree() error {
	if object.logicTree == nil {
		return nil
	}

	return object.logicTree.Flush()
}

func (object *Object) flushActiveStorageTrees() error {
	if object.metaStorageTree == nil {
		return nil
	}

	// flush active storage trees
	for _, storageTree := range object.activeStorageTrees {
		if err := storageTree.Flush(); err != nil {
			return errors.Wrap(err, "failed to commit modified storage tree entries to store")
		}
	}

	// flush master storage trees
	return object.metaStorageTree.Flush()
}

func (object *Object) CreateStorageTreeForLogic(logicID common.LogicID) error {
	_, err := object.createStorageTreeForLogic(logicID)

	return err
}

func (object *Object) CreateAsset(addr common.Address, descriptor *common.AssetDescriptor) (common.AssetID, error) {
	assetID := common.NewAssetIDv0(
		descriptor.IsLogical,
		descriptor.IsStateFul,
		descriptor.Dimension,
		descriptor.Standard,
		addr,
	)

	rawBytes, err := descriptor.Bytes()
	if err != nil {
		return "", err
	}

	if err = object.CreateRegistryEntry(assetID.String(), rawBytes); err != nil {
		return "", err
	}

	return assetID, nil
}

func (object *Object) CreateLogic(descriptor *engineio.LogicDescriptor) (common.LogicID, error) {
	// Generate the key for the LogicManifest from its hash
	key := common.BytesToHex(storage.LogicManifestKey(object.Address(), descriptor.ManifestHash))
	// Write the manifest into the dirty entries
	object.SetDirtyEntry(key, descriptor.ManifestRaw)

	// Create a new LogicObject from the LogicDescriptor
	logicObject := NewLogicObject(object.Address(), descriptor)
	// Insert the LogicObject into the state object
	if err := object.InsertNewLogicObject(logicObject.LogicID(), logicObject); err != nil {
		return "", errors.Wrap(err, "could not insert logic object into state object")
	}

	// Initialize a storage tree for the LogicID on the state object
	if err := object.CreateStorageTreeForLogic(logicObject.LogicID()); err != nil {
		return "", errors.Wrap(err, "could not init storage tree for logic")
	}

	return logicObject.LogicID(), nil
}

func (object *Object) AddAccountGenesisInfo(address common.Address, ixHash common.Hash) error {
	accInfo := common.AccountGenesisInfo{
		IxHash: ixHash,
	}

	rawData, err := accInfo.Bytes()
	if err != nil {
		return err
	}

	return object.SetStorageEntry(common.SargaLogicID, address.Bytes(), rawData)
}

func (object *Object) CreateContext(behaviouralNodes, randomNodes []id.KramaID) (common.Hash, error) {
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
	object.cache.Add(mHash, metaContextObject)
	object.cache.Add(rHash, randomContextObject)

	object.data.ContextHash = mHash

	return mHash, nil
}

func (object *Object) UpdateContext(behaviouralNodes []id.KramaID, randomNodes []id.KramaID) (common.Hash, error) {
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

		behaviouralObj.AddNodes(behaviouralNodes, MaxBehaviourContextSize)

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

		randomObj.AddNodes(randomNodes, MaxRandomContextSize)

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

func (object *Object) ContextHash() common.Hash {
	return object.data.ContextHash
}

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

func (object *Object) loadRegistryObject() error {
	if object.data.AssetRegistry.IsNil() {
		object.registry = &RegistryObject{
			Entries: make(map[string][]byte),
		}

		return nil
	}

	data, err := object.db.GetAssetRegistry(object.address, object.data.AssetRegistry)
	if err != nil {
		return err
	}

	object.registry = new(RegistryObject)

	if err = object.registry.FromBytes(data); err != nil {
		return err
	}

	return nil
}

func (object *Object) GetStorageTree(logicID common.LogicID) (tree.MerkleTree, error) {
	storageTree, ok := object.activeStorageTrees[logicID.String()]
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
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic storage tree")
	}

	object.activeStorageTrees[logicID.String()] = storageTree

	return storageTree, nil
}

func (object *Object) SetStorageEntry(logicID common.LogicID, key, value []byte) (err error) {
	merkleTree, err := object.GetStorageTree(logicID)
	if err != nil {
		return err
	}

	return merkleTree.Set(key, value)
}

func (object *Object) GetStorageEntry(logicID common.LogicID, key []byte) (value []byte, err error) {
	merkleTree, err := object.GetStorageTree(logicID)
	if err != nil {
		return nil, err
	}

	return merkleTree.Get(key)
}

func (object *Object) GetDirtyStorage() LogicStorageObject {
	return object.dirtyEntries
}

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
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate storage tree")
	}

	object.metaStorageTree = merkleTree

	return object.metaStorageTree, nil
}

func (object *Object) createStorageTreeForLogic(logicID common.LogicID) (tree.MerkleTree, error) {
	if _, err := object.getMetaStorageTree(); err != nil {
		return nil, err
	}

	newStorageTree, err := tree.NewKramaHashTree(
		object.address,
		common.NilHash,
		object.db,
		blake256.New(),
		storage.Storage,
	)
	if err != nil {
		return nil, err
	}

	object.activeStorageTrees[logicID.String()] = newStorageTree

	return newStorageTree, object.metaStorageTree.Set(logicID.Bytes(), common.NilHash.Bytes())
}

func (object *Object) isLogicRegistered(logicID common.LogicID) error {
	_, err := object.getLogicObject(logicID)
	if err != nil {
		return err
	}

	return nil
}

func (object *Object) getMetaLogicTree() (tree.MerkleTree, error) {
	if object.logicTree != nil {
		return object.logicTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.address,
		object.data.LogicRoot,
		object.db,
		blake256.New(),
		storage.Logic,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic tree")
	}

	object.logicTree = merkleTree

	return object.logicTree, nil
}

func (object *Object) getLogicObject(logicID common.LogicID) (*LogicObject, error) {
	logicTree, err := object.getMetaLogicTree()
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

// InsertNewLogicObject inserts the logicID and logicObject into the logicsTree
// If the logicID is registered, this returns an error
func (object *Object) InsertNewLogicObject(logicID common.LogicID, logicObject *LogicObject) error {
	if err := object.isLogicRegistered(logicID); err == nil {
		return errors.New("logic already registered")
	}

	logicTree, err := object.getMetaLogicTree()
	if err != nil {
		return errors.Wrap(err, "failed to load logic tree")
	}

	rawLogicObject, err := logicObject.Bytes()
	if err != nil {
		return err
	}

	if err = logicTree.Set(logicID.Bytes(), rawLogicObject); err != nil {
		return errors.Wrap(err, "failed to add logic object to tree")
	}

	return nil
}

// FetchLogicObject returns the LogicObject associated with the given logicID,
// This returns an error if the logicID is not registered
func (object *Object) FetchLogicObject(logicID common.LogicID) (*LogicObject, error) {
	return object.getLogicObject(logicID)
}

// GenerateLogicContextObject returns a LogicContextObject scoped to a given types.LogicID
func (object *Object) GenerateLogicContextObject(logicID common.LogicID) *LogicContextObject {
	return NewLogicContextObject(logicID, object)
}

func (object *Object) HasFuel(amount *big.Int) (bool, error) {
	if amount.Sign() == -1 {
		return false, errors.New("invalid transfer amount")
	}

	// Fetch sender balance object
	balances, err := object.BalanceOf(common.KMOITokenAssetID)
	if err != nil {
		return false, err
	}

	// Check if sender has sufficient balance
	if balances.Cmp(amount) == -1 {
		return false, nil
	}

	return true, nil
}

func (object *Object) DeductFuel(amount *big.Int) {
	// Remove amount from sender balance for asset
	object.SubBalance(common.KMOITokenAssetID, amount)
}
