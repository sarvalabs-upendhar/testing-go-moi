package guna

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"sync"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/telemetry/tracing"
	"golang.org/x/sync/errgroup"

	lru "github.com/hashicorp/golang-lru"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/types"
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
	AddEntries(msg ptypes.SyncReputationInfo) error
	GetInclusivity(id id.KramaID) (int64, error)
	GetAllEntries() (chan *ptypes.SyncReputationInfo, error)
	SenatusHandler(msg *pubsub.Message) error
	HandleHelloMessages(msgs []*ptypes.HelloMsg) (int, error)
	Start(id id.KramaID, ntq int32, publicKey []byte, address []multiaddr.Multiaddr) error
}

var (
	GenesisAddress = types.BytesToAddress(types.GetHash([]byte("sargaAccount")).Bytes())
	GenesisLogicID = types.LogicID(types.BytesToHex(types.GetHash([]byte("sargaContract")).Bytes()))
)

type StateManager struct {
	ctx              context.Context
	logger           hclog.Logger
	db               *dhruva.PersistenceManager
	cache            *lru.Cache
	senatus          Senatus
	network          server
	objects          map[types.Address]*StateObject
	dirtyObjects     map[types.Address]*StateObject
	dirtyObjectsLock sync.Mutex
	objectsLock      sync.Mutex
	client           *http.Client
	metrics          *Metrics
}

func NewStateManager(
	ctx context.Context,
	db *dhruva.PersistenceManager,
	logger hclog.Logger,
	cache *lru.Cache,
	network server,
	metrics *Metrics,
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
		objects:      make(map[types.Address]*StateObject),
		dirtyObjects: make(map[types.Address]*StateObject),
		logger:       logger.Named("State-Manager"),
		metrics:      metrics,
	}

	senatus, err := NewReputationEngine(ctx, logger, sm, db)
	if err != nil {
		return nil, err
	}

	sm.senatus = senatus

	sm.metrics.initMetrics()

	return sm, nil
}

func (sm *StateManager) createStateObject(addr types.Address, accType types.AccType) *StateObject {
	journal := new(Journal)
	stateObject := NewStateObject(addr, sm.cache, journal, sm.db, types.Account{}, accType)

	sm.dirtyObjects[addr] = stateObject

	return stateObject
}

func (sm *StateManager) cleanupDirtyObject(addr types.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
	sm.metrics.captureActiveStateObjects(-1)
}

func (sm *StateManager) CreateDirtyObject(addr types.Address, accType types.AccType) *StateObject {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	obj := sm.createStateObject(addr, accType)

	sm.dirtyObjects[addr] = obj.Copy()
	sm.metrics.captureActiveStateObjects(1)

	return sm.dirtyObjects[addr]
}

func (sm *StateManager) GetDirtyObject(addr types.Address) (*StateObject, error) {
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

func (sm *StateManager) GetLatestStateObject(addr types.Address) (*StateObject, error) {
	sm.objectsLock.Lock()
	defer sm.objectsLock.Unlock()

	object, ok := sm.objects[addr]
	if ok {
		if object.journal == nil {
			object.journal = new(Journal)
		}

		return object, nil
	}
	// get the latest tesseract
	t, err := sm.GetLatestTesseract(addr, false)
	if err != nil {
		return nil, err
	}

	return sm.GetStateObjectByHash(addr, t.Body.StateHash)
}

func (sm *StateManager) GetStateObjectByHash(addr types.Address, hash types.Hash) (*StateObject, error) {
	// read the state
	data, err := sm.db.GetAccount(addr, hash)
	if err != nil {
		return nil, errors.Wrap(types.ErrStateNotFound, err.Error())
	}

	acc := new(types.Account)
	if err = polo.Depolorize(acc, data); err != nil {
		log.Fatal(err)
	}

	newJournal := new(Journal)
	sObj := NewStateObject(addr, sm.cache, newJournal, sm.db, *acc, acc.AccType)

	sObj.balance, err = getBalanceObject(addr, acc.Balance, sm.db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch balance object")
	}

	//  add the new object to map
	sm.objects[addr] = sObj

	return sObj, nil
}

func (sm *StateManager) DeleteStateObject(addr types.Address) {
	sm.objectsLock.Lock()
	defer sm.objectsLock.Unlock()

	delete(sm.objects, addr)
	sm.cleanupDirtyObject(addr)
}

func (sm *StateManager) getLatestTesseractHash(addr types.Address) (types.Hash, error) {
	if addr.IsNil() {
		return types.NilHash, types.ErrInvalidAddress
	}

	hash, isCached := sm.cache.Get(addr)
	if isCached {
		tesseractID, ok := hash.(types.Hash)
		if !ok {
			return types.NilHash, types.ErrInterfaceConversion
		}

		return tesseractID, nil
	}

	accMetaInfo, err := sm.db.GetAccountMetaInfo(addr.Bytes())
	if err != nil {
		return types.NilHash, errors.Wrap(err, "account meta info fetch failed")
	}

	sm.cache.Add(addr, accMetaInfo.TesseractHash)

	return accMetaInfo.TesseractHash, nil
}

func (sm *StateManager) fetchTesseractByHash(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	object, isCached := sm.cache.Get(hash)
	if !isCached {
		ts, err := sm.FetchTesseractFromDB(hash, withInteractions)
		if err != nil {
			return nil, err
		}

		sm.cache.Add(hash, ts)

		return ts, nil
	}

	ts, ok := object.(*types.Tesseract)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return ts, nil
}

func (sm *StateManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	sm.logger.Debug("Fetching  latest tesseract", addr.Hex())

	tesseractHash, err := sm.getLatestTesseractHash(addr)
	if err != nil {
		return nil, errors.Wrap(err, "latest tesseract hash fetch failed")
	}

	return sm.fetchTesseractByHash(tesseractHash, withInteractions)
}

func (sm *StateManager) FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	var interactions types.Interactions
	// Fetch Tesseract from DB
	buf, err := sm.db.GetTesseract(hash)
	if err != nil {
		return nil, err
	}

	// canonicalTesseract is a clone of the tesseract. The only difference is that it won't have the interactions field.
	canonicalTesseract := new(types.CanonicalTesseract)

	if err = polo.Depolorize(canonicalTesseract, buf); err != nil {
		return nil, errors.Wrap(err, "failed to depolarize tesseract")
	}

	if withInteractions {
		// Fetch interactions from DB
		buf, err = sm.db.GetInteractions(canonicalTesseract.Body.InteractionHash)

		if err != nil {
			return nil, errors.Wrap(err, types.ErrFetchingInteractions.Error())
		}

		if err = polo.Depolorize(interactions, buf); err != nil {
			return nil, errors.Wrap(err, "failed to depolarize interactions")
		}
	}

	tesseract := &types.Tesseract{
		Header: canonicalTesseract.Header,
		Body:   canonicalTesseract.Body,
		Ixns:   interactions,
		Seal:   canonicalTesseract.Seal,
	}

	return tesseract, nil
}

func (sm *StateManager) Cleanup(address types.Address) {
	sm.objectsLock.Lock()
	delete(sm.objects, address)
	sm.objectsLock.Unlock()

	sm.cleanupDirtyObject(address)
}

func (sm *StateManager) Revert(snap *StateObject) error {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	if snap != nil {
		sm.logger.Info("Reverting back the state object", "addr", snap.address.Hex())
		sm.dirtyObjects[snap.address] = snap
		sm.metrics.captureNumOfReverts(1)
	}

	return nil
}

func (sm *StateManager) getContextObject(addr types.Address, hash types.Hash) (*gtypes.ContextObject, error) {
	contextData, isAvailable := sm.cache.Get(hash)
	if isAvailable {
		contextObject, ok := contextData.(*gtypes.ContextObject)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return contextObject, nil
	}

	rawData, err := sm.db.GetContext(addr, hash)
	if err != nil {
		return nil, types.ErrContextStateNotFound
	}

	object := new(gtypes.ContextObject)

	if err := polo.Depolorize(object, rawData); err != nil {
		return nil, errors.Wrap(err, "contextObject deserialization failed")
	}

	sm.cache.Add(hash, object)

	return object, nil
}

func (sm *StateManager) getMetaContextObject(addr types.Address, hash types.Hash) (*gtypes.MetaContextObject, error) {
	metaData, isAvailable := sm.cache.Get(hash)
	if isAvailable {
		metaContextObject, ok := metaData.(*gtypes.MetaContextObject)
		if !ok {
			return nil, types.ErrInterfaceConversion
		}

		return metaContextObject, nil
	}

	rawData, err := sm.db.GetContext(addr, hash)
	if err != nil {
		return nil, types.ErrContextStateNotFound
	}

	object := new(gtypes.MetaContextObject)

	if err = polo.Depolorize(object, rawData); err != nil {
		return nil, errors.Wrap(err, "MetaContextObject deserialization failed")
	}

	sm.cache.Add(hash, object)

	return object, nil
}

// fetchParticipantContextByHash fetches the context info based on the give hash
// and returns a NodeSet which holds the kramaIDs and public keys
func (sm *StateManager) fetchParticipantContextByHash(addr types.Address, hash types.Hash) (
	behaviouralSet, randomSet *ktypes.NodeSet,
	err error,
) {
	behaviouralContext, randomContext, err := sm.getContextByHash(addr, hash)
	if err != nil {
		sm.logger.Error("failed to retrieve sender context nodes", "error", err)

		return nil, nil, types.ErrAccountNotFound
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = ktypes.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return nil, nil, types.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = ktypes.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return nil, nil, types.ErrPublicKeyNotFound
		}
	}

	return behaviouralSet, randomSet, nil
}

func (sm *StateManager) fetchLatestParticipantContext(addr types.Address) (
	contextHash types.Hash,
	behaviouralSet, randomSet *ktypes.NodeSet,
	err error,
) {
	contextHash, behaviouralContext, randomContext, err := sm.GetContextByHash(addr, types.NilHash)
	if err != nil {
		sm.logger.Error("failed to retrieve sender context nodes", "error", err)

		return types.NilHash, nil, nil, types.ErrAccountNotFound
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = ktypes.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return types.NilHash, nil, nil, types.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = ktypes.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return types.NilHash, nil, nil, types.ErrPublicKeyNotFound
		}
	}

	return contextHash, behaviouralSet, randomSet, nil
}

func (sm *StateManager) GetCommittedContextHash(add types.Address) (types.Hash, error) {
	tesseract, err := sm.GetLatestTesseract(add, false)
	if err != nil {
		return types.NilHash, err
	}

	return tesseract.Body.ContextHash, nil
}

func (sm *StateManager) getContextByHash(addr types.Address, hash types.Hash) ([]id.KramaID, []id.KramaID, error) {
	metaContextObject, err := sm.getMetaContextObject(addr, hash)
	if err != nil {
		return nil, nil, errors.Wrap(err, "metaContextObject fetch failed")
	}

	behaviourContext, err := sm.getContextObject(addr, metaContextObject.BehaviouralContext)
	if err != nil {
		return nil, nil, errors.Wrap(err, "behaviouralContextObject fetch failed")
	}

	randomContext, err := sm.getContextObject(addr, metaContextObject.RandomContext)
	if err != nil {
		return nil, nil, errors.Wrap(err, "randomContextObject fetch failed")
	}

	return behaviourContext.Ids, randomContext.Ids, nil
}

// GetContextByHash fetches context using hash, if hash is nil, it returns the latest context info
func (sm *StateManager) GetContextByHash(
	address types.Address,
	hash types.Hash,
) (types.Hash, []id.KramaID, []id.KramaID, error) {
	if address.IsNil() && hash.IsNil() {
		return types.NilHash, nil, nil, types.ErrEmptyHashAndAddress
	}

	if hash.IsNil() {
		ts, err := sm.GetLatestTesseract(address, false)
		if err != nil {
			return types.NilHash, nil, nil, errors.Wrap(err, "tesseract fetch failed")
		}

		sm.logger.Debug("Fetching context info", "addr", address.Hex(), ts.Body.ContextHash.Hex())

		hash = ts.Body.ContextHash
	}

	behaviourSet, randomSet, err := sm.getContextByHash(address, hash)
	if err != nil {
		return types.NilHash, nil, nil, err
	}

	return hash, behaviourSet, randomSet, nil
}

func (sm *StateManager) FetchContextLock(ts *types.Tesseract) (*ktypes.ICSNodes, error) {
	ix := ts.Interactions()[0]
	ics := ktypes.NewICSNodes(6)

	for address, info := range ts.Header.ContextLock {
		if address == ix.FromAddress() {
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(ktypes.SenderBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(ktypes.SenderRandomSet, randomSet)
		} else if address == ix.ToAddress() || address == GenesisAddress {
			if info.ContextHash.IsNil() {
				continue
			}

			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(ktypes.ReceiverBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(ktypes.ReceiverRandomSet, randomSet)
		}
	}

	return ics, nil
}

// FetchInteractionContext returns a nodeSet which holds the latest context info of the interaction participants
func (sm *StateManager) FetchInteractionContext(ctx context.Context, ix *types.Interaction) (
	map[types.Address]types.Hash,
	[]*ktypes.NodeSet,
	error,
) {
	_, span := tracing.Span(ctx, "guna.StateManger", "FetchInteractionContext")
	defer span.End()

	var (
		nodeSet       = make([]*ktypes.NodeSet, 6)
		behaviourSet  *ktypes.NodeSet
		randomSet     *ktypes.NodeSet
		contextHash   types.Hash
		err           error
		contextHashes = make(map[types.Address]types.Hash)
	)

	if !ix.FromAddress().IsNil() {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.FromAddress())
		if err != nil {
			return nil, nil, err
		}

		contextHashes[ix.FromAddress()] = contextHash
		nodeSet[ktypes.SenderBehaviourSet] = behaviourSet
		nodeSet[ktypes.SenderRandomSet] = randomSet
	}

	if !ix.ToAddress().IsNil() {
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

func (sm *StateManager) IsGenesis(addr types.Address) (bool, error) {
	if addr.IsNil() {
		return false, nil
	}

	genesisObject, err := sm.GetLatestStateObject(GenesisAddress)
	if err != nil {
		return false, errors.Wrap(types.ErrObjectNotFound, err.Error())
	}
	// Fetch the account info from genesis state
	_, err = genesisObject.GetStorageEntry(GenesisLogicID, addr.Bytes())
	if errors.Is(err, types.ErrKeyNotFound) {
		return true, nil
	}

	return false, err
}

func (sm *StateManager) GetLatestNonce(addr types.Address) (uint64, error) {
	if addr.IsNil() {
		return 0, types.ErrInvalidAddress
	}

	object, err := sm.GetLatestStateObject(addr)
	if err != nil {
		return 0, err
	}

	return object.data.Nonce, nil
}

func (sm *StateManager) GetBalances(addrs types.Address) (*gtypes.BalanceObject, error) {
	stateObject, err := sm.GetLatestStateObject(addrs)
	if err != nil {
		return nil, err
	}

	return stateObject.balance.Copy(), nil
}

func (sm *StateManager) GetBalance(addr types.Address, assetID types.AssetID) (*big.Int, error) {
	if _, ok := sm.objects[addr]; ok {
		return sm.objects[addr].balance.Bal[assetID], nil
	}

	return nil, errors.New("invalid asset id")
}

func (sm *StateManager) GetAccountMetaInfo(addr types.Address) (*types.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr.Bytes())
}

func (sm *StateManager) GetAccountInfo(addr types.Address, stateHash types.Hash) (*types.Account, error) {
	rawData, err := sm.db.GetAccount(addr, stateHash)
	if err != nil {
		return nil, err
	}

	accInfo := new(types.Account)

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
	if len(ids) == 0 {
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
	data, err := json.Marshal(Request{types.KIPPeerIDToString(ids)})
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
