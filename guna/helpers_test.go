package guna

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	ktypes "github.com/sarvalabs/moichain/krama/types"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
	"github.com/munna0908/smt"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	DhruvaDB "github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/guna/tree"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/require"
)

type MockServer struct{}

func (m *MockServer) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	// TODO implement me
	panic("implement me")
}

func mockServer() *MockServer {
	return new(MockServer)
}

type MockDB struct {
	tesseracts          map[types.Hash][]byte
	latestTesseractHash map[types.Address]types.Hash
	accounts            map[types.Hash][]byte
	context             map[types.Hash][]byte
	interactions        map[types.Hash][]byte
	balances            map[types.Hash][]byte
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

func (m *MockDB) NewBatchWriter() DhruvaDB.BatchWriter {
	return nil
}

func (m *MockDB) GetEntries(prefix []byte) chan types.DBEntry {
	return nil
}

func (m *MockDB) GetAccountMetaInfo(id []byte) (*types.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[types.BytesToHex(id)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (m *MockDB) insertTesseract(t *testing.T, ts *types.Tesseract) {
	t.Helper()

	bytes, err := ts.Canonical().Bytes()
	require.NoError(t, err)
	hash, err := ts.Hash()
	require.NoError(t, err)

	m.tesseracts[hash] = bytes
}

func (m *MockDB) insertIxns(t *testing.T, hash types.Hash, ixns types.Interactions) {
	t.Helper()

	bytes, err := ixns.Bytes()
	require.NoError(t, err)

	m.interactions[hash] = bytes
}

func (m *MockDB) setAccountMetaInfo(acc *types.AccountMetaInfo) {
	m.accMetaInfo[types.BytesToHex(acc.Address[:])] = acc
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
	if ok {
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

type CreateTesseractParams struct {
	address           types.Address
	height            uint64
	ixns              types.Interactions
	tesseractCallback func(ts *types.Tesseract)
}

func createTesseracts(t *testing.T, count int, paramsMap map[int]*CreateTesseractParams) []*types.Tesseract {
	t.Helper()

	tesseracts := make([]*types.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*CreateTesseractParams{}
	}

	for i := 0; i < count; i++ {
		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func mockCache(t *testing.T) *lru.Cache {
	t.Helper()

	cache, err := lru.New(1200)
	require.NoError(t, err)

	return cache
}

func createTesseract(t *testing.T, params *CreateTesseractParams) *types.Tesseract {
	t.Helper()

	if params == nil {
		params = &CreateTesseractParams{}
	}

	if params.address.IsNil() {
		params.address = tests.RandomAddress(t)
	}

	ts := &types.Tesseract{
		Header: types.TesseractHeader{
			Address: params.address,
			Height:  params.height,
		},
		Body: types.TesseractBody{},
		Ixns: params.ixns,
	}

	if params.ixns != nil {
		hash, err := params.ixns.Hash()
		require.NoError(t, err)

		ts.Body.InteractionHash = hash
	}

	if params.tesseractCallback != nil {
		params.tesseractCallback(ts)
	}

	return ts
}

func getTesseractParamsWithStateHash(address types.Address, hash types.Hash) *CreateTesseractParams {
	return &CreateTesseractParams{
		address: address,
		tesseractCallback: func(ts *types.Tesseract) {
			insertStateHash(ts, hash)
		},
	}
}

func getTesseractParamsWithContextHash(address types.Address, hash types.Hash) *CreateTesseractParams {
	return &CreateTesseractParams{
		address: address,
		tesseractCallback: func(ts *types.Tesseract) {
			ts.Body.ContextHash = hash
		},
	}
}

func getTesseractParamsMapWithIxns(t *testing.T, tsCount int) map[int]*CreateTesseractParams {
	t.Helper()

	tesseractParams := make(map[int]*CreateTesseractParams, tsCount)
	addresses := getAddresses(t, 4*tsCount) // for each interaction, sender and receiver addresses needed
	ixns := createIxns(t, 2*tsCount, getIxParamsMapWithAddresses(addresses[:2*tsCount], addresses[2*tsCount:]))

	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &CreateTesseractParams{
			ixns: ixns[i*2 : i*2+2], // allocate two interactions per tesseract
		}
	}

	return tesseractParams
}

type CreateStateManagerParams struct {
	db              *MockDB
	dbCallback      func(db *MockDB)
	serverCallback  func(n *MockServer)
	senatusCallBack func(sm *MockSenatus)
	smCallBack      func(sm *StateManager)
}

func createTestStateManager(t *testing.T, params *CreateStateManagerParams) *StateManager {
	t.Helper()

	var (
		db      = mockDB()
		server  = mockServer()
		senatus = mockSenatus(t)
	)

	if params == nil {
		params = &CreateStateManagerParams{}
	}

	if params.db != nil {
		db = params.db
	}

	if params.dbCallback != nil {
		params.dbCallback(db)
	}

	if params.serverCallback != nil {
		params.serverCallback(server)
	}

	sm, err := NewStateManager(context.Background(), db, hclog.NewNullLogger(), mockCache(t), server, NilMetrics())
	require.NoError(t, err)

	if params.smCallBack != nil {
		params.smCallBack(sm)
	}

	if params.senatusCallBack != nil {
		sm.senatus = senatus
		params.senatusCallBack(senatus) // senatus called after instantiating state manager inorder to override senatus
	}

	return sm
}

type CreateStateObjectParams struct {
	address                   types.Address
	cache                     *lru.Cache
	journal                   *Journal
	db                        *MockDB
	account                   types.Account
	accType                   types.AccountType
	soCallback                func(so *StateObject)
	metaStorageTreeCallback   func(so *StateObject)
	dbCallback                func(db *MockDB)
	activeStorageTreeCallback func(activeStorageTrees map[string]tree.MerkleTree)
}

func createTestStateObject(t *testing.T, params *CreateStateObjectParams) *StateObject {
	t.Helper()

	var (
		addr = tests.RandomAddress(t)
		db   = mockDB()
	)

	if params == nil {
		params = &CreateStateObjectParams{}
	}

	if !params.address.IsNil() {
		addr = params.address
	}

	if params.db != nil {
		db = params.db
	}

	if params.dbCallback != nil {
		params.dbCallback(db)
	}

	so := NewStateObject(addr, params.cache, mockJournal(), db, params.account, params.accType)

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

func createTestStateObjects(t *testing.T, count int, paramsMap map[int]*CreateStateObjectParams) []*StateObject {
	t.Helper()

	objects := make([]*StateObject, count)

	if paramsMap == nil {
		paramsMap = map[int]*CreateStateObjectParams{}
	}

	for i := 0; i < count; i++ {
		objects[i] = createTestStateObject(t, paramsMap[i])
	}

	return objects
}

func mockJournal() *Journal {
	return new(Journal)
}

func getStateObjectParamsWithBalance(
	t *testing.T,
	balanceHash types.Hash,
	balance *gtypes.BalanceObject,
) *CreateStateObjectParams {
	t.Helper()

	return &CreateStateObjectParams{
		soCallback: func(so *StateObject) {
			so.data.Balance = balanceHash
			so.balance = balance
		},
	}
}

func getTestBalances(t *testing.T, count int) ([]*gtypes.BalanceObject, []types.Hash) {
	t.Helper()

	balances := make([]*gtypes.BalanceObject, count)
	hashes := make([]types.Hash, count)

	for i := 0; i < count; i++ {
		balances[i], hashes[i] = getTestBalance(t)
	}

	return balances, hashes
}

func getTestAccounts(t *testing.T, balanceHash []types.Hash, count int) ([]*types.Account, []types.Hash) {
	t.Helper()

	accounts := make([]*types.Account, count)
	hashes := make([]types.Hash, count)

	for i := 0; i < count; i++ {
		acc, stateHash := getTestAccount(t, func(acc *types.Account) {
			acc.Balance = balanceHash[i]
		})

		accounts[i] = acc
		hashes[i] = stateHash
	}

	return accounts, hashes
}

type CreateIxParams struct {
	ixDataCallback func(ix *types.IxData)
}

func createIX(t *testing.T, params *CreateIxParams) *types.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &types.IxData{
		Input: types.IxInput{},
	}

	if params.ixDataCallback != nil {
		params.ixDataCallback(data)
	}

	ix := types.NewInteraction(*data, []byte{})

	return ix
}

func createIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) types.Interactions {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make(types.Interactions, count)

	for i := 0; i < count; i++ {
		ixns[i] = createIX(t, paramsMap[i])
	}

	return ixns
}

func getIxParamsWithAddress(from types.Address, to types.Address) *CreateIxParams {
	return &CreateIxParams{
		ixDataCallback: func(ix *types.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
		},
	}
}

func getIxParamsMapWithAddresses(
	from []types.Address,
	to []types.Address,
) map[int]*CreateIxParams {
	count := len(from)
	ixParams := make(map[int]*CreateIxParams, count)

	for i := 0; i < count; i++ {
		ixParams[i] = getIxParamsWithAddress(from[i], to[i])
	}

	return ixParams
}

type MockMerkleTree struct {
	dbStorage  map[string][]byte
	dirty      map[string][]byte
	merkleRoot types.Hash
	flushHook  func() error
}

func mockMerkleTreeWithDirtyStorage() *MockMerkleTree {
	return &MockMerkleTree{
		dirty: make(map[string][]byte),
	}
}

func mockMerkleTreeWithDB() *MockMerkleTree {
	return &MockMerkleTree{
		dirty:     make(map[string][]byte),
		dbStorage: make(map[string][]byte),
	}
}

func (m *MockMerkleTree) RootHash() (types.Hash, error) {
	return m.merkleRoot, nil
}

func (m *MockMerkleTree) Commit() error {
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
	}

	for k, v := range m.dbStorage {
		merkleTree.dbStorage[k] = v
	}

	return &merkleTree
}

func (m *MockMerkleTree) Set(key []byte, value []byte) error {
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

	for k, v := range m.dirty {
		m.dbStorage[k] = v
	}

	return nil
}

func (m *MockMerkleTree) Close() error {
	return nil
}

type MockSenatus struct {
	publicKeys map[id.KramaID][]byte
	start      map[id.KramaID]bool
}

func mockSenatus(t *testing.T) *MockSenatus {
	t.Helper()

	return &MockSenatus{
		publicKeys: make(map[id.KramaID][]byte),
	}
}

func (m *MockSenatus) AddNewPeer(key id.KramaID, data *ReputationInfo) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) UpdateAddress(key id.KramaID, addrs []string) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) UpdateNTQ(key id.KramaID, ntq int32) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) UpdateInclusivity(key id.KramaID, delta int64) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) GetAddress(key id.KramaID) (multiAddrs []multiaddr.Multiaddr, err error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) GetNTQ(id id.KramaID) (int32, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) UpdatePublicKey(key id.KramaID, pk []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) GetPublicKey(ctx context.Context, id id.KramaID) ([]byte, error) {
	val, ok := m.publicKeys[id]
	if !ok {
		return nil, context.Canceled
	}

	return val, nil
}

func (m *MockSenatus) setPublicKey(id id.KramaID, pk []byte) {
	m.publicKeys[id] = pk
}

func (m *MockSenatus) AddEntries(msg ptypes.SyncReputationInfo) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) GetInclusivity(id id.KramaID) (int64, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) GetAllEntries() (chan *ptypes.SyncReputationInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) SenatusHandler(msg *pubsub.Message) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) HandleHelloMessages(msgs []*ptypes.HelloMsg) (int, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockSenatus) Start(id id.KramaID, ntq int32, publicKey []byte, address []multiaddr.Multiaddr) error {
	m.start[id] = true

	return nil
}

func getAccMetaInfos(t *testing.T, count int) []*types.AccountMetaInfo {
	t.Helper()

	accMetaInfo := make([]*types.AccountMetaInfo, count)
	for i := 0; i < count; i++ {
		accMetaInfo[i] = tests.GetRandomAccMetaInfo(t, 8)
	}

	return accMetaInfo
}

func getTesseractHash(t *testing.T, ts *types.Tesseract) types.Hash {
	t.Helper()

	hash, err := ts.Hash()
	require.NoError(t, err)

	return hash
}

func getTestBalance(t *testing.T) (*gtypes.BalanceObject, types.Hash) {
	t.Helper()

	balance := &gtypes.BalanceObject{
		Balances: types.AssetMap{
			tests.GetRandomAssetID(t, types.NilAddress): big.NewInt(100),
		},
	}

	data, err := balance.Bytes()
	require.NoError(t, err)

	return balance, types.GetHash(data)
}

func getTestAccount(t *testing.T, callBack func(acc *types.Account)) (*types.Account, types.Hash) {
	t.Helper()

	acc := new(types.Account)
	if callBack != nil {
		callBack(acc)
	}

	accHash, err := acc.Hash()
	assert.NoError(t, err)

	return acc, accHash
}

func insertStateObject(sm *StateManager, so *StateObject) {
	sm.objects[so.address] = so
}

func AddAssetInBalance(t *testing.T, so *StateObject) {
	t.Helper()

	so.balance.Balances[tests.GetRandomAssetID(t, types.NilAddress)] = big.NewInt(100)
}

func insertStateHash(ts *types.Tesseract, hash types.Hash) {
	ts.Body.StateHash = hash
}

func storeTesseractHashInCache(t *testing.T, cache *lru.Cache, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		cache.Add(ts.Address(), getTesseractHash(t, ts))
	}
}

func getContextObjects(
	t *testing.T,
	id []id.KramaID,
	idsPerObj int,
	objCount int,
) ([]*gtypes.ContextObject, []types.Hash) {
	t.Helper()

	obj := make([]*gtypes.ContextObject, objCount)
	hashes := make([]types.Hash, objCount)

	for i := 0; i < objCount; i++ {
		obj[i] = &gtypes.ContextObject{
			Ids: id[i*idsPerObj : i*idsPerObj+idsPerObj],
		}

		hash, err := obj[i].Hash()
		require.NoError(t, err)

		hashes[i] = hash
	}

	return obj, hashes
}

func storeInSmCache(sm *StateManager, k, v interface{}) {
	sm.cache.Add(k, v)
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

func setPublicKeys(s *MockSenatus, ids []id.KramaID, pk [][]byte) {
	for i, kramaID := range ids {
		if !(string(kramaID) == "") {
			s.setPublicKey(kramaID, pk[i])
		}
	}
}

func insertObject(sm *StateManager, so *StateObject) {
	sm.objects[so.address] = so
}

func insertDirtyObject(sm *StateManager, objects ...*StateObject) {
	for _, obj := range objects {
		sm.dirtyObjects[obj.address] = obj
	}
}

func insertTesseractsInDB(t *testing.T, db *MockDB, tesseracts ...*types.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		db.insertTesseract(t, ts)

		if ts.Interactions() != nil {
			db.insertIxns(t, ts.InteractionHash(), ts.Interactions())
		}
	}
}

func insertAccountsInDB(t *testing.T, db *MockDB, hashes []types.Hash, acc ...*types.Account) {
	t.Helper()

	for i, hash := range hashes {
		db.setAccount(t, hash, acc[i])
	}
}

func insertBalancesInDB(t *testing.T, db *MockDB, hashes []types.Hash, balances ...*gtypes.BalanceObject) {
	t.Helper()

	for i, bal := range balances {
		db.setBalance(t, hashes[i], bal)
	}
}

func insertContextsInDB(t *testing.T, db *MockDB, context ...*gtypes.ContextObject) {
	t.Helper()

	for _, ctx := range context {
		db.setContext(t, ctx)
	}
}

func insertMetaContextsInDB(t *testing.T, db *MockDB, context ...*gtypes.MetaContextObject) {
	t.Helper()

	for _, ctx := range context {
		db.setMetaContext(t, ctx)
	}
}

type ICSNodes struct {
	senderBeh    *ktypes.NodeSet
	senderRand   *ktypes.NodeSet
	receiverBeh  *ktypes.NodeSet
	receiverRand *ktypes.NodeSet
}

func getICSNodes(
	senderBeh *ktypes.NodeSet,
	senderRand *ktypes.NodeSet,
	receiverBeh *ktypes.NodeSet,
	receiverRand *ktypes.NodeSet,
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

func insertInContextLock(ts *types.Tesseract, address types.Address, hash types.Hash) {
	ts.Header.ContextLock[address] = types.ContextLockInfo{
		ContextHash: hash,
	}
}

func getAddresses(t *testing.T, count int) []types.Address {
	t.Helper()

	addresses := make([]types.Address, count)
	for i := 0; i < count; i++ {
		addresses[i] = tests.RandomAddress(t)
	}

	return addresses
}

func getAccountSetupArgs(
	t *testing.T,
	address types.Address,
	behNodes []id.KramaID,
	randNodes []id.KramaID,
	accType int,
	assetDetails []*types.AssetDescriptor,
	balanceInfo map[types.AssetID]*big.Int,
) *gtypes.AccountSetupArgs {
	t.Helper()

	return &gtypes.AccountSetupArgs{
		Address:            address,
		MoiID:              tests.RandomAddress(t).Hex(),
		BehaviouralContext: behNodes,
		RandomContext:      randNodes,
		AccType:            types.AccountType(accType),
		Assets:             assetDetails,
		Balances:           balanceInfo,
	}
}

func getAsset(dimension int, totalSupply int, symbol string, isFungible bool, isMintable bool) *types.AssetDescriptor {
	return &types.AssetDescriptor{
		Dimension:  uint8(dimension),
		Supply:     big.NewInt(int64(totalSupply)),
		Symbol:     symbol,
		IsFungible: isFungible,
		IsMintable: isMintable,
	}
}

func getDirtyEntries(t *testing.T, count int) Storage {
	t.Helper()

	d := make(Storage, count)

	for i := 0; i < count; i++ {
		d[tests.RandomHash(t).Hex()] = tests.RandomAddress(t).Bytes()
	}

	return d
}

func getEntries(t *testing.T, count int) []string {
	t.Helper()

	entries := make([]string, count)

	for i := 0; i < count; i++ {
		str, err := tests.GetRandomUpperCaseString(t, 10)
		require.NoError(t, err)

		entries[i] = str
	}

	return entries
}

func setEntries(t *testing.T, m *MockMerkleTree, keys, values []string) {
	t.Helper()

	for i := 0; i < len(keys); i += 1 {
		err := m.Set([]byte(keys[i]), []byte(values[i]))
		require.NoError(t, err)
	}
}

func storeInMerkleTree(t *testing.T, m *MockMerkleTree, k, v []byte) {
	t.Helper()

	err := m.Set(k, v)
	require.NoError(t, err)
}

func createMetaStorageTree(
	t *testing.T,
	db *MockDB,
	address types.Address,
	logicID types.LogicID,
	storageKeys [][]byte,
	storageValues [][]byte,
) (*tree.KramaHashTree, types.Hash) {
	t.Helper()

	_, storageRoot := createTestKramaHashTree(t, db, address, storageKeys, storageValues)

	return createTestKramaHashTree(t, db, address, [][]byte{logicID}, [][]byte{storageRoot.Bytes()})
}

func createTestKramaHashTree(
	t *testing.T,
	db *MockDB,
	address types.Address,
	keys [][]byte,
	values [][]byte,
) (*tree.KramaHashTree, types.Hash) {
	t.Helper()

	kt, err := tree.NewKramaHashTree(address, types.NilHash, db, blakeHasher, dhruva.Storage)
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

func validateTesseract(t *testing.T, ts *types.Tesseract, expectedTS *types.Tesseract, withInteractions bool) {
	t.Helper()

	if withInteractions { // check if tesseracts matches
		require.Equal(t, expectedTS, ts)

		return
	}

	require.Equal(t, expectedTS.Canonical(), ts.Canonical())
	require.Equal(t, 0, len(ts.Ixns)) // make sure returned tesseract has zero ixns
}

func checkForTesseractInSMCache(t *testing.T, sm *StateManager, ts *types.Tesseract, withInteractions bool) {
	t.Helper()

	object, isCached := sm.cache.Get(getTesseractHash(t, ts))
	if withInteractions {
		require.False(t, isCached) // make sure tesseract not cached

		return
	}

	require.True(t, isCached) // make sure tesseract cached, if fetched without interactions

	cachedTS, ok := object.(*types.Tesseract)
	require.True(t, ok)

	require.Equal(t, ts, cachedTS) // make sure cached tesseract matches
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

func CheckForDirtyObject(t *testing.T, sm *StateManager, address types.Address, exists bool) {
	t.Helper()

	_, ok := sm.dirtyObjects[address]
	require.Equal(t, exists, ok)
}

func CheckIfDirtyObjectEqual(t *testing.T, sm *StateManager, address types.Address, expectedObj *StateObject) {
	t.Helper()

	obj, ok := sm.dirtyObjects[address]
	require.True(t, ok)
	require.Equal(t, expectedObj, obj)
}

func CheckForObject(t *testing.T, sm *StateManager, address types.Address, exists bool) {
	t.Helper()

	_, ok := sm.objects[address]
	require.Equal(t, exists, ok)
}

func checkForCache(t *testing.T, sm *StateManager, address types.Address) {
	t.Helper()

	_, isCached := sm.cache.Get(address) // check if added to cache
	require.True(t, isCached)
}

func checkForStateObject(t *testing.T, expectedObj *StateObject, obj *StateObject) {
	t.Helper()

	require.Equal(t, expectedObj.balance, obj.balance)
	require.Equal(t, expectedObj.data, obj.data)
	require.Equal(t, expectedObj.address, obj.address)
	require.Equal(t, expectedObj.accType, obj.accType)
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

func CheckIfNodesetEqual(
	t *testing.T,
	expectedBeh *ktypes.NodeSet,
	expectedRand *ktypes.NodeSet,
	beh *ktypes.NodeSet,
	rand *ktypes.NodeSet,
) {
	t.Helper()

	require.Equal(t, expectedBeh, beh)
	require.Equal(t, expectedRand, rand)
}

func checkForOtherAccountsInSargaObject(
	t *testing.T,
	obj *StateObject,
	accounts []*gtypes.AccountSetupArgs,
) {
	t.Helper()

	// check if other accounts address inserted in to sarga account storage
	for _, info := range accounts {
		val, err := obj.GetStorageEntry(
			SargaLogicID,
			info.Address.Bytes(),
		)
		require.NoError(t, err)

		genesisInfo := types.AccountGenesisInfo{
			IxHash: GenesisIxHash,
		}
		rawGenesisInfo, err := polo.Polorize(genesisInfo)
		assert.NoError(t, err)

		require.Equal(t, val, rawGenesisInfo)
	}
}

func checkForObjectCreation(t *testing.T, sm *StateManager, address types.Address, contextHash types.Hash) {
	t.Helper()

	// check if dirty object created
	obj, err := sm.GetDirtyObject(address)
	require.NoError(t, err)

	// check if context created
	_, ok := obj.dirtyEntries[types.BytesToHex(dhruva.ContextObjectKey(address, contextHash))]
	require.True(t, ok)

	// check if object committed
	data, err := obj.balance.Bytes()
	require.NoError(t, err)

	hash := types.GetHash(data)
	key := types.BytesToHex(dhruva.BalanceObjectKey(address, hash))
	val, ok := obj.dirtyEntries[key]
	require.True(t, ok)
	require.Equal(t, data, val)
}

func checkForAssetCreation(t *testing.T, obj *StateObject, asset *types.AssetDescriptor) {
	t.Helper()

	// check asset
	assetID, assetHash, data, err := gtypes.GetAssetID(
		&types.AssetDescriptor{
			Owner:      obj.address,
			Dimension:  asset.Dimension,
			IsFungible: asset.IsFungible,
			IsMintable: asset.IsMintable,
			Symbol:     asset.Symbol,
			Supply:     asset.Supply,
			LogicID:    asset.LogicID,
		},
	)
	require.NoError(t, err)

	v, ok := obj.dirtyEntries[assetHash.String()]
	require.True(t, ok)

	require.Equal(t, data, v)

	// check if balance updated
	supply, err := obj.BalanceOf(assetID)
	require.NoError(t, err)

	require.Equal(t, asset.Supply, supply)
}
