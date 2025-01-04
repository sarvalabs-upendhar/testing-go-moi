package state

import (
	"context"
	"encoding/hex"
	"math/big"

	"github.com/moby/locker"
	"github.com/sarvalabs/go-moi/crypto"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

const (
	MinimumContextSize = 1
	StateObjectLRUSize = 500
)

type Store interface {
	GetAccount(addr identifiers.Address, stateHash common.Hash) ([]byte, error)
	GetContext(addr identifiers.Address, contextHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error)
	GetDeeds(addr identifiers.Address, registryHash common.Hash) ([]byte, error)
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
	GetTesseract(
		hash common.Hash,
		withInteractions bool,
		withCommitInfo bool,
	) (*common.Tesseract, error)
}

type StateManager struct {
	cfg    *config.StateConfig
	logger hclog.Logger

	cache       *lru.Cache
	objectCache *lru.Cache
	treeCache   *fastcache.Cache

	objectLocks *locker.Locker
	sysLocks    *locker.Locker

	db      Store
	metrics *Metrics
}

func NewStateManager(
	db Store,
	logger hclog.Logger,
	cache *lru.Cache,
	metrics *Metrics,
	cfg *config.StateConfig,
	cacheStateObjects bool,
) (*StateManager, error) {
	sm := &StateManager{
		cfg:         cfg,
		cache:       cache,
		db:          db,
		treeCache:   fastcache.New(int(cfg.TreeCacheSize)),
		objectLocks: locker.New(),
		sysLocks:    locker.New(),

		logger:  logger.Named("State-Manager"),
		metrics: metrics,
	}

	if cacheStateObjects {
		var err error

		if sm.objectCache, err = lru.New(StateObjectLRUSize); err != nil {
			return nil, err
		}
	}

	sm.metrics.InitMetrics()

	return sm, nil
}

func (sm *StateManager) CreateStateObject(addr identifiers.Address,
	accType common.AccountType, isGenesis bool,
) *Object {
	stateObject := NewStateObject(addr, sm.cache, sm.treeCache, sm.db,
		common.Account{AccType: accType}, sm.metrics, isGenesis)

	return stateObject
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

func (sm *StateManager) RemoveCachedObject(addr identifiers.Address) {
	sm.logger.Trace("removing cached state object", addr)
	sm.objectLocks.Lock(addr.Hex())

	defer func() {
		if err := sm.objectLocks.Unlock(addr.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "addr", addr)
		}
	}()

	sm.objectCache.Remove(addr)
}

func (sm *StateManager) GetLatestStateObject(addr identifiers.Address) (*Object, error) {
	if sm.objectCache != nil {
		sm.objectLocks.Lock(addr.Hex())

		defer func() {
			if err := sm.objectLocks.Unlock(addr.Hex()); err != nil {
				sm.logger.Error("failed to unlock object", "err", err, "addr", addr)
			}
		}()

		data, isCached := sm.objectCache.Get(addr)
		if isCached {
			so, ok := data.(*Object)
			if !ok {
				return nil, common.ErrInterfaceConversion
			}

			sm.metrics.AddObjectCacheHitCount(1)

			return so, nil
		}
	}

	accMetaInfo, err := sm.GetAccountMetaInfo(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch acc meta info")
	}

	so, err := sm.GetStateObjectByHash(addr, accMetaInfo.StateHash)
	if err != nil {
		return nil, err
	}

	if sm.objectCache != nil {
		sm.objectCache.Add(addr, so)
		sm.metrics.AddObjectCacheMissCount(1)
	}

	return so, err
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

	sObj := NewStateObject(addr, sm.cache, sm.treeCache, sm.db, *acc, sm.metrics, false)

	return sObj, nil
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

// getTesseractByHash returns tesseract with/without interactions
// - with interactions always fetches from db
// - without interactions fetch from either cache or db
func (sm *StateManager) getTesseractByHash(
	hash common.Hash,
	withInteractions, withCommitInfo bool,
) (*common.Tesseract, error) {
	if withInteractions || withCommitInfo {
		return sm.db.GetTesseract(hash, withInteractions, withCommitInfo)
	}

	object, isCached := sm.cache.Get(hash)
	if !isCached {
		ts, err := sm.db.GetTesseract(hash, withInteractions, withCommitInfo)
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
		ts.FuelUsed(),
		ts.FuelLimit(),
		ts.ConsensusInfo(),
		ts.Seal(),
		ts.SealBy(),
		common.Interactions{},
		nil,
		nil,
	), nil
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

func (sm *StateManager) GetLatestContextAndPublicKeys(addr identifiers.Address) (
	latestContextHash common.Hash,
	behaviourSet, randomSet []kramaid.KramaID,
	bePublicKeys, rePublicKeys [][]byte,
	err error,
) {
	latestContextHash, err = sm.GetCommittedContextHash(addr)
	if err != nil {
		return common.NilHash, nil, nil, nil, nil, err
	}

	behaviourSet, randomSet, err = sm.GetContext(addr, latestContextHash)
	if err != nil {
		return common.NilHash, nil, nil, nil, nil, err
	}

	if len(behaviourSet) > 0 {
		bePublicKeys, err = sm.GetPublicKeys(context.Background(), behaviourSet...)
		if err != nil {
			sm.logger.Error("failed to retrieve the public key of behavioural set", "err", err)

			return common.NilHash, nil, nil, nil, nil, common.ErrPublicKeyNotFound
		}
	}

	if len(randomSet) > 0 {
		rePublicKeys, err = sm.GetPublicKeys(context.Background(), randomSet...)
		if err != nil {
			sm.logger.Error("failed to retrieve the public key of random set", "err", err)

			return common.NilHash, nil, nil, nil, nil, common.ErrPublicKeyNotFound
		}
	}

	return latestContextHash, behaviourSet, randomSet, bePublicKeys, rePublicKeys, err
}

func (sm *StateManager) GetCommittedContextHash(addr identifiers.Address) (common.Hash, error) {
	accMetaInfo, err := sm.GetAccountMetaInfo(addr)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to fetch account meta info")
	}

	return accMetaInfo.ContextHash, nil
}

func (sm *StateManager) GetICSSeed(addr identifiers.Address) ([32]byte, error) {
	metaInfo, err := sm.GetAccountMetaInfo(addr)
	if err != nil {
		return common.NilHash, err
	}

	ts, err := sm.getTesseractByHash(metaInfo.TesseractHash, false, false)
	if err != nil {
		return common.NilHash, err
	}

	return ts.ICSSeed(), nil
}

func (sm *StateManager) GetContext(
	addr identifiers.Address,
	hash common.Hash,
) (
	common.NodeList,
	common.NodeList,
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

// GetContextByHash fetches context using hash, if hash is nil, it returns error
func (sm *StateManager) GetContextByHash(
	address identifiers.Address,
	hash common.Hash,
) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error) {
	if address.IsNil() || hash.IsNil() {
		return common.NilHash, nil, nil, common.ErrEmptyHashAndAddress
	}

	behaviourSet, randomSet, err := sm.GetContext(address, hash)
	if err != nil {
		return common.NilHash, nil, nil, err
	}

	return hash, behaviourSet, randomSet, nil
}

func (sm *StateManager) IsInitialTesseract(ts *common.Tesseract, addr identifiers.Address) (bool, error) {
	var (
		accountRegistered bool
		err               error
	)

	if info, ok := ts.State(common.SargaAddress); !ok {
		accountRegistered, err = sm.IsAccountRegistered(addr)
	} else {
		sm.logger.Debug(
			"Checking for new account",
			"addr", addr,
			"height", info.Height,
			"ts-hash", info.TransitiveLink,
		)

		accountRegistered, err = sm.IsAccountRegisteredAt(addr, info.TransitiveLink)
	}

	return !accountRegistered && ts.Height(addr) == 0, err
}

func (sm *StateManager) IsAccountRegistered(addr identifiers.Address) (bool, error) {
	if addr.IsNil() {
		return true, nil
	}

	sm.sysLocks.Lock(common.SargaAddress.Hex())

	defer func() {
		if err := sm.sysLocks.Unlock(common.SargaAddress.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "addr", common.SargaAddress.Hex())
		}
	}()

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
	ts, err := sm.getTesseractByHash(tesseractHash, false, false)
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

func (sm *StateManager) GetBalances(addrs identifiers.Address, stateHash common.Hash) (common.AssetMap, error) {
	stateObject, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	balances, err := stateObject.Balances()
	if err != nil {
		return nil, err
	}

	return balances, nil
}

func (sm *StateManager) GetDeeds(
	addrs identifiers.Address, stateHash common.Hash,
) (map[string]*common.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	deeds, err := stateObject.Deeds()
	if err != nil {
		return nil, err
	}

	entries := make(map[string]*common.AssetDescriptor)

	for aid := range deeds.Entries {
		assetID := identifiers.AssetID(aid)

		stateObject, err = sm.getStateObject(assetID.Address(), common.NilHash)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch state object")
		}

		entries[aid], err = stateObject.GetState(assetID)
		if err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func (sm *StateManager) GetMandates(
	addrs identifiers.Address, stateHash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	stateObject, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return stateObject.Mandates()
}

func (sm *StateManager) GetLockups(
	addrs identifiers.Address, stateHash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	stateObject, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return stateObject.Lockups()
}

func (sm *StateManager) GetBalance(
	addrs identifiers.Address,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	so, err := sm.getStateObject(addrs, stateHash)
	if err != nil {
		return big.NewInt(0), errors.Wrap(err, "failed to fetch state object")
	}

	return so.BalanceOf(assetID)
}

func (sm *StateManager) GetAssetInfo(assetID identifiers.AssetID, state common.Hash) (*common.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(assetID.Address(), state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return stateObject.GetState(assetID)
}

func (sm *StateManager) GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(addr)
}

func (sm *StateManager) GetAccountState(addr identifiers.Address, stateHash common.Hash) (*common.Account, error) {
	rawData, err := sm.db.GetAccount(addr, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "account state not found")
	}

	accInfo := new(common.Account)

	if err = accInfo.FromBytes(rawData); err != nil {
		return nil, err
	}

	return accInfo, nil
}

func (sm *StateManager) GetPublicKeys(ctx context.Context, ids ...kramaid.KramaID) ([][]byte, error) {
	if len(ids) == 0 {
		return nil, errors.New("Empty Ids")
	}

	sm.sysLocks.Lock(common.GuardianLogicAddr.Hex())

	defer func() {
		if err := sm.sysLocks.Unlock(common.GuardianLogicAddr.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "addr", common.GuardianLogicAddr)
		}
	}()

	object, err := sm.getStateObject(common.GuardianLogicAddr, common.NilHash)
	if err != nil {
		return nil, err
	}

	storageReader := NewLogicStorageObject(common.GuardianLogicID, object)

	return guardianregistry.GetGuardianPublicKeys(storageReader, ids...)
}

func (sm *StateManager) GetGuardianIncentives(id kramaid.KramaID) (uint64, error) {
	sm.sysLocks.Lock(common.GuardianLogicAddr.Hex())

	defer func() {
		if err := sm.sysLocks.Unlock(common.GuardianLogicAddr.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "addr", common.GuardianLogicAddr)
		}
	}()

	object, err := sm.getStateObject(common.GuardianLogicAddr, common.NilHash)
	if err != nil {
		return 0, err
	}

	storageReader := NewLogicStorageObject(common.GuardianLogicID, object)

	return guardianregistry.GetGuardianIncentive(storageReader, id)
}

func (sm *StateManager) GetRegisteredGuardiansCount() (int, error) {
	sm.sysLocks.Lock(common.GuardianLogicAddr.Hex())

	defer func() {
		if err := sm.sysLocks.Unlock(common.GuardianLogicAddr.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "addr", common.GuardianLogicAddr)
		}
	}()

	object, err := sm.getStateObject(common.GuardianLogicAddr, common.NilHash)
	if err != nil {
		return 0, err
	}

	storageReader := NewLogicStorageObject(common.GuardianLogicID, object)

	return guardianregistry.GetGuardiansCount(storageReader)
}

func (sm *StateManager) GetTotalIncentives() (uint64, error) {
	sm.sysLocks.Lock(common.GuardianLogicAddr.Hex())

	defer func() {
		if err := sm.sysLocks.Unlock(common.GuardianLogicAddr.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "addr", common.GuardianLogicAddr)
		}
	}()

	object, err := sm.getStateObject(common.GuardianLogicAddr, common.NilHash)
	if err != nil {
		return 0, err
	}

	storageReader := NewLogicStorageObject(common.GuardianLogicID, object)

	return guardianregistry.GetTotalIncentives(storageReader)
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
	newRoot *common.RootNode,
	logicStorageTreeRoots map[string]*common.RootNode,
	so *Object,
) error {
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

func (sm *StateManager) SyncAssetTree(
	newRoot *common.RootNode,
	so *Object,
) error {
	assetTree, err := so.getAssetTree()
	if err != nil {
		return err
	}

	return sm.syncTree(assetTree, newRoot)
}

func (sm *StateManager) SyncLogicTree(
	newRoot *common.RootNode,
	so *Object,
) error {
	logicTree, err := so.getLogicTree()
	if err != nil {
		return err
	}

	return sm.syncTree(logicTree, newRoot)
}

// GetPersistentStorageEntry returns the storage data associated with the given slot and logicID
func (sm *StateManager) GetPersistentStorageEntry(
	logicID identifiers.LogicID, slot []byte, state common.Hash,
) ([]byte, error) {
	so, err := sm.getStateObject(logicID.Address(), state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.GetStorageEntry(logicID, slot)
}

// GetEphemeralStorageEntry returns the storage data associated with the given slot and logicID
func (sm *StateManager) GetEphemeralStorageEntry(
	addr identifiers.Address,
	logicID identifiers.LogicID,
	slot []byte,
	state common.Hash,
) ([]byte, error) {
	so, err := sm.getStateObject(addr, state)
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

func (sm *StateManager) getAuxiliaryStateObjects() (ObjectMap, error) {
	auxiliaryObjects := make(ObjectMap)

	auxObj, err := sm.GetLatestStateObject(common.SargaAddress)
	if err != nil {
		return nil, errors.Wrap(err, "state object fetch failed")
	}

	auxiliaryObjects[common.SargaAddress] = auxObj.Copy()

	return auxiliaryObjects, nil
}

func (sm *StateManager) LoadTransitionObjects(
	ixps map[identifiers.Address]common.ParticipantInfo,
) (*Transition, error) {
	// Create a new objects map
	objects := make(ObjectMap)

	for addr, p := range ixps {
		if p.IsGenesis {
			objects[addr] = sm.CreateStateObject(addr, p.AccType, true)

			continue
		}

		obj, err := sm.GetLatestStateObject(addr)
		if err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}

		// copy inorder to avoid modifications to cached object

		objects[addr] = obj.Copy()
	}

	auxiliaryObjects, err := sm.getAuxiliaryStateObjects()
	if err != nil {
		return nil, err
	}

	return NewTransition(objects, auxiliaryObjects), nil
}

func (sm *StateManager) FetchIxStateObjects(
	ixns common.Interactions,
	hashes map[identifiers.Address]common.Hash,
) (
	*Transition, error,
) {
	var err error

	ps := ixns.Participants()

	// Create a new objects map
	objects := make(ObjectMap)

	for addr, p := range ps {
		if p.IsGenesis {
			objects[addr] = sm.CreateStateObject(addr, p.AccType, true)

			continue
		}

		if stateHash, ok := hashes[addr]; !ok {
			if objects[addr], err = sm.GetLatestStateObject(addr); err != nil {
				return nil, errors.Wrap(err, "state object fetch failed")
			}
		} else if objects[addr], err = sm.GetStateObjectByHash(addr, stateHash); err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}
	}

	auxiliaryObjects, err := sm.getAuxiliaryStateObjects()
	if err != nil {
		return nil, err
	}

	return NewTransition(objects, auxiliaryObjects), nil
}

func (sm *StateManager) IsSealValid(ts *common.Tesseract) (bool, error) {
	publicKey, err := sm.GetPublicKeys(context.Background(), ts.SealBy())
	if err != nil {
		sm.logger.Error("Error fetching the public key", "err", err)

		return false, err
	}

	rawData, err := ts.SignBytes()
	if err != nil {
		return false, err
	}

	return crypto.Verify(rawData, ts.Seal(), publicKey[0])
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
