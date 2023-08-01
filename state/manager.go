package state

import (
	"context"
	"encoding/hex"
	"math/big"
	"net/http"
	"sync"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/pisa"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-moi/telemetry/tracing"
)

const (
	MinimumContextSize = 1
)

type Store interface {
	GetAccount(addr common.Address, stateHash common.Hash) ([]byte, error)
	GetContext(addr common.Address, contextHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id common.Address) (*common.AccountMetaInfo, error)
	GetInteractions(ixHash common.Hash) ([]byte, error)
	GetTesseract(tsHash common.Hash) ([]byte, error)
	GetBalance(addr common.Address, balanceHash common.Hash) ([]byte, error)
	GetAssetRegistry(addr common.Address, registryHash common.Hash) ([]byte, error)
	GetMerkleTreeEntry(address common.Address, prefix storage.Prefix, key []byte) ([]byte, error)
	SetMerkleTreeEntry(address common.Address, prefix storage.Prefix, key, value []byte) error
	SetMerkleTreeEntries(address common.Address, prefix storage.Prefix, entries map[string][]byte) error
	WritePreImages(address common.Address, entries map[common.Hash][]byte) error
	GetPreImage(address common.Address, hash common.Hash) ([]byte, error)
	DeleteEntry(key []byte) error
	CreateEntry(key []byte, value []byte) error
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	UpdateEntry(key []byte, newValue []byte) error
	NewBatchWriter() db.BatchWriter
}

type senatus interface {
	GetPublicKey(kramaID id.KramaID) ([]byte, error)
	UpdatePublicKey(kramaID id.KramaID, pk []byte) error
}

type StateManager struct {
	ctx    context.Context
	logger hclog.Logger
	cache  *lru.Cache

	db Store

	senatus senatus
	client  *http.Client

	dirtyObjectsLock sync.Mutex
	dirtyObjects     map[common.Address]*Object

	metrics *Metrics
}

func NewStateManager(
	ctx context.Context,
	db Store,
	logger hclog.Logger,
	cache *lru.Cache,
	metrics *Metrics,
	senatus senatus,
) (*StateManager, error) {
	sm := &StateManager{
		ctx:     ctx,
		cache:   cache,
		db:      db,
		senatus: senatus,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConns:    1024,
			MaxConnsPerHost: 1000,
		}},
		dirtyObjects: make(map[common.Address]*Object),
		logger:       logger.Named("State-Manager"),
		metrics:      metrics,
	}

	sm.metrics.initMetrics()

	return sm, nil
}

func (sm *StateManager) createStateObject(addr common.Address, accType common.AccountType) *Object {
	journal := new(Journal)
	stateObject := NewStateObject(addr, sm.cache, journal, sm.db, common.Account{AccType: accType})

	return stateObject
}

func (sm *StateManager) cleanupDirtyObject(addr common.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
	sm.metrics.captureActiveStateObjects(float64(len(sm.dirtyObjects)))
}

func (sm *StateManager) CreateDirtyObject(addr common.Address, accType common.AccountType) *Object {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	obj := sm.createStateObject(addr, accType)

	sm.dirtyObjects[addr] = obj.Copy()
	sm.metrics.captureActiveStateObjects(float64(len(sm.dirtyObjects)))

	return sm.dirtyObjects[addr]
}

func (sm *StateManager) FlushDirtyObject(addrs common.Address) error {
	so, err := sm.GetDirtyObject(addrs)
	if err != nil {
		return errors.Wrap(err, "failed to fetch state object")
	}

	return so.flush()
}

func (sm *StateManager) GetDirtyObject(addr common.Address) (*Object, error) {
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

	sm.metrics.captureActiveStateObjects(float64(len(sm.dirtyObjects)))

	return sm.dirtyObjects[addr], nil
}

func (sm *StateManager) getStateObject(addr common.Address, stateHash common.Hash) (*Object, error) {
	if stateHash.IsNil() {
		return sm.GetLatestStateObject(addr)
	}

	return sm.GetStateObjectByHash(addr, stateHash)
}

func (sm *StateManager) GetLatestStateObject(addr common.Address) (*Object, error) {
	t, err := sm.GetLatestTesseract(addr, false)
	if err != nil {
		return nil, err
	}

	obj, err := sm.GetStateObjectByHash(addr, t.StateHash())
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (sm *StateManager) GetStateObjectByHash(addr common.Address, hash common.Hash) (*Object, error) {
	// read the state
	data, err := sm.db.GetAccount(addr, hash)
	if err != nil {
		return nil, errors.Wrap(common.ErrStateNotFound, err.Error())
	}

	acc := new(common.Account)
	if err = acc.FromBytes(data); err != nil {
		return nil, err
	}

	sObj := NewStateObject(addr, sm.cache, new(Journal), sm.db, *acc)

	return sObj, nil
}

func (sm *StateManager) GetLatestTesseract(addr common.Address, withInteractions bool) (*common.Tesseract, error) {
	tesseractHash, err := sm.getLatestTesseractHash(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch latest tesseract hash")
	}

	return sm.getTesseractByHash(tesseractHash, withInteractions)
}

func (sm *StateManager) GetLogicIDs(addr common.Address, stateHash common.Hash) ([]common.LogicID, error) {
	obj, err := sm.getStateObject(addr, stateHash)
	if err != nil {
		return nil, err
	}

	logicIDs := make([]common.LogicID, 0)

	logicTree, err := obj.getMetaLogicTree()
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

			logicIDs = append(logicIDs, common.BytesToLogicID(logicID))
		}
	}

	return logicIDs, nil
}

func (sm *StateManager) FetchTesseractFromDB(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
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

	if withInteractions && canonicalTesseract.Header.ClusterID != common.GenesisIdentifier {
		// Fetch interactions from DB
		gridHash, err := canonicalTesseract.GridHash()
		if err != nil {
			return nil, err
		}

		rawIxns, err := sm.db.GetInteractions(gridHash)
		if err != nil {
			return nil, errors.Wrap(err, common.ErrFetchingInteractions.Error())
		}

		if err := interactions.FromBytes(rawIxns); err != nil {
			return nil, err
		}
	}

	return canonicalTesseract.ToTesseract(*interactions), nil
}

func (sm *StateManager) getLatestTesseractHash(addr common.Address) (common.Hash, error) {
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
func (sm *StateManager) getTesseractByHash(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
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

	return common.NewTesseract(ts.Header(), ts.Body(), nil, nil, ts.Seal(), ts.Sealer()), nil
}

func (sm *StateManager) Cleanup(address common.Address) {
	sm.cleanupDirtyObject(address)
}

func (sm *StateManager) Revert(snap *Object) error {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	if snap != nil {
		sm.logger.Info("Reverting back the state object", "addr", snap.address.Hex())
		sm.dirtyObjects[snap.address] = snap
		sm.metrics.captureNumOfReverts(1)
	}

	return nil
}

func (sm *StateManager) getContextObject(addr common.Address, hash common.Hash) (*ContextObject, error) {
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

func (sm *StateManager) getMetaContextObject(addr common.Address, hash common.Hash) (*MetaContextObject, error) {
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
func (sm *StateManager) fetchParticipantContextByHash(addr common.Address, hash common.Hash) (
	behaviouralSet, randomSet *common.NodeSet,
	err error,
) {
	behaviouralContext, randomContext, err := sm.getContext(addr, hash)
	if err != nil {
		sm.logger.Error("Failed to retrieve the sender context nodes", "err", err)

		return nil, nil, err
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = common.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of behavioural set", "err", err)

			return nil, nil, common.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = common.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of random set", "err", err)

			return nil, nil, common.ErrPublicKeyNotFound
		}
	}

	return behaviouralSet, randomSet, nil
}

func (sm *StateManager) fetchLatestParticipantContext(addr common.Address) (
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
		behaviouralSet = common.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of behavioural set", "err", err)

			return common.NilHash, nil, nil, common.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = common.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("Failed to retrieve the public key of random set", "err", err)

			return common.NilHash, nil, nil, common.ErrPublicKeyNotFound
		}
	}

	return latestContextHash, behaviouralSet, randomSet, nil
}

func (sm *StateManager) GetCommittedContextHash(add common.Address) (common.Hash, error) {
	tesseract, err := sm.GetLatestTesseract(add, false)
	if err != nil {
		return common.NilHash, err
	}

	return tesseract.ContextHash(), nil
}

func (sm *StateManager) getContext(addr common.Address, hash common.Hash) ([]id.KramaID, []id.KramaID, error) {
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

// GetParticipantContextRaw loads the context info of a participant into the give map
func (sm *StateManager) GetParticipantContextRaw(
	address common.Address,
	hash common.Hash,
	rawContext map[common.Hash][]byte,
) error {
	metaObjectRaw, err := sm.db.GetContext(address, hash)
	if err != nil {
		return err
	}

	metaObject := new(MetaContextObject)
	if err = metaObject.FromBytes(metaObjectRaw); err != nil {
		return err
	}

	rawContext[hash] = metaObjectRaw

	if !metaObject.BehaviouralContext.IsNil() {
		behavioural, err := sm.db.GetContext(address, metaObject.BehaviouralContext)
		if err != nil {
			return errors.Wrap(err, "failed to fetch behavioural context")
		}

		rawContext[metaObject.BehaviouralContext] = behavioural
	}

	if !metaObject.RandomContext.IsNil() {
		random, err := sm.db.GetContext(address, metaObject.RandomContext)
		if err != nil {
			return errors.Wrap(err, "failed to fetch behavioural context")
		}

		rawContext[metaObject.RandomContext] = random
	}

	return nil
}

func (sm *StateManager) GetNodeSet(ids []id.KramaID) (*common.NodeSet, error) {
	var (
		publicKeys [][]byte
		err        error
	)

	if len(ids) > 0 {
		publicKeys, err = sm.GetPublicKeys(ids...)
		if err != nil {
			return nil, err
		}
	}

	return common.NewNodeSet(ids, publicKeys), nil
}

func (sm *StateManager) FetchICSNodeSet(
	ts *common.Tesseract,
	info *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	icsNodeSets, err := sm.FetchContextLock(ts)
	if err != nil {
		return nil, err
	}

	if info.Responses == nil {
		return nil, errors.New("nil responses slice")
	}

	randomSet, err := sm.GetNodeSet(info.RandomSet)
	if err != nil {
		return nil, err
	}

	icsNodeSets.UpdateNodeSet(common.RandomSet, randomSet)

	observerSet, err := sm.GetNodeSet(info.ObserverSet)
	if err != nil {
		return nil, err
	}

	icsNodeSets.UpdateNodeSet(common.ObserverSet, observerSet)

	for index, set := range icsNodeSets.Nodes {
		if set != nil && info.Responses[index] != nil {
			set.Responses = info.Responses[index]
		}
	}

	return icsNodeSets, nil
}

func (sm *StateManager) GetICSNodeSetFromRawContext(
	ts *common.Tesseract,
	rawContext map[common.Hash][]byte,
	clusterInfo *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	ix := ts.Interactions()[0]
	ics := common.NewICSNodeSet(6)

	for address, contextLock := range ts.ContextLock() {
		if contextLock.ContextHash == common.NilHash {
			continue
		}

		metaObject := new(MetaContextObject)
		if err := metaObject.FromBytes(rawContext[contextLock.ContextHash]); err != nil {
			return nil, err
		}

		if address == ix.Sender() {
			rawBytes, ok := rawContext[metaObject.BehaviouralContext]
			if ok {
				behaviourObject := new(ContextObject)
				if err := behaviourObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(behaviourObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(common.SenderBehaviourSet, nodeSet)
			}

			rawBytes, ok = rawContext[metaObject.RandomContext]
			if ok {
				randomObject := new(ContextObject)
				if err := randomObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(randomObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(common.SenderRandomSet, nodeSet)
			}
		} else if address == ix.Receiver() || address == common.SargaAddress {
			rawBytes, ok := rawContext[metaObject.BehaviouralContext]
			if ok {
				behaviourObject := new(ContextObject)
				if err := behaviourObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(behaviourObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(common.ReceiverBehaviourSet, nodeSet)
			}

			rawBytes, ok = rawContext[metaObject.RandomContext]
			if ok {
				randomObject := new(ContextObject)
				if err := randomObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(randomObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(common.ReceiverRandomSet, nodeSet)
			}
		}
	}

	randomSet, err := sm.GetNodeSet(clusterInfo.RandomSet)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(common.RandomSet, randomSet)

	observerSet, err := sm.GetNodeSet(clusterInfo.ObserverSet)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(common.ObserverSet, observerSet)

	for index, set := range ics.Nodes {
		if set != nil && clusterInfo.Responses[index] != nil {
			set.Responses = clusterInfo.Responses[index]
		}
	}

	return ics, nil
}

// GetContextByHash fetches context using hash, if hash is nil, it returns error
func (sm *StateManager) GetContextByHash(
	address common.Address,
	hash common.Hash,
) (common.Hash, []id.KramaID, []id.KramaID, error) {
	if address.IsNil() || hash.IsNil() {
		return common.NilHash, nil, nil, common.ErrEmptyHashAndAddress
	}

	behaviourSet, randomSet, err := sm.getContext(address, hash)
	if err != nil {
		return common.NilHash, nil, nil, err
	}

	return hash, behaviourSet, randomSet, nil
}

func (sm *StateManager) FetchContextLock(ts *common.Tesseract) (*common.ICSNodeSet, error) {
	ix := ts.Interactions()[0]
	ics := common.NewICSNodeSet(6)

	for address, info := range ts.ContextLock() {
		if address == ix.Sender() {
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(common.SenderBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(common.SenderRandomSet, randomSet)
		} else if address == ix.Receiver() || address == common.SargaAddress {
			if info.ContextHash.IsNil() {
				continue
			}

			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(common.ReceiverBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(common.ReceiverRandomSet, randomSet)
		}
	}

	return ics, nil
}

// FetchInteractionContext returns a nodeSet which holds the latest context info of the interaction participants
func (sm *StateManager) FetchInteractionContext(ctx context.Context, ix *common.Interaction) (
	map[common.Address]common.Hash,
	[]*common.NodeSet,
	error,
) {
	_, span := tracing.Span(ctx, "guna.StateManger", "FetchInteractionContext")
	defer span.End()

	var (
		behaviourSet  *common.NodeSet
		randomSet     *common.NodeSet
		contextHash   common.Hash
		err           error
		contextHashes = make(map[common.Address]common.Hash)
		nodeSet       = make([]*common.NodeSet, 6)
	)

	if !ix.Sender().IsNil() {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.Sender())
		if err != nil {
			return nil, nil, err
		}

		contextHashes[ix.Sender()] = contextHash
		nodeSet[common.SenderBehaviourSet] = behaviourSet
		nodeSet[common.SenderRandomSet] = randomSet
	}

	if !ix.Receiver().IsNil() {
		if err = sm.getReceiverContext(ix, nodeSet, contextHashes); err != nil {
			return nil, nil, err
		}
	}

	return contextHashes, nodeSet, err
}

func (sm *StateManager) getReceiverContext(
	ix *common.Interaction,
	nodeSet []*common.NodeSet,
	contextHashes map[common.Address]common.Hash,
) error {
	var (
		behaviourSet *common.NodeSet
		randomSet    *common.NodeSet
		contextHash  common.Hash
	)

	accountRegistered, err := sm.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	if !accountRegistered {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(common.SargaAddress)
		if err != nil {
			return err
		}

		contextHashes[common.SargaAddress] = contextHash
	} else {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.Receiver())
		if err != nil {
			return err
		}

		contextHashes[ix.Receiver()] = contextHash
	}

	nodeSet[common.ReceiverBehaviourSet] = behaviourSet
	nodeSet[common.ReceiverRandomSet] = randomSet

	return nil
}

func (sm *StateManager) IsAccountRegistered(addr common.Address) (bool, error) {
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

func (sm *StateManager) IsAccountRegisteredAt(addr common.Address, tesseractHash common.Hash) (bool, error) {
	ts, err := sm.getTesseractByHash(tesseractHash, false)
	if err != nil {
		return false, err
	}

	sargaObject, err := sm.GetStateObjectByHash(ts.Address(), ts.Body().StateHash)
	if err != nil {
		return false, err
	}

	_, err = sargaObject.GetStorageEntry(common.SargaLogicID, addr.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) GetNonce(addr common.Address, stateHash common.Hash) (uint64, error) {
	if addr.IsNil() {
		return 0, common.ErrInvalidAddress
	}

	so, err := sm.getStateObject(addr, stateHash)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fetch state object")
	}

	return so.data.Nonce, nil
}

func (sm *StateManager) GetBalances(addrs common.Address, stateHash common.Hash) (*BalanceObject, error) {
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

func (sm *StateManager) GetRegistry(addrs common.Address, stateHash common.Hash) (map[string][]byte, error) {
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
	addrs common.Address,
	assetID common.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	so, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.BalanceOf(assetID)
}

func (sm *StateManager) GetAssetInfo(assetID common.AssetID, stateHash common.Hash) (*common.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(assetID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	rawDescriptor, err := stateObject.GetRegistryEntry(assetID.String())
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	ad := new(common.AssetDescriptor)
	if err = ad.FromBytes(rawDescriptor); err != nil {
		return nil, err
	}

	return ad, nil
}

func (sm *StateManager) GetAccTypeUsingStateObject(address common.Address) (common.AccountType, error) {
	so, err := sm.GetDirtyObject(address)
	if err != nil {
		return 0, err
	}

	return so.accType, nil
}

func (sm *StateManager) GetAccountMetaInfo(addr common.Address) (*common.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr)
}

func (sm *StateManager) GetAccountState(addr common.Address, stateHash common.Hash) (*common.Account, error) {
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

func (sm *StateManager) GetPublicKeys(ids ...id.KramaID) ([][]byte, error) {
	if len(ids) == 0 {
		return nil, errors.New("Empty Ids")
	}

	publicKeys := make([][]byte, len(ids))

	g, _ := errgroup.WithContext(sm.ctx)

	for index, kramaID := range ids {
		i, k := index, kramaID

		g.Go(
			func() error {
				return func(id id.KramaID, index int) error {
					pk, err := sm.senatus.GetPublicKey(id)
					if err != nil {
						keys, err := sm.GetPublicKeyFromContract(id)
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

func (sm *StateManager) GetPublicKeyFromContract(ids ...id.KramaID) (keys [][]byte, err error) {
	pk := make([][]byte, 0, len(ids))

	object, err := sm.getStateObject(common.GuardianLogicAddr, common.NilHash)
	if err != nil {
		return nil, err
	}

	data, err := object.GetStorageEntry(common.GuardianLogicID, pisa.SlotHash(GuardianSLot))
	if err != nil {
		return nil, err
	}

	var guardians Guardians
	if err = polo.Depolorize(&guardians, data); err != nil {
		return nil, errors.Wrap(err, "failed to depolorize guardians")
	}

	for _, kramaID := range ids {
		guardian, ok := guardians[string(kramaID)]
		if !ok {
			return nil, errors.New("public key not found")
		}

		pk = append(pk, guardian.PublicKey)
	}

	return pk, nil
}

// IsLogicRegistered checks if the logicID is registered with the account.
// If the logicID is not registered, this returns an error
func (sm *StateManager) IsLogicRegistered(logicID common.LogicID) error {
	so, err := sm.GetLatestStateObject(logicID.Address())
	if err != nil {
		return err
	}

	return so.isLogicRegistered(logicID)
}

func (sm *StateManager) SyncStorageTrees(
	address common.Address,
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

	g, _ := errgroup.WithContext(sm.ctx)

	for logic, rootNode := range logicStorageTreeRoots {
		storageRoot, logicID := rootNode, logic

		g.Go(func() error {
			return sm.syncLogicStorageTree(
				so,
				common.LogicID(logicID),
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
	logicID common.LogicID,
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
	address common.Address,
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

	logicTree, err := so.getMetaLogicTree()
	if err != nil {
		return err
	}

	return sm.syncTree(logicTree, newRoot)
}

// GetStorageEntry returns the storage data associated with the given slot and logicID
func (sm *StateManager) GetStorageEntry(logicID common.LogicID, slot []byte, stateHash common.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.GetStorageEntry(logicID, slot)
}

// GetLogicManifest returns the manifest associated with the given logicID
func (sm *StateManager) GetLogicManifest(logicID common.LogicID, stateHash common.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	logicObject, err := so.getLogicObject(logicID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch logic object")
	}

	logicManifest, err := sm.db.ReadEntry(storage.LogicManifestKey(logicID.Address(), logicObject.ManifestHash))
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
