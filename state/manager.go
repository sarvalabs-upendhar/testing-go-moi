package state

import (
	"context"
	"encoding/hex"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/moby/locker"
	"github.com/sarvalabs/go-moi/crypto"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

const (
	MinimumContextSize = 1
	StateObjectLRUSize = 500
)

type Store interface {
	GetAccount(id identifiers.Identifier, stateHash common.Hash) ([]byte, error)
	GetContext(id identifiers.Identifier, contextHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	GetDeeds(id identifiers.Identifier, registryHash common.Hash) ([]byte, error)
	GetAccountKeys(id identifiers.Identifier, stateHash common.Hash) ([]byte, error)
	GetMerkleTreeEntry(id identifiers.Identifier, prefix storage.PrefixTag, key []byte) ([]byte, error)
	SetMerkleTreeEntry(id identifiers.Identifier, prefix storage.PrefixTag, key, value []byte) error
	SetMerkleTreeEntries(id identifiers.Identifier, prefix storage.PrefixTag, entries map[string][]byte) error
	WritePreImages(id identifiers.Identifier, entries map[common.Hash][]byte) error
	GetPreImage(id identifiers.Identifier, hash common.Hash) ([]byte, error)
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

	systemRegistry *SystemRegistry

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

		systemRegistry: NewSystemObjectRegistry(),
		logger:         logger.Named("State-Manager"),
		metrics:        metrics,
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

func (sm *StateManager) CreateStateObject(
	id identifiers.Identifier,
	accType common.AccountType, isGenesis bool,
) *Object {
	stateObject := NewStateObject(id, sm.cache, sm.treeCache, sm.db,
		common.Account{AccType: accType}, sm.metrics, isGenesis)

	return stateObject
}

func (sm *StateManager) CreateSystemObject(id identifiers.Identifier) *SystemObject {
	systemObject := NewSystemObject(sm.CreateStateObject(id, common.SystemAccount, true))

	sm.systemRegistry.SetSystemObject(systemObject)

	return systemObject
}

func (sm *StateManager) initSystemStateObject(id identifiers.Identifier) (*SystemObject, error) {
	stateObject, err := sm.GetLatestStateObject(id)
	if err != nil {
		stateObject = sm.CreateStateObject(id, common.SystemAccount, true)
	}

	systemObject := NewSystemObject(stateObject)

	if err = systemObject.Init(); err != nil {
		return nil, err
	}

	sm.systemRegistry.SetSystemObject(systemObject)

	return systemObject, nil
}

func (sm *StateManager) GetSystemObject() *SystemObject {
	systemObject := sm.systemRegistry.GetSystemObject()
	if systemObject != nil {
		return systemObject
	}

	systemObject, err := sm.initSystemStateObject(common.SystemAccountID)
	if err != nil {
		panic(err)
	}

	return systemObject
}

func (sm *StateManager) GetConsensusNodes(
	id identifiers.Identifier, hash common.Hash,
) (common.NodeList, common.Hash, error) {
	consensusNodes, consensusHash, err := sm.GetConsensusNodeIDs(id, hash)
	if err != nil {
		return nil, common.NilHash, err
	}

	validators, err := sm.systemRegistry.GetSystemObject().GetValidatorsByKramaID(consensusNodes)
	if err != nil {
		return nil, common.NilHash, err
	}

	return validators, consensusHash, nil
}

func (sm *StateManager) HasParticipantStateAt(id identifiers.Identifier, stateHash common.Hash) bool {
	if _, err := sm.db.GetAccount(id, stateHash); err != nil {
		return false
	}

	return true
}

func (sm *StateManager) getStateObject(id identifiers.Identifier, stateHash common.Hash) (*Object, error) {
	if stateHash.IsNil() {
		return sm.GetLatestStateObject(id)
	}

	return sm.GetStateObjectByHash(id, stateHash)
}

func (sm *StateManager) RefreshCachedObject(id identifiers.Identifier, sysObj *SystemObject) {
	sm.objectLocks.Lock(id.Hex())

	defer func() {
		if err := sm.objectLocks.Unlock(id.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "id", id)
		}
	}()

	sm.objectCache.Remove(id)

	if sysObj != nil && sysObj.id == id {
		sm.systemRegistry.SetSystemObject(sysObj)
	}
}

func (sm *StateManager) GetLatestStateObject(id identifiers.Identifier) (*Object, error) {
	if sm.objectCache != nil {
		sm.objectLocks.Lock(id.Hex())

		defer func() {
			if err := sm.objectLocks.Unlock(id.Hex()); err != nil {
				sm.logger.Error("failed to unlock object", "err", err, "id", id)
			}
		}()

		data, isCached := sm.objectCache.Get(id)
		if isCached {
			so, ok := data.(*Object)
			if !ok {
				return nil, common.ErrInterfaceConversion
			}

			sm.metrics.AddObjectCacheHitCount(1)

			return so, nil
		}
	}

	accMetaInfo, err := sm.GetAccountMetaInfo(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch acc meta info")
	}

	so, err := sm.GetStateObjectByHash(id, accMetaInfo.StateHash)
	if err != nil {
		return nil, err
	}

	if sm.objectCache != nil {
		sm.objectCache.Add(id, so)
		sm.metrics.AddObjectCacheMissCount(1)
	}

	return so, err
}

func (sm *StateManager) GetStateObjectByHash(id identifiers.Identifier, hash common.Hash) (*Object, error) {
	// read the state
	data, err := sm.db.GetAccount(id, hash)
	if err != nil {
		return nil, errors.Wrap(common.ErrStateNotFound, err.Error())
	}

	acc := new(common.Account)
	if err = acc.FromBytes(data); err != nil {
		return nil, err
	}

	sObj := NewStateObject(id, sm.cache, sm.treeCache, sm.db, *acc, sm.metrics, false)

	return sObj, nil
}

func (sm *StateManager) GetSubAccountCount(id identifiers.Identifier) (uint64, error) {
	accMetaInfo, err := sm.GetAccountMetaInfo(id)
	if err != nil {
		return 0, common.ErrAccountNotFound
	}

	mCtx, err := sm.GetMetaContextObject(id, accMetaInfo.ContextHash)
	if err != nil {
		return 0, err
	}

	return uint64(len(mCtx.SubAccounts)), nil
}

func (sm *StateManager) GetLogicIDs(id identifiers.Identifier, stateHash common.Hash) ([]identifiers.LogicID, error) {
	obj, err := sm.getStateObject(id, stateHash)
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

			logicIDs = append(logicIDs, identifiers.LogicID(logicID))
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

func (sm *StateManager) GetMetaContextObject(id identifiers.Identifier, hash common.Hash) (*MetaContextObject, error) {
	metaData, isAvailable := sm.cache.Get(hash)
	if isAvailable {
		metaContextObject, ok := metaData.(*MetaContextObject)
		if !ok {
			return nil, common.ErrInterfaceConversion
		}

		return metaContextObject, nil
	}

	rawData, err := sm.db.GetContext(id, hash)
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

func (sm *StateManager) GetLatestContextAndPublicKeys(id identifiers.Identifier) (
	latestContextHash common.Hash,
	consensusNodesHash common.Hash,
	vals []*common.ValidatorInfo,
	err error,
) {
	latestContextHash, err = sm.GetCommittedContextHash(id)
	if err != nil {
		return common.NilHash, common.NilHash, nil, err
	}

	consensusNodes, consensusNodesHash, err := sm.GetConsensusNodes(id, latestContextHash)
	if err != nil {
		return common.NilHash, common.NilHash, nil, err
	}

	return latestContextHash, consensusNodesHash, consensusNodes, nil
}

func (sm *StateManager) GetCommittedContextHash(id identifiers.Identifier) (common.Hash, error) {
	accMetaInfo, err := sm.GetAccountMetaInfo(id)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to fetch account meta info")
	}

	if id.IsParticipantVariant() {
		accMetaInfo, err = sm.GetAccountMetaInfo(accMetaInfo.InheritedAccount)
		if err != nil {
			return common.NilHash, errors.Wrap(err, "failed to fetch account meta info of sub account")
		}
	}

	return accMetaInfo.ContextHash, nil
}

func (sm *StateManager) GetICSSeed(id identifiers.Identifier) ([32]byte, error) {
	metaInfo, err := sm.GetAccountMetaInfo(id)
	if err != nil {
		return common.NilHash, err
	}

	ts, err := sm.getTesseractByHash(metaInfo.TesseractHash, false, false)
	if err != nil {
		return common.NilHash, err
	}

	return ts.ICSSeed(), nil
}

func (sm *StateManager) GetConsensusNodeIDs(
	id identifiers.Identifier,
	hash common.Hash,
) (
	[]identifiers.KramaID,
	common.Hash,
	error,
) {
	if id.IsParticipantVariant() {
		accMetaInfo, err := sm.db.GetAccountMetaInfo(id)
		if err != nil {
			return nil, common.NilHash, errors.Wrap(err, "failed to fetch acc meta info")
		}

		id = accMetaInfo.InheritedAccount
	}

	metaContextObject, err := sm.GetMetaContextObject(id, hash)
	if err != nil {
		return nil, common.NilHash, errors.Wrap(err, "metaContextObject fetch failed")
	}

	return metaContextObject.ConsensusNodes, metaContextObject.ConsensusNodesHash, nil
}

// GetParticipantContextRaw loads the context info of a participant into the given map
func (sm *StateManager) GetParticipantContextRaw(
	id identifiers.Identifier,
	hash common.Hash,
	rawContext map[string][]byte,
) error {
	metaObjectRaw, err := sm.db.GetContext(id, hash)
	if err != nil {
		return err
	}

	metaObject := new(MetaContextObject)
	if err = metaObject.FromBytes(metaObjectRaw); err != nil {
		return err
	}

	rawContext[hash.String()] = metaObjectRaw

	return nil
}

// GetConsensusNodesByHash fetches context using hash, if hash is nil, it returns error
func (sm *StateManager) GetConsensusNodesByHash(
	id identifiers.Identifier,
	hash common.Hash,
) ([]identifiers.KramaID, error) {
	if id.IsNil() || hash.IsNil() {
		return nil, common.ErrEmptyHashAndID
	}

	nodes, _, err := sm.GetConsensusNodeIDs(id, hash)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func (sm *StateManager) IsInitialTesseract(ts *common.Tesseract, id identifiers.Identifier) (bool, error) {
	var (
		accountRegistered bool
		err               error
	)

	if info, ok := ts.State(common.SargaAccountID); !ok {
		accountRegistered, err = sm.IsAccountRegistered(id)
	} else {
		sm.logger.Debug(
			"Checking for new account",
			"id", id,
			"height", info.Height,
			"ts-hash", info.TransitiveLink,
		)

		accountRegistered, err = sm.IsAccountRegisteredAt(id, info.TransitiveLink)
	}

	return !accountRegistered && ts.Height(id) == 0, err
}

func (sm *StateManager) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	if id.IsNil() {
		return true, nil
	}

	sm.sysLocks.Lock(common.SargaAccountID.String())

	defer func() {
		if err := sm.sysLocks.Unlock(common.SargaAccountID.String()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "id", common.SargaAccountID.Hex())
		}
	}()

	sargaObject, err := sm.GetLatestStateObject(common.SargaAccountID)
	if err != nil {
		return true, errors.Wrap(err, common.ErrObjectNotFound.Error())
	}

	// Fetch the account info from genesis state
	_, err = sargaObject.GetStorageEntry(common.SargaLogicID, id.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) IsAccountRegisteredAt(id identifiers.Identifier, tesseractHash common.Hash) (bool, error) {
	ts, err := sm.getTesseractByHash(tesseractHash, false, false)
	if err != nil {
		return false, err
	}

	sargaObject, err := sm.GetStateObjectByHash(common.SargaAccountID, ts.StateHash(common.SargaAccountID))
	if err != nil {
		return false, err
	}

	_, err = sargaObject.GetStorageEntry(common.SargaLogicID, id.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	return true, err
}

func (sm *StateManager) GetSequenceID(id identifiers.Identifier, keyID uint64, stateHash common.Hash) (uint64, error) {
	if id.IsNil() {
		return 0, common.ErrInvalidIdentifier
	}

	so, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fetch state object")
	}

	return so.SequenceID(keyID)
}

func (sm *StateManager) GetPublicKey(id identifiers.Identifier, keyID uint64, stateHash common.Hash) ([]byte, error) {
	so, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.PublicKey(keyID)
}

func (sm *StateManager) GetBalances(id identifiers.Identifier, stateHash common.Hash) (common.AssetMap, error) {
	stateObject, err := sm.getStateObject(id, stateHash)
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
	id identifiers.Identifier, stateHash common.Hash,
) (map[identifiers.Identifier]*common.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	deeds, err := stateObject.Deeds()
	if err != nil {
		return nil, err
	}

	entries := make(map[identifiers.Identifier]*common.AssetDescriptor)

	for aid := range deeds.Entries {
		assetID := identifiers.AssetID(aid)

		stateObject, err = sm.getStateObject(assetID.AsIdentifier(), common.NilHash)
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
	id identifiers.Identifier, stateHash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	stateObject, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return stateObject.Mandates()
}

func (sm *StateManager) GetLockups(
	id identifiers.Identifier, stateHash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	stateObject, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return stateObject.Lockups()
}

func (sm *StateManager) GetAccountKeys(id identifiers.Identifier, stateHash common.Hash) (common.AccountKeys, error) {
	so, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.AccountKeys()
}

func (sm *StateManager) GetBalance(
	id identifiers.Identifier,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	so, err := sm.getStateObject(id, stateHash)
	if err != nil {
		return big.NewInt(0), errors.Wrap(err, "failed to fetch state object")
	}

	return so.BalanceOf(assetID)
}

func (sm *StateManager) GetAssetInfo(assetID identifiers.AssetID, state common.Hash) (*common.AssetDescriptor, error) {
	stateObject, err := sm.getStateObject(assetID.AsIdentifier(), state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return stateObject.GetState(assetID)
}

func (sm *StateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	return sm.db.GetAccountMetaInfo(id)
}

func (sm *StateManager) GetAccountState(id identifiers.Identifier, stateHash common.Hash) (*common.Account, error) {
	rawData, err := sm.db.GetAccount(id, stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "account state not found")
	}

	accInfo := new(common.Account)

	if err = accInfo.FromBytes(rawData); err != nil {
		return nil, err
	}

	return accInfo, nil
}

func (sm *StateManager) GetPublicKeys(ids ...identifiers.KramaID) ([][]byte, error) {
	if len(ids) == 0 {
		return nil, errors.New("Empty Ids")
	}

	sm.sysLocks.Lock(common.SystemAccountID.Hex())

	defer func() {
		if err := sm.sysLocks.Unlock(common.SystemAccountID.Hex()); err != nil {
			sm.logger.Error("failed to unlock object", "err", err, "id", common.SystemAccountID)
		}
	}()

	pubkeys, err := sm.GetSystemObject().GetValidatorPublicKeys(ids)
	if err != nil {
		return nil, err
	}

	return pubkeys, nil
}

// IsLogicRegistered checks if the logicID is registered with the account.
// If the logicID is not registered, this returns an error
func (sm *StateManager) IsLogicRegistered(logicID identifiers.LogicID) error {
	so, err := sm.GetLatestStateObject(logicID.AsIdentifier())
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
		storageRoot := rootNode

		logicID, err := hex.DecodeString(logic)
		if err != nil {
			return err
		}

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
	so, err := sm.getStateObject(logicID.AsIdentifier(), state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.GetStorageEntry(logicID, slot)
}

// GetEphemeralStorageEntry returns the storage data associated with the given slot and logicID
func (sm *StateManager) GetEphemeralStorageEntry(
	id identifiers.Identifier,
	logicID identifiers.LogicID,
	slot []byte,
	state common.Hash,
) ([]byte, error) {
	so, err := sm.getStateObject(id, state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	return so.GetStorageEntry(logicID, slot)
}

// GetLogicManifest returns the manifest associated with the given logicID
func (sm *StateManager) GetLogicManifest(logicID identifiers.LogicID, stateHash common.Hash) ([]byte, error) {
	so, err := sm.getStateObject(logicID.AsIdentifier(), stateHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch state object")
	}

	logicObject, err := so.getLogicObject(logicID.AsIdentifier())
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch logic object")
	}

	logicManifest, err := sm.db.ReadEntry(storage.LogicManifestKey(logicID.AsIdentifier(), logicObject.ManifestHash()))
	if err != nil {
		return nil, errors.Wrap(err, common.ErrFetchingLogicManifest.Error())
	}

	return logicManifest, nil
}

func (sm *StateManager) GetValidators() []*common.Validator {
	return sm.GetSystemObject().Validators()
}

func (sm *StateManager) GetValidatorByKramaID(kramaID identifiers.KramaID) (*common.Validator, error) {
	validator, err := sm.GetSystemObject().ValidatorByKramaID(kramaID)
	if err != nil {
		return nil, err
	}

	return validator, nil
}

func (sm *StateManager) getAuxiliaryStateObjects() (ObjectMap, error) {
	auxiliaryObjects := make(ObjectMap)

	auxObj, err := sm.GetLatestStateObject(common.SargaAccountID)
	if err != nil {
		return nil, errors.Wrap(err, "state object fetch failed")
	}

	auxiliaryObjects[common.SargaAccountID] = auxObj.Copy()

	return auxiliaryObjects, nil
}

func (sm *StateManager) LoadTransitionObjects(
	ixps map[identifiers.Identifier]common.ParticipantInfo,
	psState common.ParticipantsState,
) (*Transition, error) {
	var (
		sysObj *SystemObject
		obj    *Object
		err    error
	)

	// Create a new objects map
	objects := make(ObjectMap)

	for id, p := range ixps {
		// we should only include the system object in the transition if it is part of the ics
		if id == common.SystemAccountID {
			sysObj = sm.GetSystemObject().Copy()

			continue
		}

		if p.IsGenesis {
			objects[id] = sm.CreateStateObject(id, p.AccType, true)

			continue
		}

		// if psState is not passed or is nil, we fetch the latest state object
		if psState == nil {
			obj, err = sm.GetLatestStateObject(id)
			if err != nil {
				return nil, errors.Wrap(err, "state object fetch failed")
			}

			// copy inorder to avoid modifications to cached object
			objects[id] = obj.Copy()

			continue
		}

		state, ok := psState[id]
		if !ok {
			return nil, errors.Wrap(err, "participant state not found")
		}

		ts, err := sm.getTesseractByHash(state.TransitiveLink, false, false)
		if err != nil {
			return nil, errors.Wrap(err, "tesseract fetch failed")
		}

		obj, err = sm.GetStateObjectByHash(id, ts.StateHash(id))
		if err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}

		objects[id] = obj.Copy()
	}

	auxiliaryObjects, err := sm.getAuxiliaryStateObjects()
	if err != nil {
		return nil, err
	}

	return NewTransition(sysObj, objects, auxiliaryObjects), nil
}

func (sm *StateManager) FetchIxStateObjects(
	ixns common.Interactions,
	hashes map[identifiers.Identifier]common.Hash,
) (
	*Transition, error,
) {
	var (
		sysObj *SystemObject
		err    error
	)

	ps := ixns.Participants()

	// Create a new objects map
	objects := make(ObjectMap)

	for id, p := range ps {
		// we should only include the system object in the transition if it is part of the ics
		if id == common.SystemAccountID {
			sysObj = sm.GetSystemObject()

			continue
		}

		if p.IsGenesis {
			objects[id] = sm.CreateStateObject(id, p.AccType, true)

			continue
		}

		if stateHash, ok := hashes[id]; !ok {
			if objects[id], err = sm.GetLatestStateObject(id); err != nil {
				return nil, errors.Wrap(err, "state object fetch failed")
			}
		} else if objects[id], err = sm.GetStateObjectByHash(id, stateHash); err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}
	}

	auxiliaryObjects, err := sm.getAuxiliaryStateObjects()
	if err != nil {
		return nil, err
	}

	return NewTransition(sysObj, objects, auxiliaryObjects), nil
}

func (sm *StateManager) IsSealValid(ts *common.Tesseract) (bool, error) {
	publicKey, err := sm.GetPublicKeys([]identifiers.KramaID{ts.SealBy()}...)
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
