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

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/guna/tree"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/telemetry/tracing"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

const (
	MinimumContextSize = 1
)

type Store interface {
	GetAccount(addr types.Address, stateHash types.Hash) ([]byte, error)
	GetContext(addr types.Address, contextHash types.Hash) ([]byte, error)
	GetAccountMetaInfo(id types.Address) (*types.AccountMetaInfo, error)
	GetInteractions(ixHash types.Hash) ([]byte, error)
	GetTesseract(tsHash types.Hash) ([]byte, error)
	GetBalance(addr types.Address, balanceHash types.Hash) ([]byte, error)
	GetAssetRegistry(addr types.Address, registryHash types.Hash) ([]byte, error)
	GetMerkleTreeEntry(address types.Address, prefix dhruva.Prefix, key []byte) ([]byte, error)
	SetMerkleTreeEntry(address types.Address, prefix dhruva.Prefix, key, value []byte) error
	SetMerkleTreeEntries(address types.Address, prefix dhruva.Prefix, entries map[string][]byte) error
	WritePreImages(address types.Address, entries map[types.Hash][]byte) error
	GetPreImage(address types.Address, hash types.Hash) ([]byte, error)
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
	dirtyObjects     map[types.Address]*StateObject

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
		dirtyObjects: make(map[types.Address]*StateObject),
		logger:       logger.Named("State-Manager"),
		metrics:      metrics,
	}

	sm.metrics.initMetrics()

	return sm, nil
}

func (sm *StateManager) createStateObject(addr types.Address, accType types.AccountType) *StateObject {
	journal := new(Journal)
	stateObject := NewStateObject(addr, sm.cache, journal, sm.db, types.Account{AccType: accType})

	return stateObject
}

func (sm *StateManager) cleanupDirtyObject(addr types.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
	sm.metrics.captureActiveStateObjects(float64(len(sm.dirtyObjects)))
}

func (sm *StateManager) CreateDirtyObject(addr types.Address, accType types.AccountType) *StateObject {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	obj := sm.createStateObject(addr, accType)

	sm.dirtyObjects[addr] = obj.Copy()
	sm.metrics.captureActiveStateObjects(float64(len(sm.dirtyObjects)))

	return sm.dirtyObjects[addr]
}

func (sm *StateManager) FlushDirtyObject(addrs types.Address) error {
	so, err := sm.GetDirtyObject(addrs)
	if err != nil {
		return errors.Wrap(err, "failed to fetch state object")
	}

	return so.flush()
}

func (sm *StateManager) GetDirtyObject(addr types.Address) (*StateObject, error) {
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

func (sm *StateManager) getStateObject(addr types.Address, stateHash types.Hash) (*StateObject, error) {
	if stateHash.IsNil() {
		return sm.GetLatestStateObject(addr)
	}

	return sm.GetStateObjectByHash(addr, stateHash)
}

func (sm *StateManager) GetLatestStateObject(addr types.Address) (*StateObject, error) {
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

func (sm *StateManager) GetStateObjectByHash(addr types.Address, hash types.Hash) (*StateObject, error) {
	// read the state
	data, err := sm.db.GetAccount(addr, hash)
	if err != nil {
		return nil, errors.Wrap(types.ErrStateNotFound, err.Error())
	}

	acc := new(types.Account)
	if err = acc.FromBytes(data); err != nil {
		return nil, err
	}

	sObj := NewStateObject(addr, sm.cache, new(Journal), sm.db, *acc)

	return sObj, nil
}

func (sm *StateManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	tesseractHash, err := sm.getLatestTesseractHash(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch latest tesseract hash")
	}

	return sm.getTesseractByHash(tesseractHash, withInteractions)
}

func (sm *StateManager) GetLogicIDs(addr types.Address, stateHash types.Hash) ([]types.LogicID, error) {
	obj, err := sm.getStateObject(addr, stateHash)
	if err != nil {
		return nil, err
	}

	logicIDs := make([]types.LogicID, 0)

	logicTree, err := obj.getMetaLogicTree()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load meta logic tree")
	}

	it := logicTree.NewIterator()

	for it.Next() {
		if it.Leaf() {
			logicID, err := obj.logicTree.GetPreImageKey(types.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			logicIDs = append(logicIDs, types.BytesToLogicID(logicID))
		}
	}

	return logicIDs, nil
}

func (sm *StateManager) FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	// Fetch Tesseract from DB
	rawTesseract, err := sm.db.GetTesseract(hash)
	if err != nil {
		return nil, err
	}

	// canonicalTesseract is a clone of the tesseract. The only difference is that it won't have the interactions field.
	canonicalTesseract := new(types.CanonicalTesseract)

	if err = canonicalTesseract.FromBytes(rawTesseract); err != nil {
		return nil, err
	}

	interactions := new(types.Interactions)

	if withInteractions && canonicalTesseract.Header.Height > 0 {
		// Fetch interactions from DB
		gridHash, err := canonicalTesseract.GridHash()
		if err != nil {
			return nil, err
		}

		rawIxns, err := sm.db.GetInteractions(gridHash)
		if err != nil {
			return nil, errors.Wrap(err, types.ErrFetchingInteractions.Error())
		}

		if err := interactions.FromBytes(rawIxns); err != nil {
			return nil, err
		}
	}

	return canonicalTesseract.ToTesseract(*interactions), nil
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

	accMetaInfo, err := sm.db.GetAccountMetaInfo(addr)
	if err != nil {
		return types.NilHash, errors.Wrap(err, "account meta info fetch failed")
	}

	sm.cache.Add(addr, accMetaInfo.TesseractHash)

	return accMetaInfo.TesseractHash, nil
}

// getTesseractByHash returns tesseract with/without interactions
// - with interactions always fetches from db
// - without interactions fetches from cache or db
func (sm *StateManager) getTesseractByHash(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
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

	ts, ok := object.(*types.Tesseract)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return types.NewTesseract(ts.Header(), ts.Body(), nil, nil, ts.Seal(), ts.Sealer()), nil
}

func (sm *StateManager) Cleanup(address types.Address) {
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

	if err := object.FromBytes(rawData); err != nil {
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

	if err = object.FromBytes(rawData); err != nil {
		return nil, errors.Wrap(err, "MetaContextObject deserialization failed")
	}

	sm.cache.Add(hash, object)

	return object, nil
}

// fetchParticipantContextByHash fetches the context info based on the give hash
// and returns a NodeSet which holds the kramaIDs and public keys
func (sm *StateManager) fetchParticipantContextByHash(addr types.Address, hash types.Hash) (
	behaviouralSet, randomSet *types.NodeSet,
	err error,
) {
	behaviouralContext, randomContext, err := sm.getContext(addr, hash)
	if err != nil {
		sm.logger.Error("failed to retrieve sender context nodes", "error", err)

		return nil, nil, err
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = types.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return nil, nil, types.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = types.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return nil, nil, types.ErrPublicKeyNotFound
		}
	}

	return behaviouralSet, randomSet, nil
}

func (sm *StateManager) fetchLatestParticipantContext(addr types.Address) (
	latestContextHash types.Hash,
	behaviouralSet, randomSet *types.NodeSet,
	err error,
) {
	latestContextHash, err = sm.GetCommittedContextHash(addr)
	if err != nil {
		return types.NilHash, nil, nil, err
	}

	behaviouralContext, randomContext, err := sm.getContext(addr, latestContextHash)
	if err != nil {
		return types.NilHash, nil, nil, err
	}

	if len(behaviouralContext) > 0 {
		behaviouralSet = types.NewNodeSet(behaviouralContext, nil)

		if behaviouralSet.PublicKeys, err = sm.GetPublicKeys(behaviouralContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return types.NilHash, nil, nil, types.ErrPublicKeyNotFound
		}
	}

	if len(randomContext) > 0 {
		randomSet = types.NewNodeSet(randomContext, nil)

		if randomSet.PublicKeys, err = sm.GetPublicKeys(randomContext...); err != nil {
			sm.logger.Error("failed to retrieve public Key", "error", err)

			return types.NilHash, nil, nil, types.ErrPublicKeyNotFound
		}
	}

	return latestContextHash, behaviouralSet, randomSet, nil
}

func (sm *StateManager) GetCommittedContextHash(add types.Address) (types.Hash, error) {
	tesseract, err := sm.GetLatestTesseract(add, false)
	if err != nil {
		return types.NilHash, err
	}

	return tesseract.ContextHash(), nil
}

func (sm *StateManager) getContext(addr types.Address, hash types.Hash) ([]id.KramaID, []id.KramaID, error) {
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
	address types.Address,
	hash types.Hash,
	rawContext map[types.Hash][]byte,
) error {
	metaObjectRaw, err := sm.db.GetContext(address, hash)
	if err != nil {
		return err
	}

	metaObject := new(gtypes.MetaContextObject)
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

func (sm *StateManager) GetNodeSet(ids []id.KramaID) (*types.NodeSet, error) {
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

	return types.NewNodeSet(ids, publicKeys), nil
}

func (sm *StateManager) GetICSNodeSetFromRawContext(
	ts *types.Tesseract,
	rawContext map[types.Hash][]byte,
	clusterInfo *types.ICSClusterInfo,
) (*types.ICSNodeSet, error) {
	ix := ts.Interactions()[0]
	ics := types.NewICSNodeSet(6)

	for address, contextLock := range ts.ContextLock() {
		if contextLock.ContextHash == types.NilHash {
			continue
		}

		metaObject := new(gtypes.MetaContextObject)
		if err := metaObject.FromBytes(rawContext[contextLock.ContextHash]); err != nil {
			return nil, err
		}

		if address == ix.Sender() {
			rawBytes, ok := rawContext[metaObject.BehaviouralContext]
			if ok {
				behaviourObject := new(gtypes.ContextObject)
				if err := behaviourObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(behaviourObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(types.SenderBehaviourSet, nodeSet)
			}

			rawBytes, ok = rawContext[metaObject.RandomContext]
			if ok {
				randomObject := new(gtypes.ContextObject)
				if err := randomObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(randomObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(types.SenderRandomSet, nodeSet)
			}
		} else if address == ix.Receiver() || address == types.SargaAddress {
			rawBytes, ok := rawContext[metaObject.BehaviouralContext]
			if ok {
				behaviourObject := new(gtypes.ContextObject)
				if err := behaviourObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(behaviourObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(types.ReceiverBehaviourSet, nodeSet)
			}

			rawBytes, ok = rawContext[metaObject.RandomContext]
			if ok {
				randomObject := new(gtypes.ContextObject)
				if err := randomObject.FromBytes(rawBytes); err != nil {
					return nil, err
				}

				nodeSet, err := sm.GetNodeSet(randomObject.Ids)
				if err != nil {
					return nil, err
				}

				ics.UpdateNodeSet(types.ReceiverRandomSet, nodeSet)
			}
		}
	}

	randomSet, err := sm.GetNodeSet(clusterInfo.RandomSet)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(types.RandomSet, randomSet)

	observerSet, err := sm.GetNodeSet(clusterInfo.ObserverSet)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(types.ObserverSet, observerSet)

	for index, set := range ics.Nodes {
		if set != nil && clusterInfo.Responses[index] != nil {
			set.Responses = clusterInfo.Responses[index]
		}
	}

	return ics, nil
}

// GetContextByHash fetches context using hash, if hash is nil, it returns error
func (sm *StateManager) GetContextByHash(
	address types.Address,
	hash types.Hash,
) (types.Hash, []id.KramaID, []id.KramaID, error) {
	if address.IsNil() || hash.IsNil() {
		return types.NilHash, nil, nil, types.ErrEmptyHashAndAddress
	}

	behaviourSet, randomSet, err := sm.getContext(address, hash)
	if err != nil {
		return types.NilHash, nil, nil, err
	}

	return hash, behaviourSet, randomSet, nil
}

func (sm *StateManager) FetchContextLock(ts *types.Tesseract) (*types.ICSNodeSet, error) {
	ix := ts.Interactions()[0]
	ics := types.NewICSNodeSet(6)

	for address, info := range ts.ContextLock() {
		if address == ix.Sender() {
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(types.SenderBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(types.SenderRandomSet, randomSet)
		} else if address == ix.Receiver() || address == types.SargaAddress {
			if info.ContextHash.IsNil() {
				continue
			}

			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(types.ReceiverBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(types.ReceiverRandomSet, randomSet)
		}
	}

	return ics, nil
}

// FetchInteractionContext returns a nodeSet which holds the latest context info of the interaction participants
func (sm *StateManager) FetchInteractionContext(ctx context.Context, ix *types.Interaction) (
	map[types.Address]types.Hash,
	[]*types.NodeSet,
	error,
) {
	_, span := tracing.Span(ctx, "guna.StateManger", "FetchInteractionContext")
	defer span.End()

	var (
		behaviourSet  *types.NodeSet
		randomSet     *types.NodeSet
		contextHash   types.Hash
		err           error
		contextHashes = make(map[types.Address]types.Hash)
		nodeSet       = make([]*types.NodeSet, 6)
	)

	if !ix.Sender().IsNil() {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.Sender())
		if err != nil {
			return nil, nil, err
		}

		contextHashes[ix.Sender()] = contextHash
		nodeSet[types.SenderBehaviourSet] = behaviourSet
		nodeSet[types.SenderRandomSet] = randomSet
	}

	if !ix.Receiver().IsNil() {
		if err = sm.getReceiverContext(ix, nodeSet, contextHashes); err != nil {
			return nil, nil, err
		}
	}

	return contextHashes, nodeSet, err
}

func (sm *StateManager) getReceiverContext(
	ix *types.Interaction,
	nodeSet []*types.NodeSet,
	contextHashes map[types.Address]types.Hash,
) error {
	var (
		behaviourSet *types.NodeSet
		randomSet    *types.NodeSet
		contextHash  types.Hash
	)

	accountRegistered, err := sm.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	if !accountRegistered {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(types.SargaAddress)
		if err != nil {
			return err
		}

		contextHashes[types.SargaAddress] = contextHash
	} else {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.Receiver())
		if err != nil {
			return err
		}

		contextHashes[ix.Receiver()] = contextHash
	}

	nodeSet[types.ReceiverBehaviourSet] = behaviourSet
	nodeSet[types.ReceiverRandomSet] = randomSet

	return nil
}

func (sm *StateManager) IsAccountRegistered(addr types.Address) (bool, error) {
	if addr.IsNil() {
		return true, nil
	}

	sargaObject, err := sm.GetLatestStateObject(types.SargaAddress)
	if err != nil {
		return true, errors.Wrap(err, types.ErrObjectNotFound.Error())
	}

	// Fetch the account info from genesis state
	_, err = sargaObject.GetStorageEntry(types.SargaLogicID, addr.Bytes())
	if errors.Is(err, types.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) IsAccountRegisteredAt(addr types.Address, tesseractHash types.Hash) (bool, error) {
	ts, err := sm.getTesseractByHash(tesseractHash, false)
	if err != nil {
		return false, err
	}

	sargaObject, err := sm.GetStateObjectByHash(ts.Address(), ts.Body().StateHash)
	if err != nil {
		return false, err
	}

	_, err = sargaObject.GetStorageEntry(types.SargaLogicID, addr.Bytes())
	if errors.Is(err, types.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) GetNonce(addr types.Address, stateHash types.Hash) (uint64, error) {
	if addr.IsNil() {
		return 0, types.ErrInvalidAddress
	}

	so, err := sm.getStateObject(addr, stateHash)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fetch state object")
	}

	return so.data.Nonce, nil
}

func (sm *StateManager) GetBalances(addrs types.Address, stateHash types.Hash) (*gtypes.BalanceObject, error) {
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

func (sm *StateManager) GetRegistry(addrs types.Address, stateHash types.Hash) (map[string][]byte, error) {
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
	addrs types.Address,
	assetID types.AssetID,
	stateHash types.Hash,
) (*big.Int, error) {
	so, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.BalanceOf(assetID)
}

func (sm *StateManager) GetAssetInfo(assetID types.AssetID, stateHash types.Hash) (*types.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(assetID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	rawDescriptor, err := stateObject.GetRegistryEntry(assetID.String())
	if err != nil {
		return nil, types.ErrAssetNotFound
	}

	ad := new(types.AssetDescriptor)
	if err = ad.FromBytes(rawDescriptor); err != nil {
		return nil, err
	}

	return ad, nil
}

func (sm *StateManager) GetAccTypeUsingStateObject(address types.Address) (types.AccountType, error) {
	so, err := sm.GetDirtyObject(address)
	if err != nil {
		return 0, err
	}

	return so.accType, nil
}

func (sm *StateManager) GetAccountMetaInfo(addr types.Address) (*types.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr)
}

func (sm *StateManager) GetAccountState(addr types.Address, stateHash types.Hash) (*types.Account, error) {
	rawData, err := sm.db.GetAccount(addr, stateHash)
	if err != nil {
		return nil, err
	}

	accInfo := new(types.Account)

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
							return err
						}

						if len(keys) == 0 {
							return nil
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

func (sm *StateManager) GetPublicKeyFromContract(ids ...id.KramaID) (keys [][]byte, err error) {
	return RetrievePublicKeys(ids, sm.client, sm.logger)
}

// IsLogicRegistered checks if the logicID is registered with the account.
// If the logicID is not registered, this returns an error
func (sm *StateManager) IsLogicRegistered(logicID types.LogicID) error {
	so, err := sm.GetLatestStateObject(logicID.Address())
	if err != nil {
		return err
	}

	return so.isLogicRegistered(logicID)
}

func (sm *StateManager) SyncStorageTrees(
	address types.Address,
	newRoot *types.RootNode,
	logicStorageTreeRoots map[string]*types.RootNode,
) error {
	var (
		so  *StateObject
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
				types.LogicID(logicID),
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
	so *StateObject,
	logicID types.LogicID,
	newRoot *types.RootNode,
) error {
	storageTree, err := so.GetStorageTree(logicID)
	if err != nil {
		switch {
		case errors.Is(err, types.ErrLogicStorageTreeNotFound):
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
	newRoot *types.RootNode,
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
	address types.Address,
	newRoot *types.RootNode,
) error {
	var (
		so  *StateObject
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
func (sm *StateManager) GetStorageEntry(logicID types.LogicID, slot []byte, stateHash types.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.GetStorageEntry(logicID, slot)
}

// GetLogicManifest returns the manifest associated with the given logicID
func (sm *StateManager) GetLogicManifest(logicID types.LogicID, stateHash types.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	logicObject, err := so.getLogicObject(logicID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch logic object")
	}

	logicManifest, err := sm.db.ReadEntry(types.FromHex(logicObject.ManifestHash.Hex()))
	if err != nil {
		return nil, errors.Wrap(err, types.ErrFetchingLogicManifest.Error())
	}

	return logicManifest, nil
}

var RetrievePublicKeys = func(ids []id.KramaID, client *http.Client, logger hclog.Logger) (keys [][]byte, err error) {
	data, err := json.Marshal(Request{utils.KramaIDToString(ids)})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "http://91.107.196.74/api/fetchPublicKeys", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	response, err := client.Do(req)
	if err != nil {
		logger.Error("Api fetch failed", "error", err, "kramaIDs", ids)

		return nil, err
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Panicln(err)
	}

	defer response.Body.Close()

	if response.StatusCode != 200 {
		logger.Error("Http request failed", response.StatusCode, string(body))
	}

	data1 := new(Response)

	if err = json.Unmarshal(body, data1); err != nil {
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

func doesRootMatch(root1 types.RootNode, root2 types.RootNode) (bool, error) {
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
