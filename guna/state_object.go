package guna

import (
	"log"
	"math/big"
	"sync"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/decred/dcrd/crypto/blake256"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/guna/tree"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type Storage map[string][]byte

var blakeHasher = blake256.New()

func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}

type StateObject struct {
	journal *Journal
	mtx     sync.RWMutex
	cache   *lru.Cache

	address types.Address
	accType types.AccountType
	data    types.Account

	db *dhruva.PersistenceManager

	balance        *gtypes.BalanceObject
	assetApprovals *gtypes.ApprovalObject

	logicTree       tree.MerkleTree
	metaStorageTree tree.MerkleTree
	fileTrie        tree.MerkleTree //nolint

	dirtyEntries Storage
	receipts     types.Receipts

	activeStorageTrees map[string]tree.MerkleTree
	files              map[types.Hash][]byte
}

func NewStateObject(
	id types.Address,
	cache *lru.Cache,
	j *Journal,
	db *dhruva.PersistenceManager,
	account types.Account,
	accType types.AccountType,
) *StateObject {
	return &StateObject{
		journal: j,
		accType: accType,
		cache:   cache,
		db:      db,
		data:    account,
		address: id,
		balance: &gtypes.BalanceObject{
			Balances: make(types.AssetMap),
			PrvHash:  types.NilHash,
		},
		assetApprovals: &gtypes.ApprovalObject{
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

func (s *StateObject) BalanceOf(id types.AssetID) (*big.Int, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if v, ok := s.balance.Balances[id]; ok {
		return v, nil
	}

	return nil, types.ErrAssetNotFound
}

func (s *StateObject) AddBalance(aid types.AssetID, amount *big.Int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	bal, ok := s.balance.Balances[aid]
	if ok {
		s.setBalance(aid, new(big.Int).Add(amount, bal))
	} else {
		s.balance.Balances[aid] = amount
	}
}

func (s *StateObject) SubBalance(aid types.AssetID, amount *big.Int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if bal, ok := s.balance.Balances[aid]; ok && bal != nil {
		s.setBalance(aid, new(big.Int).Sub(bal, amount))
	} else {
		log.Panicln("asset not found")
	}
}

func (s *StateObject) setBalance(aid types.AssetID, amount *big.Int) {
	s.balance.Balances[aid] = amount
}

func (s *StateObject) GetAccountType() types.AccountType {
	return s.accType
}

func (s *StateObject) AccountState() types.Account {
	return s.data
}

func (s *StateObject) Copy() *StateObject {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	j := new(Journal)
	sObj := NewStateObject(s.address, s.cache, j, s.db, s.data, s.data.AccType)

	sObj.balance = s.balance.Copy()
	sObj.assetApprovals = s.assetApprovals.Copy()
	sObj.dirtyEntries = s.dirtyEntries.Copy()

	if s.logicTree != nil {
		sObj.logicTree = s.logicTree.Copy()
	}

	if s.metaStorageTree != nil {
		sObj.metaStorageTree = s.metaStorageTree.Copy() // TODO: Check if we require deep copy
	}

	for k, v := range sObj.files {
		sObj.files[k] = v
	}

	return sObj
}

func (s *StateObject) commitBalanceObject() ([]byte, error) {
	data, err := s.balance.Bytes()
	if err != nil {
		return nil, err
	}

	hash := types.GetHash(data)

	s.journal.append(BalanceUpdation{
		addr: &s.address,
		id:   hash,
	})

	key := types.BytesToHex(dhruva.BalanceObjectKey(s.address, hash))
	s.dirtyEntries[key] = data
	s.data.Balance = hash

	return hash.Bytes(), nil
}

func (s *StateObject) commitAccount() (types.Hash, error) {
	s.data.Nonce++

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
	s.dirtyEntries[key] = data

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
	s.dirtyEntries[key] = rawData

	return hash, nil
}

func (s *StateObject) commitStorage() (types.Hash, error) {
	if s.metaStorageTree == nil {
		return s.data.StorageRoot, nil
	}
	// Add the updated logic-id <=> storage-root in master storage merkleTree
	for logicID, merkleTree := range s.activeStorageTrees {
		if !merkleTree.IsDirty() {
			continue
		}

		if err := merkleTree.Commit(); err != nil {
			return types.NilHash, errors.Wrap(err, "failed to commit storage tree")
		}

		rootHash, err := merkleTree.Root()
		if err != nil {
			return types.NilHash, err
		}

		if err = s.metaStorageTree.Set(types.FromHex(logicID), rootHash.Bytes()); err != nil {
			return types.NilHash, err
		}
	}

	if !s.metaStorageTree.IsDirty() {
		return s.data.StorageRoot, nil
	}

	if err := s.metaStorageTree.Commit(); err != nil {
		return types.NilHash, err
	}

	rootHash, err := s.metaStorageTree.Root()
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

// commitLogics commits and the logic tree and flushes the changes to db
func (s *StateObject) commitLogics() (types.Hash, error) {
	if s.logicTree == nil {
		return s.data.LogicRoot, nil
	}

	err := s.logicTree.Commit()
	if err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	s.data.LogicRoot, err = s.logicTree.Root()
	if err != nil {
		return types.NilHash, err
	}

	return s.data.LogicRoot, nil
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
			return errors.Wrap(err, "failed to commit modified storage tree entries to db")
		}
	}

	// flush master storage trees
	return s.metaStorageTree.Flush()
}

func (s *StateObject) Commit() (types.Hash, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, err := s.commitBalanceObject(); err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit balance object")
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

func (s *StateObject) CreateStorageTreeForLogic(logicID types.LogicID) error {
	_, err := s.createStorageTreeForLogic(logicID)

	return err
}

func (s *StateObject) CreateAsset(descriptor *types.AssetDescriptor) (types.AssetID, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	descriptor.Owner = s.address

	assetID, assetHash, data, err := gtypes.GetAssetID(descriptor)
	if err != nil {
		return "", errors.Wrap(err, "failed to polorize asset data")
	}

	s.journal.append(AssetCreation{
		addr: &s.address,
		id:   assetHash,
	})

	key := assetHash.String()
	s.dirtyEntries[key] = data

	// Update the balance
	if _, ok := s.balance.Balances[assetID]; ok {
		return "", errors.Wrap(types.ErrAssetCreation, "asset already exists")
	}

	s.balance.Balances[assetID] = descriptor.Supply

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

	return s.SetStorageEntry(SargaLogicID, address.Bytes(), rawData)
}

func (s *StateObject) CreateContext(behaviouralNodes, randomNodes []id.KramaID) (types.Hash, error) {
	if len(behaviouralNodes)+len(randomNodes) < minimumContextSize {
		return types.NilHash, errors.New("livness size not met")
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
	s.mtx.Lock()
	defer s.mtx.Unlock()

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
			return nil, errors.Wrap(types.ErrUpdatingContextObject, err.Error())
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

func getBalanceObject(
	addr types.Address,
	hash types.Hash,
	db *dhruva.PersistenceManager,
) (*gtypes.BalanceObject, error) {
	data, err := db.GetBalance(addr, hash)
	if err != nil {
		return nil, err
	}

	balObject := new(gtypes.BalanceObject)

	if err := balObject.FromBytes(data); err != nil {
		return nil, err
	}

	return balObject, nil
}

func (s *StateObject) GetStorageTree(logicID types.LogicID) (tree.MerkleTree, error) {
	storageTree, ok := s.activeStorageTrees[logicID.Hex()]
	if ok {
		return storageTree, nil
	}

	metaStorageTree, err := s.getMetaStorageTree()
	if err != nil {
		return nil, err
	}

	root, err := metaStorageTree.Get(logicID)
	if err != nil {
		return nil, types.ErrLogicStorageTreeNotFound
	}

	storageTree, err = tree.NewKramaHashTree(s.address, types.BytesToHash(root), s.db, blakeHasher, dhruva.Storage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic storage tree")
	}

	s.activeStorageTrees[logicID.Hex()] = storageTree

	return storageTree, nil
}

func (s *StateObject) SetStorageEntry(logicID types.LogicID, key, value []byte) (err error) {
	merkleTree, ok := s.activeStorageTrees[logicID.Hex()]
	if ok {
		return merkleTree.Set(key, value)
	}

	merkleTree, err = s.GetStorageTree(logicID)
	if err != nil {
		return err
	}

	return merkleTree.Set(key, value)
}

func (s *StateObject) GetStorageEntry(logicID types.LogicID, key []byte) (value []byte, err error) {
	merkleTree, ok := s.activeStorageTrees[logicID.Hex()]
	if ok {
		return merkleTree.Get(key)
	}

	merkleTree, err = s.GetStorageTree(logicID)
	if err != nil {
		return nil, err
	}

	return merkleTree.Get(key)
}

func (s *StateObject) GetDirtyStorage() Storage {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.dirtyEntries
}

func (s *StateObject) getMetaStorageTree() (tree.MerkleTree, error) {
	if s.metaStorageTree != nil {
		return s.metaStorageTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(s.address, s.data.StorageRoot, s.db, blakeHasher, dhruva.Storage)
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

	newStorageTree, err := tree.NewKramaHashTree(s.address, types.NilHash, s.db, blakeHasher, dhruva.Storage)
	if err != nil {
		return nil, err
	}

	s.activeStorageTrees[logicID.Hex()] = newStorageTree

	return newStorageTree, s.metaStorageTree.Set(logicID, types.NilHash.Bytes())
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

	rawObject, err := logicTree.Get(logicID)
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

	if err = logicTree.Set(logicID, rawLogicObject); err != nil {
		return errors.Wrap(err, "failed to add logic object to tree")
	}

	return nil
}

// FetchLogicObject returns the LogicObject associated with the given logicID,
// This returns an error if the logicID is not registered
func (s *StateObject) FetchLogicObject(logicID types.LogicID) (*gtypes.LogicObject, error) {
	return s.getLogicObject(logicID)
}
