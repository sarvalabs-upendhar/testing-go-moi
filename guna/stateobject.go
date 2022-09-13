package guna

import (
	"github.com/pkg/errors"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"log"
	"math/big"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/dhruva"
)

type Storage map[ktypes.Hash][]byte

func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}

type StateObject struct {
	Address        ktypes.Address
	accType        ktypes.AccType
	journal        *journal
	mtx            sync.RWMutex
	cache          *lru.Cache
	db             *dhruva.PersistenceManager
	data           ktypes.Account
	contextHash    ktypes.Hash
	balance        *ktypes.BalanceObject
	assetApprovals *ktypes.ApprovalObject

	logicTrie   Trie
	storageTrie Trie // nolint
	fileTrie    Trie // nolint

	dirtyEntries Storage
	receipts     ktypes.Receipts

	logics  map[ktypes.Hash]*ktypes.LogicData
	storage map[ktypes.Hash][]byte
	files   map[ktypes.Hash][]byte
}

func NewStateObject(
	id ktypes.Address,
	cache *lru.Cache,
	j *journal,
	db *dhruva.PersistenceManager,
	accType ktypes.AccType,
) *StateObject {
	s := &StateObject{
		journal:     j,
		accType:     accType,
		cache:       cache,
		db:          db,
		Address:     id,
		contextHash: ktypes.NilHash,
		balance: &ktypes.BalanceObject{
			Bal:     make(ktypes.AssetMap),
			PrvHash: ktypes.NilHash,
		},
		assetApprovals: &ktypes.ApprovalObject{
			Approvals: make(map[ktypes.Address]ktypes.AssetMap),
			PrvHash:   ktypes.NilHash,
		},
		logics:       make(map[ktypes.Hash]*ktypes.LogicData),
		storage:      make(map[ktypes.Hash][]byte),
		files:        make(map[ktypes.Hash][]byte),
		dirtyEntries: make(Storage),
		receipts:     make(ktypes.Receipts),
	}

	return s
}

func (s *StateObject) getLogicTrie(db *dhruva.PersistenceManager, root []byte) Trie {
	if s.logicTrie == nil {
		return NewSmtTrie(root, db)
	}

	return s.logicTrie
}

func (s *StateObject) BalanceOf(id ktypes.AssetID) (*big.Int, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if v, ok := s.balance.Bal[id]; ok {
		return v, nil
	} else {
		return nil, ktypes.ErrAssetNotFound
	}
}
func (s *StateObject) AddBalance(aid ktypes.AssetID, amount *big.Int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	bal, ok := s.balance.Bal[aid]
	if ok {
		s.setBalance(aid, new(big.Int).Add(amount, bal))
	} else {
		s.balance.Bal[aid] = amount
	}
}
func (s *StateObject) SubBalance(aid ktypes.AssetID, amount *big.Int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if bal, ok := s.balance.Bal[aid]; ok && bal != nil {
		s.setBalance(aid, new(big.Int).Sub(bal, amount))
	} else {
		log.Fatal("asset not found")
	}
}
func (s *StateObject) setBalance(aid ktypes.AssetID, amount *big.Int) {
	s.balance.Bal[aid] = amount
}
func (s *StateObject) GetLogic(logicID ktypes.Hash) (data *ktypes.LogicData, err error) {
	if (logicID == ktypes.Hash{}) {
		return nil, errors.New("invalid logicID")
	}

	ld, ok := s.logics[logicID]
	if !ok {
		data, err := s.getLogicTrie(s.db, s.data.LogicRoot.Bytes()).TryGet(logicID.Bytes())
		if err != nil {
			return nil, err
		}

		if data != nil {
			msg := new(ktypes.LogicData)
			if err := polo.Depolorize(msg, data); err != nil {
				log.Fatal(err)
			}

			return msg, nil
		}
	}

	return ld, nil
}
func (s *StateObject) GetAccountType() ktypes.AccType {
	return s.accType
}
func (s *StateObject) Copy() *StateObject {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	j := new(journal)
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

	for k, v := range sObj.storage {
		sObj.storage[k] = v
	}

	return sObj
}
func (s *StateObject) commitBalanceObject() ([]byte, error) {
	data := polo.Polorize(s.balance)
	cID := ktypes.GetHash(data)

	s.journal.append(BalanceUpdation{
		addr: &s.Address,
		id:   cID,
	})

	s.dirtyEntries[cID] = data
	s.data.Balance = cID

	return cID.Bytes(), nil
}
func (s *StateObject) commitAccount() (ktypes.Hash, error) {
	s.data.Nonce++
	s.data.ContextHash = s.contextHash // this is redundant

	data := polo.Polorize(s.data)
	cID := ktypes.GetHash(data)

	s.journal.append(AccountUpdation{
		addr: &s.Address,
		id:   cID,
	})

	s.dirtyEntries[cID] = data

	return cID, nil
}

func (s *StateObject) commitStorage() (ktypes.Hash, error) {
	data := polo.Polorize(s.storage)
	cID := ktypes.GetHash(data)

	s.journal.append(StorageUpdation{
		addr: &s.Address,
		id:   cID,
	})

	s.dirtyEntries[cID] = data
	s.data.StorageRoot = cID

	return cID, nil
}

func (s *StateObject) Commit() (ktypes.Hash, error) {
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
		return ktypes.NilHash, errors.Wrap(ktypes.ErrCommitFailed, err.Error())
	}

	if _, err := s.commitStorage(); err != nil {
		return ktypes.NilHash, errors.Wrap(ktypes.ErrCommitFailed, err.Error())
	}

	accCid, err := s.commitAccount()
	if err != nil {
		return ktypes.NilHash, errors.Wrap(ktypes.ErrCommitFailed, err.Error())
	}

	return accCid, nil
}

func (s *StateObject) CreateLogic(logicID ktypes.Hash, data *ktypes.LogicData) error {
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
) (ktypes.AssetID, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var (
		logicID ktypes.Hash
		err     error
	)

	if code != nil {
		logicID, data := ktypes.GetLogicID(code, false)

		if err = s.CreateLogic(logicID, data); err != nil {
			return "", err
		}
	}

	assetID, assetHash, data := ktypes.GetAssetID(
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

	s.dirtyEntries[assetHash] = data

	// Update the balance
	if _, ok := s.balance.Bal[assetID]; !ok {
		s.balance.Bal[assetID] = big.NewInt(totalSupply)

		return assetID, nil
	}

	return "", errors.Wrap(ktypes.ErrAssetCreation, "asset already exists")
}

func CopyBytes(b []byte) (copiedBytes []byte) {
	if b == nil {
		return nil
	}

	copiedBytes = make([]byte, len(b))

	copy(copiedBytes, b)

	return
}

func (s *StateObject) AddAccountGenesisInfo(address ktypes.Address, ixHash ktypes.Hash) {
	accInfo := ktypes.AccountGenesisInfo{
		IxHash: ixHash,
	}
	rawData := polo.Polorize(&accInfo)

	s.storage[ktypes.GetHash(address.Bytes())] = rawData
}

func (s *StateObject) commitContextObject(obj interface{}) (ktypes.Hash, error) {
	// Add type checks here
	data := polo.Polorize(obj)
	hash := ktypes.GetHash(data)

	s.journal.append(ContextUpdation{
		addr: &s.Address,
		id:   hash,
	})

	s.dirtyEntries[hash] = data
	//s.cache.Add(hash, obj)

	return hash, nil
}

func (s *StateObject) CreateContext(behaviouralNodes, randomNodes []id.KramaID) (ktypes.Hash, error) {
	if len(behaviouralNodes)+len(randomNodes) < minimumContextSize {
		return ktypes.NilHash, errors.New("livness size not met")
	}

	behaviouralContextObject := new(ktypes.ContextObject)
	randomContextObject := new(ktypes.ContextObject)
	metaContextObject := new(ktypes.MetaContextObject)

	behaviouralContextObject.Ids = append(behaviouralContextObject.Ids, behaviouralNodes...)
	randomContextObject.Ids = append(randomContextObject.Ids, randomNodes...)

	bHash, err := s.commitContextObject(behaviouralContextObject)
	if err != nil {
		return ktypes.NilHash, errors.Wrap(ktypes.ErrContextCreation, err.Error())
	}

	rHash, err := s.commitContextObject(randomContextObject)
	if err != nil {
		return ktypes.NilHash, errors.Wrap(ktypes.ErrContextCreation, err.Error())
	}

	metaContextObject.BehaviouralContext = bHash
	metaContextObject.RandomContext = rHash
	metaContextObject.PreviousHash = ktypes.NilHash

	mHash, err := s.commitContextObject(metaContextObject)
	if err != nil {
		return ktypes.NilHash, errors.Wrap(ktypes.ErrContextCreation, err.Error())
	}

	//TODO:journal this
	s.cache.Add(bHash, behaviouralContextObject)
	s.cache.Add(mHash, metaContextObject)
	s.cache.Add(rHash, randomContextObject)

	s.contextHash = mHash

	return mHash, nil
}
func (s *StateObject) UpdateContext(behaviouralNodes []id.KramaID, randomNodes []id.KramaID) (ktypes.Hash, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var (
		err                 error
		behaviourObjectHash ktypes.Hash
		randomObjectHash    ktypes.Hash
	)

	metaObj, err := s.getMetaContextObjectCopy()
	if err != nil {
		return ktypes.NilHash, err
	}
	// Set the previous Hash
	metaObj.PreviousHash = s.contextHash

	if len(behaviouralNodes) > 0 {
		//log.Println("!!!!...Adding behaviour context...!!!", behaviouralNodes, s.contextHash)
		behaviouralObj, err := s.getContextObjectCopy(metaObj.BehaviouralContext)
		if err != nil {
			return ktypes.NilHash, err
		}

		behaviouralObj.AddNodes(behaviouralNodes, ktypes.MaxBehaviourContextSize)

		behaviourObjectHash, err = s.commitContextObject(behaviouralObj)
		if err != nil {
			return ktypes.NilHash, err
		}
	}

	if len(randomNodes) > 0 {
		//.Println("!!!!...Adding random context...!!!", randomNodes, s.contextHash)
		randomObj, err := s.getContextObjectCopy(metaObj.RandomContext)
		if err != nil {
			return ktypes.NilHash, err
		}

		randomObj.AddNodes(randomNodes, ktypes.MaxRandomContextSize)

		//TODO:Sort based on the stake of the nodes

		randomObjectHash, err = s.commitContextObject(randomObj)
		if err != nil {
			return ktypes.NilHash, err
		}
	}

	//TODO:Sort based on the stake of the nodes

	if behaviourObjectHash != ktypes.NilHash {
		metaObj.BehaviouralContext = behaviourObjectHash
	}

	if randomObjectHash != ktypes.NilHash {
		metaObj.RandomContext = randomObjectHash
	}

	contextHash, err := s.commitContextObject(metaObj)
	if err != nil {
		return ktypes.NilHash, err
	}

	s.contextHash = contextHash
	s.data.ContextHash = contextHash

	return contextHash, nil
}

func (s *StateObject) GetContextHash() ktypes.Hash {
	return s.contextHash
}

func (s *StateObject) SetStorageEntry(key ktypes.Hash, value []byte) {
	s.storage[key] = value
}

func (s *StateObject) GetStorageEntry(key ktypes.Hash) ([]byte, error) {
	value, ok := s.storage[key]
	if !ok {
		return nil, ktypes.ErrStorageEntryNotFound
	}

	return value, nil
}

func (s *StateObject) GetDirtyStorage() Storage {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.dirtyEntries
}

func (s *StateObject) getMetaContextObjectCopy() (*ktypes.MetaContextObject, error) {
	data, isAvailable := s.cache.Get(s.contextHash)
	if isAvailable {
		metaContextObject, ok := data.(*ktypes.MetaContextObject)
		if !ok {
			return nil, ktypes.ErrInterfaceConversion
		}

		return metaContextObject.Copy(), nil
	}

	rawData, err := s.db.ReadEntry(s.contextHash.Bytes())
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrUpdatingContextObject, err.Error())
	}

	obj := new(ktypes.MetaContextObject)

	if err := polo.Depolorize(obj, rawData); err != nil {
		return nil, err
	}

	s.cache.Add(s.contextHash, obj)

	return obj.Copy(), nil
}
func (s *StateObject) getContextObjectCopy(hash ktypes.Hash) (*ktypes.ContextObject, error) {
	data, isAvailable := s.cache.Get(hash)
	if !isAvailable {
		rawData, err := s.db.ReadEntry(hash.Bytes())
		if err != nil {
			return nil, errors.Wrap(ktypes.ErrUpdatingContextObject, err.Error())
		}

		obj := new(ktypes.ContextObject)

		if err := polo.Depolorize(obj, rawData); err != nil {
			return nil, err
		}

		s.cache.Add(hash, obj)

		return obj.Copy(), nil
	}

	contextObject, ok := data.(*ktypes.ContextObject)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return contextObject.Copy(), nil
}
func getBalanceObject(hash ktypes.Hash, db *dhruva.PersistenceManager) (*ktypes.BalanceObject, error) {
	data, err := db.ReadEntry(hash.Bytes())
	if err != nil {
		return nil, err
	}

	balObject := new(ktypes.BalanceObject)

	if err = polo.Depolorize(balObject, data); err != nil {
		return nil, err
	}

	return balObject, nil
}
func getStorage(hash ktypes.Hash, db *dhruva.PersistenceManager) (map[ktypes.Hash][]byte, error) {
	data, err := db.ReadEntry(hash.Bytes())
	if err != nil {
		return nil, err
	}

	storageEntries := make(map[ktypes.Hash][]byte)

	if err = polo.Depolorize(&storageEntries, data); err != nil {
		return nil, err
	}

	return storageEntries, nil
}
