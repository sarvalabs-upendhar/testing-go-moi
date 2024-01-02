package state

import (
	"context"
	"math/big"
	"math/rand"
	"reflect"
	"sync"
	"testing"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/manishmeganathan/depgraph"
	"github.com/munna0908/smt"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
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
	tesseracts          map[common.Hash][]byte
	latestTesseractHash map[identifiers.Address]common.Hash
	accounts            map[common.Hash][]byte
	context             map[common.Hash][]byte
	interactions        map[common.Hash][]byte
	balances            map[common.Hash][]byte
	assetRegistry       map[common.Hash][]byte
	merkleTreeEntries   map[string][]byte
	preImages           map[common.Hash][]byte
	data                map[string][]byte
	accMetaInfo         map[string]*common.AccountMetaInfo
	createEntryHook     func() error
}

func mockDB() *MockDB {
	return &MockDB{
		tesseracts:          make(map[common.Hash][]byte),
		latestTesseractHash: make(map[identifiers.Address]common.Hash),
		accounts:            make(map[common.Hash][]byte),
		assetRegistry:       make(map[common.Hash][]byte),
		context:             make(map[common.Hash][]byte),
		interactions:        make(map[common.Hash][]byte),
		balances:            make(map[common.Hash][]byte),
		merkleTreeEntries:   make(map[string][]byte),
		preImages:           make(map[common.Hash][]byte),
		data:                make(map[string][]byte),
		accMetaInfo:         make(map[string]*common.AccountMetaInfo),
	}
}

func (m *MockDB) ReadEntry(key []byte) ([]byte, error) {
	data, ok := m.data[common.BytesToHex(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) GetAssetRegistry(addr identifiers.Address, registryHash common.Hash) ([]byte, error) {
	data, ok := m.assetRegistry[registryHash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) setAssetRegistry(t *testing.T, registryHash common.Hash, registry *RegistryObject) {
	t.Helper()

	rawData, err := registry.Bytes()
	require.NoError(t, err)

	m.assetRegistry[registryHash] = rawData
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

func (m *MockDB) GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[id.Hex()]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (m *MockDB) insertTesseract(t *testing.T, ts *common.Tesseract) {
	t.Helper()

	bytes, err := ts.Canonical().Bytes()
	require.NoError(t, err)

	m.tesseracts[ts.Hash()] = bytes
}

func (m *MockDB) insertIxns(t *testing.T, hash common.Hash, ixns common.Interactions) {
	t.Helper()

	bytes, err := ixns.Bytes()
	require.NoError(t, err)

	m.interactions[hash] = bytes
}

func (m *MockDB) setAccountMetaInfo(acc *common.AccountMetaInfo) {
	m.accMetaInfo[acc.Address.Hex()] = acc
}

func (m *MockDB) GetMerkleTreeEntry(address identifiers.Address, prefix storage.PrefixTag, key []byte) ([]byte, error) {
	entry, ok := m.merkleTreeEntries[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return entry, nil
}

func (m *MockDB) SetMerkleTreeEntry(address identifiers.Address, prefix storage.PrefixTag, key, value []byte) error {
	m.merkleTreeEntries[string(key)] = value

	return nil
}

func (m *MockDB) SetMerkleTreeEntries(
	address identifiers.Address,
	prefix storage.PrefixTag,
	entries map[string][]byte,
) error {
	for k, v := range entries {
		m.merkleTreeEntries[k] = v
	}

	return nil
}

func (m *MockDB) WritePreImages(address identifiers.Address, entries map[common.Hash][]byte) error {
	for k, v := range entries {
		m.preImages[k] = v
	}

	return nil
}

func (m *MockDB) GetPreImage(address identifiers.Address, hash common.Hash) ([]byte, error) {
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

func (m *MockDB) GetAccount(addr identifiers.Address, hash common.Hash) ([]byte, error) {
	account, ok := m.accounts[hash]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (m *MockDB) setContext(t *testing.T, object *ContextObject) {
	t.Helper()

	hash, err := object.Hash()
	require.NoError(t, err)

	bytes, err := polo.Polorize(object)
	require.NoError(t, err)

	m.context[hash] = bytes
}

func (m *MockDB) setMetaContext(t *testing.T, object *MetaContextObject) {
	t.Helper()

	hash, err := object.Hash()
	require.NoError(t, err)

	bytes, err := object.Bytes()
	require.NoError(t, err)

	m.context[hash] = bytes
}

func (m *MockDB) GetContext(addr identifiers.Address, hash common.Hash) ([]byte, error) {
	tsContext, ok := m.context[hash]
	if !ok {
		return nil, common.ErrContextStateNotFound
	}

	return tsContext, nil
}

func (m *MockDB) GetInteractions(hash common.Hash) ([]byte, error) {
	interactions, ok := m.interactions[hash]
	if !ok {
		return nil, common.ErrFetchingInteractions
	}

	return interactions, nil
}

func (m *MockDB) GetTesseract(hash common.Hash) ([]byte, error) {
	tesseracts, ok := m.tesseracts[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	return tesseracts, nil
}

func (m *MockDB) GetBalance(addr identifiers.Address, hash common.Hash) ([]byte, error) {
	balance, ok := m.balances[hash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return balance, nil
}

func (m *MockDB) setBalance(t *testing.T, hash common.Hash, bal *BalanceObject) {
	t.Helper()

	bytes, err := bal.Bytes()
	require.NoError(t, err)

	m.balances[hash] = bytes
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
		mDB.insertTesseract(t, ts)

		if ts.Interactions() != nil {
			mDB.insertIxns(t, ts.GridHash(), ts.Interactions())
		}
	}
}

func insertAccountsInDB(t *testing.T, db Store, hashes []common.Hash, acc ...*common.Account) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, hash := range hashes {
		mDB.setAccount(t, hash, acc[i])
	}
}

func insertBalancesInDB(t *testing.T, db Store, hashes []common.Hash, balances ...*BalanceObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, bal := range balances {
		mDB.setBalance(t, hashes[i], bal)
	}
}

func insertAssetRegistryInDB(t *testing.T, db Store, hashes []common.Hash, registry ...*RegistryObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, bal := range registry {
		mDB.setAssetRegistry(t, hashes[i], bal)
	}
}

func insertContextsInDB(t *testing.T, db Store, context ...*ContextObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for _, ctx := range context {
		mDB.setContext(t, ctx)
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

	bytes, err := polo.Polorize(m.dbStorage)
	if err != nil {
		return err
	}

	m.merkleRoot = common.BytesToHash(bytes)

	return nil
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
	return m.dirty != nil && len(m.dirty) > 0
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

func getMerkleTreeWithFlushedEntries(t *testing.T, keys [][]byte, values [][]byte) *MockMerkleTree {
	t.Helper()

	merkleTree := getMerkleTreeWithEntries(t, keys, values)
	err := merkleTree.Flush()
	require.NoError(t, err)

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

func getMerkleTreeWithSetHook(t *testing.T, keys [][]byte, values [][]byte, setHook func() error) *MockMerkleTree {
	t.Helper()

	merkleTree := getMerkleTreeWithEntries(t, keys, values) // insert entries to make tree dirty
	merkleTree.setHook = setHook

	return merkleTree
}

func getRoot(t *testing.T, m *MockMerkleTree) common.Hash {
	t.Helper()

	bytes, err := polo.Polorize(m.dbStorage)
	require.NoError(t, err)

	return common.BytesToHash(bytes)
}

type MockSenatus struct {
	lock       sync.RWMutex
	publicKeys map[kramaid.KramaID][]byte
}

func mockSenatus(t *testing.T) *MockSenatus {
	t.Helper()

	return &MockSenatus{
		lock:       sync.RWMutex{},
		publicKeys: make(map[kramaid.KramaID][]byte),
	}
}

func (m *MockSenatus) AddPublicKeys(ids []kramaid.KramaID, publicKeys [][]byte) error {
	for index, kramaID := range ids {
		m.publicKeys[kramaID] = publicKeys[index]
	}

	return nil
}

func (m *MockSenatus) UpdatePublicKey(key kramaid.KramaID, pk []byte) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.publicKeys[key] = pk

	return nil
}

func (m *MockSenatus) GetPublicKey(kramaID kramaid.KramaID) ([]byte, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	val, ok := m.publicKeys[kramaID]
	if !ok {
		return nil, common.ErrKramaIDNotFound
	}

	return val, nil
}

type MockContract struct {
	publicKeys map[kramaid.KramaID][]byte
}

func NewMockContract(t *testing.T, kramaIDs []kramaid.KramaID, publicKeys [][]byte) *MockContract {
	t.Helper()

	mockContract := new(MockContract)
	mockContract.publicKeys = make(map[kramaid.KramaID][]byte)

	for i := 0; i < len(kramaIDs); i++ {
		mockContract.publicKeys[kramaIDs[i]] = publicKeys[i]
	}

	return mockContract
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

func getTesseractParamsWithStateHash(address identifiers.Address, hash common.Hash) *tests.CreateTesseractParams {
	return &tests.CreateTesseractParams{
		Address: address,
		BodyCallback: func(body *common.TesseractBody) {
			body.StateHash = hash
		},
	}
}

func getTesseractParamsWithContextHash(address identifiers.Address, hash common.Hash) *tests.CreateTesseractParams {
	return &tests.CreateTesseractParams{
		Address: address,
		BodyCallback: func(body *common.TesseractBody) {
			body.ContextHash = hash
		},
	}
}

func getTesseractHash(t *testing.T, ts *common.Tesseract) common.Hash {
	t.Helper()

	hash := ts.Hash()

	return hash
}

func storeTesseractHashInCache(t *testing.T, cache *lru.Cache, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		cache.Add(ts.Address(), ts.Hash())
	}
}

func insertInContextLock(header *common.TesseractHeader, address identifiers.Address, hash common.Hash) {
	header.ContextLock[address] = common.ContextLockInfo{
		ContextHash: hash,
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
		mockSenatus(t),
	)
	require.NoError(t, err)

	if params.smCallBack != nil {
		params.smCallBack(sm)
	}

	return sm
}

func mockJournal() *Journal {
	return new(Journal)
}

func insertContextHash(so *Object, hash common.Hash) {
	so.data.ContextHash = hash
}

func insertDirtyObject(sm *StateManager, objects ...*Object) {
	for _, obj := range objects {
		sm.dirtyObjects[obj.address] = obj
	}
}

func storeInSmCache(sm *StateManager, k, v interface{}) {
	sm.cache.Add(k, v)
}

type createStateObjectParams struct {
	address                   identifiers.Address
	cache                     *lru.Cache
	journal                   *Journal
	db                        *MockDB
	account                   *common.Account
	soCallback                func(so *Object)
	metaStorageTreeCallback   func(so *Object)
	dbCallback                func(db *MockDB)
	activeStorageTreeCallback func(activeStorageTrees map[string]tree.MerkleTree)
}

func createTestStateObject(t *testing.T, params *createStateObjectParams) *Object {
	t.Helper()

	var (
		addr  = tests.RandomAddress(t)
		mDB   = mockDB()
		cache = mockCache(t)
		data  = new(common.Account)
	)

	if params == nil {
		params = &createStateObjectParams{}
	}

	if !params.address.IsNil() {
		addr = params.address
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

	so := NewStateObject(addr, cache, mockJournal(), mDB, *data)
	so.metaStorageTree = mockMerkleTreeWithDB()
	so.logicTree = mockMerkleTreeWithDB()

	if params.soCallback != nil {
		params.soCallback(so)
	}

	if params.activeStorageTreeCallback != nil {
		params.activeStorageTreeCallback(so.activeStorageTrees)
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

func stateObjectParamsWithBalance(
	t *testing.T,
	balanceHash common.Hash,
	balance *BalanceObject,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.data.Balance = balanceHash
			so.balance = balance
		},
	}
}

func stateObjectParamsWithRegistry(
	t *testing.T,
	registryHash common.Hash,
	registry *RegistryObject,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.data.AssetRegistry = registryHash
			so.registry = registry
		},
	}
}

func stateObjectParamsWithASTAndMST(
	t *testing.T,
	ast map[string]tree.MerkleTree,
	mst tree.MerkleTree,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.activeStorageTrees = ast
			so.metaStorageTree = mst
		},
	}
}

func stateObjectParamsWithAST(t *testing.T, ast map[string]tree.MerkleTree) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.activeStorageTrees = ast
		},
	}
}

func stateObjectParamsWithInvalidMST(t *testing.T) *createStateObjectParams {
	t.Helper()

	return stateObjectParamsWithMST(t, identifiers.NilAddress, nil, nil, tests.RandomHash(t))
}

func stateObjectParamsWithMST(
	t *testing.T,
	address identifiers.Address,
	db Store,
	mst tree.MerkleTree,
	root common.Hash,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		address: address,
		db:      mDB,
		soCallback: func(so *Object) {
			so.metaStorageTree = mst
			so.data.StorageRoot = root
		},
	}
}

func stateObjectParamsWithLogicTree(
	t *testing.T,
	address identifiers.Address,
	db Store,
	logicTree tree.MerkleTree,
	root common.Hash,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		address: address,
		db:      mDB,
		soCallback: func(so *Object) {
			so.logicTree = logicTree
			so.data.LogicRoot = root // set logic root as it needs to be returned
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

// stateObjectParamsWithContextObject stores object in db if inDB is true else stores in cache
func stateObjectParamsWithContextObject(
	t *testing.T,
	obj *ContextObject,
	hash common.Hash,
	addToDB bool,
) *createStateObjectParams {
	t.Helper()

	if addToDB {
		return &createStateObjectParams{
			soCallback: func(so *Object) {
				mDB, ok := so.db.(*MockDB)
				require.True(t, ok)

				rawData, err := obj.Bytes()
				require.NoError(t, err)

				mDB.context[hash] = rawData
			},
		}
	}

	return &createStateObjectParams{
		soCallback: func(so *Object) {
			so.cache.Add(hash, obj)
		},
	}
}

func stateObjectParamsWithTestData(t *testing.T, areTreesNil bool) *createStateObjectParams {
	t.Helper()

	keys, values := getEntries(t, 2)

	return &createStateObjectParams{
		soCallback: func(s *Object) {
			s.accType = common.LogicAccount
			acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
				acc.ContextHash = tests.RandomHash(t)
			})

			s.data = *acc
			s.balance, _ = getTestBalance(t, getAssetMap(getAssetIDsAndBalances(t, 2)))
			s.approvals.PrvHash = tests.RandomHash(t) // initialize any one field in asset approvals object

			s.logicTree = getMerkleTreeWithFlushedEntries(t, keys[0:1], values[0:1])

			s.files = make(map[common.Hash][]byte)
			s.files[tests.RandomHash(t)] = tests.RandomHash(t).Bytes()
			s.files[tests.RandomHash(t)] = tests.RandomHash(t).Bytes()
			s.registry, _ = getTestRegistryObject(
				t,
				map[string][]byte{
					tests.RandomHash(t).String(): tests.RandomHash(t).Bytes(),
				})

			if areTreesNil {
				s.logicTree = nil
			}
		},
		metaStorageTreeCallback: func(s *Object) {
			s.metaStorageTree = getMerkleTreeWithFlushedEntries(t, keys[1:], values[1:])
			if areTreesNil {
				s.metaStorageTree = nil
			}
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
	require.NotEqual(t,
		reflect.ValueOf(sObj.approvals.Approvals).Pointer(),
		reflect.ValueOf(copiedSO.approvals.Approvals).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(sObj.balance.AssetMap).Pointer(),
		reflect.ValueOf(copiedSO.balance.AssetMap).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(sObj.approvals).Pointer(),
		reflect.ValueOf(copiedSO.approvals).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(sObj.balance).Pointer(),
		reflect.ValueOf(copiedSO.balance).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(sObj.journal).Pointer(),
		reflect.ValueOf(copiedSO.journal).Pointer(),
	)
}

func getAssetIDsAndBalances(t *testing.T, count int) ([]identifiers.AssetID, []*big.Int) {
	t.Helper()

	ids := make([]identifiers.AssetID, count)
	for i := 0; i < count; i++ {
		ids[i] = tests.GetRandomAssetID(t, tests.RandomAddress(t))
	}

	return ids, tests.GetRandomNumbers(t, 10000, count)
}

func getAssetMap(assetIDs []identifiers.AssetID, balances []*big.Int) common.AssetMap {
	assetMap := make(common.AssetMap)

	for i := 0; i < len(assetIDs); i++ {
		assetMap[assetIDs[i]] = balances[i]
	}

	return assetMap
}

func getAssetMaps(assetIDs []identifiers.AssetID, balances []*big.Int, assetsPerAssetMap int) []common.AssetMap {
	assetMapCount := len(assetIDs) / assetsPerAssetMap
	assetMap := make([]common.AssetMap, assetMapCount)

	for i := 0; i < assetMapCount; i++ {
		assetMap[i] = getAssetMap(
			assetIDs[i*assetsPerAssetMap:i*assetsPerAssetMap+assetsPerAssetMap],
			balances[i*assetsPerAssetMap:i*assetsPerAssetMap+assetsPerAssetMap],
		)
	}

	return assetMap
}

func getTestRegistryObject(t *testing.T, entries map[string][]byte) (*RegistryObject, common.Hash) {
	t.Helper()

	registry := &RegistryObject{
		Entries: entries,
	}

	data, err := registry.Bytes()
	require.NoError(t, err)

	return registry, common.GetHash(data)
}

func getTestBalance(t *testing.T, assetMap common.AssetMap) (*BalanceObject, common.Hash) {
	t.Helper()

	balance := &BalanceObject{
		AssetMap: assetMap,
		PrvHash:  tests.RandomHash(t),
	}

	data, err := balance.Bytes()
	require.NoError(t, err)

	return balance, common.GetHash(data)
}

func getTestBalances(t *testing.T, assetMap []common.AssetMap, count int) ([]*BalanceObject, []common.Hash) {
	t.Helper()

	balances := make([]*BalanceObject, count)
	hashes := make([]common.Hash, count)

	for i := 0; i < count; i++ {
		balances[i], hashes[i] = getTestBalance(t, assetMap[i])
	}

	return balances, hashes
}

func getTestAccounts(t *testing.T, balanceHash []common.Hash, count int) ([]*common.Account, []common.Hash) {
	t.Helper()

	accounts := make([]*common.Account, count)
	hashes := make([]common.Hash, count)

	for i := 0; i < count; i++ {
		acc, stateHash := tests.GetTestAccount(t, func(acc *common.Account) {
			acc.Balance = balanceHash[i]
		})

		accounts[i] = acc
		hashes[i] = stateHash
	}

	return accounts, hashes
}

func getAccMetaInfos(t *testing.T, count int) []*common.AccountMetaInfo {
	t.Helper()

	accMetaInfo := make([]*common.AccountMetaInfo, count)
	for i := 0; i < count; i++ {
		accMetaInfo[i] = tests.GetRandomAccMetaInfo(t, 8)
	}

	return accMetaInfo
}

func getMetaContextObject(
	t *testing.T,
	behHash common.Hash,
	randHash common.Hash,
) (*MetaContextObject, common.Hash) {
	t.Helper()

	mCtx := &MetaContextObject{
		BehaviouralContext: behHash,
		RandomContext:      randHash,
	}

	hash, err := mCtx.Hash()
	require.NoError(t, err)

	return mCtx, hash
}

func getMetaContextObjects(t *testing.T, hashes []common.Hash) ([]*MetaContextObject, []common.Hash) {
	t.Helper()

	count := len(hashes)
	mObj := make([]*MetaContextObject, count/2)
	mHashes := make([]common.Hash, count/2)

	j := 0

	for i := 0; i < count; i += 2 {
		mObj[j], mHashes[j] = getMetaContextObject(t, hashes[i], hashes[i+1])
		j += 1
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

func getRawContextObjects(t *testing.T, obj []*ContextObject) [][]byte {
	t.Helper()

	rawContextObjects := make([][]byte, len(obj))

	for i := 0; i < len(obj); i++ {
		rawContextObject, err := polo.Polorize(obj[i])
		assert.NoError(t, err)

		rawContextObjects[i] = rawContextObject
	}

	return rawContextObjects
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

type ICSNodes struct {
	senderBeh    *common.NodeSet
	senderRand   *common.NodeSet
	receiverBeh  *common.NodeSet
	receiverRand *common.NodeSet
}

func getICSNodes(
	senderBeh *common.NodeSet,
	senderRand *common.NodeSet,
	receiverBeh *common.NodeSet,
	receiverRand *common.NodeSet,
) *ICSNodes {
	return &ICSNodes{
		senderBeh:    senderBeh,
		senderRand:   senderRand,
		receiverBeh:  receiverBeh,
		receiverRand: receiverRand,
	}
}

func mockContextLock() map[identifiers.Address]common.ContextLockInfo {
	return make(map[identifiers.Address]common.ContextLockInfo)
}

func getTestAssetDescriptor(t *testing.T, operator identifiers.Address, symbol string) *common.AssetDescriptor {
	t.Helper()

	return &common.AssetDescriptor{
		Symbol:     symbol,
		Operator:   operator,
		Supply:     big.NewInt(10000),
		Dimension:  4,
		IsStateFul: false,
		IsLogical:  false,
		LogicID:    tests.GetLogicID(t, tests.RandomAddress(t)),
	}
}

func createMetaStorageTree(
	t *testing.T,
	db Store,
	address identifiers.Address,
	logicID identifiers.LogicID,
	storageKeys [][]byte,
	storageValues [][]byte,
) (*tree.KramaHashTree, common.Hash) {
	t.Helper()

	_, storageRoot := createTestKramaHashTree(t, db, address, storage.Storage, storageKeys, storageValues)

	return createTestKramaHashTree(
		t,
		db,
		address,
		storage.Storage,
		[][]byte{logicID.Bytes()},
		[][]byte{storageRoot.Bytes()})
}

func createTestKramaHashTree(
	t *testing.T,
	db Store,
	address identifiers.Address,
	prefix storage.PrefixTag,
	keys [][]byte,
	values [][]byte,
) (*tree.KramaHashTree, common.Hash) {
	t.Helper()

	kt, err := tree.NewKramaHashTree(address, common.NilHash, db, blake256.New(), prefix)
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

func getTesseractParams(
	address identifiers.Address,
	ixns common.Interactions,
	hash common.Hash,
) *tests.CreateTesseractParams {
	return &tests.CreateTesseractParams{
		Ixns: ixns,
		HeaderCallback: func(header *common.TesseractHeader) {
			header.ContextLock = mockContextLock()
			insertInContextLock(header, address, hash)
		},
		BodyCallback: func(body *common.TesseractBody) {
			body.ContextHash = hash
		},
	}
}

func createRandomArrayOfBits(t *testing.T, count int) []*common.ArrayOfBits {
	t.Helper()

	arrayOfBits := make([]*common.ArrayOfBits, 0)

	for i := 0; i < count; i++ {
		arrayOfBit := common.ArrayOfBits{
			Size:     1,
			Elements: []uint64{uint64(rand.Intn(256))},
		}

		arrayOfBits = append(arrayOfBits, &arrayOfBit)
	}

	return arrayOfBits
}

func getCopiedAST(ast map[string]tree.MerkleTree) map[string]tree.MerkleTree {
	copiedAST := make(map[string]tree.MerkleTree, len(ast))

	for logic, merkleTree := range ast {
		copiedAST[logic] = merkleTree.Copy()
	}

	return copiedAST
}

// getASTWithDefaultFlushedEntries returns ast with merkle trees of tree count
func getASTWithDefaultFlushedEntries(
	t *testing.T,
	treeCount int,
	entriesPerTree int,
) map[string]tree.MerkleTree {
	t.Helper()

	logicIds := tests.GetLogicIDs(t, treeCount)
	keys, values := getEntries(t, entriesPerTree)
	ast := getActiveStorageTreesWithFlushedEntries(t, logicIds, keys, values)

	return ast
}

// getASTWithDefaultDirtyEntries returns ast with merkle trees of tree count that have both flushed and dirty entries
func getASTWithDefaultDirtyEntries(
	t *testing.T,
	treeCount int,
	entriesPerTree int,
) map[string]tree.MerkleTree {
	t.Helper()

	ast := getASTWithDefaultFlushedEntries(t, treeCount, entriesPerTree)

	for _, merkleTree := range ast {
		insertRandomEntriesInMerkleTree(t, merkleTree, entriesPerTree)
	}

	return ast
}

func getMerkleTreeWithDefaultFlushedEntries(t *testing.T, entriesPerTree int) tree.MerkleTree {
	t.Helper()

	keys, values := getEntries(t, entriesPerTree)

	return getMerkleTreeWithFlushedEntries(t, keys, values)
}

// insertRandomEntriesInMerkleTress makes tree dirty by inserting random entries
func insertRandomEntriesInMerkleTree(t *testing.T, merkleTree tree.MerkleTree, entriesPerTree int) {
	t.Helper()

	keys, values := getEntries(t, entriesPerTree)

	for i, key := range keys {
		err := merkleTree.Set(key, values[i])
		require.NoError(t, err)
	}
}

// getMerkleTreeWithDefaultDirtyEntries return merkle tree that has both flushed and dirty entries
func getMerkleTreeWithDefaultDirtyEntries(t *testing.T, entriesPerTree int) tree.MerkleTree {
	t.Helper()

	mst := getMerkleTreeWithDefaultFlushedEntries(t, entriesPerTree)
	insertRandomEntriesInMerkleTree(t, mst, entriesPerTree)

	return mst
}

func getActiveStorageTrees(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[string(logicIds[i])] = getMerkleTreeWithEntries(t, keys, values)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithFlushedEntries(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[string(logicIds[i])] = getMerkleTreeWithFlushedEntries(t, keys, values)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithCommitHook(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[string(logicIds[i])] = getMerkleTreeWithCommitHook(t, keys, values, commitHook)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithFlushHook(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[string(logicIds[i])] = getMerkleTreeWithFlushHook(t, keys, values, commitHook)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithRootHook(
	t *testing.T,
	logicIds []identifiers.LogicID,
	keys [][]byte,
	values [][]byte,
	rootHashHook func() (common.Hash, error),
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[string(logicIds[i])] = getMerkleTreeWithRootHashHook(t, keys, values, rootHashHook)
	}

	return activeStorageTrees
}

func getContextObjFromCache(t *testing.T, so *Object, hash common.Hash) *ContextObject {
	t.Helper()

	bCtxData, ok := so.cache.Get(hash)
	require.True(t, ok)
	ctx, ok := bCtxData.(*ContextObject)
	require.True(t, ok)

	return ctx
}

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

	key := common.BytesToHex(storage.ContextObjectKey(s.address, hash))
	rawData, err := s.GetDirtyEntry(key)
	require.NoError(t, err)

	obj := new(MetaContextObject)
	err = obj.FromBytes(rawData)
	require.NoError(t, err)

	return obj
}

func getContextObjectFromDirtyEntries(t *testing.T, s *Object, hash common.Hash) *ContextObject {
	t.Helper()

	key := common.BytesToHex(storage.ContextObjectKey(s.address, hash))
	rawData, err := s.GetDirtyEntry(key)
	require.NoError(t, err)

	obj := new(ContextObject)
	err = obj.FromBytes(rawData)
	require.NoError(t, err)

	return obj
}

func checkForTesseractInSMCache(t *testing.T, sm *StateManager, ts *common.Tesseract, withInteractions bool) {
	t.Helper()

	object, isCached := sm.cache.Get(ts.Hash())
	if withInteractions {
		require.False(t, isCached) // make sure tesseract not cached

		return
	}

	require.True(t, isCached) // make sure tesseract cached, if fetched without interactions

	cachedTS, ok := object.(*common.Tesseract)
	require.True(t, ok)

	require.Equal(t, ts, cachedTS) // make sure cached tesseract matches
}

func checkForDirtyObject(t *testing.T, sm *StateManager, address identifiers.Address, exists bool) {
	t.Helper()

	_, ok := sm.dirtyObjects[address]
	require.Equal(t, exists, ok)
}

func checkIfDirtyObjectEqual(t *testing.T, sm *StateManager, address identifiers.Address, expectedObj *Object) {
	t.Helper()

	obj, ok := sm.dirtyObjects[address]
	require.True(t, ok)
	require.Equal(t, expectedObj, obj)
}

func checkForCache(t *testing.T, sm *StateManager, address identifiers.Address) {
	t.Helper()

	_, isCached := sm.cache.Get(address) // check if added to cache
	require.True(t, isCached)
}

func checkIfContextMatches(
	t *testing.T,
	expectedBeh *ContextObject,
	expectedRand *ContextObject,
	beh []kramaid.KramaID,
	rand []kramaid.KramaID,
) {
	t.Helper()

	require.Equal(t, expectedBeh.Ids, beh)
	require.Equal(t, expectedRand.Ids, rand)
}

func checkIfNodesetEqual(
	t *testing.T,
	expectedBeh *common.NodeSet,
	expectedRand *common.NodeSet,
	beh *common.NodeSet,
	rand *common.NodeSet,
) {
	t.Helper()

	require.Equal(t, expectedBeh, beh)
	require.Equal(t, expectedRand, rand)
}

func checkIfTreesAreEqual(t *testing.T, expectedTree tree.MerkleTree, actualTree tree.MerkleTree) {
	t.Helper()

	err := expectedTree.Commit()
	require.NoError(t, err)

	err = actualTree.Commit()
	require.NoError(t, err)

	expectedRoot, err := expectedTree.RootHash()
	require.NoError(t, err)

	actualRoot, err := actualTree.RootHash()
	require.NoError(t, err)

	require.Equal(t, expectedRoot, actualRoot)
}

func validateStateObject(t *testing.T, so *Object, accType common.AccountType, address identifiers.Address) {
	t.Helper()

	require.Equal(t, address, so.address)
	require.Equal(t, accType, so.accType)
	require.NotNil(t, so.journal)
	require.NotNil(t, so.db)
	require.NotNil(t, so.data)
	require.NotNil(t, so.cache)
}

func checkForStateObject(t *testing.T, expectedObj *Object, obj *Object) {
	t.Helper()

	require.Equal(t, expectedObj.balance, obj.balance)
	require.Equal(t, expectedObj.data, obj.data)
	require.Equal(t, expectedObj.address, obj.address)
	require.Equal(t, expectedObj.accType, obj.accType)
}

func checkIfStateObjectAreEqual(
	t *testing.T,
	expectedObj *Object,
	actualObj *Object,
	areTreesNil bool,
) {
	t.Helper()

	require.NotNil(t, actualObj.db)
	require.NotNil(t, actualObj.cache)
	require.NotNil(t, actualObj.journal)
	require.Equal(t, expectedObj.balance.AssetMap, actualObj.balance.AssetMap)
	require.Equal(t, expectedObj.approvals, actualObj.approvals)
	require.Equal(t, expectedObj.dirtyEntries, actualObj.dirtyEntries)
	require.Equal(t, expectedObj.data, actualObj.data)
	require.Equal(t, expectedObj.files, actualObj.files)

	if expectedObj.logicTree != nil {
		checkIfTreesAreEqual(t, expectedObj.logicTree, actualObj.logicTree)
	}

	if expectedObj.metaStorageTree != nil {
		checkIfTreesAreEqual(t, expectedObj.metaStorageTree, actualObj.metaStorageTree)
	}

	if areTreesNil {
		require.Nil(t, actualObj.logicTree)
		require.Nil(t, actualObj.metaStorageTree)
	}
}

func checkForBalances(t *testing.T, sObj *Object, expectedBalance *big.Int, assetID identifiers.AssetID) {
	t.Helper()

	actualBalance, err := sObj.BalanceOf(assetID)
	require.NoError(t, err)
	require.Equal(t, expectedBalance, actualBalance)
}

func checkForRegistry(
	t *testing.T,
	sObj *Object,
	expectedRegistry *RegistryObject,
	actualRegistryHash common.Hash,
) {
	t.Helper()

	expectedRegistryData, err := expectedRegistry.Bytes()
	require.NoError(t, err)

	expectedRegistryHash := common.GetHash(expectedRegistryData)
	require.Equal(t, expectedRegistryHash, actualRegistryHash)

	// check if registry data in dirty entries and state object is same
	key := common.BytesToHex(storage.RegistryObjectKey(sObj.address, expectedRegistryHash))
	actualRegistryData, err := sObj.GetDirtyEntry(key) // get registry data from dirty entries
	require.NoError(t, err)

	require.Equal(t, expectedRegistryData, actualRegistryData)

	require.Equal(t, sObj.data.AssetRegistry, actualRegistryHash) // check if registry hash inserted in account
}

func checkForBalance(
	t *testing.T,
	sObj *Object,
	expectedBalance *BalanceObject,
	actualBalanceHash common.Hash,
	journalIndex int,
) {
	t.Helper()

	expectedBalanceData, err := expectedBalance.Bytes()
	require.NoError(t, err)

	expectedBalanceHash := common.GetHash(expectedBalanceData)
	require.Equal(t, expectedBalanceHash, actualBalanceHash)

	// check if balance data in dirty entries and state object is same
	key := common.BytesToHex(storage.BalanceObjectKey(sObj.address, expectedBalanceHash))
	actualBalanceData, err := sObj.GetDirtyEntry(key) // get balance data from dirty entries
	require.NoError(t, err)

	require.Equal(t, expectedBalanceData, actualBalanceData)

	require.Equal(t, sObj.data.Balance.Bytes(), actualBalanceHash.Bytes()) // check if balance hash inserted in account

	// check if address and balance hash inserted in journal
	checkJournalEntries(t, sObj.Journal(), sObj.address, journalIndex, actualBalanceHash)
}

func checkForAccount(
	t *testing.T,
	sObj *Object,
	expectedAcc *common.Account,
	actualAccHash common.Hash,
	journalIndex int,
) {
	t.Helper()

	expectedAccData, err := expectedAcc.Bytes()
	require.NoError(t, err)

	expectedAccHash := common.GetHash(expectedAccData)
	require.Equal(t, expectedAccHash, actualAccHash)

	// check if account data in dirty entries and state object is same
	key := common.BytesToHex(storage.AccountKey(sObj.address, expectedAccHash))
	actualAccData, err := sObj.GetDirtyEntry(key) // get account data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedAccData, actualAccData)

	// check if address and acc hash inserted in journal
	checkJournalEntries(t, sObj.Journal(), sObj.Address(), journalIndex, actualAccHash)
}

func checkJournalEntries(t *testing.T, journal *Journal, addr identifiers.Address, journalIndex int, hash common.Hash) {
	t.Helper()

	require.Equal(t, hash.Bytes(), journal.entries[journalIndex].cID().Bytes())
	require.Equal(t, addr, *journal.entries[journalIndex].modifiedAddress())
}

func checkForContextObject(
	t *testing.T,
	sObj *Object,
	ctxObject ContextObject,
	actualCtxHash common.Hash,
) {
	t.Helper()

	expectedObjData, err := ctxObject.Bytes()
	require.NoError(t, err)

	expectedCtxHash := common.GetHash(expectedObjData)
	require.Equal(t, expectedCtxHash, actualCtxHash)

	// check if ctxObject data in dirty entries matches
	key := common.BytesToHex(storage.ContextObjectKey(sObj.address, expectedCtxHash))
	actualObjData, err := sObj.GetDirtyEntry(key) // get ctx object data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedObjData, actualObjData)
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
	key := common.BytesToHex(storage.ContextObjectKey(sObj.address, expectedCtxHash))
	actualObjData, err := sObj.GetDirtyEntry(key) // get ctx object data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedObjData, actualObjData)
}

func checkIfActiveStorageTreesAreCommitted(
	t *testing.T,
	inputAST map[string]tree.MerkleTree,
	sObj *Object,
) {
	t.Helper()

	for logicID, merkleTree := range inputAST {
		inputMerkleTree, ok := merkleTree.(*MockMerkleTree) // convert inorder to extract concrete type
		require.True(t, ok)

		expectedRoot := getRoot(t, inputMerkleTree)

		actualMerkleTree, ok := sObj.activeStorageTrees[logicID].(*MockMerkleTree)
		require.True(t, ok)

		// make sure merkleTree is not committed, if merkle tree doesn't have dirty entries
		if !inputMerkleTree.IsDirty() {
			require.Equal(t, inputMerkleTree.merkleRoot, actualMerkleTree.merkleRoot) // check merkle root didn't change

			continue
		}

		// make sure merkle root updated
		require.Equal(t, expectedRoot, actualMerkleTree.merkleRoot)

		// make sure metaStorageTree has logicID,storageRoot pair
		actualRoot, err := sObj.metaStorageTree.Get(common.FromHex(logicID))
		require.NoError(t, err)

		require.Equal(t, expectedRoot.Bytes(), actualRoot)
	}
}

func checkIfMetaStorageTreeCommitted(
	t *testing.T,
	inputMST tree.MerkleTree,
	sObj *Object,
	actualRoot common.Hash,
	journalIndex int,
) {
	t.Helper()

	inputMerkleTree, ok := inputMST.(*MockMerkleTree)
	require.True(t, ok)

	expectedRoot := getRoot(t, inputMerkleTree)

	actualMerkleTree, ok := sObj.metaStorageTree.(*MockMerkleTree)
	require.True(t, ok)

	require.Equal(t, sObj.data.StorageRoot, actualRoot) // make sure storage root returned and stored are same

	// make sure merkle tree is not committed, if merkle tree doesn't have dirty entries
	if !inputMerkleTree.IsDirty() {
		require.Equal(t, inputMerkleTree.merkleRoot, actualMerkleTree.merkleRoot) // check merkle root didn't change

		return
	}

	require.Equal(t, expectedRoot, actualMerkleTree.merkleRoot)
	require.Equal(t, expectedRoot, actualRoot)
	checkJournalEntries(t, sObj.Journal(), sObj.Address(), journalIndex, expectedRoot)
}

func checkIfLogicTreeCommitted(
	t *testing.T,
	inputLogicTree tree.MerkleTree,
	sObj *Object,
	actualRoot common.Hash,
) {
	t.Helper()

	inputMerkleTree, ok := inputLogicTree.(*MockMerkleTree)
	require.True(t, ok)

	expectedRoot := getRoot(t, inputMerkleTree)

	actualMerkleTree, ok := sObj.logicTree.(*MockMerkleTree)
	require.True(t, ok)

	require.Equal(t, sObj.data.LogicRoot, actualRoot) // make sure storage root returned and stored are same
	require.Equal(t, expectedRoot, actualMerkleTree.merkleRoot)
	require.Equal(t, expectedRoot, actualRoot)
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
		storageTree, ok := s.activeStorageTrees[string(logicID)]
		require.True(t, ok)

		checkIfMerkleTreeFlushed(t, storageTree, isFlushed)
	}
}

func checkForContextUpdate(
	t *testing.T,
	sObj *Object,
	cObj []*ContextObject,
	metaHash common.Hash,
	behaviouralNodes []kramaid.KramaID,
	randomNodes []kramaid.KramaID,
) {
	t.Helper()

	// get context objects from dirty entries
	actualMetaContext := getMetaContextObjectFromDirtyEntries(t, sObj, metaHash)
	actualBehaviouralContext := getContextObjectFromDirtyEntries(t, sObj, actualMetaContext.BehaviouralContext)
	actualRandomContext := getContextObjectFromDirtyEntries(t, sObj, actualMetaContext.RandomContext)

	cObj[0].Ids = append(cObj[0].Ids, behaviouralNodes...)
	cObj[1].Ids = append(cObj[1].Ids, randomNodes...)

	// check if context objects has updated nodes
	require.Equal(t, cObj[0].Ids, actualBehaviouralContext.Ids)
	require.Equal(t, cObj[1].Ids, actualRandomContext.Ids)
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

func validateTesseract(t *testing.T, ts *common.Tesseract, expectedTS *common.Tesseract, withInteractions bool) {
	t.Helper()

	if withInteractions { // check if tesseracts matches
		ts.Hash() // calculate hash to fill hash field in tesseract

		require.Equal(t, expectedTS, ts)

		return
	}

	require.Equal(t, expectedTS.Canonical(), ts.Canonical())
	require.Equal(t, 0, len(ts.Interactions())) // make sure returned tesseract has zero ixns
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

func getDirtyEntries(t *testing.T, count int) LogicStorageObject {
	t.Helper()

	d := make(LogicStorageObject, count)

	for i := 0; i < count; i++ {
		d[tests.RandomHash(t).Hex()] = tests.RandomAddress(t).Bytes()
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

	actualRegistryData, err := s.GetRegistryEntry(string(assetID))
	require.NoError(t, err)

	expectedRegistryData, err := assetDescriptor.Bytes()
	require.NoError(t, err)

	require.Equal(t, actualRegistryData, expectedRegistryData)
	require.Equal(t, s.Address(), assetDescriptor.Operator) // check if address is assigned to operator
}

func getTestAssetID(addr identifiers.Address, asset *common.AssetDescriptor) identifiers.AssetID {
	identifiers.NewAssetIDv0(asset.IsLogical, asset.IsStateFul, asset.Dimension, uint16(asset.Standard), addr)

	return identifiers.NewAssetIDv0(asset.IsLogical, asset.IsStateFul, asset.Dimension, uint16(asset.Standard), addr)
}

func getContextObjects(
	t *testing.T,
	ids []kramaid.KramaID,
	idsPerObj int,
	objCount int,
) ([]*ContextObject, []common.Hash) {
	t.Helper()

	obj := make([]*ContextObject, objCount)
	hashes := make([]common.Hash, objCount)

	for i := 0; i < objCount; i++ {
		copiedIds := make([]kramaid.KramaID, idsPerObj)

		copy(copiedIds, ids[i*idsPerObj:i*idsPerObj+idsPerObj])

		obj[i] = &ContextObject{
			Ids: copiedIds,
		}

		hash, err := obj[i].Hash()
		require.NoError(t, err)

		hashes[i] = hash
	}

	return obj, hashes
}
