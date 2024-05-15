package state

import (
	"context"
	"encoding/hex"
	"math/big"
	"net/http"
	"sync"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

const (
	MinimumContextSize = 1
)

type Store interface {
	GetAccount(addr identifiers.Address, stateHash common.Hash) ([]byte, error)
	GetContext(addr identifiers.Address, contextHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error)
	GetInteractions(tsHash common.Hash) ([]byte, error)
	GetTesseract(tsHash common.Hash) ([]byte, error)
	GetBalance(addr identifiers.Address, balanceHash common.Hash) ([]byte, error)
	GetAssetRegistry(addr identifiers.Address, registryHash common.Hash) ([]byte, error)
	GetMerkleTreeEntry(address identifiers.Address, prefix storage.PrefixTag, key []byte) ([]byte, error)
	SetMerkleTreeEntry(address identifiers.Address, prefix storage.PrefixTag, key, value []byte) error
	SetMerkleTreeEntries(address identifiers.Address, prefix storage.PrefixTag, entries map[string][]byte) error
	WritePreImages(address identifiers.Address, entries map[common.Hash][]byte) error
	GetPreImage(address identifiers.Address, hash common.Hash) ([]byte, error)
	DeleteEntry(key []byte) error
	CreateEntry(key []byte, value []byte) error
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	UpdateEntry(key []byte, newValue []byte) error
	NewBatchWriter() db.BatchWriter
}

type senatus interface {
	GetPublicKey(kramaID kramaid.KramaID) ([]byte, error)
	UpdatePublicKey(kramaID kramaid.KramaID, pk []byte) error
}

type StateManager struct {
	cfg       *config.StateConfig
	logger    hclog.Logger
	cache     *lru.Cache
	treeCache *fastcache.Cache

	db Store

	senatus senatus
	client  *http.Client

	dirtyObjectsLock sync.Mutex
	dirtyObjects     map[identifiers.Address]*Object

	metrics *Metrics
}

func NewStateManager(
	db Store,
	logger hclog.Logger,
	cache *lru.Cache,
	metrics *Metrics,
	senatus senatus,
	cfg *config.StateConfig,
) (*StateManager, error) {
	sm := &StateManager{
		cfg:     cfg,
		cache:   cache,
		db:      db,
		senatus: senatus,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConns:    1024,
			MaxConnsPerHost: 1000,
		}},
		dirtyObjects: make(map[identifiers.Address]*Object),
		treeCache:    fastcache.New(int(cfg.TreeCacheSize)),

		logger:  logger.Named("State-Manager"),
		metrics: metrics,
	}

	sm.metrics.InitMetrics()

	return sm, nil
}

func (sm *StateManager) CreateStateObject(addr identifiers.Address, accType common.AccountType) *Object {
	stateObject := NewStateObject(addr, sm.cache, sm.treeCache, sm.db, common.Account{AccType: accType}, sm.metrics)

	return stateObject
}

func (sm *StateManager) cleanupDirtyObject(addr identifiers.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
	sm.metrics.CaptureActiveStateObjects(float64(len(sm.dirtyObjects)))
}

func (sm *StateManager) CreateDirtyObject(addr identifiers.Address, accType common.AccountType) *Object {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	obj := sm.CreateStateObject(addr, accType)

	sm.dirtyObjects[addr] = obj.Copy()
	sm.metrics.CaptureActiveStateObjects(float64(len(sm.dirtyObjects)))

	return sm.dirtyObjects[addr]
}

func (sm *StateManager) FlushDirtyObject(addrs identifiers.Address) error {
	so, err := sm.GetDirtyObject(addrs)
	if err != nil {
		return errors.Wrap(err, "failed to fetch state object")
	}

	return so.flush()
}

func (sm *StateManager) GetDirtyObject(addr identifiers.Address) (*Object, error) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	object, ok := sm.dirtyObjects[addr]
	if ok {
		return object, nil
	}

	dirtyObject, err := sm.GetLatestStateObject(addr)
	if err != nil {
		return nil, err
	}

	sm.dirtyObjects[addr] = dirtyObject.Copy()

	sm.metrics.CaptureActiveStateObjects(float64(len(sm.dirtyObjects)))

	return sm.dirtyObjects[addr], nil
}

func (sm *StateManager) HasParticipantStateAt(addr identifiers.Address, stateHash common.Hash) bool {
	if _, err := sm.db.GetAccount(addr, stateHash); err != nil {
		return false
	}

	return true
}

func (sm *StateManager) getStateObject(addr identifiers.Address, stateHash common.Hash) (*Object, error) {
	if stateHash.IsNil() {
		return sm.GetLatestStateObject(addr)
	}

	return sm.GetStateObjectByHash(addr, stateHash)
}

func (sm *StateManager) GetEmptyStateObject() *Object {
	addr := identifiers.NewAddressFromBytes(identifiers.NilAddress.Bytes())

	return NewStateObject(addr, sm.cache, sm.treeCache, sm.db, common.Account{}, sm.metrics)
}

func (sm *StateManager) GetLatestStateObject(addr identifiers.Address) (*Object, error) {
	t, err := sm.GetLatestTesseract(addr, false)
	if err != nil {
		return nil, err
	}

	return sm.GetStateObjectByHash(addr, t.StateHash(addr))
}

func (sm *StateManager) GetStateObjectByHash(addr identifiers.Address, hash common.Hash) (*Object, error) {
	// read the state
	data, err := sm.db.GetAccount(addr, hash)
	if err != nil {
		return nil, errors.Wrap(common.ErrStateNotFound, err.Error())
	}

	acc := new(common.Account)
	if err = acc.FromBytes(data); err != nil {
		return nil, err
	}

	sObj := NewStateObject(addr, sm.cache, sm.treeCache, sm.db, *acc, sm.metrics)

	return sObj, nil
}

func (sm *StateManager) GetLatestTesseract(addr identifiers.Address, withInteractions bool) (*common.Tesseract, error) {
	tesseractHash, err := sm.getLatestTesseractHash(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch latest tesseract hash")
	}

	return sm.getTesseractByHash(tesseractHash, withInteractions)
}

func (sm *StateManager) GetLogicIDs(addr identifiers.Address, stateHash common.Hash) ([]identifiers.LogicID, error) {
	obj, err := sm.getStateObject(addr, stateHash)
	if err != nil {
		return nil, err
	}

	logicIDs := make([]identifiers.LogicID, 0)

	logicTree, err := obj.getLogicTree()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load meta logic tree")
	}

	it := logicTree.NewIterator()

	for it.Next() {
		if it.Leaf() {
			logicID, err := obj.logicTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			logicIDs = append(logicIDs, identifiers.LogicID(hex.EncodeToString(logicID)))
		}
	}

	return logicIDs, nil
}

func (sm *StateManager) FetchTesseractFromDB(
	hash common.Hash,
	withInteractions bool,
) (*common.Tesseract, error) {
	// Fetch Tesseract from DB
	rawTesseract, err := sm.db.GetTesseract(hash)
	if err != nil {
		return nil, err
	}

	// canonicalTesseract is a clone of the tesseract. The only difference is that it won't have the interactions field.
	canonicalTesseract := new(common.CanonicalTesseract)

	if err = canonicalTesseract.FromBytes(rawTesseract); err != nil {
		return nil, err
	}

	interactions := new(common.Interactions)

	// Fetch interactions for non-genesis tesseracts from DB
	if withInteractions && canonicalTesseract.ConsensusInfo.ClusterID != common.GenesisIdentifier {
		rawIxns, err := sm.db.GetInteractions(hash)
		if err != nil {
			return nil, errors.Wrap(err, common.ErrFetchingInteractions.Error())
		}

		if err := interactions.FromBytes(rawIxns); err != nil {
			return nil, err
		}
	}

	ts := canonicalTesseract.ToTesseract(*interactions, nil)

	return ts, nil
}

func (sm *StateManager) getLatestTesseractHash(addr identifiers.Address) (common.Hash, error) {
	if addr.IsNil() {
		return common.NilHash, common.ErrInvalidAddress
	}

	hash, isCached := sm.cache.Get(addr)
	if isCached {
		tesseractID, ok := hash.(common.Hash)
		if !ok {
			return common.NilHash, common.ErrInterfaceConversion
		}

		return tesseractID, nil
	}

	accMetaInfo, err := sm.db.GetAccountMetaInfo(addr)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "account meta info fetch failed")
	}

	sm.cache.Add(addr, accMetaInfo.TesseractHash)

	return accMetaInfo.TesseractHash, nil
}

// getTesseractByHash returns tesseract with/without interactions
// - with interactions always fetches from db
// - without interactions fetches from cache or db
func (sm *StateManager) getTesseractByHash(
	hash common.Hash,
	withInteractions bool,
) (*common.Tesseract, error) {
	if withInteractions {
		return sm.FetchTesseractFromDB(hash, withInteractions)
	}

	object, isCached := sm.cache.Get(hash)
	if !isCached {
		ts, err := sm.FetchTesseractFromDB(hash, withInteractions)
		if err != nil {
			return nil, err
		}

		sm.cache.Add(hash, ts)

		return ts, nil
	}

	ts, ok := object.(*common.Tesseract)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	return common.NewTesseract(
		ts.Participants(),
		ts.InteractionsHash(),
		ts.ReceiptsHash(),
		ts.Epoch(),
		ts.Timestamp(),
		ts.Operator(),
		ts.FuelUsed(),
		ts.FuelLimit(),
		ts.ConsensusInfo(),
		ts.Seal(),
		ts.SealBy(),
		nil,
		nil,
	), nil
}

func (sm *StateManager) Cleanup(address identifiers.Address) {
	sm.cleanupDirtyObject(address)
}

func (sm *StateManager) UpdateStateObjects(objs ObjectMap) error {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	for addr, obj := range objs {
		if _, ok := sm.dirtyObjects[addr]; ok {
			return errors.New("dirty object already exists")
		}

		sm.dirtyObjects[addr] = obj
	}

	return nil
}

func (sm *StateManager) getContextObject(addr identifiers.Address, hash common.Hash) (*ContextObject, error) {
	contextData, isAvailable := sm.cache.Get(hash)
	if isAvailable {
		contextObject, ok := contextData.(*ContextObject)
		if !ok {
			return nil, common.ErrInterfaceConversion
		}

		return contextObject, nil
	}

	rawData, err := sm.db.GetContext(addr, hash)
	if err != nil {
		return nil, common.ErrContextStateNotFound
	}

	object := new(ContextObject)

	if err := object.FromBytes(rawData); err != nil {
		return nil, errors.Wrap(err, "contextObject deserialization failed")
	}

	sm.cache.Add(hash, object)

	return object, nil
}

func (sm *StateManager) getMetaContextObject(addr identifiers.Address, hash common.Hash) (*MetaContextObject, error) {
	metaData, isAvailable := sm.cache.Get(hash)
	if isAvailable {
		metaContextObject, ok := metaData.(*MetaContextObject)
		if !ok {
			return nil, common.ErrInterfaceConversion
		}

		return metaContextObject, nil
	}

	rawData, err := sm.db.GetContext(addr, hash)
	if err != nil {
		return nil, common.ErrContextStateNotFound
	}

	object := new(MetaContextObject)

	if err = object.FromBytes(rawData); err != nil {
		return nil, errors.Wrap(err, "MetaContextObject deserialization failed")
	}

	sm.cache.Add(hash, object)

	return object, nil
}

// fetchParticipantContextByHash fetches the context info based on the give hash
// and returns a NodeSet which holds the kramaIDs and public keys
func (sm *StateManager) fetchParticipantContextByHash(addr identifiers.Address, hash common.Hash) (
	behaviouralSet, randomSet *common.NodeSet,
	err error,
) {
	behaviouralContext, randomContext, err := sm.getContext(addr, hash)
	if err != nil {
		sm.logger.Error("Failed to retrieve the sender context nodes", "err", err)

		return nil, nil, err
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = common.NewNodeSet(behaviouralContext, nil, uint32(len(behaviouralContext)))

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(context.Background(), behaviouralContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of behavioural set", "err", err)

			return nil, nil, common.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = common.NewNodeSet(randomContext, nil, uint32(len(randomContext)))

		if randomSet.PublicKeys, err = sm.GetPublicKeys(context.Background(), randomContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of random set", "err", err)

			return nil, nil, common.ErrPublicKeyNotFound
		}
	}

	return behaviouralSet, randomSet, nil
}

func (sm *StateManager) FetchLatestParticipantContext(addr identifiers.Address) (
	latestContextHash common.Hash,
	behaviouralSet, randomSet *common.NodeSet,
	err error,
) {
	latestContextHash, err = sm.GetCommittedContextHash(addr)
	if err != nil {
		return common.NilHash, nil, nil, err
	}

	behaviouralContext, randomContext, err := sm.getContext(addr, latestContextHash)
	if err != nil {
		return common.NilHash, nil, nil, err
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = common.NewNodeSet(behaviouralContext, nil, uint32(len(behaviouralContext)))

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(context.Background(), behaviouralContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of behavioural set", "err", err)

			return common.NilHash, nil, nil, common.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = common.NewNodeSet(randomContext, nil, uint32(len(randomContext)))

		if randomSet.PublicKeys, err = sm.GetPublicKeys(context.Background(), randomContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of random set", "err", err)

			return common.NilHash, nil, nil, common.ErrPublicKeyNotFound
		}
	}

	return latestContextHash, behaviouralSet, randomSet, nil
}

func (sm *StateManager) GetCommittedContextHash(addr identifiers.Address) (common.Hash, error) {
	tesseract, err := sm.GetLatestTesseract(addr, false)
	if err != nil {
		return common.NilHash, err
	}

	return tesseract.LatestContextHash(addr), nil
}

func (sm *StateManager) getContext(
	addr identifiers.Address,
	hash common.Hash,
) (
	[]kramaid.KramaID,
	[]kramaid.KramaID,
	error,
) {
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

// GetParticipantContextRaw loads the context info of a participant into the given map
func (sm *StateManager) GetParticipantContextRaw(
	address identifiers.Address,
	hash common.Hash,
	rawContext map[string][]byte,
) error {
	metaObjectRaw, err := sm.db.GetContext(address, hash)
	if err != nil {
		return err
	}

	metaObject := new(MetaContextObject)
	if err = metaObject.FromBytes(metaObjectRaw); err != nil {
		return err
	}

	rawContext[hash.String()] = metaObjectRaw

	if !metaObject.BehaviouralContext.IsNil() {
		behavioural, err := sm.db.GetContext(address, metaObject.BehaviouralContext)
		if err != nil {
			return errors.Wrap(err, "failed to fetch behavioural context")
		}

		rawContext[metaObject.BehaviouralContext.String()] = behavioural
	}

	if !metaObject.RandomContext.IsNil() {
		random, err := sm.db.GetContext(address, metaObject.RandomContext)
		if err != nil {
			return errors.Wrap(err, "failed to fetch random context")
		}

		rawContext[metaObject.RandomContext.String()] = random
	}

	return nil
}

func (sm *StateManager) NodeSet(ids []kramaid.KramaID, setSizeWithoutDelta uint32) (*common.NodeSet, error) {
	var (
		publicKeys [][]byte
		err        error
	)

	if len(ids) == 0 {
		return nil, err
	}

	publicKeys, err = sm.GetPublicKeys(context.Background(), ids...)
	if err != nil {
		return nil, err
	}

	return common.NewNodeSet(ids, publicKeys, setSizeWithoutDelta), nil
}

func (sm *StateManager) NodeSetFromRawContextObject(raw []byte) (*common.NodeSet, error) {
	obj := new(ContextObject)
	if err := obj.FromBytes(raw); err != nil {
		return nil, err
	}

	return sm.NodeSet(obj.Ids, uint32(len(obj.Ids)))
}

func (sm *StateManager) FetchICSNodeSet(
	ts *common.Tesseract,
	info *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	if info.Responses == nil {
		return nil, errors.New("nil responses slice")
	}

	addrs := ts.Addresses()
	ps := ts.Participants()

	ics := common.NewICSNodeSet(2*len(addrs) + 2)

	for index, addr := range addrs {
		if ps[addr].PreviousContext == common.NilHash {
			continue
		}

		position := index * 2

		behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(addr, ps[addr].PreviousContext)
		if err != nil {
			return nil, err
		}

		if behaviourSet != nil {
			ics.UpdateNodeSet(position, behaviourSet)
			ics.UpdateNodeSetResponses(position, info.Responses[position])
		}

		if randomSet != nil {
			ics.UpdateNodeSet(position+1, randomSet)
			ics.UpdateNodeSetResponses(position+1, info.Responses[position+1])
		}
	}

	randomSet, err := sm.NodeSet(info.RandomSet, info.RandomSetSizeWithoutDelta)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ics.RandomSetPosition(), randomSet)
	ics.UpdateNodeSetResponses(ics.RandomSetPosition(), info.Responses[ics.RandomSetPosition()])

	observerSet, err := sm.NodeSet(info.ObserverSet, uint32(len(info.ObserverSet)))
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ics.ObserverSetPosition(), observerSet)
	ics.UpdateNodeSetResponses(ics.ObserverSetPosition(), info.Responses[ics.ObserverSetPosition()])

	return ics, nil
}

func (sm *StateManager) GetICSNodeSetFromRawContext(
	ts *common.Tesseract,
	rawContext map[string][]byte,
	info *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	contextHashes := make([]common.Hash, 0)
	addrs := ts.Addresses()
	ps := ts.Participants()

	ics := common.NewICSNodeSet(2*len(addrs) + 2)

	for index, addr := range addrs {
		position := index * 2

		if ps[addr].PreviousContext == common.NilHash || ps[addr].LatestContext == ps[addr].PreviousContext {
			continue
		}

		metaObject := new(MetaContextObject)
		if err := metaObject.FromBytes(rawContext[ps[addr].PreviousContext.String()]); err != nil {
			return nil, err
		}

		rawBytes, ok := rawContext[metaObject.BehaviouralContext.String()]
		if ok {
			nodeSet, err := sm.NodeSetFromRawContextObject(rawBytes)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(position, nodeSet)
			ics.UpdateNodeSetResponses(position, info.Responses[position])
		}

		rawBytes, ok = rawContext[metaObject.RandomContext.String()]
		if ok {
			nodeSet, err := sm.NodeSetFromRawContextObject(rawBytes)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(position+1, nodeSet)
			ics.UpdateNodeSetResponses(position+1, info.Responses[position+1])
		}

		contextHashes = append(contextHashes, ps[addr].PreviousContext)
		contextHashes = append(contextHashes, metaObject.BehaviouralContext)
		contextHashes = append(contextHashes, metaObject.RandomContext)
	}

	// delete the context hashes from delta separately instead of deleting in above for loop
	// because sender and receiver context nodes can be same then we cannot extract receiver context nodes
	for _, hash := range contextHashes {
		// delete context hashes, inorder to avoid writing dirty entries to db
		delete(rawContext, hash.String())
	}

	randomSet, err := sm.NodeSet(info.RandomSet, info.RandomSetSizeWithoutDelta)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ics.RandomSetPosition(), randomSet)
	ics.UpdateNodeSetResponses(ics.RandomSetPosition(), info.Responses[ics.RandomSetPosition()])

	observerSet, err := sm.NodeSet(info.ObserverSet, uint32(len(info.ObserverSet)))
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ics.ObserverSetPosition(), observerSet)
	ics.UpdateNodeSetResponses(ics.ObserverSetPosition(), info.Responses[ics.ObserverSetPosition()])

	return ics, nil
}

// GetContextByHash fetches context using hash, if hash is nil, it returns error
func (sm *StateManager) GetContextByHash(
	address identifiers.Address,
	hash common.Hash,
) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error) {
	if address.IsNil() || hash.IsNil() {
		return common.NilHash, nil, nil, common.ErrEmptyHashAndAddress
	}

	behaviourSet, randomSet, err := sm.getContext(address, hash)
	if err != nil {
		return common.NilHash, nil, nil, err
	}

	return hash, behaviourSet, randomSet, nil
}

/*
func (sm *StateManager) FetchContextLock(ts *common.Tesseract) (*common.ICSNodeSet, error) {
	ix := ts.Interactions()[0]
	addrs := ts.Addresses()
	ps := ts.Participants()

	ics := common.NewICSNodeSet(len(addrs) + 2)

	for position, addr := range ts.Addresses() {
		if ps[addr].PreviousContext == common.NilHash {
			continue
		}

		behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(addr, ps[addr].PreviousContext)
		if err != nil {
			return nil, err
		}

	}

	for address, info := range ts.Participants() {
		if address == ix.Sender() {
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.PreviousContext)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(common.SenderBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(common.SenderRandomSet, randomSet)
		} else if address == ix.Receiver() || address == common.SargaAddress {
			if info.PreviousContext.IsNil() {
				continue
			}

			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.PreviousContext)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(common.ReceiverBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(common.ReceiverRandomSet, randomSet)
		}
	}

	return ics, nil
}
*/

func (sm *StateManager) IsAccountRegistered(addr identifiers.Address) (bool, error) {
	if addr.IsNil() {
		return true, nil
	}

	sargaObject, err := sm.GetLatestStateObject(common.SargaAddress)
	if err != nil {
		return true, errors.Wrap(err, common.ErrObjectNotFound.Error())
	}

	// Fetch the account info from genesis state
	_, err = sargaObject.GetStorageEntry(common.SargaLogicID, addr.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) IsAccountRegisteredAt(addr identifiers.Address, tesseractHash common.Hash) (bool, error) {
	ts, err := sm.getTesseractByHash(tesseractHash, false)
	if err != nil {
		return false, err
	}

	sargaObject, err := sm.GetStateObjectByHash(common.SargaAddress, ts.StateHash(common.SargaAddress))
	if err != nil {
		return false, err
	}

	_, err = sargaObject.GetStorageEntry(common.SargaLogicID, addr.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error) {
	if addr.IsNil() {
		return 0, common.ErrInvalidAddress
	}

	so, err := sm.getStateObject(addr, stateHash)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fetch state object")
	}

	return so.data.Nonce, nil
}

func (sm *StateManager) GetBalances(addrs identifiers.Address, stateHash common.Hash) (*BalanceObject, error) {
	stateObject, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	balances, err := stateObject.Balances()
	if err != nil {
		return nil, err
	}

	return balances.Copy(), nil
}

func (sm *StateManager) GetRegistry(addrs identifiers.Address, stateHash common.Hash) (map[string][]byte, error) {
	stateObject, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	registry, err := stateObject.Registry()
	if err != nil {
		return nil, err
	}

	return registry.Entries, nil
}

func (sm *StateManager) GetBalance(
	addrs identifiers.Address,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	so, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.BalanceOf(assetID)
}

func (sm *StateManager) GetAssetInfo(assetID identifiers.AssetID, state common.Hash) (*common.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(assetID.Address(), state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	rawDescriptor, err := stateObject.GetRegistryEntry(string(assetID))
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	ad := new(common.AssetDescriptor)
	if err = ad.FromBytes(rawDescriptor); err != nil {
		return nil, err
	}

	return ad, nil
}

func (sm *StateManager) GetAccTypeUsingStateObject(address identifiers.Address) (common.AccountType, error) {
	so, err := sm.GetDirtyObject(address)
	if err != nil {
		return 0, err
	}

	return so.accType, nil
}

func (sm *StateManager) GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr)
}

func (sm *StateManager) GetAccountState(addr identifiers.Address, stateHash common.Hash) (*common.Account, error) {
	rawData, err := sm.db.GetAccount(addr, stateHash)
	if err != nil {
		return nil, err
	}

	accInfo := new(common.Account)

	if err = accInfo.FromBytes(rawData); err != nil {
		return nil, err
	}

	return accInfo, nil
}

type Response struct {
	Data []string `json:"data"`
}
type Request struct {
	Ids []string `json:"kramaIDs"`
}

func (sm *StateManager) GetPublicKeys(ctx context.Context, ids ...kramaid.KramaID) ([][]byte, error) {
	if len(ids) == 0 {
		return nil, errors.New("Empty Ids")
	}

	publicKeys := make([][]byte, len(ids))

	g, _ := errgroup.WithContext(ctx)

	for index, kramaID := range ids {
		i, k := index, kramaID

		g.Go(
			func() error {
				return func(id kramaid.KramaID, index int) error {
					pk, err := sm.senatus.GetPublicKey(id)
					if err != nil {
						object, err := sm.getStateObject(common.GuardianLogicAddr, common.NilHash)
						if err != nil {
							return err
						}

						keys, err := guardianregistry.GetGuardianPublicKeys(object, id)
						if err != nil {
							sm.logger.Error("Failed to fetch the public key", "krama-ID", id)

							return err
						}

						if len(keys) == 0 {
							return nil
						}

						pk = keys[0]

						if err := sm.senatus.UpdatePublicKey(id, pk); err != nil {
							sm.logger.Error("Error updating the public key", "err", err)

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

// IsLogicRegistered checks if the logicID is registered with the account.
// If the logicID is not registered, this returns an error
func (sm *StateManager) IsLogicRegistered(logicID identifiers.LogicID) error {
	so, err := sm.GetLatestStateObject(logicID.Address())
	if err != nil {
		return err
	}

	return so.isLogicRegistered(logicID)
}

func (sm *StateManager) SyncStorageTrees(
	ctx context.Context,
	address identifiers.Address,
	newRoot *common.RootNode,
	logicStorageTreeRoots map[string]*common.RootNode,
) error {
	var (
		so  *Object
		err error
	)

	so, err = sm.GetDirtyObject(address)
	if err != nil {
		return err
	}

	metaStorageTree, err := so.getMetaStorageTree()
	if err != nil {
		return err
	}

	g, _ := errgroup.WithContext(ctx)

	for logic, rootNode := range logicStorageTreeRoots {
		storageRoot, logicID := rootNode, logic

		g.Go(func() error {
			return sm.syncLogicStorageTree(
				so,
				identifiers.LogicID(logicID),
				storageRoot,
			)
		})
	}

	if err = g.Wait(); err != nil {
		return err
	}

	// sync the metaStorageTree
	return sm.syncTree(metaStorageTree, newRoot)
}

func (sm *StateManager) syncLogicStorageTree(
	so *Object,
	logicID identifiers.LogicID,
	newRoot *common.RootNode,
) error {
	storageTree, err := so.GetStorageTree(logicID)
	if err != nil {
		switch {
		case errors.Is(err, common.ErrLogicStorageTreeNotFound):
			storageTree, err = so.createStorageTreeForLogic(logicID)
			if err != nil {
				return err
			}

		default:
			return err
		}
	}

	return sm.syncTree(storageTree, newRoot)
}

func (sm *StateManager) syncTree(
	tree tree.MerkleTree,
	newRoot *common.RootNode,
) error {
	// TODO: We're assuming that all key value entries are available
	match, err := doesRootMatch(tree.Root(), *newRoot)
	if err != nil {
		return err
	}

	if match {
		return nil
	}

	for key, value := range newRoot.HashTable {
		rawKey, err := hex.DecodeString(key)
		if err != nil {
			return err
		}

		if err = tree.Set(rawKey, value); err != nil {
			return errors.Wrap(err, "failed to set entry")
		}
	}

	if err = tree.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	match, err = doesRootMatch(tree.Root(), *newRoot)
	if err != nil {
		return err
	}

	if !match {
		return errors.New("updated root doesn't match")
	}

	if err = tree.Flush(); err != nil {
		return errors.Wrap(err, "failed to flush")
	}

	return nil
}

func (sm *StateManager) SyncLogicTree(
	address identifiers.Address,
	newRoot *common.RootNode,
) error {
	var (
		so  *Object
		err error
	)

	so, err = sm.GetDirtyObject(address)
	if err != nil {
		return err
	}

	logicTree, err := so.getLogicTree()
	if err != nil {
		return err
	}

	return sm.syncTree(logicTree, newRoot)
}

// GetStorageEntry returns the storage data associated with the given slot and logicID
func (sm *StateManager) GetStorageEntry(logicID identifiers.LogicID, slot []byte, state common.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.GetStorageEntry(logicID, slot)
}

// GetLogicManifest returns the manifest associated with the given logicID
func (sm *StateManager) GetLogicManifest(logicID identifiers.LogicID, stateHash common.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	logicObject, err := so.getLogicObject(logicID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch logic object")
	}

	logicManifest, err := sm.db.ReadEntry(storage.LogicManifestKey(logicID.Address(), logicObject.ManifestHash()))
	if err != nil {
		return nil, errors.Wrap(err, common.ErrFetchingLogicManifest.Error())
	}

	return logicManifest, nil
}

func doesRootMatch(root1 common.RootNode, root2 common.RootNode) (bool, error) {
	hash1, err := root1.Hash()
	if err != nil {
		return false, err
	}

	hash2, err := root2.Hash()
	if err != nil {
		return false, err
	}

	return hash1 == hash2, nil
}
