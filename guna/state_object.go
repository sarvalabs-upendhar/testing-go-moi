package guna

import (
	"log"
	"math/big"

	"github.com/decred/dcrd/crypto/blake256"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/guna/tree"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type Storage map[string][]byte

var blakeHasher = blake256.New()

func (s Storage) Copy() Storage {
	storage := make(Storage)

	for key, value := range s {
		v := make([]byte, len(value))

		copy(v, value)

		storage[key] = v
	}

	return storage
}

type StateObject struct {
	journal *Journal
	cache   *lru.Cache

	address types.Address
	accType types.AccountType
	data    types.Account

	db Store

	balance   *gtypes.BalanceObject
	approvals *gtypes.ApprovalObject
	registry  *gtypes.RegistryObject

	logicTree       tree.MerkleTree
	metaStorageTree tree.MerkleTree
	fileTree        tree.MerkleTree //nolint:unused

	dirtyEntries Storage
	receipts     types.Receipts

	activeStorageTrees map[string]tree.MerkleTree
	files              map[types.Hash][]byte
}

func NewStateObject(
	id types.Address,
	cache *lru.Cache,
	j *Journal,
	db Store,
	account types.Account,
) *StateObject {
	return &StateObject{
		journal:  j,
		accType:  account.AccType,
		cache:    cache,
		db:       db,
		data:     account,
		address:  id,
		balance:  nil,
		registry: nil,
		approvals: &gtypes.ApprovalObject{
			Approvals: make(map[types.Address]types.AssetMap),
			PrvHash:   types.NilHash,
		},
		files:              make(map[types.Hash][]byte),
		dirtyEntries:       make(Storage),
		receipts:           make(types.Receipts),
		activeStorageTrees: make(map[string]tree.MerkleTree, 4),
	}
}

func (s *StateObject) Address() types.Address {
	return s.address
}

func (s *StateObject) Data() *types.Account {
	return &s.data
}

func (s *StateObject) AccountType() types.AccountType {
	return s.accType
}

func (s *StateObject) AccountState() types.Account {
	return s.data
}

func (s *StateObject) Journal() *Journal {
	return s.journal
}

func (s *StateObject) Registry() (*gtypes.RegistryObject, error) {
	if s.registry == nil {
		if err := s.loadRegistryObject(); err != nil {
			return nil, errors.Wrap(err, "failed to load registry object")
		}
	}

	return s.registry, nil
}

func (s *StateObject) GetRegistryEntry(key string) ([]byte, error) {
	object, err := s.Registry()
	if err != nil {
		return nil, err
	}

	v, ok := object.Entries[key]
	if !ok {
		return nil, types.ErrRegistryEntryNotFound
	}

	return v, nil
}

func (s *StateObject) CreateRegistryEntry(key string, info []byte) error {
	object, err := s.Registry()
	if err != nil {
		return err
	}

	if _, ok := object.Entries[key]; ok {
		return types.ErrAssetAlreadyRegistered
	}

	object.Entries[key] = info

	return err
}

func (s *StateObject) UpdateRegistryEntry(key string, info []byte) error {
	object, err := s.Registry()
	if err != nil {
		return err
	}

	object.Entries[key] = info

	return err
}

func (s *StateObject) Balances() (*gtypes.BalanceObject, error) {
	if s.balance == nil {
		if err := s.loadBalanceObject(); err != nil {
			return nil, errors.Wrap(err, "failed to load balance object")
		}
	}

	return s.balance, nil
}

func (s *StateObject) BalanceOf(id types.AssetID) (*big.Int, error) {
	balObject, err := s.Balances()
	if err != nil {
		return nil, err
	}

	if v, ok := balObject.AssetMap[id]; ok {
		return v, nil
	}

	return nil, types.ErrAssetNotFound
}

func (s *StateObject) AddBalance(aid types.AssetID, amount *big.Int) {
	if s.balance == nil {
		if err := s.loadBalanceObject(); err != nil {
			panic(err)
		}
	}

	bal, ok := s.balance.AssetMap[aid]
	if ok {
		s.balance.AssetMap[aid] = new(big.Int).Add(amount, bal)
	} else {
		s.balance.AssetMap[aid] = amount
	}
}

func (s *StateObject) SubBalance(aid types.AssetID, amount *big.Int) {
	if s.balance == nil {
		if err := s.loadBalanceObject(); err != nil {
			panic(err)
		}
	}

	if bal, ok := s.balance.AssetMap[aid]; ok && bal != nil {
		s.balance.AssetMap[aid] = new(big.Int).Sub(bal, amount)
	} else {
		log.Panicln("asset not found")
	}
}

// setBalance is used for test purposes only
func (s *StateObject) setBalance(assetID types.AssetID, bal *big.Int) {
	s.balance.AssetMap[assetID] = bal
}

func (s *StateObject) loadBalanceObject() error {
	if s.data.Balance.IsNil() {
		s.balance = &gtypes.BalanceObject{
			AssetMap: make(map[types.AssetID]*big.Int),
		}

		return nil
	}

	data, err := s.db.GetBalance(s.address, s.data.Balance)
	if err != nil {
		return err
	}

	s.balance = new(gtypes.BalanceObject)

	if err = s.balance.FromBytes(data); err != nil {
		return err
	}

	return nil
}

func (s *StateObject) Balance() (*gtypes.BalanceObject, error) {
	if s.balance == nil {
		if err := s.loadBalanceObject(); err != nil {
			return nil, err
		}
	}

	return s.balance, nil
}

func (s *StateObject) Copy() *StateObject {
	j := new(Journal)
	sObj := NewStateObject(s.address, s.cache, j, s.db, s.data)

	sObj.dirtyEntries = s.dirtyEntries.Copy()

	if s.balance != nil {
		sObj.balance = s.balance.Copy()
	}

	if s.approvals != nil {
		sObj.approvals = s.approvals.Copy()
	}

	if s.registry != nil {
		sObj.registry = s.registry.Copy()
	}

	if s.logicTree != nil {
		sObj.logicTree = s.logicTree.Copy()
	}

	if s.metaStorageTree != nil {
		sObj.metaStorageTree = s.metaStorageTree.Copy() // TODO: Check if we require deep copy
	}

	for key, value := range s.files {
		v := make([]byte, len(value))

		copy(v, value)

		sObj.files[key] = v
	}

	return sObj
}

func (s *StateObject) SetDirtyEntry(key string, value []byte) {
	s.dirtyEntries[key] = value
}

func (s *StateObject) GetDirtyEntry(key string) ([]byte, error) {
	val, ok := s.dirtyEntries[key]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return val, nil
}

func (s *StateObject) IncrementNonce(count uint64) {
	s.data.Nonce += count
}

func (s *StateObject) Commit() (types.Hash, error) {
	if _, err := s.commitBalanceObject(); err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit balance object")
	}

	if _, err := s.commitRegistryObject(); err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit registry object ")
	}

	if _, err := s.commitLogics(); err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	if _, err := s.commitStorage(); err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit storage tree")
	}

	accCid, err := s.commitAccount()
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit account")
	}

	return accCid, nil
}

func (s *StateObject) commitRegistryObject() (types.Hash, error) {
	if s.registry == nil || len(s.registry.Entries) == 0 {
		return types.NilHash, nil
	}

	data, err := s.registry.Bytes()
	if err != nil {
		return types.NilHash, err
	}

	hash := types.GetHash(data)

	s.journal.append(RegistryUpdation{
		addr: &s.address,
		id:   hash,
	})

	s.SetDirtyEntry(
		types.BytesToHex(dhruva.RegistryObjectKey(s.address, hash)),
		data,
	)

	s.data.AssetRegistry = hash

	return hash, nil
}

func (s *StateObject) commitBalanceObject() (types.Hash, error) {
	if s.balance == nil || len(s.balance.AssetMap) == 0 {
		return types.NilHash, nil
	}

	data, err := s.balance.Bytes()
	if err != nil {
		return types.NilHash, err
	}

	hash := types.GetHash(data)

	s.journal.append(BalanceUpdation{
		addr: &s.address,
		id:   hash,
	})

	key := types.BytesToHex(dhruva.BalanceObjectKey(s.address, hash))
	s.SetDirtyEntry(key, data)
	s.data.Balance = hash

	return hash, nil
}

func (s *StateObject) commitAccount() (types.Hash, error) {
	data, err := s.data.Bytes()
	if err != nil {
		return types.NilHash, err
	}

	hash := types.GetHash(data)

	s.journal.append(AccountUpdation{
		addr: &s.address,
		id:   hash,
	})

	key := types.BytesToHex(dhruva.AccountKey(s.address, hash))
	s.SetDirtyEntry(key, data)

	return hash, nil
}

func (s *StateObject) commitContextObject(obj gtypes.Context) (types.Hash, error) {
	// Add type checks here
	rawData, err := obj.Bytes()
	if err != nil {
		return types.NilHash, err
	}

	hash := types.GetHash(rawData)

	s.journal.append(ContextUpdation{
		addr: &s.address,
		id:   hash,
	})

	key := types.BytesToHex(dhruva.ContextObjectKey(s.address, hash))
	s.SetDirtyEntry(key, rawData)

	return hash, nil
}

func (s *StateObject) commitActiveStorageTrees() error {
	// Add the updated logic-id <=> storage-root in master storage merkleTree
	for logicID, merkleTree := range s.activeStorageTrees {
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

		if err = s.metaStorageTree.Set(types.FromHex(logicID), rootHash.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

func (s *StateObject) commitMetaStorageTree() (types.Hash, error) {
	if !s.metaStorageTree.IsDirty() {
		return s.data.StorageRoot, nil
	}

	if err := s.metaStorageTree.Commit(); err != nil {
		return types.NilHash, err
	}

	rootHash, err := s.metaStorageTree.RootHash()
	if err != nil {
		return types.NilHash, err
	}

	s.journal.append(StorageUpdation{
		addr: &s.address,
		id:   rootHash,
	})

	s.data.StorageRoot = rootHash

	return rootHash, nil
}

func (s *StateObject) commitStorage() (types.Hash, error) {
	if s.metaStorageTree == nil {
		return s.data.StorageRoot, nil
	}

	err := s.commitActiveStorageTrees()
	if err != nil {
		return types.NilHash, err
	}

	return s.commitMetaStorageTree()
}

// commitLogics commits and the logic tree and flushes the changes to db
func (s *StateObject) commitLogics() (types.Hash, error) {
	if s.logicTree == nil {
		return s.data.LogicRoot, nil
	}

	err := s.logicTree.Commit()
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	s.data.LogicRoot, err = s.logicTree.RootHash()
	if err != nil {
		return types.NilHash, err
	}

	return s.data.LogicRoot, nil
}

// flush will write all dirty entries to the database
func (s *StateObject) flush() error {
	if err := s.flushLogicTree(); err != nil {
		return errors.Wrap(err, "failed to fetch logic tree")
	}

	if err := s.flushActiveStorageTrees(); err != nil {
		return errors.Wrap(err, "failed to flush active storage trees")
	}

	for k, v := range s.GetDirtyStorage() {
		if err := s.db.CreateEntry(types.FromHex(k), v); err != nil {
			return errors.Wrap(err, "failed to write dirty entries")
		}
	}

	return nil
}

func (s *StateObject) flushLogicTree() error {
	if s.logicTree == nil {
		return nil
	}

	return s.logicTree.Flush()
}

func (s *StateObject) flushActiveStorageTrees() error {
	if s.metaStorageTree == nil {
		return nil
	}

	// flush active storage trees
	for _, storageTree := range s.activeStorageTrees {
		if err := storageTree.Flush(); err != nil {
			return errors.Wrap(err, "failed to commit modified storage tree entries to store")
		}
	}

	// flush master storage trees
	return s.metaStorageTree.Flush()
}

func (s *StateObject) CreateStorageTreeForLogic(logicID types.LogicID) error {
	_, err := s.createStorageTreeForLogic(logicID)

	return err
}

func (s *StateObject) CreateAsset(addr types.Address, descriptor *types.AssetDescriptor) (types.AssetID, error) {
	assetID := types.NewAssetIDv0(
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

	if err = s.CreateRegistryEntry(assetID.String(), rawBytes); err != nil {
		return "", err
	}

	return assetID, nil
}

func (s *StateObject) AddAccountGenesisInfo(address types.Address, ixHash types.Hash) error {
	accInfo := types.AccountGenesisInfo{
		IxHash: ixHash,
	}

	rawData, err := accInfo.Bytes()
	if err != nil {
		return err
	}

	return s.SetStorageEntry(types.SargaLogicID, address.Bytes(), rawData)
}

func (s *StateObject) CreateContext(behaviouralNodes, randomNodes []id.KramaID) (types.Hash, error) {
	if len(behaviouralNodes)+len(randomNodes) < MinimumContextSize {
		return types.NilHash, errors.New("liveliness size not met")
	}

	var (
		behaviouralContextObject = new(gtypes.ContextObject)
		randomContextObject      = new(gtypes.ContextObject)
		metaContextObject        = new(gtypes.MetaContextObject)
	)

	behaviouralContextObject.Ids = append(behaviouralContextObject.Ids, behaviouralNodes...)
	randomContextObject.Ids = append(randomContextObject.Ids, randomNodes...)

	bHash, err := s.commitContextObject(behaviouralContextObject)
	if err != nil {
		return types.NilHash, errors.Wrap(types.ErrContextCreation, err.Error())
	}

	rHash, err := s.commitContextObject(randomContextObject)
	if err != nil {
		return types.NilHash, errors.Wrap(types.ErrContextCreation, err.Error())
	}

	metaContextObject.BehaviouralContext = bHash
	metaContextObject.RandomContext = rHash
	metaContextObject.PreviousHash = types.NilHash

	mHash, err := s.commitContextObject(metaContextObject)
	if err != nil {
		return types.NilHash, errors.Wrap(types.ErrContextCreation, err.Error())
	}

	// TODO:journal this
	s.cache.Add(bHash, behaviouralContextObject)
	s.cache.Add(mHash, metaContextObject)
	s.cache.Add(rHash, randomContextObject)

	s.data.ContextHash = mHash

	return mHash, nil
}

func (s *StateObject) UpdateContext(behaviouralNodes []id.KramaID, randomNodes []id.KramaID) (types.Hash, error) {
	if len(behaviouralNodes) == 0 && len(randomNodes) == 0 {
		return s.data.ContextHash, nil
	}

	var (
		err                 error
		behaviourObjectHash types.Hash
		randomObjectHash    types.Hash
	)

	metaObj, err := s.getMetaContextObjectCopy()
	if err != nil {
		return types.NilHash, err
	}

	// Set the previous Hash
	metaObj.PreviousHash = s.ContextHash()

	if len(behaviouralNodes) > 0 {
		behaviouralObj, err := s.getContextObjectCopy(metaObj.BehaviouralContext)
		if err != nil {
			return types.NilHash, err
		}

		behaviouralObj.AddNodes(behaviouralNodes, gtypes.MaxBehaviourContextSize)

		behaviourObjectHash, err = s.commitContextObject(behaviouralObj)
		if err != nil {
			return types.NilHash, err
		}
	}

	if len(randomNodes) > 0 {
		randomObj, err := s.getContextObjectCopy(metaObj.RandomContext)
		if err != nil {
			return types.NilHash, err
		}

		randomObj.AddNodes(randomNodes, gtypes.MaxRandomContextSize)

		// TODO:Sort based on the stake of the nodes

		randomObjectHash, err = s.commitContextObject(randomObj)
		if err != nil {
			return types.NilHash, err
		}
	}

	// TODO:Sort based on the stake of the nodes

	if !behaviourObjectHash.IsNil() {
		metaObj.BehaviouralContext = behaviourObjectHash
	}

	if !randomObjectHash.IsNil() {
		metaObj.RandomContext = randomObjectHash
	}

	contextHash, err := s.commitContextObject(metaObj)
	if err != nil {
		return types.NilHash, err
	}

	s.data.ContextHash = contextHash

	return contextHash, nil
}

func (s *StateObject) ContextHash() types.Hash {
	return s.data.ContextHash
}

func (s *StateObject) getMetaContextObjectCopy() (*gtypes.MetaContextObject, error) {
	data, isAvailable := s.cache.Get(s.ContextHash())
	if isAvailable {
		metaContextObject, ok := data.(*gtypes.MetaContextObject)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return metaContextObject.Copy(), nil
	}

	rawData, err := s.db.GetContext(s.address, s.ContextHash())
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch meta context object")
	}

	obj := new(gtypes.MetaContextObject)

	if err := obj.FromBytes(rawData); err != nil {
		return nil, err
	}

	s.cache.Add(s.ContextHash(), obj)

	return obj.Copy(), nil
}

func (s *StateObject) getContextObjectCopy(hash types.Hash) (*gtypes.ContextObject, error) {
	data, isAvailable := s.cache.Get(hash)
	if !isAvailable {
		rawData, err := s.db.GetContext(s.address, hash)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch context object")
		}

		obj := new(gtypes.ContextObject)

		if err := obj.FromBytes(rawData); err != nil {
			return nil, err
		}

		s.cache.Add(hash, obj)

		return obj.Copy(), nil
	}

	contextObject, ok := data.(*gtypes.ContextObject)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return contextObject.Copy(), nil
}

func (s *StateObject) loadRegistryObject() error {
	if s.data.AssetRegistry.IsNil() {
		s.registry = &gtypes.RegistryObject{
			Entries: make(map[string][]byte),
		}

		return nil
	}

	data, err := s.db.GetAssetRegistry(s.address, s.data.AssetRegistry)
	if err != nil {
		return err
	}

	s.registry = new(gtypes.RegistryObject)

	if err = s.registry.FromBytes(data); err != nil {
		return err
	}

	return nil
}

func (s *StateObject) GetStorageTree(logicID types.LogicID) (tree.MerkleTree, error) {
	storageTree, ok := s.activeStorageTrees[logicID.String()]
	if ok {
		return storageTree, nil
	}

	metaStorageTree, err := s.getMetaStorageTree()
	if err != nil {
		return nil, err
	}

	root, err := metaStorageTree.Get(logicID.Bytes())
	if err != nil {
		return nil, types.ErrLogicStorageTreeNotFound
	}

	storageTree, err = tree.NewKramaHashTree(s.address, types.BytesToHash(root), s.db, blake256.New(), dhruva.Storage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic storage tree")
	}

	s.activeStorageTrees[logicID.String()] = storageTree

	return storageTree, nil
}

func (s *StateObject) SetStorageEntry(logicID types.LogicID, key, value []byte) (err error) {
	merkleTree, err := s.GetStorageTree(logicID)
	if err != nil {
		return err
	}

	return merkleTree.Set(key, value)
}

func (s *StateObject) GetStorageEntry(logicID types.LogicID, key []byte) (value []byte, err error) {
	merkleTree, err := s.GetStorageTree(logicID)
	if err != nil {
		return nil, err
	}

	return merkleTree.Get(key)
}

func (s *StateObject) GetDirtyStorage() Storage {
	return s.dirtyEntries
}

func (s *StateObject) getMetaStorageTree() (tree.MerkleTree, error) {
	if s.metaStorageTree != nil {
		return s.metaStorageTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(s.address, s.data.StorageRoot, s.db, blake256.New(), dhruva.Storage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate storage tree")
	}

	s.metaStorageTree = merkleTree

	return s.metaStorageTree, nil
}

func (s *StateObject) createStorageTreeForLogic(logicID types.LogicID) (tree.MerkleTree, error) {
	if _, err := s.getMetaStorageTree(); err != nil {
		return nil, err
	}

	newStorageTree, err := tree.NewKramaHashTree(s.address, types.NilHash, s.db, blake256.New(), dhruva.Storage)
	if err != nil {
		return nil, err
	}

	s.activeStorageTrees[logicID.String()] = newStorageTree

	return newStorageTree, s.metaStorageTree.Set(logicID.Bytes(), types.NilHash.Bytes())
}

func (s *StateObject) isLogicRegistered(logicID types.LogicID) error {
	_, err := s.getLogicObject(logicID)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateObject) getMetaLogicTree() (tree.MerkleTree, error) {
	if s.logicTree != nil {
		return s.logicTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(s.address, s.data.LogicRoot, s.db, blakeHasher, dhruva.Logic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic tree")
	}

	s.logicTree = merkleTree

	return s.logicTree, nil
}

func (s *StateObject) getLogicObject(logicID types.LogicID) (*gtypes.LogicObject, error) {
	logicTree, err := s.getMetaLogicTree()
	if err != nil {
		return nil, err
	}

	rawObject, err := logicTree.Get(logicID.Bytes())
	if err != nil {
		return nil, err
	}

	logicObject := new(gtypes.LogicObject)

	if err = logicObject.FromBytes(rawObject); err != nil {
		return nil, err
	}

	return logicObject, nil
}

// InsertNewLogicObject inserts the logicID and logicObject into the logicsTree
// If the logicID is registered, this returns an error
func (s *StateObject) InsertNewLogicObject(logicID types.LogicID, logicObject *gtypes.LogicObject) error {
	if err := s.isLogicRegistered(logicID); err == nil {
		return errors.New("logic already registered")
	}

	logicTree, err := s.getMetaLogicTree()
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
func (s *StateObject) FetchLogicObject(logicID types.LogicID) (*gtypes.LogicObject, error) {
	return s.getLogicObject(logicID)
}

// GenerateLogicContextObject returns a LogicContextObject scoped to a given types.LogicID
func (s *StateObject) GenerateLogicContextObject(logicID types.LogicID) *gtypes.LogicContextObject {
	return gtypes.NewLogicContextObject(logicID, s)
}
