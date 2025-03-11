package state

import (
	"context"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/hashicorp/go-hclog"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/manishmeganathan/depgraph"
	"github.com/munna0908/smt"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

var (
	emptyKeys   [][]byte
	emptyValues [][]byte
)

type MockServer struct{}

func mockServer() *MockServer {
	return new(MockServer)
}

func (m *MockServer) Subscribe(
	ctx context.Context,
	topic string,
	validator utils.WrappedVal,
	defaultValidator bool,
	handler func(msg *pubsub.Message) error,
) error {
	// TODO implement me
	panic("implement me")
}

type MockDB struct {
	tesseracts          map[common.Hash]*common.Tesseract
	latestTesseractHash map[identifiers.Identifier]common.Hash
	accounts            map[common.Hash][]byte
	context             map[common.Hash][]byte
	balances            map[common.Hash][]byte
	assetDeeds          map[common.Hash][]byte
	accountKeys         map[common.Hash][]byte
	merkleTreeEntries   map[string][]byte
	preImages           map[common.Hash][]byte
	data                map[string][]byte
	accMetaInfo         map[string]*common.AccountMetaInfo
	createEntryHook     func() error
}

func mockDB() *MockDB {
	return &MockDB{
		tesseracts:          make(map[common.Hash]*common.Tesseract),
		latestTesseractHash: make(map[identifiers.Identifier]common.Hash),
		accounts:            make(map[common.Hash][]byte),
		assetDeeds:          make(map[common.Hash][]byte),
		context:             make(map[common.Hash][]byte),
		balances:            make(map[common.Hash][]byte),
		accountKeys:         make(map[common.Hash][]byte),
		merkleTreeEntries:   make(map[string][]byte),
		preImages:           make(map[common.Hash][]byte),
		data:                make(map[string][]byte),
		accMetaInfo:         make(map[string]*common.AccountMetaInfo),
	}
}

func (m *MockDB) setAccountKeys(t *testing.T, hash common.Hash, accountKeys common.AccountKeys) {
	t.Helper()

	bytes, err := accountKeys.Bytes()
	require.NoError(t, err)

	m.accountKeys[hash] = bytes
}

func (m *MockDB) GetAccountKeys(id identifiers.Identifier, stateHash common.Hash) ([]byte, error) {
	accountKeys, ok := m.accountKeys[stateHash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return accountKeys, nil
}

func (m *MockDB) GetTesseract(hash common.Hash, withInteractions, withCommitInfo bool) (*common.Tesseract, error) {
	ts, ok := m.tesseracts[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	if !withCommitInfo {
		tsCopy = *tsCopy.GetTesseractWithoutCommitInfo()
	}

	return &tsCopy, nil
}

func (m *MockDB) ReadEntry(key []byte) ([]byte, error) {
	data, ok := m.data[common.BytesToHex(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) GetDeeds(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	data, ok := m.assetDeeds[hash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) setAssetDeeds(t *testing.T, deedsHash common.Hash, deeds *Deeds) {
	t.Helper()

	rawData, err := deeds.Bytes()
	require.NoError(t, err)

	m.assetDeeds[deedsHash] = rawData
}

func (m *MockDB) CreateEntry(key, value []byte) error {
	if m.createEntryHook != nil {
		return m.createEntryHook()
	}

	hexKey := common.BytesToHex(key)
	if _, ok := m.data[hexKey]; ok {
		return common.ErrKeyExists
	}

	m.data[hexKey] = value

	return nil
}

func (m *MockDB) Contains(key []byte) (bool, error) {
	if _, ok := m.data[common.BytesToHex(key)]; ok {
		return true, nil
	}

	return false, nil
}

func (m *MockDB) UpdateEntry(key, value []byte) error {
	hexKey := common.BytesToHex(key)

	if _, ok := m.data[hexKey]; !ok {
		return common.ErrKeyNotFound
	}

	m.data[hexKey] = value

	return nil
}

func (m *MockDB) NewBatchWriter() db.BatchWriter {
	return nil
}

func (m *MockDB) GetEntries(prefix []byte) chan common.DBEntry {
	return nil
}

func (m *MockDB) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[id.Hex()]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (m *MockDB) setAccountMetaInfo(acc *common.AccountMetaInfo) {
	m.accMetaInfo[acc.ID.Hex()] = acc
}

func (m *MockDB) GetMerkleTreeEntry(id identifiers.Identifier, prefix storage.PrefixTag, key []byte) ([]byte, error) {
	entry, ok := m.merkleTreeEntries[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return entry, nil
}

func (m *MockDB) SetMerkleTreeEntry(id identifiers.Identifier, prefix storage.PrefixTag, key, value []byte) error {
	m.merkleTreeEntries[string(key)] = value

	return nil
}

func (m *MockDB) SetMerkleTreeEntries(
	id identifiers.Identifier,
	prefix storage.PrefixTag,
	entries map[string][]byte,
) error {
	for k, v := range entries {
		m.merkleTreeEntries[k] = v
	}

	return nil
}

func (m *MockDB) WritePreImages(id identifiers.Identifier, entries map[common.Hash][]byte) error {
	for k, v := range entries {
		m.preImages[k] = v
	}

	return nil
}

func (m *MockDB) GetPreImage(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	preImage, ok := m.preImages[hash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return preImage, nil
}

func (m *MockDB) setAccount(t *testing.T, hash common.Hash, acc *common.Account) {
	t.Helper()

	bytes, err := acc.Bytes()
	require.NoError(t, err)

	m.accounts[hash] = bytes
}

func (m *MockDB) GetAccount(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	account, ok := m.accounts[hash]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (m *MockDB) setMetaContext(t *testing.T, object *MetaContextObject) {
	t.Helper()

	hash, err := object.Hash()
	require.NoError(t, err)

	bytes, err := object.Bytes()
	require.NoError(t, err)

	m.context[hash] = bytes
}

func (m *MockDB) GetContext(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	tsContext, ok := m.context[hash]
	if !ok {
		return nil, common.ErrContextStateNotFound
	}

	return tsContext, nil
}

func (m *MockDB) GetBalance(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	balance, ok := m.balances[hash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return balance, nil
}

func (m *MockDB) DeleteEntry(key []byte) error {
	panic("mock DeleteEntry not implemented")
}

func getMockDB(t *testing.T, db Store) *MockDB {
	t.Helper()

	if db != nil {
		mDB, ok := db.(*MockDB)
		require.True(t, ok)

		return mDB
	}

	return nil
}

func insertTesseractsInDB(t *testing.T, db Store, tesseracts ...*common.Tesseract) {
	t.Helper()
	mDB := getMockDB(t, db)

	for _, ts := range tesseracts {
		mDB.tesseracts[ts.Hash()] = ts
	}
}

func insertAccountsInDB(t *testing.T, db Store, hashes []common.Hash, acc ...*common.Account) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, hash := range hashes {
		mDB.setAccount(t, hash, acc[i])
	}
}

func insertAccKeysInDB(t *testing.T, db Store, hash common.Hash, accKeys common.AccountKeys) {
	t.Helper()

	mDB := getMockDB(t, db)

	mDB.setAccountKeys(t, hash, accKeys)
}

func insertSargaAccount(t *testing.T, db Store) {
	t.Helper()

	mDB := getMockDB(t, db)

	stateHash := tests.RandomHash(t)

	mDB.setAccountMetaInfo(&common.AccountMetaInfo{
		ID:        common.SargaAccountID,
		StateHash: stateHash,
	})

	mDB.setAccount(t, stateHash, new(common.Account))
}

func insertAssetDeedsInDB(t *testing.T, db Store, hashes []common.Hash, deeds ...*Deeds) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, bal := range deeds {
		mDB.setAssetDeeds(t, hashes[i], bal)
	}
}

func insertMetaContextsInDB(t *testing.T, db Store, context ...*MetaContextObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for _, ctx := range context {
		mDB.setMetaContext(t, ctx)
	}
}

type MockMerkleTree struct {
	dbStorage    map[string][]byte
	dirty        map[string][]byte
	merkleRoot   common.Hash
	isFlushed    bool
	isCommitted  bool
	flushHook    func() error
	rootHashHook func() (common.Hash, error)
	commitHook   func() error
	setHook      func() error
}

func (m *MockMerkleTree) Root() common.RootNode {
	return common.RootNode{
		MerkleRoot: m.merkleRoot,
		HashTable:  m.dirty,
	}
}

func mockMerkleTreeWithDB() *MockMerkleTree {
	return &MockMerkleTree{
		dirty:     make(map[string][]byte),
		dbStorage: make(map[string][]byte),
	}
}

func (m *MockMerkleTree) RootHash() (common.Hash, error) {
	if m.rootHashHook != nil {
		return m.rootHashHook()
	}

	return m.merkleRoot, nil
}

func (m *MockMerkleTree) Commit() error {
	if m.commitHook != nil {
		return m.commitHook()
	}

	bytes, err := polo.Polorize(m.dirty)
	if err != nil {
		return err
	}

	m.merkleRoot = common.GetHash(bytes)

	if !m.isCommitted {
		m.isCommitted = true
	} else {
		return errors.New("Already committed")
	}

	return nil
}

func (m *MockMerkleTree) IsCommitted() bool {
	return m.isCommitted
}

func (m *MockMerkleTree) NewIterator() smt.Iterator {
	// TODO implement me
	panic("implement me")
}

func (m *MockMerkleTree) GetPreImageKey(hashKey common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMerkleTree) IsDirty() bool {
	return len(m.dirty) > 0
}

func (m *MockMerkleTree) Copy() tree.MerkleTree {
	merkleTree := MockMerkleTree{
		dbStorage: make(map[string][]byte),
		dirty:     make(map[string][]byte),
	}

	for key, value := range m.dbStorage {
		v := make([]byte, len(value))

		copy(v, value)

		merkleTree.dbStorage[key] = v
	}

	for key, value := range m.dirty {
		v := make([]byte, len(value))

		copy(v, value)

		merkleTree.dirty[key] = v
	}

	merkleTree.merkleRoot = m.merkleRoot
	merkleTree.isFlushed = m.isFlushed

	return &merkleTree
}

func (m *MockMerkleTree) Set(key []byte, value []byte) error {
	if m.setHook != nil {
		return m.setHook()
	}

	if m.dirty == nil {
		m.dirty = make(map[string][]byte)
	}

	m.dirty[common.BytesToHex(key)] = value
	m.isCommitted = false

	return nil
}

func (m *MockMerkleTree) Delete(key []byte) error {
	if exists, _ := m.Has(key); exists {
		delete(m.dbStorage, common.BytesToHex(key))

		return nil
	}

	return common.ErrKeyNotFound
}

func (m *MockMerkleTree) Get(key []byte) ([]byte, error) {
	if exists, _ := m.Has(key); exists {
		val := m.dirty[common.BytesToHex(key)]

		return val, nil
	}

	return nil, common.ErrKeyNotFound
}

func (m *MockMerkleTree) Has(key []byte) (bool, error) {
	_, ok := m.dirty[common.BytesToHex(key)]

	return ok, nil
}

func (m *MockMerkleTree) Flush() error {
	if m.flushHook != nil {
		return m.flushHook()
	}

	m.isFlushed = true

	for k, v := range m.dirty {
		m.dbStorage[k] = v
	}

	m.dirty = nil

	return nil
}

func (m *MockMerkleTree) Close() error {
	return nil
}

func setEntries(t *testing.T, m *MockMerkleTree, keys, values [][]byte) {
	t.Helper()

	for i := 0; i < len(keys); i += 1 {
		err := m.Set(keys[i], values[i])
		require.NoError(t, err)
	}
}

func getMerkleTreeWithEntries(t *testing.T, keys [][]byte, values [][]byte) *MockMerkleTree {
	t.Helper()

	merkleTree := mockMerkleTreeWithDB()

	for i, key := range keys {
		err := merkleTree.Set(key, values[i])
		require.NoError(t, err)
	}

	return merkleTree
}

func getMerkleTreeWithCommitHook(
	t *testing.T,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) *MockMerkleTree {
	t.Helper()

	merkleTree := getMerkleTreeWithEntries(t, keys, values) // insert entries to make tree dirty
	merkleTree.commitHook = commitHook

	return merkleTree
}

func getMerkleTreeWithFlushHook(
	t *testing.T,
	keys [][]byte,
	values [][]byte,
	flushHook func() error,
) *MockMerkleTree {
	t.Helper()

	merkleTree := getMerkleTreeWithEntries(t, keys, values) // insert entries to make tree dirty
	merkleTree.flushHook = flushHook

	return merkleTree
}

func getMerkleTreeWithRootHashHook(
	t *testing.T,
	keys [][]byte,
	values [][]byte,
	rootHashHook func() (common.Hash, error),
) *MockMerkleTree {
	t.Helper()

	merkleTree := getMerkleTreeWithEntries(t, keys, values) // insert entries to make tree dirty
	merkleTree.rootHashHook = rootHashHook

	return merkleTree
}

func getMerkleTreeWithHook(t *testing.T, keys [][]byte, values [][]byte, setHook func() error) *MockMerkleTree {
	t.Helper()

	merkleTree := getMerkleTreeWithEntries(t, keys, values) // insert entries to make tree dirty
	merkleTree.setHook = setHook

	return merkleTree
}

func getRoot(t *testing.T, m *MockMerkleTree) common.Hash {
	t.Helper()

	bytes, err := polo.Polorize(m.dirty)
	require.NoError(t, err)

	return common.GetHash(bytes)
}

func createAssetObject(t *testing.T) *AssetObject {
	t.Helper()

	return &AssetObject{
		Balance: big.NewInt(45000),
		Lockup: map[identifiers.Identifier]*big.Int{
			tests.RandomIdentifier(t): big.NewInt(3000),
		},
		Mandate: map[identifiers.Identifier]*Mandate{
			tests.RandomIdentifier(t): {
				Amount:    big.NewInt(2000),
				ExpiresAt: uint64(time.Now().Add(1 * time.Hour).Unix()),
			},
		},
		Properties: &common.AssetDescriptor{
			Symbol: "MOI",
			Supply: big.NewInt(50000),
		},
	}
}

type createLogicObjectParams struct {
	id            identifiers.LogicID
	logicCallback func(object *LogicObject)
}

func createLogicObject(t *testing.T, params *createLogicObjectParams) *LogicObject {
	t.Helper()

	if params == nil {
		params = &createLogicObjectParams{}
	}

	logicObject := &LogicObject{ID: params.id, EngineKind: engineio.PISA}
	if params.logicCallback != nil {
		params.logicCallback(logicObject)
	}

	return logicObject
}

func getLogicObjectParamsWithLogicID(logicID identifiers.LogicID) *createLogicObjectParams {
	return &createLogicObjectParams{
		id: logicID,
		logicCallback: func(object *LogicObject) {
			object.Dependencies = depgraph.NewDependencyGraph()
		},
	}
}

func getTesseractParamsWithStateHash(id identifiers.Identifier, hash common.Hash) *tests.CreateTesseractParams {
	return &tests.CreateTesseractParams{
		IDs: []identifiers.Identifier{id},
		Participants: common.ParticipantsState{
			id: {
				StateHash: hash,
			},
		},
	}
}

func storeTesseractHashInCache(t *testing.T, cache *lru.Cache, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		for id := range ts.Participants() {
			cache.Add(id, ts.Hash())
		}
	}
}

func mockCache(t *testing.T) *lru.Cache {
	t.Helper()

	cache, err := lru.New(1200)
	require.NoError(t, err)

	return cache
}

type createStateManagerParams struct {
	db             *MockDB
	dbCallback     func(db *MockDB)
	serverCallback func(n *MockServer)
	smCallBack     func(sm *StateManager)
}

func createTestStateManager(t *testing.T, params *createStateManagerParams) *StateManager {
	t.Helper()

	var (
		mDB    = mockDB()
		server = mockServer()
	)

	if params == nil {
		params = &createStateManagerParams{}
	}

	if params.db != nil {
		mDB = params.db
	}

	if params.dbCallback != nil {
		params.dbCallback(mDB)
	}

	if params.serverCallback != nil {
		params.serverCallback(server)
	}

	sm, err := NewStateManager(
		mDB,
		hclog.NewNullLogger(),
		mockCache(t),
		NilMetrics(),
		&config.StateConfig{
			TreeCacheSize: 22,
		},
		true,
	)
	require.NoError(t, err)

	if params.smCallBack != nil {
		params.smCallBack(sm)
	}

	return sm
}

func insertContextHash(so *Object, hash common.Hash) {
	so.data.ContextHash = hash
}

func storeInSmCache(sm *StateManager, k, v interface{}) {
	sm.cache.Add(k, v)
}

func setGuardianPublicKeys(t *testing.T, state *Object, ids []identifiers.KramaID, publicKeys [][]byte) {
	t.Helper()

	for i, id := range ids {
		// Encode and hash the krama ID
		encoded, _ := polo.Polorize(id)
		hashed := blake2b.Sum256(encoded)

		// Generate a storage access key for Registry.Guardians[kramaID].PubKey
		key := pisa.GenerateStorageKey(guardianregistry.SlotGuardians, pisa.MapKey(hashed), pisa.ClsFld(3))

		pk, err := polo.Polorize(publicKeys[i])
		require.NoError(t, err)

		// Retrieve the value for the storage key
		err = state.SetStorageEntry(common.GuardianLogicID, key, pk)
		require.NoError(t, err)
	}
}

func createGuardianLogic(t *testing.T, sm *StateManager, kramaIDs []identifiers.KramaID, publicKeys [][]byte) {
	t.Helper()

	so := sm.CreateStateObject(common.GuardianAccountID, common.RegularAccount, true)
	so.storageTreeTxns[common.GuardianLogicID] = iradix.New().Txn()
	sm.objectCache.Add(common.GuardianAccountID, so)
	setGuardianPublicKeys(t, so, kramaIDs, publicKeys)
}

type createStateObjectParams struct {
	id                        identifiers.Identifier
	cache                     *lru.Cache
	db                        *MockDB
	account                   *common.Account
	soCallback                func(so *Object)
	metaStorageTreeCallback   func(so *Object)
	dbCallback                func(db *MockDB)
	activeStorageTreeCallback func(activeStorageTrees map[identifiers.LogicID]tree.MerkleTree)
}

func createTestStateObject(t *testing.T, params *createStateObjectParams) *Object {
	t.Helper()

	var (
		id    = tests.RandomIdentifierWithZeroVariant(t)
		mDB   = mockDB()
		cache = mockCache(t)
		data  = new(common.Account)
	)

	if params == nil {
		params = &createStateObjectParams{}
	}

	if !params.id.IsNil() {
		id = params.id
	}

	if params.db != nil {
		mDB = params.db
	}

	if params.dbCallback != nil {
		params.dbCallback(mDB)
	}

	if params.cache != nil {
		cache = params.cache
	}

	if params.account != nil {
		data = params.account
	}

	so := NewStateObject(id, cache, tests.NewTestTreeCache(), mDB, *data, NilMetrics(), false)
	so.metaStorageTree = mockMerkleTreeWithDB()
	so.logicTree = mockMerkleTreeWithDB()

	if params.soCallback != nil {
		params.soCallback(so)
	}

	if params.activeStorageTreeCallback != nil {
		params.activeStorageTreeCallback(so.storageTrees)
	}

	if params.metaStorageTreeCallback != nil {
		params.metaStorageTreeCallback(so)
	}

	return so
}

func createTestStateObjects(t *testing.T, count int, paramsMap map[int]*createStateObjectParams) []*Object {
	t.Helper()

	objects := make([]*Object, count)

	if paramsMap == nil {
		paramsMap = map[int]*createStateObjectParams{}
	}

	for i := 0; i < count; i++ {
		objects[i] = createTestStateObject(t, paramsMap[i])
	}

	return objects
}

func stateObjectParamsWithDeeds(
	t *testing.T,
	hash common.Hash,
	deeds *Deeds,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.data.AssetDeeds = hash
			so.deeds = deeds
		},
	}
}

func stateObjectParamsWithAssetTree(
	t *testing.T,
	id identifiers.Identifier,
	db Store,
	assetTree tree.MerkleTree,
	root common.Hash,
	txn *iradix.Txn,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		id: id,
		db: mDB,
		soCallback: func(so *Object) {
			so.assetTree = assetTree
			so.assetTreeTxn = txn
			so.data.AssetRoot = root // set asset root as it needs to be returned
		},
	}
}

func soParamsWithStorageTreesAndTxns(
	t *testing.T,
	ast map[identifiers.LogicID]tree.MerkleTree,
	txns map[identifiers.LogicID]*iradix.Txn,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.storageTrees = ast
			so.storageTreeTxns = txns
		},
	}
}

func stateObjectParamsWithASTAndMST(
	t *testing.T,
	ast map[identifiers.LogicID]tree.MerkleTree,
	mst tree.MerkleTree,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.storageTrees = ast
			so.metaStorageTree = mst
		},
	}
}

func stateObjectParamsWithStorageTree(
	t *testing.T,
	ast map[identifiers.LogicID]tree.MerkleTree,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.storageTrees = ast
		},
	}
}

func stateObjectParamsWithInvalidMST(t *testing.T) *createStateObjectParams {
	t.Helper()

	return stateObjectParamsWithMST(t, identifiers.Nil, nil, nil, tests.RandomHash(t))
}

func stateObjectParamsWithMST(
	t *testing.T,
	id identifiers.Identifier,
	db Store,
	mst tree.MerkleTree,
	root common.Hash,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		id: id,
		db: mDB,
		soCallback: func(so *Object) {
			so.metaStorageTree = mst
			so.data.StorageRoot = root
		},
	}
}

func stateObjectParamsWithLogicTree(
	t *testing.T,
	id identifiers.Identifier,
	db Store,
	logicTree tree.MerkleTree,
	root common.Hash,
	txn *iradix.Txn,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		id: id,
		db: mDB,
		soCallback: func(so *Object) {
			so.logicTree = logicTree
			so.data.LogicRoot = root // set logic root as it needs to be returned
			so.logicTreeTxn = txn
		},
	}
}

// stateObjectParamsWithMetaContextObject stores object in db if inDB is true else stores in cache
func stateObjectParamsWithMetaContextObject(
	t *testing.T,
	obj *MetaContextObject,
	hash common.Hash,
	addToDB bool,
) *createStateObjectParams {
	t.Helper()

	if addToDB {
		return &createStateObjectParams{
			soCallback: func(so *Object) {
				insertContextHash(so, hash)
				mDB := getMockDB(t, so.db)

				rawData, err := obj.Bytes()
				require.NoError(t, err)

				mDB.context[hash] = rawData
			},
		}
	}

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.data.ContextHash = hash
			so.cache.Add(so.ContextHash(), obj)
		},
	}
}

func stateObjectParamsWithTestData(t *testing.T, areTreesNil bool) *createStateObjectParams {
	t.Helper()

	keys, values := getEntries(t, 3)

	return &createStateObjectParams{
		soCallback: func(s *Object) {
			s.accType = common.LogicAccount
			acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
				acc.ContextHash = tests.RandomHash(t)
			})

			s.isGenesis = true
			s.data = *acc

			if !areTreesNil {
				s.assetTree = getMerkleTreeWithEntries(t, keys[0:1], values[0:1])
				s.logicTree = getMerkleTreeWithEntries(t, keys[1:2], values[1:2])
				s.metaStorageTree = getMerkleTreeWithEntries(t, keys[2:], values[2:])
			}

			s.files = make(map[common.Hash][]byte)
			s.files[tests.RandomHash(t)] = tests.RandomHash(t).Bytes()
			s.files[tests.RandomHash(t)] = tests.RandomHash(t).Bytes()
			s.deeds, _ = getTestDeeds(
				t,
				map[identifiers.Identifier]struct{}{
					tests.RandomIdentifier(t): {},
				})

			logicIDs := tests.GetLogicIDs(t, 1)
			s.logicTreeTxn = getTxnWithLogicObjects(
				t,
				createLogicObject(t, getLogicObjectParamsWithLogicID(logicIDs[0])))
			s.storageTreeTxns = getStorageTxnsWithEntries(t, logicIDs, keys, values)
		},
	}
}

func checkForReferences(t *testing.T, sObj, copiedSO *Object) {
	t.Helper()

	require.NotEqual(t,
		reflect.ValueOf(sObj.files).Pointer(),
		reflect.ValueOf(copiedSO.files).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(sObj.dirtyEntries).Pointer(),
		reflect.ValueOf(copiedSO.dirtyEntries).Pointer(),
	)
}

func getAssetIDsAndBalances(t *testing.T, count int) ([]identifiers.AssetID, []*big.Int) {
	t.Helper()

	ids := make([]identifiers.AssetID, count)
	for i := 0; i < count; i++ {
		ids[i] = tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	}

	return ids, tests.GetRandomNumbers(t, 10000, count)
}

func getTestDeeds(t *testing.T, entries map[identifiers.Identifier]struct{}) (*Deeds, common.Hash) {
	t.Helper()

	deeds := &Deeds{
		Entries: entries,
	}

	data, err := deeds.Bytes()
	require.NoError(t, err)

	return deeds, common.GetHash(data)
}

func setAssetBalance(t *testing.T, so *Object, assetID identifiers.AssetID, amount *big.Int) {
	t.Helper()

	setAssetObject(t, so, assetID, &AssetObject{
		Balance: amount,
	})
}

func setAssetLockups(
	t *testing.T, so *Object, assetIDs []identifiers.AssetID,
	amounts []*big.Int, ids []identifiers.Identifier, lockupAmounts []*big.Int,
) {
	t.Helper()

	lockups := make(map[identifiers.Identifier]*big.Int)

	if len(ids) > 0 {
		for idx, id := range ids {
			lockups[id] = lockupAmounts[idx]
		}
	}

	for idx, assetID := range assetIDs {
		setAssetObject(t, so, assetID, &AssetObject{
			Balance: amounts[idx],
			Lockup:  lockups,
		})
	}
}

func setAssetMandates(
	t *testing.T, so *Object, assetIDs []identifiers.AssetID,
	amounts []*big.Int, ids []identifiers.Identifier, mandateAmounts []*big.Int,
) {
	t.Helper()

	mandates := make(map[identifiers.Identifier]*Mandate)

	if len(ids) > 0 {
		for idx, id := range ids {
			mandates[id] = &Mandate{
				Amount:    mandateAmounts[idx],
				ExpiresAt: uint64(time.Now().Add(1 * time.Hour).Unix()),
			}
		}
	}

	for idx, assetID := range assetIDs {
		setAssetObject(t, so, assetID, &AssetObject{
			Balance: amounts[idx],
			Mandate: mandates,
		})
	}
}

func setAssetState(t *testing.T, so *Object, assetID identifiers.AssetID, state *common.AssetDescriptor) {
	t.Helper()

	setAssetObject(t, so, assetID, &AssetObject{
		Balance:    state.Supply,
		Properties: state,
	})
}

func setAssetObject(t *testing.T, so *Object, assetID identifiers.AssetID, assetObj *AssetObject) {
	t.Helper()

	assert.NoError(t, so.InsertNewAssetObject(assetID, assetObj))

	_, err := so.commitAssets()
	require.NoError(t, err)
}

func setLogicObject(t *testing.T, so *Object, logicID identifiers.LogicID, logicObj *LogicObject) {
	t.Helper()

	assert.NoError(t, so.InsertNewLogicObject(logicID, logicObj))

	_, err := so.commitLogics()
	require.NoError(t, err)
}

func createTestAssets(
	t *testing.T,
	id identifiers.Identifier,
	db *MockDB,
	assetIDs []identifiers.AssetID,
	balances []*big.Int,
) (common.AssetMap, common.Hash) {
	t.Helper()

	assets := make(common.AssetMap)

	so := NewStateObject(id, mockCache(t), nil, db, common.Account{}, NilMetrics(), false)

	for i := 0; i < len(assetIDs); i++ {
		assert.NoError(t, so.InsertNewAssetObject(assetIDs[i], &AssetObject{
			Balance: balances[i],
		}))

		assets[assetIDs[i]] = balances[i]
	}

	rootHash, err := so.commitAssets()
	require.NoError(t, err)

	err = so.flushAssetTree()
	require.NoError(t, err)

	return assets, rootHash
}

func createTestAssetInAssetAccount(
	t *testing.T, so *Object,
	assetInfo *common.AssetDescriptor,
) (identifiers.AssetID, common.Hash) {
	t.Helper()

	assetID, err := so.CreateAsset(so.Identifier(), assetInfo)
	require.NoError(t, err)

	assetRoot, err := so.commitAssets()
	require.NoError(t, err)

	err = so.flushAssetTree()
	require.NoError(t, err)

	return assetID, assetRoot
}

func createTestAssetInRegularAccount(
	t *testing.T, so *Object,
	assetID identifiers.AssetID, assetInfo *common.AssetDescriptor,
) common.Hash {
	t.Helper()

	err := so.InsertNewAssetObject(assetID, NewAssetObject(assetInfo.Supply, nil))
	require.NoError(t, err)

	assetRoot, err := so.commitAssets()
	require.NoError(t, err)

	err = so.flushAssetTree()
	require.NoError(t, err)

	return assetRoot
}

func getTestAccountKeys(t *testing.T, count int) (common.AccountKeys, common.Hash) {
	t.Helper()

	keys := make(common.AccountKeys, count)

	for i := 0; i < count; i++ {
		keys[i] = &common.AccountKey{
			ID:                 uint64(i),
			PublicKey:          tests.RandomIdentifier(t).Bytes(),
			Weight:             2000,
			SignatureAlgorithm: 0,
			Revoked:            false,
			SequenceID:         9,
		}
	}

	hash, err := keys.Hash()
	require.NoError(t, err)

	return keys, hash
}

func getTestAccounts(t *testing.T, balanceHash []common.Hash, count int) ([]*common.Account, []common.Hash) {
	t.Helper()

	accounts := make([]*common.Account, count)
	hashes := make([]common.Hash, count)

	for i := 0; i < count; i++ {
		acc, stateHash := tests.GetTestAccount(t, func(acc *common.Account) {
			acc.AssetRoot = balanceHash[i]
		})

		accounts[i] = acc
		hashes[i] = stateHash
	}

	return accounts, hashes
}

func getMetaContextObject(t *testing.T) (*MetaContextObject, common.Hash) {
	t.Helper()

	mCtx := &MetaContextObject{
		ConsensusNodesHash: tests.RandomHash(t),
		ConsensusNodes:     tests.RandomKramaIDs(t, 2),
		ComputeContext:     tests.RandomHash(t),
		DefaultMTQ:         44,
	}

	hash, err := mCtx.Hash()
	require.NoError(t, err)

	return mCtx, hash
}

func getMetaContextObjects(t *testing.T, count int) ([]*MetaContextObject, []common.Hash) {
	t.Helper()

	mObj := make([]*MetaContextObject, count)
	mHashes := make([]common.Hash, count)

	for i := 0; i < count; i += 1 {
		mObj[i], mHashes[i] = getMetaContextObject(t)
	}

	return mObj, mHashes
}

func getRawMetaObjects(t *testing.T, mObj []*MetaContextObject) [][]byte {
	t.Helper()

	rawMetaObjects := make([][]byte, len(mObj))

	for i := 0; i < len(mObj); i++ {
		rawMetaObject, err := polo.Polorize(mObj[i])
		assert.NoError(t, err)

		rawMetaObjects[i] = rawMetaObject
	}

	return rawMetaObjects
}

func getStateHashes(t *testing.T, so []*Object) []common.Hash {
	t.Helper()

	stateHashes := make([]common.Hash, 0)

	for i := 0; i < len(so); i++ {
		stateHash, err := so[i].Commit()
		assert.NoError(t, err)

		stateHashes = append(stateHashes, stateHash)
	}

	return stateHashes
}

func getTestAssetDescriptor(t *testing.T, operator identifiers.Identifier, symbol string) *common.AssetDescriptor {
	t.Helper()

	return &common.AssetDescriptor{
		Symbol:     symbol,
		Operator:   operator,
		Supply:     big.NewInt(10000),
		Dimension:  4,
		IsStateFul: false,
		IsLogical:  false,
		LogicID:    identifiers.RandomLogicIDv0().AsIdentifier(),
	}
}

func createMetaStorageTree(
	t *testing.T,
	db Store,
	id identifiers.Identifier,
	logicID identifiers.LogicID,
	storageKeys [][]byte,
	storageValues [][]byte,
) (*tree.KramaHashTree, common.Hash) {
	t.Helper()

	_, storageRoot := createTestKramaHashTree(t, db, id, storage.Storage, storageKeys, storageValues)

	return createTestKramaHashTree(
		t,
		db,
		id,
		storage.Storage,
		[][]byte{logicID.Bytes()},
		[][]byte{storageRoot.Bytes()})
}

func createTestKramaHashTree(
	t *testing.T,
	db Store,
	id identifiers.Identifier,
	prefix storage.PrefixTag,
	keys [][]byte,
	values [][]byte,
) (*tree.KramaHashTree, common.Hash) {
	t.Helper()

	kt, err := tree.NewKramaHashTree(id, common.NilHash, db, blake256.New(),
		prefix, tests.NewTestTreeCache(), tree.NilMetrics())
	require.NoError(t, err)

	for i := 0; i < len(keys); i++ {
		err = kt.Set(keys[i], values[i])
		require.NoError(t, err)
	}

	err = kt.Commit()
	require.NoError(t, err)

	err = kt.Flush()
	require.NoError(t, err)

	storageRoot, err := kt.RootHash()
	require.NoError(t, err)

	return kt, storageRoot
}

func copyStorageTrees(ast map[identifiers.LogicID]tree.MerkleTree) map[identifiers.LogicID]tree.MerkleTree {
	copiedAST := make(map[identifiers.LogicID]tree.MerkleTree, len(ast))

	for logic, merkleTree := range ast {
		copiedAST[logic] = merkleTree.Copy()
	}

	return copiedAST
}

// getStorageTreesWithDefaultEntries returns ast with dirty entries
func getStorageTreesWithDefaultEntries(
	t *testing.T,
	treeCount int,
	entriesPerTree int,
) map[identifiers.LogicID]tree.MerkleTree {
	t.Helper()

	logicIds := tests.GetLogicIDs(t, treeCount)
	keys, values := getEntries(t, entriesPerTree)

	return getStorageTrees(t, logicIds, keys, values)
}

// getMerkleTreeWithDefaultEntries return merkle tree with dirty entries
func getMerkleTreeWithDefaultEntries(t *testing.T, entriesPerTree int) tree.MerkleTree {
	t.Helper()

	keys, values := getEntries(t, entriesPerTree)

	return getMerkleTreeWithEntries(t, keys, values)
}

func getStorageTrees(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
) map[identifiers.LogicID]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	storageTrees := make(map[identifiers.LogicID]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		storageTrees[logicIds[i]] = getMerkleTreeWithEntries(t, keys, values)
	}

	return storageTrees
}

func getStorageTreesWithCommitHook(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) map[identifiers.LogicID]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[identifiers.LogicID]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i]] = getMerkleTreeWithCommitHook(t, keys, values, commitHook)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithFlushHook(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) map[identifiers.LogicID]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[identifiers.LogicID]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i]] = getMerkleTreeWithFlushHook(t, keys, values, commitHook)
	}

	return activeStorageTrees
}

func checkForTesseractInSMCache(
	t *testing.T,
	sm *StateManager,
	ts *common.Tesseract,
	withInteractions,
	withCommitInfo bool,
) {
	t.Helper()

	object, isCached := sm.cache.Get(ts.Hash())
	if withInteractions || withCommitInfo {
		require.False(t, isCached) // make sure tesseract not cached

		return
	}

	require.True(t, isCached) // make sure tesseract cached, if fetched without interactions

	cachedTS, ok := object.(*common.Tesseract)
	require.True(t, ok)

	require.Equal(t, ts, cachedTS) // make sure cached tesseract matches
}

func checkIfTreesAreEqual(t *testing.T, oldTree, newTree tree.MerkleTree) {
	t.Helper()

	err := oldTree.Commit()
	require.NoError(t, err)

	err = newTree.Commit()
	require.NoError(t, err)

	oldRoot := getRoot(t, oldTree.(*MockMerkleTree)) //nolint
	newRoot := getRoot(t, newTree.(*MockMerkleTree)) //nolint

	require.Equal(t, oldRoot, newRoot)
}

func checkIfTxnsAreEqual(t *testing.T, oldTxn, newTxn *iradix.Txn) {
	t.Helper()

	newTxn.Root().Walk(func(k []byte, v interface{}) bool {
		_, ok := oldTxn.Get(k)
		require.True(t, ok)

		return true
	})

	require.Equal(t, oldTxn.CommitOnly().Len(), newTxn.CommitOnly().Len())
}

func validateStateObject(t *testing.T, so *Object, accType common.AccountType, id identifiers.Identifier,
	isGenesis bool,
) {
	t.Helper()

	require.Equal(t, id, so.id)
	require.Equal(t, accType, so.accType)
	require.Equal(t, isGenesis, so.isGenesis)
	require.NotNil(t, so.db)
	require.NotNil(t, so.data)
	require.NotNil(t, so.cache)
}

func checkForStateObject(t *testing.T, expectedObj *Object, obj *Object) {
	t.Helper()

	require.Equal(t, expectedObj.data, obj.data)
	require.Equal(t, expectedObj.id, obj.id)
	require.Equal(t, expectedObj.accType, obj.accType)
}

func checkIfStateObjectAreEqual(
	t *testing.T,
	oldObj *Object,
	newObj *Object,
) {
	t.Helper()

	require.NotNil(t, newObj.db)
	require.NotNil(t, newObj.cache)

	require.Equal(t, oldObj.dirtyEntries, newObj.dirtyEntries)
	require.Equal(t, oldObj.data, newObj.data)
	require.Equal(t, oldObj.files, newObj.files)
	require.Equal(t, oldObj.isGenesis, newObj.isGenesis)

	if oldObj.assetTree != nil {
		checkIfTreesAreEqual(t, oldObj.assetTree, newObj.assetTree)
	} else {
		require.Nil(t, newObj.assetTree)
	}

	if oldObj.logicTree != nil {
		checkIfTreesAreEqual(t, oldObj.logicTree, newObj.logicTree)
	} else {
		require.Nil(t, newObj.logicTree)
	}

	if oldObj.metaStorageTree != nil {
		checkIfTreesAreEqual(t, oldObj.metaStorageTree, newObj.metaStorageTree)
	} else {
		require.Nil(t, newObj.metaStorageTree)
	}

	for id, sTree := range oldObj.storageTrees {
		checkIfTreesAreEqual(t, sTree, newObj.storageTrees[id])
	}

	if oldObj.assetTreeTxn != nil {
		checkIfTxnsAreEqual(t, oldObj.assetTreeTxn, newObj.assetTreeTxn)
	}

	if oldObj.logicTreeTxn != nil {
		checkIfTxnsAreEqual(t, oldObj.logicTreeTxn, newObj.logicTreeTxn)
	}

	for id, txn := range oldObj.storageTreeTxns {
		checkIfTxnsAreEqual(t, txn, newObj.storageTreeTxns[id])
	}
}

func checkForBalances(t *testing.T, sObj *Object, expectedBalance *big.Int, assetID identifiers.AssetID) {
	t.Helper()

	assetObject, err := sObj.getAssetObject(assetID, true)
	require.NoError(t, err)
	require.Equal(t, expectedBalance, assetObject.Balance)
}

func checkForMandates(
	t *testing.T, sObj *Object, assetID identifiers.AssetID,
	id identifiers.Identifier, expectedAmount *big.Int,
) {
	t.Helper()

	mandate, err := sObj.GetMandate(assetID, id)

	if expectedAmount.Cmp(big.NewInt(0)) == 0 {
		require.Error(t, err)
		require.Equal(t, common.ErrMandateNotFound, err)

		return
	}

	require.NoError(t, err)
	require.Equal(t, 0, mandate.Amount.Cmp(expectedAmount))
}

func checkForLockups(
	t *testing.T, sObj *Object, assetID identifiers.AssetID,
	id identifiers.Identifier, expectedAmount *big.Int,
) {
	t.Helper()

	lockupAmount, err := sObj.GetLockup(assetID, id)

	if expectedAmount.Cmp(big.NewInt(0)) == 0 {
		require.Error(t, err)
		require.Equal(t, common.ErrLockupNotFound, err)

		return
	}

	require.NoError(t, err)
	require.Equal(t, 0, lockupAmount.Cmp(expectedAmount))
}

func checkForDeeds(
	t *testing.T,
	sObj *Object,
	expectedDeeds *Deeds,
	actualDeedsHash common.Hash,
) {
	t.Helper()

	expectedDeedsData, err := expectedDeeds.Bytes()
	require.NoError(t, err)

	expectedDeedsHash := common.GetHash(expectedDeedsData)
	require.Equal(t, expectedDeedsHash, actualDeedsHash)

	// check if deeds data in dirty entries and state object is same
	key := common.BytesToHex(storage.DeedsKey(sObj.id, expectedDeedsHash))
	actualDeedsData, err := sObj.GetDirtyEntry(key) // get deeds data from dirty entries
	require.NoError(t, err)

	require.Equal(t, expectedDeedsData, actualDeedsData)

	require.Equal(t, sObj.data.AssetDeeds, actualDeedsHash) // check if deeds hash inserted in account
}

func checkForAccount(
	t *testing.T,
	sObj *Object,
	expectedAcc *common.Account,
	generatedAccHash common.Hash,
	journalIndex int,
) {
	t.Helper()

	expectedAccData, err := expectedAcc.Bytes()
	require.NoError(t, err)

	expectedAccHash := common.GetHash(expectedAccData)
	require.Equal(t, expectedAccHash, generatedAccHash)

	// check if account data in dirty entries and state object is same
	key := common.BytesToHex(storage.AccountKey(sObj.id, expectedAccHash))
	actualAccData, err := sObj.GetDirtyEntry(key) // get account data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedAccData, actualAccData)
}

func checkForContextObject(
	t *testing.T,
	sObj *Object,
	ctxObject *MetaContextObject,
) {
	t.Helper()

	expectedCtxHash, err := ctxObject.Hash()
	require.NoError(t, err)

	objectContextHash, err := sObj.metaContext.Hash()
	require.NoError(t, err)

	require.Equal(t, expectedCtxHash, objectContextHash)

	rawBytes, err := ctxObject.Bytes()
	require.NoError(t, err)

	// check if ctxObject data in dirty entries matches
	key := common.BytesToHex(storage.ContextObjectKey(sObj.id, expectedCtxHash))
	actualObjData, err := sObj.GetDirtyEntry(key) // get ctx object data from dirty entries
	require.NoError(t, err)
	require.Equal(t, rawBytes, actualObjData)
}

func checkIfStorageTreesAreCommitted(
	t *testing.T,
	sObj *Object,
	oldStorageTrees map[identifiers.LogicID]tree.MerkleTree,
) {
	t.Helper()

	for logicID, txn := range sObj.storageTreeTxns {
		txn.Root().Walk(func(k []byte, v interface{}) bool {
			sTree, err := sObj.GetStorageTree(logicID)
			require.NoError(t, err)

			_, err = sTree.Get(k)
			require.NoError(t, err)

			return false
		})

		for id, oldTree := range oldStorageTrees {
			sTree, err := sObj.GetStorageTree(id)
			require.NoError(t, err)

			// make sure metaStorageTree has logicID,storageRoot pair
			actualRoot, err := sObj.metaStorageTree.Get(id.Bytes())
			require.NoError(t, err)

			require.True(t, sTree.(*MockMerkleTree).isCommitted) //nolint

			rootHash := getRoot(t, sTree.(*MockMerkleTree)) //nolint

			require.Equal(t, rootHash.Bytes(), actualRoot)

			oldRoot := getRoot(t, oldTree.(*MockMerkleTree)) //nolint

			require.NotEqual(t, oldRoot, rootHash)
		}
	}
}

func checkIfMetaStorageTreeCommitted(
	t *testing.T,
	generatedStorageRoot common.Hash,
	expectedStorageRoot common.Hash,
	sObj *Object,
) {
	t.Helper()

	rootHash, err := sObj.metaStorageTree.RootHash()
	require.NoError(t, err)

	// check if expected root hash matches
	require.Equal(t, expectedStorageRoot, rootHash)
	// check if root hash matches with the generated hash
	require.Equal(t, rootHash, generatedStorageRoot)
	// check if the tree is committed
	require.True(t, sObj.metaStorageTree.(*MockMerkleTree).IsCommitted()) //nolint

	// check if the data field is updated
	require.Equal(t, sObj.data.StorageRoot, generatedStorageRoot)
}

//nolint:dupl
func checkIfAssetTreeCommitted(
	t *testing.T,
	inputAssetTree tree.MerkleTree,
	sObj *Object,
	actualRoot common.Hash,
) {
	t.Helper()

	actualMerkleTree, ok := sObj.assetTree.(*MockMerkleTree)
	require.True(t, ok)

	sObj.assetTreeTxn.Root().Walk(func(k []byte, v interface{}) bool {
		ok, err := actualMerkleTree.Has(k)
		assert.NoError(t, err)
		assert.True(t, ok)

		return true
	})

	inputMerkleTree, ok := inputAssetTree.(*MockMerkleTree)
	require.True(t, ok)

	expectedRoot := getRoot(t, actualMerkleTree)

	require.Equal(t, expectedRoot, actualMerkleTree.merkleRoot)
	require.Equal(t, expectedRoot, actualRoot)
	require.Equal(t, sObj.data.AssetRoot, actualRoot) // make sure storage root returned and stored are same

	require.NotEqual(t, inputMerkleTree.merkleRoot, expectedRoot)
}

//nolint:dupl
func checkIfLogicTreeCommitted(
	t *testing.T,
	inputLogicTree tree.MerkleTree,
	sObj *Object,
	actualRoot common.Hash,
) {
	t.Helper()

	actualMerkleTree, ok := sObj.logicTree.(*MockMerkleTree)
	require.True(t, ok)

	sObj.logicTreeTxn.Root().Walk(func(k []byte, v interface{}) bool {
		ok, err := actualMerkleTree.Has(k)
		assert.NoError(t, err)
		assert.True(t, ok)

		return true
	})

	inputMerkleTree, ok := inputLogicTree.(*MockMerkleTree)
	require.True(t, ok)

	expectedRoot := getRoot(t, actualMerkleTree)

	require.Equal(t, expectedRoot, actualMerkleTree.merkleRoot)
	require.Equal(t, expectedRoot, actualRoot)
	require.Equal(t, sObj.data.LogicRoot, actualRoot) // make sure storage root returned and stored are same

	require.NotEqual(t, inputMerkleTree.merkleRoot, expectedRoot)
}

func checkIfMerkleTreeFlushed(t *testing.T, merkle tree.MerkleTree, isFlushed bool) {
	t.Helper()

	m, ok := merkle.(*MockMerkleTree)
	require.True(t, ok)

	require.Equal(t, isFlushed, m.isFlushed)
}

func checkIfActiveStorageTreesFlushed(t *testing.T, logicIDs []identifiers.LogicID, s *Object, isFlushed bool) {
	t.Helper()

	for _, logicID := range logicIDs {
		storageTree, ok := s.storageTrees[logicID]
		require.True(t, ok)

		checkIfMerkleTreeFlushed(t, storageTree, isFlushed)
	}
}

func checkForConsensusNodesUpdate(
	t *testing.T,
	sObj *Object,
	oldMetaObject *MetaContextObject,
	consensusNodes []identifiers.KramaID,
) {
	t.Helper()

	consensusNodesHash, err := common.PoloHash(consensusNodes)
	require.NoError(t, err)

	require.Equal(t, sObj.metaContext.ConsensusNodes, consensusNodes)

	require.Equal(t, sObj.metaContext.ConsensusNodesHash, consensusNodesHash)

	// we should also ensure that only consensusNodes related fields are updated
	if oldMetaObject != nil {
		require.Equal(t, sObj.metaContext.ComputeContext, oldMetaObject.ComputeContext)
		require.Equal(t, sObj.metaContext.StorageContext, oldMetaObject.StorageContext)
		require.Equal(t, sObj.metaContext.DefaultMTQ, oldMetaObject.DefaultMTQ)
	}
}

func checkForEntryInMST(t *testing.T, s *Object, key []byte, value []byte) {
	t.Helper()

	actualValue, err := s.metaStorageTree.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, actualValue)
}

func checkForKramaHashTree(
	t *testing.T,
	expectedTree tree.MerkleTree,
	actualTree tree.MerkleTree,
) {
	t.Helper()

	expectedRoot, err := expectedTree.RootHash()
	require.NoError(t, err)

	actualRoot, err := actualTree.RootHash()
	require.NoError(t, err)

	require.Equal(t, expectedRoot, actualRoot)
}

func checkForEntryInMerkleTree(t *testing.T, merkleTree tree.MerkleTree, key []byte, value []byte) {
	t.Helper()

	actualValue, err := merkleTree.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, actualValue)
}

func checkForEntriesInMerkleTree(t *testing.T, merkleTree tree.MerkleTree, keys [][]byte, values [][]byte) {
	t.Helper()

	for i, k := range keys {
		checkForEntryInMerkleTree(t, merkleTree, k, values[i])
	}
}

func checkForEntryInTxn(t *testing.T, txn *iradix.Txn, key []byte, value []byte) {
	t.Helper()

	actualValue, ok := txn.Get(key)
	require.True(t, ok)
	require.Equal(t, value, actualValue)
}

func createAndSignTesseracts(t *testing.T, count int) ([]*common.Tesseract, [][]byte, [][]byte) {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)
	signedTS := make([][]byte, count)
	pks := make([][]byte, count)

	for i := 0; i < count; i++ {
		tesseract := tests.CreateTesseract(t, &tests.CreateTesseractParams{
			IDs:     []identifiers.Identifier{tests.RandomIdentifier(t), tests.RandomIdentifier(t)},
			Heights: []uint64{1, 4},
		})

		rawData, err := tesseract.SignBytes()
		require.NoError(t, err)

		signedData, pk := tests.SignBytes(t, rawData)

		tesseracts[i] = tesseract
		signedTS[i] = signedData
		pks[i] = pk
	}

	return tesseracts, signedTS, pks
}

// utility functions

func getRandomBytes(t *testing.T, count int) [][]byte {
	t.Helper()

	bytes := make([][]byte, count)

	for i := 0; i < count; i++ {
		str := tests.GetRandomUpperCaseString(t, 10)

		bytes[i] = []byte(str)
	}

	return bytes
}

func getEntries(t *testing.T, count int) ([][]byte, [][]byte) {
	t.Helper()

	return getRandomBytes(t, count), getRandomBytes(t, count)
}

func getDirtyEntries(t *testing.T, count int) Storage {
	t.Helper()

	d := make(Storage, count)

	for i := 0; i < count; i++ {
		d[tests.RandomHash(t).Hex()] = tests.RandomIdentifier(t).Bytes()
	}

	return d
}

func CheckAssetCreation(
	t *testing.T,
	s *Object,
	assetDescriptor *common.AssetDescriptor,
	assetID identifiers.AssetID,
) {
	t.Helper()

	state, err := s.GetState(assetID)
	require.NoError(t, err)

	actualDeedsData, err := state.Bytes()
	require.NoError(t, err)

	expectedDeedsData, err := assetDescriptor.Bytes()
	require.NoError(t, err)

	require.Equal(t, actualDeedsData, expectedDeedsData)
	require.Equal(t, s.Identifier(), assetDescriptor.Operator) // check if id is assigned to operator
}

func getStorageTxnsWithEntries(
	t *testing.T,
	logicIDs []identifiers.LogicID,
	keys, values [][]byte,
) map[identifiers.LogicID]*iradix.Txn {
	t.Helper()

	txns := make(map[identifiers.LogicID]*iradix.Txn)
	for _, logicID := range logicIDs {
		txns[logicID] = getTxnsWithEntries(t, keys, values)
	}

	return txns
}

func getTxnsWithEntries(t *testing.T, keys [][]byte, values [][]byte) *iradix.Txn {
	t.Helper()

	txn := iradix.New().Txn()

	for index, key := range keys {
		txn.Insert(key, values[index])
	}

	return txn
}

func getTxnWithAssetObjects(t *testing.T, assetIDs []identifiers.AssetID, objects ...*AssetObject) *iradix.Txn {
	t.Helper()

	txn := iradix.New().Txn()

	for idx, obj := range objects {
		txn.Insert(assetIDs[idx].Bytes(), obj)
	}

	return txn
}

func getTxnWithLogicObjects(t *testing.T, objects ...*LogicObject) *iradix.Txn {
	t.Helper()

	txn := iradix.New().Txn()

	for _, obj := range objects {
		txn.Insert(obj.LogicID().Bytes(), obj)
	}

	return txn
}

/*

func getMetaContextObjFromCache(t *testing.T, so *Object, hash common.Hash) *MetaContextObject {
	t.Helper()

	bCtxData, ok := so.cache.Get(hash)
	require.True(t, ok)
	ctx, ok := bCtxData.(*MetaContextObject)
	require.True(t, ok)

	return ctx
}

func getMetaContextObjectFromDirtyEntries(t *testing.T, s *Object, hash common.Hash) *MetaContextObject {
	t.Helper()

	key := common.BytesToHex(storage.ContextObjectKey(s.id, hash))
	rawData, err := s.GetDirtyEntry(key)
	require.NoError(t, err)

	obj := new(MetaContextObject)
	err = obj.FromBytes(rawData)
	require.NoError(t, err)

	return obj
}


func checkForMetaContextObject(
	t *testing.T,
	sObj *Object,
	ctxObject MetaContextObject,
	actualCtxHash common.Hash,
) {
	t.Helper()

	expectedObjData, err := ctxObject.Bytes()
	require.NoError(t, err)

	expectedCtxHash := common.GetHash(expectedObjData)
	require.Equal(t, expectedCtxHash, actualCtxHash)

	// check if ctxObject data in dirty entries matches
	key := common.BytesToHex(storage.ContextObjectKey(sObj.id, expectedCtxHash))
	actualObjData, err := sObj.GetDirtyEntry(key) // get ctx object data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedObjData, actualObjData)
}


*/
