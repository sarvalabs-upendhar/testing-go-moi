package guna

import (
	"context"
	"math/big"
	"reflect"
	"sync"
	"testing"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/munna0908/smt"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/guna/tree"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/jug/engineio"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

var (
	emptyKeys   [][]byte
	emptyValues [][]byte
)

type MockServer struct{}

func mockServer() *MockServer {
	return new(MockServer)
}

func (m *MockServer) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	// TODO implement me
	panic("implement me")
}

type MockDB struct {
	tesseracts          map[types.Hash][]byte
	latestTesseractHash map[types.Address]types.Hash
	accounts            map[types.Hash][]byte
	context             map[types.Hash][]byte
	interactions        map[types.Hash][]byte
	balances            map[types.Hash][]byte
	assetRegistry       map[types.Hash][]byte
	merkleTreeEntries   map[string][]byte
	preImages           map[types.Hash][]byte
	data                map[string][]byte
	accMetaInfo         map[string]*types.AccountMetaInfo
	createEntryHook     func() error
}

func mockDB() *MockDB {
	return &MockDB{
		tesseracts:          make(map[types.Hash][]byte),
		latestTesseractHash: make(map[types.Address]types.Hash),
		accounts:            make(map[types.Hash][]byte),
		assetRegistry:       make(map[types.Hash][]byte),
		context:             make(map[types.Hash][]byte),
		interactions:        make(map[types.Hash][]byte),
		balances:            make(map[types.Hash][]byte),
		merkleTreeEntries:   make(map[string][]byte),
		preImages:           make(map[types.Hash][]byte),
		data:                make(map[string][]byte),
		accMetaInfo:         make(map[string]*types.AccountMetaInfo),
	}
}

func (m *MockDB) ReadEntry(key []byte) ([]byte, error) {
	data, ok := m.data[types.BytesToHex(key)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) GetAssetRegistry(addr types.Address, registryHash types.Hash) ([]byte, error) {
	data, ok := m.assetRegistry[registryHash]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) CreateEntry(key, value []byte) error {
	if m.createEntryHook != nil {
		return m.createEntryHook()
	}

	hexKey := types.BytesToHex(key)
	if _, ok := m.data[hexKey]; ok {
		return types.ErrKeyExists
	}

	m.data[hexKey] = value

	return nil
}

func (m *MockDB) Contains(key []byte) (bool, error) {
	if _, ok := m.data[types.BytesToHex(key)]; ok {
		return true, nil
	}

	return false, nil
}

func (m *MockDB) UpdateEntry(key, value []byte) error {
	hexKey := types.BytesToHex(key)

	if _, ok := m.data[hexKey]; !ok {
		return types.ErrKeyNotFound
	}

	m.data[hexKey] = value

	return nil
}

func (m *MockDB) NewBatchWriter() db.BatchWriter {
	return nil
}

func (m *MockDB) GetEntries(prefix []byte) chan types.DBEntry {
	return nil
}

func (m *MockDB) GetAccountMetaInfo(id types.Address) (*types.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[id.Hex()]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (m *MockDB) insertTesseract(t *testing.T, ts *types.Tesseract) {
	t.Helper()

	bytes, err := ts.Canonical().Bytes()
	require.NoError(t, err)

	m.tesseracts[ts.Hash()] = bytes
}

func (m *MockDB) insertIxns(t *testing.T, hash types.Hash, ixns types.Interactions) {
	t.Helper()

	bytes, err := ixns.Bytes()
	require.NoError(t, err)

	m.interactions[hash] = bytes
}

func (m *MockDB) setAccountMetaInfo(acc *types.AccountMetaInfo) {
	m.accMetaInfo[acc.Address.Hex()] = acc
}

func (m *MockDB) GetMerkleTreeEntry(address types.Address, prefix dhruva.Prefix, key []byte) ([]byte, error) {
	entry, ok := m.merkleTreeEntries[string(key)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return entry, nil
}

func (m *MockDB) SetMerkleTreeEntry(address types.Address, prefix dhruva.Prefix, key, value []byte) error {
	m.merkleTreeEntries[string(key)] = value

	return nil
}

func (m *MockDB) SetMerkleTreeEntries(address types.Address, prefix dhruva.Prefix, entries map[string][]byte) error {
	for k, v := range entries {
		m.merkleTreeEntries[k] = v
	}

	return nil
}

func (m *MockDB) WritePreImages(address types.Address, entries map[types.Hash][]byte) error {
	for k, v := range entries {
		m.preImages[k] = v
	}

	return nil
}

func (m *MockDB) GetPreImage(address types.Address, hash types.Hash) ([]byte, error) {
	preImage, ok := m.preImages[hash]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return preImage, nil
}

func (m *MockDB) setAccount(t *testing.T, hash types.Hash, acc *types.Account) {
	t.Helper()

	bytes, err := acc.Bytes()
	require.NoError(t, err)

	m.accounts[hash] = bytes
}

func (m *MockDB) GetAccount(addr types.Address, hash types.Hash) ([]byte, error) {
	account, ok := m.accounts[hash]
	if !ok {
		return nil, types.ErrAccountNotFound
	}

	return account, nil
}

func (m *MockDB) setContext(t *testing.T, object *gtypes.ContextObject) {
	t.Helper()

	hash, err := object.Hash()
	require.NoError(t, err)

	bytes, err := polo.Polorize(object)
	require.NoError(t, err)

	m.context[hash] = bytes
}

func (m *MockDB) setMetaContext(t *testing.T, object *gtypes.MetaContextObject) {
	t.Helper()

	hash, err := object.Hash()
	require.NoError(t, err)

	bytes, err := object.Bytes()
	require.NoError(t, err)

	m.context[hash] = bytes
}

func (m *MockDB) GetContext(addr types.Address, hash types.Hash) ([]byte, error) {
	tsContext, ok := m.context[hash]
	if !ok {
		return nil, types.ErrContextStateNotFound
	}

	return tsContext, nil
}

func (m *MockDB) GetInteractions(hash types.Hash) ([]byte, error) {
	interactions, ok := m.interactions[hash]
	if !ok {
		return nil, types.ErrFetchingInteractions
	}

	return interactions, nil
}

func (m *MockDB) GetTesseract(hash types.Hash) ([]byte, error) {
	tesseracts, ok := m.tesseracts[hash]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	return tesseracts, nil
}

func (m *MockDB) GetBalance(addr types.Address, hash types.Hash) ([]byte, error) {
	balance, ok := m.balances[hash]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return balance, nil
}

func (m *MockDB) setBalance(t *testing.T, hash types.Hash, bal *gtypes.BalanceObject) {
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

func insertTesseractsInDB(t *testing.T, db Store, tesseracts ...*types.Tesseract) {
	t.Helper()

	mDB := getMockDB(t, db)

	for _, ts := range tesseracts {
		mDB.insertTesseract(t, ts)

		if ts.Interactions() != nil {
			mDB.insertIxns(t, ts.GridHash(), ts.Interactions())
		}
	}
}

func insertAccountsInDB(t *testing.T, db Store, hashes []types.Hash, acc ...*types.Account) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, hash := range hashes {
		mDB.setAccount(t, hash, acc[i])
	}
}

func insertBalancesInDB(t *testing.T, db Store, hashes []types.Hash, balances ...*gtypes.BalanceObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for i, bal := range balances {
		mDB.setBalance(t, hashes[i], bal)
	}
}

func insertContextsInDB(t *testing.T, db Store, context ...*gtypes.ContextObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for _, ctx := range context {
		mDB.setContext(t, ctx)
	}
}

func insertMetaContextsInDB(t *testing.T, db Store, context ...*gtypes.MetaContextObject) {
	t.Helper()

	mDB := getMockDB(t, db)

	for _, ctx := range context {
		mDB.setMetaContext(t, ctx)
	}
}

type MockMerkleTree struct {
	dbStorage    map[string][]byte
	dirty        map[string][]byte
	merkleRoot   types.Hash
	isFlushed    bool
	flushHook    func() error
	rootHashHook func() (types.Hash, error)
	commitHook   func() error
	setHook      func() error
}

func (m *MockMerkleTree) Root() types.RootNode {
	// TODO implement me
	panic("implement me")
}

func mockMerkleTreeWithDB() *MockMerkleTree {
	return &MockMerkleTree{
		dirty:     make(map[string][]byte),
		dbStorage: make(map[string][]byte),
	}
}

func (m *MockMerkleTree) RootHash() (types.Hash, error) {
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

	m.merkleRoot = types.BytesToHash(bytes)

	return nil
}

func (m *MockMerkleTree) NewIterator() smt.Iterator {
	// TODO implement me
	panic("implement me")
}

func (m *MockMerkleTree) GetPreImageKey(hashKey types.Hash) ([]byte, error) {
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

	m.dirty[string(key)] = value

	return nil
}

func (m *MockMerkleTree) Delete(key []byte) error {
	if exists, _ := m.Has(key); exists {
		delete(m.dbStorage, string(key))

		return nil
	}

	return types.ErrKeyNotFound
}

func (m *MockMerkleTree) Get(key []byte) ([]byte, error) {
	if exists, _ := m.Has(key); exists {
		val := m.dirty[string(key)]

		return val, nil
	}

	return nil, types.ErrKeyNotFound
}

func (m *MockMerkleTree) Has(key []byte) (bool, error) {
	_, ok := m.dirty[string(key)]

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
	rootHashHook func() (types.Hash, error),
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

func getRoot(t *testing.T, m *MockMerkleTree) types.Hash {
	t.Helper()

	bytes, err := polo.Polorize(m.dbStorage)
	require.NoError(t, err)

	return types.BytesToHash(bytes)
}

type MockSenatus struct {
	lock       sync.RWMutex
	publicKeys map[id.KramaID][]byte
}

func mockSenatus(t *testing.T) *MockSenatus {
	t.Helper()

	return &MockSenatus{
		lock:       sync.RWMutex{},
		publicKeys: make(map[id.KramaID][]byte),
	}
}

func (m *MockSenatus) AddPublicKeys(ids []id.KramaID, publicKeys [][]byte) error {
	for index, kramaID := range ids {
		m.publicKeys[kramaID] = publicKeys[index]
	}

	return nil
}

func (m *MockSenatus) UpdatePublicKey(key id.KramaID, pk []byte) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.publicKeys[key] = pk

	return nil
}

func (m *MockSenatus) GetPublicKey(kramaID id.KramaID) ([]byte, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	val, ok := m.publicKeys[kramaID]
	if !ok {
		return nil, types.ErrKramaIDNotFound
	}

	return val, nil
}

type MockContract struct {
	publicKeys map[id.KramaID][]byte
}

func NewMockContract(t *testing.T, kramaIDs []id.KramaID, publicKeys [][]byte) *MockContract {
	t.Helper()

	mockContract := new(MockContract)
	mockContract.publicKeys = make(map[id.KramaID][]byte)

	for i := 0; i < len(kramaIDs); i++ {
		mockContract.publicKeys[kramaIDs[i]] = publicKeys[i]
	}

	return mockContract
}

type createLogicObjectParams struct {
	id            types.LogicID
	logicCallback func(object *gtypes.LogicObject)
}

func createLogicObject(t *testing.T, params *createLogicObjectParams) *gtypes.LogicObject {
	t.Helper()

	if params == nil {
		params = &createLogicObjectParams{}
	}

	logicObject := &gtypes.LogicObject{ID: params.id}
	if params.logicCallback != nil {
		params.logicCallback(logicObject)
	}

	return logicObject
}

func getLogicObjectParamsWithLogicID(logicID types.LogicID) *createLogicObjectParams {
	return &createLogicObjectParams{
		id: logicID,
		logicCallback: func(object *gtypes.LogicObject) {
			object.Dependencies = engineio.NewDependencyGraph()
		},
	}
}

func getTesseractParamsWithStateHash(address types.Address, hash types.Hash) *tests.CreateTesseractParams {
	return &tests.CreateTesseractParams{
		Address: address,
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = hash
		},
	}
}

func getTesseractParamsWithContextHash(address types.Address, hash types.Hash) *tests.CreateTesseractParams {
	return &tests.CreateTesseractParams{
		Address: address,
		BodyCallback: func(body *types.TesseractBody) {
			body.ContextHash = hash
		},
	}
}

func getTesseractHash(t *testing.T, ts *types.Tesseract) types.Hash {
	t.Helper()

	hash := ts.Hash()

	return hash
}

func storeTesseractHashInCache(t *testing.T, cache *lru.Cache, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		cache.Add(ts.Address(), ts.Hash())
	}
}

func insertInContextLock(header *types.TesseractHeader, address types.Address, hash types.Hash) {
	header.ContextLock[address] = types.ContextLockInfo{
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
		context.Background(),
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

func insertContextHash(so *StateObject, hash types.Hash) {
	so.data.ContextHash = hash
}

func insertDirtyObject(sm *StateManager, objects ...*StateObject) {
	for _, obj := range objects {
		sm.dirtyObjects[obj.address] = obj
	}
}

func storeInSmCache(sm *StateManager, k, v interface{}) {
	sm.cache.Add(k, v)
}

type createStateObjectParams struct {
	address                   types.Address
	cache                     *lru.Cache
	journal                   *Journal
	db                        *MockDB
	account                   *types.Account
	soCallback                func(so *StateObject)
	metaStorageTreeCallback   func(so *StateObject)
	dbCallback                func(db *MockDB)
	activeStorageTreeCallback func(activeStorageTrees map[string]tree.MerkleTree)
}

func createTestStateObject(t *testing.T, params *createStateObjectParams) *StateObject {
	t.Helper()

	var (
		addr  = tests.RandomAddress(t)
		mDB   = mockDB()
		cache = mockCache(t)
		data  = new(types.Account)
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

func createTestStateObjects(t *testing.T, count int, paramsMap map[int]*createStateObjectParams) []*StateObject {
	t.Helper()

	objects := make([]*StateObject, count)

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
	balanceHash types.Hash,
	balance *gtypes.BalanceObject,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *StateObject) {
			so.data.Balance = balanceHash
			so.balance = balance
		},
	}
}

func stateObjectParamsWithRegistry(
	t *testing.T,
	registryHash types.Hash,
	registry *gtypes.RegistryObject,
) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *StateObject) {
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
		soCallback: func(so *StateObject) {
			so.activeStorageTrees = ast
			so.metaStorageTree = mst
		},
	}
}

func stateObjectParamsWithAST(t *testing.T, ast map[string]tree.MerkleTree) *createStateObjectParams {
	t.Helper()

	return &createStateObjectParams{
		soCallback: func(so *StateObject) {
			so.activeStorageTrees = ast
		},
	}
}

func stateObjectParamsWithInvalidMST(t *testing.T) *createStateObjectParams {
	t.Helper()

	return stateObjectParamsWithMST(t, types.NilAddress, nil, nil, tests.RandomHash(t))
}

func stateObjectParamsWithMST(
	t *testing.T,
	address types.Address,
	db Store,
	mst tree.MerkleTree,
	root types.Hash,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		address: address,
		db:      mDB,
		soCallback: func(so *StateObject) {
			so.metaStorageTree = mst
			so.data.StorageRoot = root
		},
	}
}

func stateObjectParamsWithLogicTree(
	t *testing.T,
	address types.Address,
	db Store,
	logicTree tree.MerkleTree,
	root types.Hash,
) *createStateObjectParams {
	t.Helper()

	mDB := getMockDB(t, db)

	return &createStateObjectParams{
		address: address,
		db:      mDB,
		soCallback: func(so *StateObject) {
			so.logicTree = logicTree
			so.data.LogicRoot = root // set logic root as it needs to be returned
		},
	}
}

// stateObjectParamsWithMetaContextObject stores object in db if inDB is true else stores in cache
func stateObjectParamsWithMetaContextObject(
	t *testing.T,
	obj *gtypes.MetaContextObject,
	hash types.Hash,
	addToDB bool,
) *createStateObjectParams {
	t.Helper()

	if addToDB {
		return &createStateObjectParams{
			soCallback: func(so *StateObject) {
				insertContextHash(so, hash)
				mDB := getMockDB(t, so.db)

				rawData, err := obj.Bytes()
				require.NoError(t, err)

				mDB.context[hash] = rawData
			},
		}
	}

	return &createStateObjectParams{
		soCallback: func(so *StateObject) {
			so.data.ContextHash = hash
			so.cache.Add(so.ContextHash(), obj)
		},
	}
}

// stateObjectParamsWithContextObject stores object in db if inDB is true else stores in cache
func stateObjectParamsWithContextObject(
	t *testing.T,
	obj *gtypes.ContextObject,
	hash types.Hash,
	addToDB bool,
) *createStateObjectParams {
	t.Helper()

	if addToDB {
		return &createStateObjectParams{
			soCallback: func(so *StateObject) {
				mDB, ok := so.db.(*MockDB)
				require.True(t, ok)

				rawData, err := obj.Bytes()
				require.NoError(t, err)

				mDB.context[hash] = rawData
			},
		}
	}

	return &createStateObjectParams{
		soCallback: func(so *StateObject) {
			so.cache.Add(hash, obj)
		},
	}
}

func stateObjectParamsWithTestData(t *testing.T, areTreesNil bool) *createStateObjectParams {
	t.Helper()

	keys, values := getEntries(t, 2)

	return &createStateObjectParams{
		soCallback: func(s *StateObject) {
			s.accType = types.LogicAccount
			acc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
				acc.ContextHash = tests.RandomHash(t)
			})

			s.data = *acc
			s.balance, _ = getTestBalance(t, getAssetMap(getAssetIDsAndBalances(t, 2)))
			s.approvals.PrvHash = tests.RandomHash(t) // initialize any one field in asset approvals object

			s.logicTree = getMerkleTreeWithFlushedEntries(t, keys[0:1], values[0:1])

			s.files = make(map[types.Hash][]byte)
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
		metaStorageTreeCallback: func(s *StateObject) {
			s.metaStorageTree = getMerkleTreeWithFlushedEntries(t, keys[1:], values[1:])
			if areTreesNil {
				s.metaStorageTree = nil
			}
		},
	}
}

func checkForReferences(t *testing.T, sObj, copiedSO *StateObject) {
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

func getAssetIDsAndBalances(t *testing.T, count int) ([]types.AssetID, []*big.Int) {
	t.Helper()

	ids := make([]types.AssetID, count)
	for i := 0; i < count; i++ {
		ids[i] = tests.GetRandomAssetID(t, tests.RandomAddress(t))
	}

	return ids, tests.GetRandomNumbers(t, 10000, count)
}

func getAssetMap(assetIDs []types.AssetID, balances []*big.Int) types.AssetMap {
	assetMap := make(types.AssetMap)

	for i := 0; i < len(assetIDs); i++ {
		assetMap[assetIDs[i]] = balances[i]
	}

	return assetMap
}

func getAssetMaps(assetIDs []types.AssetID, balances []*big.Int, assetsPerAssetMap int) []types.AssetMap {
	assetMapCount := len(assetIDs) / assetsPerAssetMap
	assetMap := make([]types.AssetMap, assetMapCount)

	for i := 0; i < assetMapCount; i++ {
		assetMap[i] = getAssetMap(
			assetIDs[i*assetsPerAssetMap:i*assetsPerAssetMap+assetsPerAssetMap],
			balances[i*assetsPerAssetMap:i*assetsPerAssetMap+assetsPerAssetMap],
		)
	}

	return assetMap
}

func getTestRegistryObject(t *testing.T, entries map[string][]byte) (*gtypes.RegistryObject, types.Hash) {
	t.Helper()

	registry := &gtypes.RegistryObject{
		Entries: entries,
	}

	data, err := registry.Bytes()
	require.NoError(t, err)

	return registry, types.GetHash(data)
}

func getTestBalance(t *testing.T, assetMap types.AssetMap) (*gtypes.BalanceObject, types.Hash) {
	t.Helper()

	balance := &gtypes.BalanceObject{
		AssetMap: assetMap,
		PrvHash:  tests.RandomHash(t),
	}

	data, err := balance.Bytes()
	require.NoError(t, err)

	return balance, types.GetHash(data)
}

func getTestBalances(t *testing.T, assetMap []types.AssetMap, count int) ([]*gtypes.BalanceObject, []types.Hash) {
	t.Helper()

	balances := make([]*gtypes.BalanceObject, count)
	hashes := make([]types.Hash, count)

	for i := 0; i < count; i++ {
		balances[i], hashes[i] = getTestBalance(t, assetMap[i])
	}

	return balances, hashes
}

func getTestAccounts(t *testing.T, balanceHash []types.Hash, count int) ([]*types.Account, []types.Hash) {
	t.Helper()

	accounts := make([]*types.Account, count)
	hashes := make([]types.Hash, count)

	for i := 0; i < count; i++ {
		acc, stateHash := tests.GetTestAccount(t, func(acc *types.Account) {
			acc.Balance = balanceHash[i]
		})

		accounts[i] = acc
		hashes[i] = stateHash
	}

	return accounts, hashes
}

func getAccMetaInfos(t *testing.T, count int) []*types.AccountMetaInfo {
	t.Helper()

	accMetaInfo := make([]*types.AccountMetaInfo, count)
	for i := 0; i < count; i++ {
		accMetaInfo[i] = tests.GetRandomAccMetaInfo(t, 8)
	}

	return accMetaInfo
}

func getContextObjects(
	t *testing.T,
	ids []id.KramaID,
	idsPerObj int,
	objCount int,
) ([]*gtypes.ContextObject, []types.Hash) {
	t.Helper()

	obj := make([]*gtypes.ContextObject, objCount)
	hashes := make([]types.Hash, objCount)

	for i := 0; i < objCount; i++ {
		copiedIds := make([]id.KramaID, idsPerObj)

		copy(copiedIds, ids[i*idsPerObj:i*idsPerObj+idsPerObj])

		obj[i] = &gtypes.ContextObject{
			Ids: copiedIds,
		}

		hash, err := obj[i].Hash()
		require.NoError(t, err)

		hashes[i] = hash
	}

	return obj, hashes
}

func getMetaContextObject(
	t *testing.T,
	behHash types.Hash,
	randHash types.Hash,
) (*gtypes.MetaContextObject, types.Hash) {
	t.Helper()

	mCtx := &gtypes.MetaContextObject{
		BehaviouralContext: behHash,
		RandomContext:      randHash,
	}

	hash, err := mCtx.Hash()
	require.NoError(t, err)

	return mCtx, hash
}

func getMetaContextObjects(t *testing.T, hashes []types.Hash) ([]*gtypes.MetaContextObject, []types.Hash) {
	t.Helper()

	count := len(hashes)
	mObj := make([]*gtypes.MetaContextObject, count/2)
	mHashes := make([]types.Hash, count/2)

	j := 0

	for i := 0; i < count; i += 2 {
		mObj[j], mHashes[j] = getMetaContextObject(t, hashes[i], hashes[i+1])
		j += 1
	}

	return mObj, mHashes
}

type ICSNodes struct {
	senderBeh    *types.NodeSet
	senderRand   *types.NodeSet
	receiverBeh  *types.NodeSet
	receiverRand *types.NodeSet
}

func getICSNodes(
	senderBeh *types.NodeSet,
	senderRand *types.NodeSet,
	receiverBeh *types.NodeSet,
	receiverRand *types.NodeSet,
) *ICSNodes {
	return &ICSNodes{
		senderBeh:    senderBeh,
		senderRand:   senderRand,
		receiverBeh:  receiverBeh,
		receiverRand: receiverRand,
	}
}

func mockContextLock() map[types.Address]types.ContextLockInfo {
	return make(map[types.Address]types.ContextLockInfo)
}

func getTestAssetDescriptor(t *testing.T, owner types.Address, symbol string) *types.AssetDescriptor {
	t.Helper()

	return &types.AssetDescriptor{
		Type:       types.AssetKindLogic,
		Symbol:     symbol,
		Owner:      owner,
		Supply:     big.NewInt(10000),
		Dimension:  4,
		IsStateFul: false,
		IsLogical:  false,
		LogicID:    getLogicID(t, tests.RandomAddress(t)),
	}
}

func createMetaStorageTree(
	t *testing.T,
	db Store,
	address types.Address,
	logicID types.LogicID,
	storageKeys [][]byte,
	storageValues [][]byte,
) (*tree.KramaHashTree, types.Hash) {
	t.Helper()

	_, storageRoot := createTestKramaHashTree(t, db, address, dhruva.Storage, storageKeys, storageValues)

	return createTestKramaHashTree(
		t,
		db,
		address,
		dhruva.Storage,
		[][]byte{logicID.Bytes()},
		[][]byte{storageRoot.Bytes()})
}

func createTestKramaHashTree(
	t *testing.T,
	db Store,
	address types.Address,
	prefix dhruva.Prefix,
	keys [][]byte,
	values [][]byte,
) (*tree.KramaHashTree, types.Hash) {
	t.Helper()

	kt, err := tree.NewKramaHashTree(address, types.NilHash, db, blakeHasher, prefix)
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

	logicIds := getLogicIDs(t, treeCount)
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
	logicIds []types.LogicID,
	keys [][]byte,
	values [][]byte,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i].String()] = getMerkleTreeWithEntries(t, keys, values)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithFlushedEntries(
	t *testing.T,
	logicIds []types.LogicID,
	keys [][]byte,
	values [][]byte,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i].String()] = getMerkleTreeWithFlushedEntries(t, keys, values)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithCommitHook(
	t *testing.T,
	logicIds []types.LogicID,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i].String()] = getMerkleTreeWithCommitHook(t, keys, values, commitHook)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithFlushHook(
	t *testing.T,
	logicIds []types.LogicID,
	keys [][]byte,
	values [][]byte,
	commitHook func() error,
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i].String()] = getMerkleTreeWithFlushHook(t, keys, values, commitHook)
	}

	return activeStorageTrees
}

func getActiveStorageTreesWithRootHook(
	t *testing.T,
	logicIds []types.LogicID,
	keys [][]byte,
	values [][]byte,
	rootHashHook func() (types.Hash, error),
) map[string]tree.MerkleTree {
	t.Helper()

	count := len(logicIds)
	activeStorageTrees := make(map[string]tree.MerkleTree, count)

	for i := 0; i < count; i++ {
		activeStorageTrees[logicIds[i].String()] = getMerkleTreeWithRootHashHook(t, keys, values, rootHashHook)
	}

	return activeStorageTrees
}

func getContextObjFromCache(t *testing.T, so *StateObject, hash types.Hash) *gtypes.ContextObject {
	t.Helper()

	bCtxData, ok := so.cache.Get(hash)
	require.True(t, ok)
	ctx, ok := bCtxData.(*gtypes.ContextObject)
	require.True(t, ok)

	return ctx
}

func getMetaContextObjFromCache(t *testing.T, so *StateObject, hash types.Hash) *gtypes.MetaContextObject {
	t.Helper()

	bCtxData, ok := so.cache.Get(hash)
	require.True(t, ok)
	ctx, ok := bCtxData.(*gtypes.MetaContextObject)
	require.True(t, ok)

	return ctx
}

func getMetaContextObjectFromDirtyEntries(t *testing.T, s *StateObject, hash types.Hash) *gtypes.MetaContextObject {
	t.Helper()

	key := types.BytesToHex(dhruva.ContextObjectKey(s.address, hash))
	rawData, err := s.GetDirtyEntry(key)
	require.NoError(t, err)

	obj := new(gtypes.MetaContextObject)
	err = obj.FromBytes(rawData)
	require.NoError(t, err)

	return obj
}

func getContextObjectFromDirtyEntries(t *testing.T, s *StateObject, hash types.Hash) *gtypes.ContextObject {
	t.Helper()

	key := types.BytesToHex(dhruva.ContextObjectKey(s.address, hash))
	rawData, err := s.GetDirtyEntry(key)
	require.NoError(t, err)

	obj := new(gtypes.ContextObject)
	err = obj.FromBytes(rawData)
	require.NoError(t, err)

	return obj
}

func checkForTesseractInSMCache(t *testing.T, sm *StateManager, ts *types.Tesseract, withInteractions bool) {
	t.Helper()

	object, isCached := sm.cache.Get(ts.Hash())
	if withInteractions {
		require.False(t, isCached) // make sure tesseract not cached

		return
	}

	require.True(t, isCached) // make sure tesseract cached, if fetched without interactions

	cachedTS, ok := object.(*types.Tesseract)
	require.True(t, ok)

	require.Equal(t, ts, cachedTS) // make sure cached tesseract matches
}

func checkForDirtyObject(t *testing.T, sm *StateManager, address types.Address, exists bool) {
	t.Helper()

	_, ok := sm.dirtyObjects[address]
	require.Equal(t, exists, ok)
}

func checkIfDirtyObjectEqual(t *testing.T, sm *StateManager, address types.Address, expectedObj *StateObject) {
	t.Helper()

	obj, ok := sm.dirtyObjects[address]
	require.True(t, ok)
	require.Equal(t, expectedObj, obj)
}

func checkForCache(t *testing.T, sm *StateManager, address types.Address) {
	t.Helper()

	_, isCached := sm.cache.Get(address) // check if added to cache
	require.True(t, isCached)
}

func checkIfContextMatches(
	t *testing.T,
	expectedBeh *gtypes.ContextObject,
	expectedRand *gtypes.ContextObject,
	beh []id.KramaID,
	rand []id.KramaID,
) {
	t.Helper()

	require.Equal(t, expectedBeh.Ids, beh)
	require.Equal(t, expectedRand.Ids, rand)
}

func checkIfNodesetEqual(
	t *testing.T,
	expectedBeh *types.NodeSet,
	expectedRand *types.NodeSet,
	beh *types.NodeSet,
	rand *types.NodeSet,
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

func validateStateObject(t *testing.T, so *StateObject, accType types.AccountType, address types.Address) {
	t.Helper()

	require.Equal(t, address, so.address)
	require.Equal(t, accType, so.accType)
	require.NotNil(t, so.journal)
	require.NotNil(t, so.db)
	require.NotNil(t, so.data)
	require.NotNil(t, so.cache)
}

func checkForStateObject(t *testing.T, expectedObj *StateObject, obj *StateObject) {
	t.Helper()

	require.Equal(t, expectedObj.balance, obj.balance)
	require.Equal(t, expectedObj.data, obj.data)
	require.Equal(t, expectedObj.address, obj.address)
	require.Equal(t, expectedObj.accType, obj.accType)
}

func checkIfStateObjectAreEqual(
	t *testing.T,
	expectedObj *StateObject,
	actualObj *StateObject,
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

func checkForBalances(t *testing.T, sObj *StateObject, expectedBalance *big.Int, assetID types.AssetID) {
	t.Helper()

	actualBalance, err := sObj.BalanceOf(assetID)
	require.NoError(t, err)
	require.Equal(t, expectedBalance, actualBalance)
}

func checkForRegistry(
	t *testing.T,
	sObj *StateObject,
	expectedRegistry *gtypes.RegistryObject,
	actualRegistryHash types.Hash,
) {
	t.Helper()

	expectedRegistryData, err := expectedRegistry.Bytes()
	require.NoError(t, err)

	expectedRegistryHash := types.GetHash(expectedRegistryData)
	require.Equal(t, expectedRegistryHash, actualRegistryHash)

	// check if registry data in dirty entries and state object is same
	key := types.BytesToHex(dhruva.RegistryObjectKey(sObj.address, expectedRegistryHash))
	actualRegistryData, err := sObj.GetDirtyEntry(key) // get registry data from dirty entries
	require.NoError(t, err)

	require.Equal(t, expectedRegistryData, actualRegistryData)

	require.Equal(t, sObj.data.AssetRegistry, actualRegistryHash) // check if registry hash inserted in account
}

func checkForBalance(
	t *testing.T,
	sObj *StateObject,
	expectedBalance *gtypes.BalanceObject,
	actualBalanceHash types.Hash,
	journalIndex int,
) {
	t.Helper()

	expectedBalanceData, err := expectedBalance.Bytes()
	require.NoError(t, err)

	expectedBalanceHash := types.GetHash(expectedBalanceData)
	require.Equal(t, expectedBalanceHash, actualBalanceHash)

	// check if balance data in dirty entries and state object is same
	key := types.BytesToHex(dhruva.BalanceObjectKey(sObj.address, expectedBalanceHash))
	actualBalanceData, err := sObj.GetDirtyEntry(key) // get balance data from dirty entries
	require.NoError(t, err)

	require.Equal(t, expectedBalanceData, actualBalanceData)

	require.Equal(t, sObj.data.Balance.Bytes(), actualBalanceHash.Bytes()) // check if balance hash inserted in account

	// check if address and balance hash inserted in journal
	checkJournalEntries(t, sObj.Journal(), sObj.address, journalIndex, actualBalanceHash)
}

func checkForAccount(
	t *testing.T,
	sObj *StateObject,
	expectedAcc *types.Account,
	actualAccHash types.Hash,
	journalIndex int,
) {
	t.Helper()

	expectedAccData, err := expectedAcc.Bytes()
	require.NoError(t, err)

	expectedAccHash := types.GetHash(expectedAccData)
	require.Equal(t, expectedAccHash, actualAccHash)

	// check if account data in dirty entries and state object is same
	key := types.BytesToHex(dhruva.AccountKey(sObj.address, expectedAccHash))
	actualAccData, err := sObj.GetDirtyEntry(key) // get account data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedAccData, actualAccData)

	// check if address and acc hash inserted in journal
	checkJournalEntries(t, sObj.Journal(), sObj.Address(), journalIndex, actualAccHash)
}

func checkJournalEntries(t *testing.T, journal *Journal, addr types.Address, journalIndex int, hash types.Hash) {
	t.Helper()

	require.Equal(t, hash.Bytes(), journal.entries[journalIndex].cID().Bytes())
	require.Equal(t, addr, *journal.entries[journalIndex].modifiedAddress())
}

func checkForContextObject(
	t *testing.T,
	sObj *StateObject,
	ctxObject gtypes.ContextObject,
	actualCtxHash types.Hash,
) {
	t.Helper()

	expectedObjData, err := ctxObject.Bytes()
	require.NoError(t, err)

	expectedCtxHash := types.GetHash(expectedObjData)
	require.Equal(t, expectedCtxHash, actualCtxHash)

	// check if ctxObject data in dirty entries matches
	key := types.BytesToHex(dhruva.ContextObjectKey(sObj.address, expectedCtxHash))
	actualObjData, err := sObj.GetDirtyEntry(key) // get ctx object data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedObjData, actualObjData)
}

func checkForMetaContextObject(
	t *testing.T,
	sObj *StateObject,
	ctxObject gtypes.MetaContextObject,
	actualCtxHash types.Hash,
) {
	t.Helper()

	expectedObjData, err := ctxObject.Bytes()
	require.NoError(t, err)

	expectedCtxHash := types.GetHash(expectedObjData)
	require.Equal(t, expectedCtxHash, actualCtxHash)

	// check if ctxObject data in dirty entries matches
	key := types.BytesToHex(dhruva.ContextObjectKey(sObj.address, expectedCtxHash))
	actualObjData, err := sObj.GetDirtyEntry(key) // get ctx object data from dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedObjData, actualObjData)
}

func checkIfActiveStorageTreesAreCommitted(
	t *testing.T,
	inputAST map[string]tree.MerkleTree,
	sObj *StateObject,
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
		actualRoot, err := sObj.metaStorageTree.Get(types.FromHex(logicID))
		require.NoError(t, err)

		require.Equal(t, expectedRoot.Bytes(), actualRoot)
	}
}

func checkIfMetaStorageTreeCommitted(
	t *testing.T,
	inputMST tree.MerkleTree,
	sObj *StateObject,
	actualRoot types.Hash,
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
	sObj *StateObject,
	actualRoot types.Hash,
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

func checkIfActiveStorageTreesFlushed(t *testing.T, logicIDs []types.LogicID, s *StateObject, isFlushed bool) {
	t.Helper()

	for _, logicID := range logicIDs {
		storageTree, ok := s.activeStorageTrees[logicID.String()]
		require.True(t, ok)

		checkIfMerkleTreeFlushed(t, storageTree, isFlushed)
	}
}

func checkForContextUpdate(
	t *testing.T,
	sObj *StateObject,
	cObj []*gtypes.ContextObject,
	metaHash types.Hash,
	behaviouralNodes []id.KramaID,
	randomNodes []id.KramaID,
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

func checkForEntryInMST(t *testing.T, s *StateObject, key []byte, value []byte) {
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

func validateTesseract(t *testing.T, ts *types.Tesseract, expectedTS *types.Tesseract, withInteractions bool) {
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

func getLogicID(t *testing.T, address types.Address) types.LogicID {
	t.Helper()

	return types.NewLogicIDv0(true, false, false, false, 0, address)
}

func getLogicIDs(t *testing.T, count int) []types.LogicID {
	t.Helper()

	logicIDs := make([]types.LogicID, count)

	for i := 0; i < count; i++ {
		logicIDs[i] = getLogicID(t, tests.RandomAddress(t))
	}

	return logicIDs
}

func getAddresses(t *testing.T, count int) []types.Address {
	t.Helper()

	addresses := make([]types.Address, count)
	for i := 0; i < count; i++ {
		addresses[i] = tests.RandomAddress(t)
	}

	return addresses
}

func getHashes(t *testing.T, count int) []types.Hash {
	t.Helper()

	hashes := make([]types.Hash, count)
	for i := 0; i < count; i++ {
		hashes[i] = tests.RandomHash(t)
	}

	return hashes
}

func getDirtyEntries(t *testing.T, count int) Storage {
	t.Helper()

	d := make(Storage, count)

	for i := 0; i < count; i++ {
		d[tests.RandomHash(t).Hex()] = tests.RandomAddress(t).Bytes()
	}

	return d
}

func CheckAssetCreation(
	t *testing.T,
	s *StateObject,
	assetDescriptor *types.AssetDescriptor,
	assetID types.AssetID,
) {
	t.Helper()

	actualRegistryData, err := s.GetRegistryEntry(assetID.String())
	require.NoError(t, err)

	expectedRegistryData, err := assetDescriptor.Bytes()
	require.NoError(t, err)

	require.Equal(t, actualRegistryData, expectedRegistryData)
	require.Equal(t, s.Address(), assetDescriptor.Owner) // check if address is assigned to owner
}

func getTestAssetID(addr types.Address, asset *types.AssetDescriptor) types.AssetID {
	types.NewAssetIDv0(asset.IsLogical, asset.IsStateFul, asset.Dimension, asset.Standard, addr)

	return types.NewAssetIDv0(asset.IsLogical, asset.IsStateFul, asset.Dimension, asset.Standard, addr)
}
