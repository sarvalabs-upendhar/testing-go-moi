package guna

import (
	"log"
	"math/big"
	"sync"

	"github.com/pkg/errors"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"

	lru "github.com/hashicorp/golang-lru"
	"gitlab.com/sarvalabs/moichain/dhruva"
	"gitlab.com/sarvalabs/moichain/types"
)

type Storage map[string][]byte

func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}

type StateObject struct {
	Address        types.Address
	accType        types.AccType
	journal        *Journal
	mtx            sync.RWMutex
	cache          *lru.Cache
	db             *dhruva.PersistenceManager
	data           types.Account
	contextHash    types.Hash
	balance        *types.BalanceObject
	assetApprovals *types.ApprovalObject

	storageTrie Trie //nolint
	fileTrie    Trie //nolint

	dirtyEntries Storage
	receipts     types.Receipts

	logics  map[types.Hash]*types.LogicData
	Storage map[types.Hash][]byte
	files   map[types.Hash][]byte
}

func NewStateObject(
	id types.Address,
	cache *lru.Cache,
	j *Journal,
	db *dhruva.PersistenceManager,
	accType types.AccType,
) *StateObject {
	s := &StateObject{
		journal:     j,
		accType:     accType,
		cache:       cache,
		db:          db,
		Address:     id,
		contextHash: types.NilHash,
		balance: &types.BalanceObject{
			Bal:     make(types.AssetMap),
			PrvHash: types.NilHash,
		},
		assetApprovals: &types.ApprovalObject{
			Approvals: make(map[types.Address]types.AssetMap),
			PrvHash:   types.NilHash,
		},
		logics:       make(map[types.Hash]*types.LogicData),
		Storage:      make(map[types.Hash][]byte),
		files:        make(map[types.Hash][]byte),
		dirtyEntries: make(Storage),
		receipts:     make(types.Receipts),
	}

	return s
}

func (s *StateObject) getLogicTrie(db *dhruva.PersistenceManager, root []byte) Trie {
	return nil
}

func (s *StateObject) BalanceOf(id types.AssetID) (*big.Int, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if v, ok := s.balance.Bal[id]; ok {
		return v, nil
	} else {
		return nil, types.ErrAssetNotFound
	}
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
		log.Fatal("asset not found")
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
		data, err := s.getLogicTrie(s.db, s.data.LogicRoot.Bytes()).TryGet(logicID.Bytes())
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
	sObj := NewStateObject(s.Address, s.cache, j, s.db, s.data.AccType)

	sObj.balance = s.balance.Copy()
	sObj.assetApprovals = s.assetApprovals.Copy()
	sObj.dirtyEntries = s.dirtyEntries.Copy()
	sObj.data = s.data
	sObj.contextHash = s.contextHash
	// sObj.storageTrie = s.storageTrie.Copy()
	// sObj.logicTrie = s.logicTrie.Copy()
	// sObj.fileTrie = s.fileTrie.Copy()

	for k, v := range sObj.logics {
		sObj.logics[k] = v
	}

	for k, v := range sObj.files {
		sObj.files[k] = v
	}

	for k, v := range sObj.Storage {
		sObj.Storage[k] = v
	}

	return sObj
}

func (s *StateObject) commitBalanceObject() ([]byte, error) {
	data := polo.Polorize(s.balance)
	cID := types.GetHash(data)

	s.journal.append(BalanceUpdation{
		addr: &s.Address,
		id:   cID,
	})

	key := types.BytesToHex(types.DBKey(s.Address, types.BalanceGID, cID))
	s.dirtyEntries[key] = data
	s.data.Balance = cID

	return cID.Bytes(), nil
}

func (s *StateObject) commitAccount() (types.Hash, error) {
	s.data.Nonce++
	s.data.ContextHash = s.contextHash // this is redundant

	data := polo.Polorize(s.data)
	cID := types.GetHash(data)

	s.journal.append(AccountUpdation{
		addr: &s.Address,
		id:   cID,
	})

	key := types.BytesToHex(types.DBKey(s.Address, types.AccountGID, cID))
	s.dirtyEntries[key] = data

	return cID, nil
}

func (s *StateObject) commitStorage() (types.Hash, error) {
	data := polo.Polorize(s.Storage)
	cID := types.GetHash(data)

	s.journal.append(StorageUpdation{
		addr: &s.Address,
		id:   cID,
	})

	key := types.BytesToHex(types.DBKey(s.Address, types.StorageGID, cID))
	s.dirtyEntries[key] = data
	s.data.StorageRoot = cID

	return cID, nil
}

func (s *StateObject) Commit() (types.Hash, error) {
	// for k, v := range s.logics {
	// 	protoData := v.Proto()
	// 	data, err := proto.Marshal(protoData)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	s.logicTrie.Put(k.Bytes(), data)
	// }
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, err := s.commitBalanceObject(); err != nil {
		return types.NilHash, errors.Wrap(types.ErrCommitFailed, err.Error())
	}

	if _, err := s.commitStorage(); err != nil {
		return types.NilHash, errors.Wrap(types.ErrCommitFailed, err.Error())
	}

	accCid, err := s.commitAccount()
	if err != nil {
		return types.NilHash, errors.Wrap(types.ErrCommitFailed, err.Error())
	}

	return accCid, nil
}

func (s *StateObject) CreateLogic(logicID types.Hash, data *types.LogicData) error {
	if _, ok := s.logics[logicID]; !ok {
		//data, err := s.getLogicTrie(s.db, s.data.LogicRoot.Bytes()).TryGet(logicID.Bytes())
		//if err != nil {
		//	return err
		//}
		//TODO:journal this
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
		s.Address,
		dimension,
		isFungible,
		isMintable,
		symbol,
		totalSupply,
		logicID,
	)

	s.journal.append(AssetCreation{
		addr: &s.Address,
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

func CopyBytes(b []byte) (copiedBytes []byte) {
	if b == nil {
		return nil
	}

	copiedBytes = make([]byte, len(b))

	copy(copiedBytes, b)

	return
}

func (s *StateObject) AddAccountGenesisInfo(address types.Address, ixHash types.Hash) {
	accInfo := types.AccountGenesisInfo{
		IxHash: ixHash,
	}
	rawData := polo.Polorize(&accInfo)

	s.Storage[types.GetHash(address.Bytes())] = rawData
}

func (s *StateObject) commitContextObject(obj interface{}) (types.Hash, error) {
	// Add type checks here
	data := polo.Polorize(obj)
	hash := types.GetHash(data)

	s.journal.append(ContextUpdation{
		addr: &s.Address,
		id:   hash,
	})

	key := types.BytesToHex(types.DBKey(s.Address, types.ContextGID, hash))
	s.dirtyEntries[key] = data
	// s.cache.Add(hash, obj)

	return hash, nil
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

	s.contextHash = mHash

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
	metaObj.PreviousHash = s.contextHash

	if len(behaviouralNodes) > 0 {
		// log.Println("!!!!...Adding behaviour context...!!!", behaviouralNodes, s.contextHash)
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
		//.Println("!!!!...Adding random context...!!!", randomNodes, s.contextHash)
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

	if behaviourObjectHash != types.NilHash {
		metaObj.BehaviouralContext = behaviourObjectHash
	}

	if randomObjectHash != types.NilHash {
		metaObj.RandomContext = randomObjectHash
	}

	contextHash, err := s.commitContextObject(metaObj)
	if err != nil {
		return types.NilHash, err
	}

	s.contextHash = contextHash
	s.data.ContextHash = contextHash

	return contextHash, nil
}

func (s *StateObject) GetContextHash() types.Hash {
	return s.contextHash
}

func (s *StateObject) SetStorageEntry(key types.Hash, value []byte) {
	s.Storage[key] = value
}

func (s *StateObject) GetStorageEntry(key types.Hash) ([]byte, error) {
	value, ok := s.Storage[key]
	if !ok {
		return nil, types.ErrStorageEntryNotFound
	}

	return value, nil
}

func (s *StateObject) GetDirtyStorage() Storage {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.dirtyEntries
}

func (s *StateObject) getMetaContextObjectCopy() (*types.MetaContextObject, error) {
	data, isAvailable := s.cache.Get(s.contextHash)
	if isAvailable {
		metaContextObject, ok := data.(*types.MetaContextObject)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return metaContextObject.Copy(), nil
	}

	rawData, err := s.db.GetContext(s.Address, s.contextHash)
	if err != nil {
		return nil, errors.Wrap(types.ErrUpdatingContextObject, err.Error())
	}

	obj := new(types.MetaContextObject)

	if err := polo.Depolorize(obj, rawData); err != nil {
		return nil, err
	}

	s.cache.Add(s.contextHash, obj)

	return obj.Copy(), nil
}

func (s *StateObject) getContextObjectCopy(hash types.Hash) (*types.ContextObject, error) {
	data, isAvailable := s.cache.Get(hash)
	if !isAvailable {
		rawData, err := s.db.GetContext(s.Address, hash)
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

func getStorage(addr types.Address, hash types.Hash, db *dhruva.PersistenceManager) (map[types.Hash][]byte, error) {
	data, err := db.GetStorage(addr, hash)
	if err != nil {
		return nil, err
	}

	storageEntries := make(map[types.Hash][]byte)

	if err = polo.Depolorize(&storageEntries, data); err != nil {
		return nil, err
	}

	return storageEntries, nil
}
