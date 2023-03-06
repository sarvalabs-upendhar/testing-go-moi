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

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/guna/tree"
	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/moichain/utils"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/telemetry/tracing"

	lru "github.com/hashicorp/golang-lru"

	"github.com/sarvalabs/moichain/dhruva"
)

const (
	minimumContextSize = 1
)

type store interface {
	GetAccount(addr types.Address, stateHash types.Hash) ([]byte, error)
	GetContext(addr types.Address, contextHash types.Hash) ([]byte, error)
	GetAccountMetaInfo(id []byte) (*types.AccountMetaInfo, error)
	GetInteractions(ixHash types.Hash) ([]byte, error)
	GetTesseract(tsHash types.Hash) ([]byte, error)
	GetBalance(addr types.Address, balanceHash types.Hash) ([]byte, error)
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

var (
	SargaAddress    = types.BytesToAddress(types.GetHash([]byte("sargaAccount")).Bytes())
	SargaLogicID, _ = types.NewLogicIDv0(0, true, false, 0, SargaAddress)
	GenesisIxHash   = types.GetHash([]byte("Genesis Interaction"))
)

type StateManager struct {
	ctx    context.Context
	logger hclog.Logger
	cache  *lru.Cache

	db store

	senatus senatus
	client  *http.Client

	objectsLock sync.Mutex
	objects     map[types.Address]*StateObject

	dirtyObjectsLock sync.Mutex
	dirtyObjects     map[types.Address]*StateObject

	metrics *Metrics
}

func NewStateManager(
	ctx context.Context,
	db store,
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
		objects:      make(map[types.Address]*StateObject),
		dirtyObjects: make(map[types.Address]*StateObject),
		logger:       logger.Named("State-Manager"),
		metrics:      metrics,
	}

	sm.metrics.initMetrics()

	return sm, nil
}

func (sm *StateManager) createStateObject(addr types.Address, accType types.AccountType) *StateObject {
	journal := new(Journal)
	stateObject := NewStateObject(addr, sm.cache, journal, sm.db, types.Account{}, accType)

	return stateObject
}

func (sm *StateManager) cleanupDirtyObject(addr types.Address) {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	delete(sm.dirtyObjects, addr)
	sm.metrics.captureActiveStateObjects(-1)
}

func (sm *StateManager) CreateDirtyObject(addr types.Address, accType types.AccountType) *StateObject {
	sm.dirtyObjectsLock.Lock()
	defer sm.dirtyObjectsLock.Unlock()

	obj := sm.createStateObject(addr, accType)

	sm.dirtyObjects[addr] = obj.Copy()
	sm.metrics.captureActiveStateObjects(1)

	return sm.dirtyObjects[addr]
}

func (sm *StateManager) FlushDirtyObject(addrs types.Address) error {
	so, err := sm.GetDirtyObject(addrs)
	if err != nil {
		return errors.Wrap(err, "failed to fetch state object")
	}

	if err = so.flushLogicTree(); err != nil {
		return errors.Wrap(err, "failed to fetch logic tree")
	}

	if err = so.flushActiveStorageTrees(); err != nil {
		return errors.Wrap(err, "failed to flush active storage trees")
	}

	for k, v := range so.GetDirtyStorage() {
		if err = sm.db.CreateEntry(types.FromHex(k), v); err != nil {
			return errors.Wrap(err, "failed to write dirty entries")
		}
	}

	return nil
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

	return sm.dirtyObjects[addr], nil
}

func (sm *StateManager) getStateObject(addr types.Address, stateHash types.Hash) (*StateObject, error) {
	if stateHash.IsNil() {
		return sm.GetLatestStateObject(addr)
	}

	return sm.GetStateObjectByHash(addr, stateHash)
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

	sm.objects[addr], err = sm.GetStateObjectByHash(addr, t.Body.StateHash)
	if err != nil {
		return nil, err
	}

	return sm.objects[addr], nil
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

	sObj := NewStateObject(addr, sm.cache, new(Journal), sm.db, *acc, acc.AccType)

	sObj.balance, err = getBalanceObject(addr, acc.Balance, sm.db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch balance object")
	}

	return sObj, nil
}

func (sm *StateManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	tesseractHash, err := sm.getLatestTesseractHash(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch latest tesseract hash")
	}

	return sm.getTesseractByHash(tesseractHash, withInteractions)
}

func (sm *StateManager) FetchTesseractFromDB(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	// Fetch Tesseract from DB
	buf, err := sm.db.GetTesseract(hash)
	if err != nil {
		return nil, err
	}

	// canonicalTesseract is a clone of the tesseract. The only difference is that it won't have the interactions field.
	canonicalTesseract := new(types.CanonicalTesseract)

	if err = canonicalTesseract.FromBytes(buf); err != nil {
		return nil, errors.Wrap(err, "failed to depolorize tesseract")
	}

	interactions := new(types.Interactions)

	if withInteractions && canonicalTesseract.Header.Height > 0 {
		// Fetch interactions from DB
		buf, err = sm.db.GetInteractions(canonicalTesseract.Body.InteractionHash)
		if err != nil {
			return nil, errors.Wrap(err, types.ErrFetchingInteractions.Error())
		}

		if err := interactions.FromBytes(buf); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return nil, errors.Wrap(err, "failed to depolarize interactions")
			}
		}
	}

	tesseract := &types.Tesseract{
		Header: canonicalTesseract.Header,
		Body:   canonicalTesseract.Body,
		Ixns:   *interactions,
		Seal:   canonicalTesseract.Seal,
	}

	return tesseract, nil
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

	return ts, nil
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
	behaviouralSet, randomSet *ktypes.NodeSet,
	err error,
) {
	behaviouralContext, randomContext, err := sm.getContext(addr, hash)
	if err != nil {
		sm.logger.Error("failed to retrieve sender context nodes", "error", err)

		return nil, nil, err
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

		sm.logger.Debug("Fetching context info", "addr", address.Hex(), ts.Body.ContextHash)

		hash = ts.Body.ContextHash
	}

	behaviourSet, randomSet, err := sm.getContext(address, hash)
	if err != nil {
		return types.NilHash, nil, nil, err
	}

	return hash, behaviourSet, randomSet, nil
}

func (sm *StateManager) FetchContextLock(ts *types.Tesseract) (*ktypes.ICSNodes, error) {
	ix := ts.Interactions()[0]
	ics := ktypes.NewICSNodes(6)

	for address, info := range ts.Header.ContextLock {
		if address == ix.Sender() {
			behaviourSet, randomSet, err := sm.fetchParticipantContextByHash(address, info.ContextHash)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(ktypes.SenderBehaviourSet, behaviourSet)
			ics.UpdateNodeSet(ktypes.SenderRandomSet, randomSet)
		} else if address == ix.Receiver() || address == SargaAddress {
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
		behaviourSet  *ktypes.NodeSet
		randomSet     *ktypes.NodeSet
		contextHash   types.Hash
		err           error
		contextHashes = make(map[types.Address]types.Hash)
		nodeSet       = make([]*ktypes.NodeSet, 6)
	)

	if !ix.Sender().IsNil() {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.Sender())
		if err != nil {
			return nil, nil, err
		}

		contextHashes[ix.Sender()] = contextHash
		nodeSet[ktypes.SenderBehaviourSet] = behaviourSet
		nodeSet[ktypes.SenderRandomSet] = randomSet
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
	nodeSet []*ktypes.NodeSet,
	contextHashes map[types.Address]types.Hash,
) error {
	var (
		behaviourSet *ktypes.NodeSet
		randomSet    *ktypes.NodeSet
		contextHash  types.Hash
	)

	accountRegistered, err := sm.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	if !accountRegistered {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(SargaAddress)
		if err != nil {
			return err
		}

		contextHashes[SargaAddress] = contextHash
	} else {
		contextHash, behaviourSet, randomSet, err = sm.fetchLatestParticipantContext(ix.Receiver())
		if err != nil {
			return err
		}

		contextHashes[ix.Receiver()] = contextHash
	}

	nodeSet[ktypes.ReceiverBehaviourSet] = behaviourSet
	nodeSet[ktypes.ReceiverRandomSet] = randomSet

	return nil
}

func (sm *StateManager) IsAccountRegistered(addr types.Address) (bool, error) {
	if addr.IsNil() {
		return true, nil
	}

	sargaObject, err := sm.GetLatestStateObject(SargaAddress)
	if err != nil {
		return true, errors.Wrap(types.ErrObjectNotFound, err.Error())
	}

	// Fetch the account info from genesis state
	_, err = sargaObject.GetStorageEntry(SargaLogicID, addr.Bytes())
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

	sargaObject, err := sm.GetStateObjectByHash(ts.Header.Address, ts.Body.StateHash)
	if err != nil {
		return false, err
	}

	_, err = sargaObject.GetStorageEntry(SargaLogicID, addr.Bytes())
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

	return stateObject.balance.Copy(), nil
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

func (sm *StateManager) GetAccTypeUsingStateObject(address types.Address) (types.AccountType, error) {
	so, err := sm.GetDirtyObject(address)
	if err != nil {
		return 0, err
	}

	return so.accType, nil
}

func (sm *StateManager) GetAccountMetaInfo(addr types.Address) (*types.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr.Bytes())
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

func (sm *StateManager) SetupSargaAccount(
	sargaAcc *gtypes.AccountSetupArgs,
	otherAccounts []*gtypes.AccountSetupArgs,
) (types.Hash, types.Hash, error) {
	if sargaAcc.Address != SargaAddress {
		return types.NilHash, types.NilHash, errors.New("invalid sarga account address")
	}

	stateObject := sm.CreateDirtyObject(SargaAddress, types.SargaAccount)

	if _, err := stateObject.CreateContext(sargaAcc.BehaviouralContext, sargaAcc.RandomContext); err != nil {
		return types.NilHash, types.NilHash, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := stateObject.CreateStorageTreeForLogic(SargaLogicID); err != nil {
		return types.NilHash, types.NilHash, errors.Wrap(err, "failed to create storage tree")
	}

	for _, info := range otherAccounts {
		if info.Address != SargaAddress {
			// Add account to sarga storage tree
			if err := stateObject.AddAccountGenesisInfo(info.Address, GenesisIxHash); err != nil {
				return types.NilHash, types.NilHash, err
			}
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return types.NilHash, types.NilHash, err
	}

	return stateHash, stateObject.data.ContextHash, nil
}

func (sm *StateManager) SetupNewAccount(info *gtypes.AccountSetupArgs) (types.Hash, types.Hash, error) {
	stateObject := sm.CreateDirtyObject(info.Address, info.AccType)

	if _, err := stateObject.CreateContext(info.BehaviouralContext, info.RandomContext); err != nil {
		return types.NilHash, types.NilHash, errors.Wrap(err, "context initiation failed in genesis")
	}

	if len(info.Assets) > 0 {
		for _, asset := range info.Assets {
			_, err := stateObject.CreateAsset(asset)
			if err != nil {
				return types.NilHash, types.NilHash, errors.Wrap(err, "failed to create an asset")
			}
		}
	}

	if len(info.Balances) > 0 {
		for assetID, balance := range info.Balances {
			stateObject.AddBalance(assetID, balance)
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return types.NilHash, types.NilHash, err
	}

	return stateHash, stateObject.data.ContextHash, nil
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

	g, _ := errgroup.WithContext(context.Background())

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

	g, _ := errgroup.WithContext(context.Background())

	for logic, rootNode := range logicStorageTreeRoots {
		storageRoot, logicID := rootNode, logic

		g.Go(func() error {
			return sm.syncLogicStorageTree(
				so,
				[]byte(logicID),
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
	for key, value := range newRoot.HashTable {
		if err := tree.Set([]byte(key), value); err != nil {
			return errors.Wrap(err, "failed to set entry")
		}
	}

	if err := tree.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	updatedLocalRoot, err := tree.RootHash()
	if err != nil {
		return err
	}

	actualRoot, err := newRoot.Hash()
	if err != nil {
		return err
	}

	if updatedLocalRoot != actualRoot {
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
