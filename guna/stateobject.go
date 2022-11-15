package guna

import (
	"log"
	"math/big"
	"sync"

	"github.com/decred/dcrd/crypto/blake256"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/dhruva"
	"gitlab.com/sarvalabs/moichain/guna/tree"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/types"
	"gitlab.com/sarvalabs/polo/go-polo"
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
	address        types.Address
	accType        types.AccType
	journal        *Journal
	mtx            sync.RWMutex
	cache          *lru.Cache
	db             *dhruva.PersistenceManager
	data           types.Account
	balance        *types.BalanceObject
	assetApprovals *types.ApprovalObject

	logicTrie   tree.MerkleTree //nolint
	storageTrie tree.MerkleTree //nolint
	fileTrie    tree.MerkleTree //nolint

	dirtyEntries Storage
	receipts     types.Receipts

	activeStorageTrees map[types.LogicID]tree.MerkleTree

	logics map[types.Hash]*types.LogicData
	files  map[types.Hash][]byte
}

func NewStateObject(
	id types.Address,
	cache *lru.Cache,
	j *Journal,
	db *dhruva.PersistenceManager,
	account types.Account,
	accType types.AccType,
) *StateObject {
	s := &StateObject{
		journal: j,
		accType: accType,
		cache:   cache,
		db:      db,
		data:    account,
		address: id,
		balance: &types.BalanceObject{
			Bal:     make(types.AssetMap),
			PrvHash: types.NilHash,
		},
		assetApprovals: &types.ApprovalObject{
			Approvals: make(map[types.Address]types.AssetMap),
			PrvHash:   types.NilHash,
		},
		logics:             make(map[types.Hash]*types.LogicData),
		files:              make(map[types.Hash][]byte),
		dirtyEntries:       make(Storage),
		receipts:           make(types.Receipts),
		activeStorageTrees: make(map[types.LogicID]tree.MerkleTree, 4),
	}

	return s
}

func (s *StateObject) AccountState() types.Account {
	return s.data
}

func (s *StateObject) BalanceOf(id types.AssetID) (*big.Int, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if v, ok := s.balance.Bal[id]; ok {
		return v, nil
	}

	return nil, types.ErrAssetNotFound
}

func (s *StateObject) AddBalance(aid types.AssetID, amount *big.Int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	bal, ok := s.balance.Bal[aid]
	if ok {
		s.setBalance(aid, new(big.Int).Add(amount, bal))
	} else {
		s.balance.Bal[aid] = amount
	}
}

func (s *StateObject) SubBalance(aid types.AssetID, amount *big.Int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if bal, ok := s.balance.Bal[aid]; ok && bal != nil {
		s.setBalance(aid, new(big.Int).Sub(bal, amount))
	} else {
		log.Panicln("asset not found")
	}
}

func (s *StateObject) setBalance(aid types.AssetID, amount *big.Int) {
	s.balance.Bal[aid] = amount
}

func (s *StateObject) GetLogic(logicID types.Hash) (data *types.LogicData, err error) {
	if (logicID == types.Hash{}) {
		return nil, errors.New("invalid logicID")
	}

	ld, ok := s.logics[logicID]
	if !ok {
		data, err := s.getLogicTrie(s.db, s.data.LogicRoot.Bytes()).Get(logicID.Bytes())
		if err != nil {
			return nil, err
		}

		if data != nil {
			msg := new(types.LogicData)
			if err := polo.Depolorize(msg, data); err != nil {
				log.Fatal(err)
			}

			return msg, nil
		}
	}

	return ld, nil
}

func (s *StateObject) GetAccountType() types.AccType {
	return s.accType
}

func (s *StateObject) Copy() *StateObject {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	j := new(Journal)
	sObj := NewStateObject(s.address, s.cache, j, s.db, s.data, s.data.AccType)

	sObj.balance = s.balance.Copy()
	sObj.assetApprovals = s.assetApprovals.Copy()
	sObj.dirtyEntries = s.dirtyEntries.Copy()
	sObj.data = s.data

	if s.storageTrie != nil {
		sObj.storageTrie = s.storageTrie.Copy() // TODO: Check if we require deep copy
	}
	// sObj.storageTrie = s.storageTrie.Copy()
	// sObj.logicTrie = s.logicTrie.Copy()
	// sObj.fileTrie = s.fileTrie.Copy()

	for k, v := range sObj.logics {
		sObj.logics[k] = v
	}

	for k, v := range sObj.files {
		sObj.files[k] = v
	}

	return sObj
}

func (s *StateObject) commitBalanceObject() ([]byte, error) {
	data := polo.Polorize(s.balance)
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

	data := polo.Polorize(s.data)
	hash := types.GetHash(data)

	s.journal.append(AccountUpdation{
		addr: &s.address,
		id:   hash,
	})

	key := types.BytesToHex(dhruva.AccountKey(s.address, hash))
	s.dirtyEntries[key] = data

	return hash, nil
}

func (s *StateObject) commitContextObject(obj interface{}) (types.Hash, error) {
	// Add type checks here
	data := polo.Polorize(obj)
	hash := types.GetHash(data)

	s.journal.append(ContextUpdation{
		addr: &s.address,
		id:   hash,
	})

	key := types.BytesToHex(dhruva.ContextObjectKey(s.address, hash))
	s.dirtyEntries[key] = data

	return hash, nil
}

func (s *StateObject) commitStorage() (types.Hash, error) {
	if s.storageTrie == nil {
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

		if err := s.storageTrie.Set(logicID.Bytes(), merkleTree.Root().Bytes()); err != nil {
			return types.NilHash, err
		}
	}

	if !s.storageTrie.IsDirty() {
		return s.data.StorageRoot, nil
	}

	if err := s.storageTrie.Commit(); err != nil {
		return types.NilHash, err
	}

	rootHash := s.storageTrie.Root()

	s.journal.append(StorageUpdation{
		addr: &s.address,
		id:   rootHash,
	})

	s.data.StorageRoot = rootHash

	return rootHash, nil
}

func (s *StateObject) CommitActiveStorageTreesToDB() error {
	if s.storageTrie == nil {
		return nil
	}

	// commit active storage trees
	for _, storageTree := range s.activeStorageTrees {
		if err := storageTree.Flush(); err != nil {
			return errors.Wrap(err, "failed to commit modified storage tree entries to db")
		}
	}

	// commit master storage trees
	return s.storageTrie.Flush()
}

func (s *StateObject) Commit() (types.Hash, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, err := s.commitBalanceObject(); err != nil {
		return types.NilHash, errors.Wrap(err, "failed to commit balance object")
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

func (s *StateObject) CreateStorageTreeForLogic(logicID types.LogicID) (tree.MerkleTree, error) {
	if s.storageTrie == nil {
		merkleTree, err := tree.NewKramaHashTree(s.address, s.data.StorageRoot, s.db, blake256.New())
		if err != nil {
			return nil, errors.Wrap(err, "failed to initiate storage tree")
		}

		s.storageTrie = merkleTree
	}

	newStorageTree, err := tree.NewKramaHashTree(s.address, types.NilHash, s.db, blakeHasher)
	if err != nil {
		return nil, err
	}

	s.activeStorageTrees[logicID] = newStorageTree

	return newStorageTree, s.storageTrie.Set(logicID.Bytes(), types.NilHash.Bytes())
}

func (s *StateObject) CreateLogic(logicID types.Hash, data *types.LogicData) error {
	if _, ok := s.logics[logicID]; !ok {
		// TODO:journal this
		s.logics[logicID] = data
	} else {
		return errors.New("duplicate logic")
	}

	return nil
}

func (s *StateObject) CreateAsset(
	dimension uint8,
	isFungible bool,
	isMintable bool,
	symbol string,
	totalSupply int64,
	code []byte,
) (types.AssetID, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var (
		logicID types.Hash
		err     error
	)

	if code != nil {
		logicID, data := types.GetLogicID(code, false)

		if err = s.CreateLogic(logicID, data); err != nil {
			return "", err
		}
	}

	assetID, assetHash, data := types.GetAssetID(
		s.address,
		dimension,
		isFungible,
		isMintable,
		symbol,
		totalSupply,
		logicID,
	)

	s.journal.append(AssetCreation{
		addr: &s.address,
		id:   assetHash,
	})

	key := assetHash.String()
	s.dirtyEntries[key] = data

	// Update the balance
	if _, ok := s.balance.Bal[assetID]; !ok {
		s.balance.Bal[assetID] = big.NewInt(totalSupply)

		return assetID, nil
	}

	return "", errors.Wrap(types.ErrAssetCreation, "asset already exists")
}

func (s *StateObject) AddAccountGenesisInfo(address types.Address, ixHash types.Hash) error {
	accInfo := types.AccountGenesisInfo{
		IxHash: ixHash,
	}
	rawData := polo.Polorize(&accInfo)

	return s.SetStorageEntry(GenesisLogicID, address.Bytes(), rawData)
}

func (s *StateObject) CreateContext(behaviouralNodes, randomNodes []id.KramaID) (types.Hash, error) {
	if len(behaviouralNodes)+len(randomNodes) < minimumContextSize {
		return types.NilHash, errors.New("livness size not met")
	}

	behaviouralContextObject := new(types.ContextObject)
	randomContextObject := new(types.ContextObject)
	metaContextObject := new(types.MetaContextObject)

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

		behaviouralObj.AddNodes(behaviouralNodes, types.MaxBehaviourContextSize)

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

		randomObj.AddNodes(randomNodes, types.MaxRandomContextSize)

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

func (s *StateObject) getMetaContextObjectCopy() (*types.MetaContextObject, error) {
	data, isAvailable := s.cache.Get(s.ContextHash())
	if isAvailable {
		metaContextObject, ok := data.(*types.MetaContextObject)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return metaContextObject.Copy(), nil
	}

	rawData, err := s.db.GetContext(s.address, s.ContextHash())
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch meta context object")
	}

	obj := new(types.MetaContextObject)

	if err := polo.Depolorize(obj, rawData); err != nil {
		return nil, err
	}

	s.cache.Add(s.ContextHash(), obj)

	return obj.Copy(), nil
}

func (s *StateObject) getContextObjectCopy(hash types.Hash) (*types.ContextObject, error) {
	data, isAvailable := s.cache.Get(hash)
	if !isAvailable {
		rawData, err := s.db.GetContext(s.address, hash)
		if err != nil {
			return nil, errors.Wrap(types.ErrUpdatingContextObject, err.Error())
		}

		obj := new(types.ContextObject)

		if err := polo.Depolorize(obj, rawData); err != nil {
			return nil, err
		}

		s.cache.Add(hash, obj)

		return obj.Copy(), nil
	}

	contextObject, ok := data.(*types.ContextObject)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return contextObject.Copy(), nil
}

func getBalanceObject(
	addr types.Address,
	hash types.Hash,
	db *dhruva.PersistenceManager,
) (*types.BalanceObject, error) {
	data, err := db.GetBalance(addr, hash)
	if err != nil {
		return nil, err
	}

	balObject := new(types.BalanceObject)

	if err = polo.Depolorize(balObject, data); err != nil {
		return nil, err
	}

	return balObject, nil
}

func (s *StateObject) getLogicTrie(db *dhruva.PersistenceManager, root []byte) tree.MerkleTree {
	return nil
}

func (s *StateObject) GetStorageTree(logicID types.LogicID) (tree.MerkleTree, error) {
	storageTree, ok := s.activeStorageTrees[logicID]
	if ok {
		return storageTree, nil
	}

	rawID := logicID.Bytes()
	if rawID == nil {
		return nil, types.ErrInvalidLogicID
	}

	if s.storageTrie == nil {
		merkleTree, err := tree.NewKramaHashTree(s.address, s.data.StorageRoot, s.db, blake256.New())
		if err != nil {
			return nil, errors.Wrap(err, "failed to initiate storage tree")
		}

		s.storageTrie = merkleTree
	}

	root, err := s.storageTrie.Get(rawID)
	if err != nil {
		return nil, types.ErrLogicStorageTreeNotFound
	}

	storageTree, err = tree.NewKramaHashTree(s.address, types.BytesToHash(root), s.db, blakeHasher)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic storage tree")
	}

	s.activeStorageTrees[logicID] = storageTree

	return storageTree, nil
}

func (s *StateObject) SetStorageEntry(logicID types.LogicID, key, value []byte) (err error) {
	merkleTree, ok := s.activeStorageTrees[logicID]
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
	merkleTree, ok := s.activeStorageTrees[logicID]
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
