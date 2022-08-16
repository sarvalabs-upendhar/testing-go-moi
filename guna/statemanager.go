package guna

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/dhruva"
)

const (
	minimumContextSize = 1
)

var GenesisAddress = ktypes.BytesToAddress(ktypes.GetHash([]byte("sargaAccount")).Bytes())

//type Res struct {
//	respType int
//	data     []string
//}

type StateManager struct {
	logger           hclog.Logger
	db               *dhruva.PersistenceManager
	cache            *lru.Cache
	objects          map[ktypes.Address]*StateObject
	dirtyObjects     map[ktypes.Address]*StateObject
	dirtyObjectsLock sync.Mutex
	objectsLock      sync.Mutex
	mux              *kutils.TypeMux
	client           *http.Client
}

func NewStateManager(
	db *dhruva.PersistenceManager,
	logger hclog.Logger,
	cache *lru.Cache,
	mux *kutils.TypeMux,
) *StateManager {
	sm := &StateManager{
		cache:        cache,
		db:           db,
		mux:          mux,
		client: &http.Client{Transport: &http.Transport{	
			MaxIdleConns:    1024,	
			MaxConnsPerHost: 1000,	
		}},
		objects:      make(map[ktypes.Address]*StateObject),
		dirtyObjects: make(map[ktypes.Address]*StateObject),
		logger:       logger.Named("State-Manager"),
	}

	return sm
}

func (sm *StateManager) DeleteStateObject(addr ktypes.Address) {
	sm.objectsLock.Lock()
	defer sm.objectsLock.Unlock()

	delete(sm.objects, addr)
}

func (sm *StateManager) CreateStateObject(addr ktypes.Address, accType ktypes.AccType) *StateObject {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	journal := new(journal)
	stateObject := NewStateObject(addr, sm.cache, journal, sm.db, accType)
	sm.dirtyObjects[addr] = stateObject

	return stateObject
}

func (sm *StateManager) GetLatestNonce(addr ktypes.Address) (uint64, error) {
	if addr == ktypes.NilAddress {
		return 0, ktypes.ErrInvalidNonce
	}

	object, err := sm.GetLatestStateObject(addr)
	if err != nil {
		return 0, err
	}

	return object.data.Nonce, nil
}

func (sm *StateManager) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	if addr == ktypes.NilAddress {
		return nil, ktypes.ErrInvalidAddress
	}

	sm.logger.Debug("Fetching  latest tesseract", addr.Hex())

	tesseractID, isCached := sm.cache.Get(addr)
	if !isCached {
		accDetails, err := sm.db.GetAccountMetaInfo(addr.Bytes())
		if err != nil {
			return nil, err
		}

		tesseractID = accDetails.TesseractHash

		sm.cache.Add(addr, tesseractID)
	}

	tesseract := new(ktypes.Tesseract)

	object, isCached := sm.cache.Get(tesseractID)
	if !isCached {
		buf, err := sm.db.ReadEntry(tesseractID.(ktypes.Hash).Bytes())
		if err != nil {
			return nil, err
		}

		if err = polo.Depolorize(tesseract, buf); err != nil {
			sm.logger.Error("Error depolarizing tesseract", err)

			return nil, err
		}

		sm.cache.Add(tesseractID, tesseract)

		return tesseract, nil
	}

	ts, ok := object.(*ktypes.Tesseract)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return ts, nil
}

func (sm *StateManager) CreateDirtyObject(addr ktypes.Address, accType ktypes.AccType) *StateObject {
	obj := sm.CreateStateObject(addr, accType)
	sm.dirtyObjects[addr] = obj.Copy()

	return sm.dirtyObjects[addr]
}
func (sm *StateManager) GetDirtyObject(addr ktypes.Address) (*StateObject, error) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	if _, ok := sm.dirtyObjects[addr]; !ok {
		var (
			object *StateObject
			err    error
		)

		object, err = sm.GetLatestStateObject(addr)
		if err != nil {
			return nil, err
		}

		sm.dirtyObjects[addr] = object.Copy()
	}

	return sm.dirtyObjects[addr], nil
}
func (sm *StateManager) IsGenesis(addr ktypes.Address) (bool, error) {
	if addr == ktypes.NilAddress {
		return false, nil
	}

	genesisObject, err := sm.GetLatestStateObject(GenesisAddress)
	if err != nil {
		return false, errors.Wrap(ktypes.ErrObjectNotFound, err.Error())
	}
	// Fetch the account info from genesis state
	_, err = genesisObject.GetStorageEntry(ktypes.GetHash(addr.Bytes()))
	if err != nil {
		return true, nil
	}

	return false, nil
}
func (sm *StateManager) GetStateObjectByHash(addr ktypes.Address, hash ktypes.Hash) (*StateObject, error) {
	// read the state
	data, err := sm.db.ReadEntry(hash.Bytes())
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrStateNotFound, err.Error())
	}

	acc := new(ktypes.Account)
	if err = polo.Depolorize(acc, data); err != nil {
		log.Fatal(err)
	}

	newJournal := new(journal)
	sObj := NewStateObject(addr, sm.cache, newJournal, sm.db, acc.AccType)
	sObj.data = *acc
	sObj.contextHash = acc.ContextHash

	sObj.balance, err = getBalanceObject(acc.Balance, sm.db)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrFetchingBalanceObject, err.Error())
	}

	sObj.storage, err = getStorage(acc.StorageRoot, sm.db)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrFetchingStorageObject, err.Error())
	}
	//  add the new object to map
	sm.objects[addr] = sObj

	return sObj, nil
}
func (sm *StateManager) GetLatestStateObject(addr ktypes.Address) (*StateObject, error) {
	sm.objectsLock.Lock()
	defer sm.objectsLock.Unlock()

	object, ok := sm.objects[addr]
	if ok {
		if object.journal == nil {
			object.journal = new(journal)
		}

		return object, nil
	}
	// get the latest tesseract
	t, err := sm.GetLatestTesseract(addr)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrFetchingTesseract, err.Error())
	}

	return sm.GetStateObjectByHash(addr, t.Body.StateHash)
}

// Broadcast publishes the CID's associated with the given address using bitswap
func (sm *StateManager) Broadcast(address ktypes.Address) {
	object, err := sm.GetDirtyObject(address)
	if err != nil {
		sm.logger.Error("Error fetching dirty entries for", "addr", address.Hex(), "err", err)
	}

	object.mtx.Lock()
	cIDs := object.journal.GetIDs()
	object.journal = nil
	object.mtx.Unlock()

	sm.logger.Info("Announcing CID's for address", "addr", address.Hex(), "count", len(cIDs))
	sm.db.AnnounceBatchCIDEntries(cIDs)
	//log.Printf("CID's announced for address %s", kutils.BytesToHex(address[:]))

	sm.objectsLock.Lock()
	sm.objects[address] = object
	sm.objectsLock.Unlock()

	sm.cleanupDirtyObject(address)
}

// Commit writes the changes to db and returns updated state hash of the provided account
func (sm *StateManager) Commit(addresses []ktypes.Address) ([]ktypes.Hash, error) {
	data := make([]ktypes.Hash, 0, len(addresses))

	for _, addr := range addresses {
		object := sm.objects[addr]

		root, err := object.Commit()
		if err != nil {
			return nil, err
		}

		data = append(data, root)
	}

	return data, nil
}

func (sm *StateManager) Revert(snap *StateObject) error {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	if snap != nil {
		sm.logger.Info("Reverting back the state object", "addr", snap.Address.Hex())
		sm.dirtyObjects[snap.Address] = snap
	}

	return nil
}

func (sm *StateManager) GetCommittedContextHash(add ktypes.Address) (ktypes.Hash, error) {
	tesseract, err := sm.GetLatestTesseract(add)
	if err != nil {
		return ktypes.NilHash, err
	}

	return tesseract.Body.ContextHash, nil
}

func (sm *StateManager) GetBalances(addrs ktypes.Address) (*ktypes.BalanceObject, error) {
	stateObject, err := sm.GetLatestStateObject(addrs)
	if err != nil {
		return nil, err
	}

	return stateObject.balance.Copy(), nil
}

func (sm *StateManager) GetAccountInfo(addr ktypes.Address) (*ktypes.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr.Bytes())
}
func (sm *StateManager) GetLatestContext(address ktypes.Address) (ktypes.Hash, []id.KramaID, []id.KramaID, error) {
	if address == ktypes.NilAddress {
		return ktypes.NilHash, nil, nil, nil
	}

	ts, err := sm.GetLatestTesseract(address)
	if err != nil {
		return ktypes.NilHash, nil, nil, errors.Wrap(err, "Tesseract fetch failed")
	}

	sm.logger.Debug("Fetching context info", "addr", address.Hex(), ts.Body.ContextHash.Hex())

	behaviourSet, randomSet, err := sm.GetContextByHash(ts.Body.ContextHash)
	if err != nil {
		return ktypes.NilHash, nil, nil, err
	}

	return ts.Body.ContextHash, behaviourSet, randomSet, nil
}

func (sm *StateManager) GetContextByHash(hash ktypes.Hash) ([]id.KramaID, []id.KramaID, error) {
	metaData, isAvailable := sm.cache.Get(hash)
	if !isAvailable {
		rawData, err := sm.db.ReadEntry(hash.Bytes())
		if err != nil {
			return nil, nil, ktypes.ErrContextStateNotFound
		}

		msg := new(ktypes.MetaContextObject)

		if err := polo.Depolorize(msg, rawData); err != nil {
			return nil, nil, errors.Wrap(err, "MetaContextObject deserialization failed")
		}

		metaData = msg

		sm.cache.Add(hash, msg)
	}

	metaContextObject, ok := metaData.(*ktypes.MetaContextObject)
	if !ok {
		return nil, nil, ktypes.ErrInterfaceConversion
	}

	behaviourContext, err := sm.getContextObject(metaContextObject.BehaviouralContext)
	if err != nil {
		return nil, nil, err
	}

	randomContext, err := sm.getContextObject(metaContextObject.RandomContext)
	if err != nil {
		return nil, nil, err
	}

	return behaviourContext.Ids, randomContext.Ids, nil
}
func (sm *StateManager) cleanupDirtyObject(addr ktypes.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
}

func (sm *StateManager) getContextObject(hash ktypes.Hash) (*ktypes.ContextObject, error) {
	contextData, isAvailable := sm.cache.Get(hash)
	if !isAvailable {
		rawData, err := sm.db.ReadEntry(hash.Bytes())
		if err != nil {
			return nil, ktypes.ErrContextStateNotFound
		}

		msg := new(ktypes.ContextObject)

		if err := polo.Depolorize(msg, rawData); err != nil {
			return nil, errors.Wrap(err, "ContextObject deserialization failed")
		}

		contextData = msg

		sm.cache.Add(hash, msg)
	}

	contextObject, ok := contextData.(*ktypes.ContextObject)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return contextObject, nil
}

type Response struct {
	Data []string `json:"data"`
}
type Request struct {
	Ids []string `json:"kramaIDs"`
}

func (sm *StateManager) GetPublicKeys(ids ...id.KramaID) (keys [][]byte, err error) {
	if len(ids) <= 0 {
		return nil, errors.New("Empty Ids")
	}

	data, err := json.Marshal(Request{ktypes.KIPPeerIDToString(ids)})
	if err != nil {
		return nil, err
	}


	req, err := http.NewRequest("POST", "http://45.140.185.105/api/fetchPublicKeys", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	response, err := sm.client.Do(req)	
	if err != nil {	
		sm.logger.Error("Api fetch failed", "error", err, "kramaIDs", ids)	
		return nil, err	
	}

	//res, err := http.Post("http://159.203.191.91/api/fetchPublicKeys", "application/json", bytes.NewBuffer(data))
	//if err != nil {
	//	fmt.Println(err)
	//	return nil, err
	//}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Panicln(err)
	}

	defer response.Body.Close()
	if response.StatusCode != 200 {
		sm.logger.Error("Http request failed", response.StatusCode, string(body))
	}

	data1 := new(Response)

	if err := json.Unmarshal(body, data1); err != nil {
		log.Panicln(err)
	}

	for _, v := range data1.Data {
		str, err := hex.DecodeString(v)
		if err != nil {
			return nil, err
		}

		keys = append(keys, str)
	}

	return
}

func (sm *StateManager) FetchContextLock(ts *ktypes.Tesseract) ([]*ktypes.NodeSet, error) {
	ix := ts.Body.Interactions[0]
	nodeSet := make([]*ktypes.NodeSet, 6)

	for address, info := range ts.Header.ContextLock {
		if address == ix.FromAddress() {
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(info.ContextHash)
			if err != nil {
				return nil, err
			}

			nodeSet[ktypes.SenderBehaviourSet] = behaviourSet
			nodeSet[ktypes.SenderRandomSet] = randomSet
		} else if address == ix.ToAddress() || address == GenesisAddress {
			if info.ContextHash == ktypes.NilHash {
				continue
			}
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(info.ContextHash)
			if err != nil {
				return nil, err
			}

			nodeSet[ktypes.ReceiverBehaviourSet] = behaviourSet
			nodeSet[ktypes.ReceiverRandomSet] = randomSet
		}
	}

	return nodeSet, nil
}

// FetchInteractionContext returns a nodeSet which holds the latest context info of the interaction participants
func (sm *StateManager) FetchInteractionContext(ix *ktypes.Interaction) (
	map[ktypes.Address]ktypes.Hash,
	[]*ktypes.NodeSet,
	error,
) {
	var (
		nodeSet       = make([]*ktypes.NodeSet, 6)
		behaviourSet  *ktypes.NodeSet
		randomSet     *ktypes.NodeSet
		contextHash   ktypes.Hash
		err           error
		contextHashes = make(map[ktypes.Address]ktypes.Hash)
	)

	if ix.FromAddress() != ktypes.NilAddress {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.FromAddress())
		if err != nil {
			return nil, nil, err
		}

		contextHashes[ix.FromAddress()] = contextHash
		nodeSet[ktypes.SenderBehaviourSet] = behaviourSet
		nodeSet[ktypes.SenderRandomSet] = randomSet
	}

	if ix.ToAddress() != ktypes.NilAddress {
		isGenesisAccount, err := sm.IsGenesis(ix.ToAddress())
		if err != nil {
			return nil, nil, err
		}

		if isGenesisAccount {
			contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(GenesisAddress)
			if err != nil {
				return nil, nil, err
			}

			contextHashes[GenesisAddress] = contextHash
		} else {
			contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.ToAddress())
			if err != nil {
				return nil, nil, err
			}

			contextHashes[ix.ToAddress()] = contextHash
		}

		nodeSet[ktypes.ReceiverBehaviourSet] = behaviourSet
		nodeSet[ktypes.ReceiverRandomSet] = randomSet
	}

	return contextHashes, nodeSet, err
}

// fetchParticipantContextByHash fetches the context info based on the give hash
// and returns a NodeSet which holds the kramaIDs and public keys
func (sm *StateManager) fetchParticipantContextByHash(hash ktypes.Hash) (
	behaviouralSet, randomSet *ktypes.NodeSet,
	err error) {
	behaviouralContext, randomContext, err := sm.GetContextByHash(hash)
	if err != nil {
		sm.logger.Error("failed to retrieve sender context nodes", "error", err)

		return nil, nil, ktypes.ErrAccountNotFound
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = ktypes.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return nil, nil, ktypes.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = ktypes.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return nil, nil, ktypes.ErrPublicKeyNotFound
		}
	}

	return behaviouralSet, randomSet, nil
}

func (sm *StateManager) fetchLatestParticipantContext(addr ktypes.Address) (
	contextHash ktypes.Hash,
	behaviouralSet, randomSet *ktypes.NodeSet,
	err error) {
	contextHash, behaviouralContext, randomContext, err := sm.GetLatestContext(addr)
	if err != nil {
		sm.logger.Error("failed to retrieve sender context nodes", "error", err)

		return ktypes.NilHash, nil, nil, ktypes.ErrAccountNotFound
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = ktypes.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return ktypes.NilHash, nil, nil, ktypes.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = ktypes.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return ktypes.NilHash, nil, nil, ktypes.ErrPublicKeyNotFound
		}
	}

	return contextHash, behaviouralSet, randomSet, nil
}
