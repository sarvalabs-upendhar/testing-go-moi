package guna

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"golang.org/x/sync/errgroup"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/dhruva"
)

const (
	SenatusTopic       = "MOI_PUBSUB_SENATUS"
	minimumContextSize = 1
)

type server interface {
	Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error
}
type Senatus interface {
	AddNewPeer(key id.KramaID, data *ReputationInfo) error
	UpdateAddress(key id.KramaID, addrs []string) error
	UpdateNTQ(key id.KramaID, ntq int32) error
	UpdateInclusivity(key id.KramaID, delta int64) error
	GetAddress(key id.KramaID) (multiAddrs []multiaddr.Multiaddr, err error)
	GetNTQ(id id.KramaID) (int32, error)
	UpdatePublicKey(key id.KramaID, pk []byte) error
	GetPublicKey(ctx context.Context, id id.KramaID) ([]byte, error)
	AddEntries(msg ktypes.SyncReputationInfo) error
	GetInclusivity(id id.KramaID) (int64, error)
	GetAllEntries() (chan *ktypes.SyncReputationInfo, error)
	SenatusHandler(msg *pubsub.Message) error
	HandleHelloMessages(msgs []*ktypes.HelloMsg) (int, error)
	Start(id id.KramaID, ntq int32, publicKey []byte, address []multiaddr.Multiaddr) error
}

var GenesisAddress = ktypes.BytesToAddress(ktypes.GetHash([]byte("sargaAccount")).Bytes())

type StateManager struct {
	ctx              context.Context
	logger           hclog.Logger
	db               *dhruva.PersistenceManager
	cache            *lru.Cache
	senatus          Senatus
	network          server
	objects          map[ktypes.Address]*StateObject
	dirtyObjects     map[ktypes.Address]*StateObject
	dirtyObjectsLock sync.Mutex
	objectsLock      sync.Mutex
	client           *http.Client
}

func NewStateManager(
	ctx context.Context,
	db *dhruva.PersistenceManager,
	logger hclog.Logger,
	cache *lru.Cache,
	network server,
) (*StateManager, error) {
	sm := &StateManager{
		ctx:     ctx,
		cache:   cache,
		db:      db,
		network: network,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConns:    1024,
			MaxConnsPerHost: 1000,
		}},
		objects:      make(map[ktypes.Address]*StateObject),
		dirtyObjects: make(map[ktypes.Address]*StateObject),
		logger:       logger.Named("State-Manager"),
	}

	senatus, err := NewReputationEngine(ctx, logger, sm, db)
	if err != nil {
		return nil, err
	}

	sm.senatus = senatus

	return sm, nil
}

func (sm *StateManager) createStateObject(addr ktypes.Address, accType ktypes.AccType) *StateObject {
	journal := new(journal)
	stateObject := NewStateObject(addr, sm.cache, journal, sm.db, accType)

	sm.dirtyObjects[addr] = stateObject

	return stateObject
}

func (sm *StateManager) cleanupDirtyObject(addr ktypes.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
}

func (sm *StateManager) CreateDirtyObject(addr ktypes.Address, accType ktypes.AccType) *StateObject {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	obj := sm.createStateObject(addr, accType)

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
		return nil, errors.Wrap(err, err.Error())
	}

	return sm.GetStateObjectByHash(addr, t.Body.StateHash)
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

func (sm *StateManager) DeleteStateObject(addr ktypes.Address) {
	sm.objectsLock.Lock()
	defer sm.objectsLock.Unlock()

	delete(sm.objects, addr)
	sm.cleanupDirtyObject(addr)
}

func (sm *StateManager) getLatestTesseractHash(addr ktypes.Address) (ktypes.Hash, error) {
	if addr == ktypes.NilAddress {
		return ktypes.NilHash, ktypes.ErrInvalidAddress
	}

	hash, isCached := sm.cache.Get(addr)
	if isCached {
		tesseractID, ok := hash.(ktypes.Hash)
		if !ok {
			return ktypes.NilHash, ktypes.ErrInterfaceConversion
		}

		return tesseractID, nil
	}

	accMetaInfo, err := sm.db.GetAccountMetaInfo(addr.Bytes())
	if err != nil {
		return ktypes.NilHash, errors.Wrap(err, "account meta info fetch failed")
	}

	sm.cache.Add(addr, accMetaInfo.TesseractHash)

	return accMetaInfo.TesseractHash, nil
}

func (sm *StateManager) fetchTesseractByHash(hash ktypes.Hash) (*ktypes.Tesseract, error) {
	object, isCached := sm.cache.Get(hash)
	if !isCached {
		buf, err := sm.db.ReadEntry(hash.Bytes())
		if err != nil {
			return nil, err
		}

		ts := new(ktypes.Tesseract)

		if err = polo.Depolorize(ts, buf); err != nil {
			return nil, errors.Wrap(err, "tesseract depolarization failed")
		}

		sm.cache.Add(hash, ts)

		return ts, nil
	}

	ts, ok := object.(*ktypes.Tesseract)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return ts, nil
}

func (sm *StateManager) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	sm.logger.Debug("Fetching  latest tesseract", addr.Hex())

	tesseractHash, err := sm.getLatestTesseractHash(addr)
	if err != nil {
		return nil, errors.Wrap(err, "latest tesseract hash fetch failed")
	}

	return sm.fetchTesseractByHash(tesseractHash)
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

func (sm *StateManager) Revert(snap *StateObject) error {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	if snap != nil {
		sm.logger.Info("Reverting back the state object", "addr", snap.Address.Hex())
		sm.dirtyObjects[snap.Address] = snap
	}

	return nil
}

func (sm *StateManager) getContextObject(hash ktypes.Hash) (*ktypes.ContextObject, error) {
	contextData, isAvailable := sm.cache.Get(hash)
	if isAvailable {
		contextObject, ok := contextData.(*ktypes.ContextObject)
		if !ok {
			return nil, ktypes.ErrInterfaceConversion
		}

		return contextObject, nil
	}

	rawData, err := sm.db.ReadEntry(hash.Bytes())
	if err != nil {
		return nil, ktypes.ErrContextStateNotFound
	}

	object := new(ktypes.ContextObject)

	if err := polo.Depolorize(object, rawData); err != nil {
		return nil, errors.Wrap(err, "contextObject deserialization failed")
	}

	sm.cache.Add(hash, object)

	return object, nil
}

func (sm *StateManager) getMetaContextObject(key ktypes.Hash) (*ktypes.MetaContextObject, error) {
	metaData, isAvailable := sm.cache.Get(key)
	if isAvailable {
		metaContextObject, ok := metaData.(*ktypes.MetaContextObject)
		if !ok {
			return nil, ktypes.ErrInterfaceConversion
		}

		return metaContextObject, nil
	}

	rawData, err := sm.db.ReadEntry(key.Bytes())
	if err != nil {
		return nil, ktypes.ErrContextStateNotFound
	}

	object := new(ktypes.MetaContextObject)

	if err = polo.Depolorize(object, rawData); err != nil {
		return nil, errors.Wrap(err, "MetaContextObject deserialization failed")
	}

	sm.cache.Add(key, object)

	return object, nil
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

func (sm *StateManager) GetCommittedContextHash(add ktypes.Address) (ktypes.Hash, error) {
	tesseract, err := sm.GetLatestTesseract(add)
	if err != nil {
		return ktypes.NilHash, err
	}

	return tesseract.Body.ContextHash, nil
}

func (sm *StateManager) GetContextByHash(hash ktypes.Hash) ([]id.KramaID, []id.KramaID, error) {
	metaContextObject, err := sm.getMetaContextObject(hash)
	if err != nil {
		return nil, nil, errors.Wrap(err, "metaContextObject fetch failed")
	}

	behaviourContext, err := sm.getContextObject(metaContextObject.BehaviouralContext)
	if err != nil {
		return nil, nil, errors.Wrap(err, "behaviouralContextObject fetch failed")
	}

	randomContext, err := sm.getContextObject(metaContextObject.RandomContext)
	if err != nil {
		return nil, nil, errors.Wrap(err, "randomContextObject fetch failed")
	}

	return behaviourContext.Ids, randomContext.Ids, nil
}

func (sm *StateManager) GetLatestContext(address ktypes.Address) (ktypes.Hash, []id.KramaID, []id.KramaID, error) {
	if address == ktypes.NilAddress {
		return ktypes.NilHash, nil, nil, nil
	}

	ts, err := sm.GetLatestTesseract(address)
	if err != nil {
		return ktypes.NilHash, nil, nil, errors.Wrap(err, "tesseract fetch failed")
	}

	sm.logger.Debug("Fetching context info", "addr", address.Hex(), ts.Body.ContextHash.Hex())

	behaviourSet, randomSet, err := sm.GetContextByHash(ts.Body.ContextHash)
	if err != nil {
		return ktypes.NilHash, nil, nil, err
	}

	return ts.Body.ContextHash, behaviourSet, randomSet, nil
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

func (sm *StateManager) GetBalances(addrs ktypes.Address) (*ktypes.BalanceObject, error) {
	stateObject, err := sm.GetLatestStateObject(addrs)
	if err != nil {
		return nil, err
	}

	return stateObject.balance.Copy(), nil
}

func (sm *StateManager) GetAccountMetaInfo(addr ktypes.Address) (*ktypes.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr.Bytes())
}

func (sm *StateManager) GetAccountInfo(stateHash ktypes.Hash) (*ktypes.Account, error) {
	rawData, err := sm.db.ReadEntry(stateHash.Bytes())
	if err != nil {
		return nil, err
	}

	accInfo := new(ktypes.Account)

	if err := polo.Depolorize(accInfo, rawData); err != nil {
		return nil, err
	}

	return accInfo, nil
}

func (sm *StateManager) SenatusInstance() Senatus {
	return sm.senatus
}

type Response struct {
	Data []string `json:"data"`
}
type Request struct {
	Ids []string `json:"kramaIDs"`
}

func (sm *StateManager) GetPublicKeys(ids ...id.KramaID) ([][]byte, error) {
	if len(ids) <= 0 {
		return nil, errors.New("Empty Ids")
	}

	publicKeys := make([][]byte, len(ids))

	g, ctx := errgroup.WithContext(context.Background())

	for index, kramaID := range ids {
		i, k := index, kramaID

		g.Go(
			func() error {
				return func(id id.KramaID, index int) error {
					pk, err := sm.senatus.GetPublicKey(ctx, id)
					if errors.Is(err, context.Canceled) {
						return err
					} else if err != nil {
						keys, err := sm.GetPublicKeyFromContract(ctx, id)
						if err != nil {
							return err
						}

						pk = keys[0]

						if err := sm.senatus.UpdatePublicKey(id, pk); err != nil {
							sm.logger.Error("Error updating public key", "error", err)

							return err
						}
					}

					publicKeys[index] = pk

					return nil
				}(k, i)
			})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return publicKeys, nil
}

func (sm *StateManager) GetPublicKeyFromContract(ctx context.Context, ids ...id.KramaID) (keys [][]byte, err error) {
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

	return keys, nil
}

func (sm *StateManager) Start(id id.KramaID, ntq int32, publicKey []byte, addrs []multiaddr.Multiaddr) error {
	if err := sm.network.Subscribe(sm.ctx, SenatusTopic, sm.senatus.SenatusHandler); err != nil {
		return errors.Wrap(err, "failed to subscribe senatus topic")
	}

	return sm.senatus.Start(id, ntq, publicKey, addrs)
}
