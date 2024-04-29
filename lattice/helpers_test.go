package lattice

import (
	"context"
	"encoding/json"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

var (
	validCommitSign   = []byte{1}
	invalidCommitSign = []byte{0}
)

// MockDB is an in-memory key-value database used for testing purposes
type MockDB struct {
	dbStorage                   map[string][]byte
	accounts                    map[common.Hash][]byte
	balances                    map[common.Hash][]byte
	accMetaInfos                map[identifiers.Address]*common.AccountMetaInfo
	TSHashByIxHash              map[string][]byte
	batchWriter                 *mockBatchWriter
	batchWriterSetHook          func() error
	batchWriterFlushHook        func() error
	updateMetaInfoHook          func() (int32, bool, error)
	setTesseractHook            func() error
	setInteractionsHook         func() error
	setReceiptsHook             func() error
	setTesseractHeightEntryHook func() error
	createEntryHook             func() error
}

func (m *MockDB) HasAccMetaInfoAt(addr identifiers.Address, height uint64) bool {
	accMetaInfo, err := m.GetAccountMetaInfo(addr)
	if err != nil {
		return false
	}

	if accMetaInfo.Height < height {
		return false
	}

	return true
}

func (m *MockDB) GetAssetRegistry(addr identifiers.Address, registryHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error) {
	// TODO implement me
	metaInfo, ok := m.accMetaInfos[id]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return metaInfo, nil
}

func mockDB() *MockDB {
	return &MockDB{
		dbStorage:      make(map[string][]byte),
		accMetaInfos:   make(map[identifiers.Address]*common.AccountMetaInfo),
		TSHashByIxHash: make(map[string][]byte),
	}
}

func (m *MockDB) SetIXLookup(ixHash common.Hash, tsHash common.Hash) error {
	m.TSHashByIxHash[ixHash.String()] = tsHash.Bytes()

	return nil
}

func (m *MockDB) GetIXLookup(ixHash common.Hash) ([]byte, error) {
	tsHash, ok := m.TSHashByIxHash[ixHash.String()]
	if !ok {
		return nil, common.ErrTSHashNotFound
	}

	return tsHash, nil
}

func (m *MockDB) CreateEntry(key []byte, value []byte) error {
	if m.createEntryHook != nil {
		return m.createEntryHook()
	}

	m.dbStorage[string(key)] = value

	return nil
}

func (m *MockDB) ReadEntry(key []byte) ([]byte, error) {
	val, ok := m.dbStorage[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

func (m *MockDB) Contains(key []byte) (bool, error) {
	_, ok := m.dbStorage[string(key)]

	return ok, nil
}

func (m *MockDB) UpdateAccMetaInfo(
	id identifiers.Address,
	height uint64,
	tesseractHash common.Hash,
	accType common.AccountType,
) (int32, bool, error) {
	if m.updateMetaInfoHook != nil {
		return m.updateMetaInfoHook()
	}

	m.accMetaInfos[id] = &common.AccountMetaInfo{
		Address:       id,
		Type:          accType,
		Height:        height,
		TesseractHash: tesseractHash,
	}

	return 8, true, nil
}

func (m *MockDB) GetAccMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, bool) {
	val, ok := m.accMetaInfos[id]

	return val, ok
}

func (m *MockDB) GetTesseract(hash common.Hash) ([]byte, error) {
	data, ok := m.dbStorage[hash.String()]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetTesseract(hash common.Hash, data []byte) error {
	if m.setTesseractHook != nil {
		return m.setTesseractHook()
	}

	m.dbStorage[hash.String()] = data

	return nil
}

func (m *MockDB) HasTesseract(hash common.Hash) bool {
	_, ok := m.dbStorage[hash.String()]

	return ok
}

func (m *MockDB) GetInteractions(tesseractHash common.Hash) ([]byte, error) {
	if ix, ok := m.dbStorage[tesseractHash.String()]; ok {
		return ix, nil
	}

	return nil, common.ErrKeyNotFound
}

func (m *MockDB) GetAccount(addr identifiers.Address, hash common.Hash) ([]byte, error) {
	account, ok := m.accounts[hash]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (m *MockDB) SetInteractions(tsHash common.Hash, data []byte) error {
	if m.setInteractionsHook != nil {
		return m.setInteractionsHook()
	}

	m.dbStorage[tsHash.String()] = data

	return nil
}

func (m *MockDB) GetTesseractHeightEntry(addr identifiers.Address, height uint64) ([]byte, error) {
	key := addr.Hex() + strconv.Itoa(int(height))

	data, ok := m.dbStorage[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetTesseractHeightEntry(addr identifiers.Address, height uint64, hash common.Hash) error {
	if m.setTesseractHeightEntryHook != nil {
		return m.setTesseractHeightEntryHook()
	}

	key := addr.Hex() + strconv.Itoa(int(height))

	m.dbStorage[key] = hash.Bytes()

	return nil
}

func (m *MockDB) GetBalance(addr identifiers.Address, hash common.Hash) ([]byte, error) {
	balance, ok := m.balances[hash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return balance, nil
}

func (m *MockDB) GetReceipts(tsHash common.Hash) ([]byte, error) {
	if m.setReceiptsHook != nil {
		return nil, m.setReceiptsHook()
	}

	data, ok := m.dbStorage[tsHash.String()]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetReceipts(tsHash common.Hash, data []byte) error {
	if m.setReceiptsHook != nil {
		return m.setReceiptsHook()
	}

	m.dbStorage[tsHash.String()] = data

	return nil
}

func (m *MockDB) GetContext(addr identifiers.Address, contextHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetMerkleTreeEntry(address identifiers.Address, prefix storage.PrefixTag, key []byte) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetMerkleTreeEntry(address identifiers.Address, prefix storage.PrefixTag, key, value []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetMerkleTreeEntries(
	address identifiers.Address,
	prefix storage.PrefixTag,
	entries map[string][]byte,
) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) WritePreImages(address identifiers.Address, entries map[common.Hash][]byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetPreImage(address identifiers.Address, hash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) DeleteEntry(key []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) UpdateEntry(key []byte, newValue []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) NewBatchWriter() db.BatchWriter {
	m.batchWriter = &mockBatchWriter{
		dbStorage: make(map[string][]byte),
		dirty:     make(map[string][]byte),
		setHook:   m.batchWriterSetHook,
		flushHook: m.batchWriterFlushHook,
	}

	return m.batchWriter
}

type mockBatchWriter struct {
	dbStorage map[string][]byte
	dirty     map[string][]byte
	setHook   func() error
	flushHook func() error
}

func newMockBatchWriter() *mockBatchWriter {
	return &mockBatchWriter{
		dbStorage: make(map[string][]byte),
		dirty:     make(map[string][]byte),
	}
}

func (bw *mockBatchWriter) WriteBuffer(buf []byte) error {
	// TODO implement me
	panic("implement me")
}

func (bw *mockBatchWriter) Set(key []byte, value []byte) error {
	if bw.setHook != nil {
		return bw.setHook()
	}

	bw.dirty[string(key)] = value

	return nil
}

func (bw *mockBatchWriter) Get(key []byte) ([]byte, bool) {
	val, ok := bw.dirty[string(key)]

	return val, ok
}

func (bw *mockBatchWriter) Flush() error {
	if bw.flushHook != nil {
		return bw.flushHook()
	}

	for k, v := range bw.dirty {
		bw.dbStorage[k] = v
	}

	bw.dirty = make(map[string][]byte)

	return nil
}

type MockNetwork struct {
	kramaID kramaid.KramaID
}

// mock network implementation

func (n *MockNetwork) SetKramaID(id kramaid.KramaID) {
	n.kramaID = id
}

func (n *MockNetwork) GetKramaID() kramaid.KramaID {
	return n.kramaID
}

func (n *MockNetwork) Broadcast(topic string, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (n *MockNetwork) Unsubscribe(topic string) error {
	// TODO implement me
	panic("implement me")
}

func (n *MockNetwork) Subscribe(
	ctx context.Context,
	topic string,
	validator utils.WrappedVal,
	defaultValidator bool,
	handler func(msg *pubsub.Message) error,
) error {
	// TODO implement me
	panic("implement me")
}

type MockExec struct {
	receipts                common.Receipts
	accountHashes           common.AccStateHashes
	revertHook              func() error
	executeInteractionsHook func() (common.Receipts, common.AccStateHashes, error)
	clusterID               common.ClusterID
}

func (e *MockExec) Cleanup(clusterID common.ClusterID) {
	e.clusterID = clusterID
}

func (e *MockExec) SpawnExecutor() *compute.IxExecutor {
	sm := mockStateManager()

	return compute.NewManager(sm, hclog.NewNullLogger(), nil, compute.NilMetrics()).SpawnExecutor()
}

func mockExec(t *testing.T) *MockExec {
	t.Helper()

	return new(MockExec)
}

// mock execution implementation
func (e *MockExec) ExecuteInteractions(
	ixs common.Interactions,
	ctx *common.ExecutionContext,
) (common.Receipts, common.AccStateHashes, error) {
	if e.executeInteractionsHook != nil {
		return e.executeInteractionsHook()
	}

	return e.receipts, e.accountHashes, nil
}

func (e *MockExec) Revert(clusterID common.ClusterID) error {
	if e.revertHook != nil {
		return e.revertHook()
	}

	return nil
}

type Context struct {
	behaviourNodes []kramaid.KramaID
	randomNodes    []kramaid.KramaID
}

type AccHash struct {
	contextHash common.Hash
	stateHash   common.Hash
}

type MockStateManager struct {
	dirtyObjects        map[identifiers.Address]*state.Object
	objects             map[identifiers.Address]*state.Object
	context             map[common.Hash]*Context
	publicKeys          map[kramaid.KramaID][]byte
	latestTesseracts    map[identifiers.Address]*common.Tesseract
	dbTesseracts        map[common.Hash]*common.Tesseract
	registeredAccounts  map[identifiers.Address]*AccHash
	cleanUp             map[identifiers.Address]bool
	accountTypes        map[identifiers.Address]common.AccountType
	flushedDirtyObjects map[identifiers.Address]bool

	flushHook               func() error
	GetAccTypeHook          func() error
	newAccountHook          func() (common.Hash, common.Hash, error)
	accountRegistrationHook func(hash common.Hash) (bool, error)
	createDirtyObjectHook   func() *state.Object
}

func (sm *MockStateManager) UpdateStateObjects(objs state.ObjectMap) error {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) CreateStateObject(_ identifiers.Address, _ common.AccountType) *state.Object {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetEmptyStateObject() *state.Object {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetLatestStateObject(addr identifiers.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetStateObjectByHash(addr identifiers.Address, hash common.Hash) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetLogicIDs(addr identifiers.Address, hash common.Hash) ([]identifiers.LogicID, error) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) Revert(object *state.Object) error {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetNodeSet(ids []kramaid.KramaID) (*common.NodeSet, error) {
	pks := make([][]byte, 0, len(ids))

	for _, v := range ids {
		pk, ok := sm.publicKeys[v]
		if !ok {
			return nil, common.ErrKeyNotFound
		}

		pks = append(pks, pk)
	}

	return common.NewNodeSet(ids, pks, 0), nil
}

func mockStateManager() *MockStateManager {
	return &MockStateManager{
		dirtyObjects:        make(map[identifiers.Address]*state.Object),
		objects:             make(map[identifiers.Address]*state.Object),
		context:             make(map[common.Hash]*Context),
		publicKeys:          make(map[kramaid.KramaID][]byte),
		latestTesseracts:    make(map[identifiers.Address]*common.Tesseract),
		dbTesseracts:        make(map[common.Hash]*common.Tesseract),
		registeredAccounts:  make(map[identifiers.Address]*AccHash),
		cleanUp:             make(map[identifiers.Address]bool),
		accountTypes:        make(map[identifiers.Address]common.AccountType),
		flushedDirtyObjects: make(map[identifiers.Address]bool),
	}
}

func (sm *MockStateManager) isCleanup(addrs identifiers.Address) bool {
	if _, ok := sm.cleanUp[addrs]; ok {
		return true
	}

	return false
}

func (sm *MockStateManager) CreateDirtyObject(addr identifiers.Address, accType common.AccountType) *state.Object {
	if sm.createDirtyObjectHook != nil {
		return sm.createDirtyObjectHook()
	}

	if obj, ok := sm.dirtyObjects[addr]; ok {
		return obj
	}

	obj := state.NewStateObject(addr, mockCache(), nil, mockDB(),
		common.Account{AccType: accType}, state.NilMetrics())
	sm.dirtyObjects[addr] = obj.Copy()

	return sm.dirtyObjects[addr]
}

func (sm *MockStateManager) GetDirtyObject(addr identifiers.Address) (*state.Object, error) {
	return sm.dirtyObjects[addr], nil
}

func (sm *MockStateManager) setAccType(address identifiers.Address, accountType common.AccountType) {
	sm.accountTypes[address] = accountType
}

func (sm *MockStateManager) GetAccTypeUsingStateObject(address identifiers.Address) (common.AccountType, error) {
	if sm.GetAccTypeHook != nil {
		return 0, sm.GetAccTypeHook()
	}

	accType, ok := sm.accountTypes[address]
	if !ok {
		return 0, errors.New("account type not found")
	}

	return accType, nil
}

func (sm *MockStateManager) DeleteStateObject(addr identifiers.Address) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) FlushDirtyObject(addrs identifiers.Address) error {
	if sm.flushHook != nil {
		return sm.flushHook()
	}

	sm.flushedDirtyObjects[addrs] = true

	return nil
}

func (sm *MockStateManager) getFlushedDirtyObject(addrs identifiers.Address) bool {
	_, ok := sm.flushedDirtyObjects[addrs]

	return ok
}

func (sm *MockStateManager) GetLatestTesseract(addr identifiers.Address, withIxns bool) (*common.Tesseract, error) {
	ts, ok := sm.latestTesseracts[addr]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	copyTS := *ts // copy, so that stored tesseract won't be modified

	if !withIxns {
		copyTS = *copyTS.GetTesseractWithoutIxns()
	}

	return &copyTS, nil
}

func (sm *MockStateManager) insertContextNodes(
	ctxHash common.Hash,
	behaviouralNodes []kramaid.KramaID,
	randomNodes ...kramaid.KramaID,
) {
	sm.context[ctxHash] = &Context{
		behaviourNodes: behaviouralNodes,
		randomNodes:    randomNodes,
	}
}

func (sm *MockStateManager) GetContextByHash(
	addr identifiers.Address,
	hash common.Hash,
) (
	common.Hash,
	[]kramaid.KramaID,
	[]kramaid.KramaID,
	error,
) {
	c, ok := sm.context[hash]

	if !ok {
		return common.NilHash, nil, nil, common.ErrContextStateNotFound
	}

	return hash, c.behaviourNodes, c.randomNodes, nil
}

func (sm *MockStateManager) GetPublicKeys(ctx context.Context, id ...kramaid.KramaID) ([][]byte, error) {
	keys := make([][]byte, 0)

	for _, v := range id {
		key, ok := sm.publicKeys[v]
		if !ok {
			return nil, common.ErrKeyNotFound
		}

		keys = append(keys, key)
	}

	return keys, nil
}

func (sm *MockStateManager) Cleanup(addrs identifiers.Address) {
	sm.cleanUp[addrs] = true
}

func (sm *MockStateManager) insertRegisteredAcc(addr identifiers.Address) {
	sm.registeredAccounts[addr] = &AccHash{}
}

func (sm *MockStateManager) IsAccountRegistered(addr identifiers.Address) (bool, error) {
	if sm.accountRegistrationHook != nil {
		return sm.accountRegistrationHook(common.NilHash)
	}

	_, ok := sm.registeredAccounts[addr]

	return ok, nil
}

func (sm *MockStateManager) IsAccountRegisteredAt(addr identifiers.Address, tesseractHash common.Hash) (bool, error) {
	if sm.accountRegistrationHook != nil {
		return sm.accountRegistrationHook(tesseractHash)
	}

	_, ok := sm.registeredAccounts[addr]

	return ok, nil
}

func (sm *MockStateManager) FetchTesseractFromDB(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	ts, ok := sm.dbTesseracts[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract wont't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (sm *MockStateManager) InsertLatestTesseracts(t *testing.T, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		for _, addr := range ts.Addresses() {
			sm.latestTesseracts[addr] = ts
		}
	}
}

func (sm *MockStateManager) InsertTesseractsInDB(t *testing.T, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		sm.dbTesseracts[ts.Hash()] = ts
	}
}

func (sm *MockStateManager) setPublicKey(id kramaid.KramaID, pk []byte) {
	sm.publicKeys[id] = pk
}

type MockIXPool struct {
	reset map[common.Hash]bool
}

type MockSenatus struct {
	WalletCount           map[kramaid.KramaID]int32
	UpdateWalletCountHook func() error
}

func (s *MockSenatus) UpdateWalletCount(peerID kramaid.KramaID, delta int32) error {
	if s.UpdateWalletCountHook != nil {
		return s.UpdateWalletCountHook()
	}

	s.WalletCount[peerID] += delta

	return nil
}

func (i *MockIXPool) ResetWithHeaders(ts *common.Tesseract) {
	i.reset[ts.Hash()] = true
}

func (i *MockIXPool) IsReset(hash common.Hash) bool {
	if _, ok := i.reset[hash]; ok {
		return true
	}

	return false
}

type CreateIxParams struct {
	ixDataCallback func(ix *common.IxData)
}

func createIX(t *testing.T, params *CreateIxParams) *common.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &common.IxData{
		Input: common.IxInput{},
	}

	if params.ixDataCallback != nil {
		params.ixDataCallback(data)
	}

	ix, err := common.NewInteraction(*data, []byte{})
	require.NoError(t, err)

	return ix
}

func createIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) common.Interactions {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make(common.Interactions, count)

	for i := 0; i < count; i++ {
		ixns[i] = createIX(t, paramsMap[i])
	}

	return ixns
}

func getIxParamsWithAddress(from identifiers.Address, to identifiers.Address) *CreateIxParams {
	return &CreateIxParams{
		ixDataCallback: func(ix *common.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
			ix.Input.Type = common.IxValueTransfer
		},
	}
}

func getIxParamsMapWithAddresses(
	from []identifiers.Address,
	to []identifiers.Address,
) map[int]*CreateIxParams {
	ixParams := make(map[int]*CreateIxParams, len(from))

	for i := 0; i < len(from); i++ {
		ixParams[i] = getIxParamsWithAddress(from[i], to[i])
	}

	return ixParams
}

func mockCache() *lru.Cache {
	cache, _ := lru.New(1200)

	return cache
}

func mockNetwork(t *testing.T) *MockNetwork {
	t.Helper()

	return &MockNetwork{}
}

func createTesseractsWithChain(t *testing.T, count int, paramsMap map[int]*createTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	tesseracts[0] = createTesseract(t, paramsMap[0])

	for i := 1; i < count; i++ {
		paramsMap[i].participantsCallback = func(participants common.Participants) {
			hash := tesseracts[i-1].Hash()
			p := participants[tesseracts[0].AnyAddress()]
			p.TransitiveLink = hash
			participants[tesseracts[0].AnyAddress()] = p
		}

		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

type createTesseractParams struct {
	Addresses            []identifiers.Address
	Heights              []uint64
	Participants         common.Participants
	participantsCallback func(participants common.Participants)
	TSDataCallback       func(ts *tests.TesseractData)

	Ixns     common.Interactions
	Receipts common.Receipts
}

func defaultTesseractData() *tests.TesseractData {
	// vote-set is a bit array
	voteSet := common.ArrayOfBits{
		Size:     1,                 // represents node tsCount
		Elements: make([]uint64, 1), // each element holds eight votes
	}

	voteSet.Size = 5         // there are 5 ics nodes
	voteSet.Elements[0] = 31 // first 5 ics nodes voted yes

	return &tests.TesseractData{
		InteractionsHash: common.NilHash,
		ReceiptsHash:     common.NilHash,
		Epoch:            big.NewInt(0),
		Timestamp:        0,
		Operator:         "",
		FuelUsed:         100,
		FuelLimit:        100,
		ConsensusInfo: common.PoXtData{
			BFTVoteSet: voteSet.Copy(),
		},

		// non canonical fields
		Seal:   nil,
		SealBy: "",
	}
}

// CreateTesseract creates a tesseract using tessseract params fields
// if any field thats not available in params need to be initialized using TesseractCallback field
func createTesseract(t *testing.T, params *createTesseractParams) *common.Tesseract {
	t.Helper()

	var (
		// participants     common.Participants
		interactionsHash common.Hash
		tsData           = defaultTesseractData()
	)

	if params == nil {
		params = &createTesseractParams{}
	}

	if params.Participants == nil {
		params.Participants = make(common.Participants)
	}

	if len(params.Addresses) == 0 {
		addr := tests.RandomAddress(t)
		params.Addresses = []identifiers.Address{addr}
		params.Participants[addr] = common.State{}

		if len(params.Heights) != 0 {
			params.Participants[addr] = common.State{
				Height: params.Heights[0],
			}
		}
	}

	if params.Ixns != nil {
		hash, err := params.Ixns.Hash()
		require.NoError(t, err)

		interactionsHash = hash
	}

	if params.TSDataCallback != nil {
		params.TSDataCallback(tsData)
	}

	if params.participantsCallback != nil {
		params.participantsCallback(params.Participants)
	}

	return common.NewTesseract(
		params.Participants,
		interactionsHash,
		tsData.ReceiptsHash,
		tsData.Epoch,
		tsData.Timestamp,
		tsData.Operator,
		tsData.FuelUsed,
		tsData.FuelLimit,
		tsData.ConsensusInfo,
		tsData.Seal,
		tsData.SealBy,
		params.Ixns,
		params.Receipts,
	)
}

func createTesseracts(t *testing.T, count int, paramsMap map[int]*createTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	for i := 0; i < count; i++ {
		if paramsMap[i] == nil {
			paramsMap[i] = &createTesseractParams{
				Heights: []uint64{uint64(i)},
			}
		}

		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func tesseractParamsWithCommitSign(commitSign []byte) *createTesseractParams {
	return &createTesseractParams{
		TSDataCallback: func(ts *tests.TesseractData) {
			ts.ConsensusInfo.CommitSignature = commitSign
		},
	}
}

func smCallbackWithRegisteredAcc(address identifiers.Address) func(sm *MockStateManager) {
	return func(sm *MockStateManager) {
		sm.insertRegisteredAcc(address)
	}
}

func smCallbackWithAccRegistrationHook() func(sm *MockStateManager) {
	return func(sm *MockStateManager) {
		sm.accountRegistrationHook = func(hash common.Hash) (bool, error) {
			return false, common.ErrAccountNotFound
		}
	}
}

func getTesseractParamsMapWithIxns(t *testing.T, tsCount int) map[int]*createTesseractParams {
	t.Helper()

	tesseractParams := make(map[int]*createTesseractParams, tsCount)
	addresses := tests.GetAddresses(t, 4*tsCount) // for each interaction, sender and receiver addresses needed
	ixns := createIxns(t, 2*tsCount, getIxParamsMapWithAddresses(addresses[:2*tsCount], addresses[2*tsCount:]))

	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &createTesseractParams{
			Ixns: ixns[i*2 : i*2+2], // allocate two interactions per tesseract
		}
	}

	return tesseractParams
}

type CreateChainParams struct {
	db                   *MockDB
	sm                   *MockStateManager
	senatus              *MockSenatus
	ixPool               *MockIXPool
	dbCallback           func(db *MockDB)
	ixPoolCallback       func(ixPool *MockIXPool)
	smCallBack           func(sm *MockStateManager)
	senatusCallback      func(senatus *MockSenatus)
	networkCallback      func(network *MockNetwork)
	execCallback         func(exec *MockExec)
	chainManagerCallback func(c *ChainManager)
}

func mockChainConfig() *config.ChainConfig {
	return &config.ChainConfig{}
}

func mockSenatus(t *testing.T) *MockSenatus {
	t.Helper()

	return &MockSenatus{
		WalletCount: make(map[kramaid.KramaID]int32),
	}
}

func mockIXPool(t *testing.T) *MockIXPool {
	t.Helper()

	return &MockIXPool{
		reset: make(map[common.Hash]bool),
	}
}

func createTestChainManager(t *testing.T, params *CreateChainParams) *ChainManager {
	t.Helper()

	if params == nil {
		params = &CreateChainParams{}
	}

	var (
		db      = mockDB()
		sm      = mockStateManager()
		senatus = mockSenatus(t)
		ixPool  = mockIXPool(t)
		exec    = mockExec(t)
		network = mockNetwork(t)
	)

	if params.senatus != nil {
		senatus = params.senatus
	}

	if params.sm != nil {
		sm = params.sm
	}

	if params.db != nil {
		db = params.db
	}

	if params.ixPool != nil {
		ixPool = params.ixPool
	}

	if params.ixPoolCallback != nil {
		params.ixPoolCallback(ixPool)
	}

	if params.smCallBack != nil {
		params.smCallBack(sm)
	}

	if params.senatusCallback != nil {
		params.senatusCallback(senatus)
	}

	if params.networkCallback != nil {
		params.networkCallback(network)
	}

	if params.execCallback != nil {
		params.execCallback(exec)
	}

	if params.dbCallback != nil {
		params.dbCallback(db)
	}

	c, err := NewChainManager(
		mockChainConfig(),
		db,
		sm,
		hclog.NewNullLogger(),
		&utils.TypeMux{},
		network,
		ixPool,
		mockCache(),
		exec,
		senatus,
		NilMetrics(),
		MockAggregateSignVerifier,
	)
	require.NoError(t, err)

	if params.chainManagerCallback != nil {
		params.chainManagerCallback(c)
	}

	return c
}

func fetchContextFromLattice(
	t *testing.T,
	address identifiers.Address,
	ts common.Tesseract,
	c *ChainManager,
) []kramaid.KramaID {
	t.Helper()

	peers := make([]kramaid.KramaID, 0)

	for {
		if len(peers) >= 10 {
			break
		}

		delta, _ := ts.GetContextDelta(address)
		peers = append(peers, delta.BehaviouralNodes...)
		peers = append(peers, delta.RandomNodes...)

		_, behaviour, random, err := c.sm.GetContextByHash(address, ts.PreviousContextHash(address))
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.TransitiveLink(address).IsNil() {
			break
		}

		t, err := c.GetTesseract(ts.TransitiveLink(address), false)
		if err != nil {
			return nil
		}

		ts = *t
	}

	return peers
}

func insertTesseractsInDB(t *testing.T, db store, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		bytes, err := ts.Bytes()
		require.NoError(t, err)

		err = db.SetTesseract(ts.Hash(), bytes)
		require.NoError(t, err)
	}
}

func insertAccMetaInfo(t *testing.T, db store, ts *common.Tesseract) {
	t.Helper()

	for addr, s := range ts.Participants() {
		_, _, err := db.UpdateAccMetaInfo(addr, s.Height, ts.Hash(), common.RegularAccount)
		require.NoError(t, err)
	}
}

func getDeltaGroup(t *testing.T, behaviouralCount int, randomCount int, replaceCount int) *common.DeltaGroup {
	t.Helper()

	return &common.DeltaGroup{
		Role:             1,
		BehaviouralNodes: tests.RandomKramaIDs(t, behaviouralCount),
		RandomNodes:      tests.RandomKramaIDs(t, randomCount),
		ReplacedNodes:    tests.RandomKramaIDs(t, replaceCount),
	}
}

func insertTesseractsInCache(t *testing.T, c *ChainManager, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		c.tesseracts.Add(ts.Hash(), ts)
	}
}

func getPublicKeys(t *testing.T, count int) [][]byte {
	t.Helper()

	var pk [][]byte

	for i := 0; i < count; i++ {
		addr := tests.RandomAddress(t).Bytes()
		pk = append(pk, addr)
	}

	return pk
}

func insertTesseractByHeight(t *testing.T, db store, ts *common.Tesseract) {
	t.Helper()

	for addr, s := range ts.Participants() {
		err := db.SetTesseractHeightEntry(addr, s.Height, ts.Hash())
		require.NoError(t, err)
	}
}

func getICSNodeset(t *testing.T, count int) *common.ICSNodeSet {
	t.Helper()

	ics := common.NewICSNodeSet(6)

	senderBehaviourSet := tests.RandomKramaIDs(t, count)
	senderRandomSet := tests.RandomKramaIDs(t, count)
	receiverBehaviourSet := tests.RandomKramaIDs(t, count)
	receiverRandomSet := tests.RandomKramaIDs(t, count)
	randomNodes := tests.RandomKramaIDs(t, count)

	ics.UpdateNodeSet(common.SenderBehaviourSet, common.NewNodeSet(senderBehaviourSet, getPublicKeys(t, count), 0))
	ics.UpdateNodeSet(common.SenderRandomSet, common.NewNodeSet(senderRandomSet, getPublicKeys(t, count), 0))
	ics.UpdateNodeSet(common.ReceiverBehaviourSet, common.NewNodeSet(receiverBehaviourSet, getPublicKeys(t, count), 0))
	ics.UpdateNodeSet(common.ReceiverRandomSet, common.NewNodeSet(receiverRandomSet, getPublicKeys(t, count), 0))
	ics.UpdateNodeSet(common.RandomSet, common.NewNodeSet(randomNodes, getPublicKeys(t, count), 0))

	return ics
}

// MockAggregateSignVerifier returns true if first byte is true else return false
func MockAggregateSignVerifier(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error) {
	if aggSignature[0] == 1 {
		return true, nil
	}

	return false, common.ErrSignatureVerificationFailed
}

func getReceipt(ixHash common.Hash) *common.Receipt {
	return &common.Receipt{
		IxType:    1,
		IxHash:    ixHash,
		FuelUsed:  rand.Uint64(),
		ExtraData: make(json.RawMessage, 0),
	}
}

func getIX(t *testing.T) *common.Interaction {
	t.Helper()

	return createIX(
		t,
		getIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t)),
	)
}

func getIxAndReceipt(t *testing.T) (*common.Interaction, *common.Receipt) {
	t.Helper()
	ix := getIX(t)

	return ix, getReceipt(ix.Hash())
}

func getIxAndReceipts(t *testing.T, ixCount int) ([]*common.Interaction, common.Receipts) {
	t.Helper()

	var ixs []*common.Interaction

	receipts := make(map[common.Hash]*common.Receipt, ixCount)

	for i := 0; i < ixCount; i++ {
		ix, r := getIxAndReceipt(t)
		ixs = append(ixs, ix)
		receipts[ix.Hash()] = r
	}

	return ixs, receipts
}

func tesseractParamsWithContextDelta(
	t *testing.T,
	address identifiers.Address,
	behaviouralCount, randomCount, replacedCount int,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.Participants{
			address: {
				ContextDelta: *getDeltaGroup(t, behaviouralCount, randomCount, replacedCount),
			},
		},
	}
}

func insertReceipts(t *testing.T, db store, tsHash common.Hash, receipts common.Receipts) {
	t.Helper()

	rawData, err := receipts.Bytes()
	require.NoError(t, err)

	err = db.SetReceipts(tsHash, rawData)
	require.NoError(t, err)
}

// signs tesseract and stores signed bytes in seal also stores the public key associated with krama id
func signTesseract(t *testing.T, sm *MockStateManager, ts *common.Tesseract) {
	t.Helper()

	rawData, err := ts.Bytes()
	require.NoError(t, err)
	ids := tests.RandomKramaIDs(t, 1)

	seal, pk := tests.SignBytes(t, rawData) // calculate the seal of tesseract
	ts.SetSeal(seal)                        // store seal in tesseract
	ts.SetSealBy(ids[0])

	sm.setPublicKey(ids[0], pk) // store the public key for the signed sender
}

// getTesseractAddedEvent extracts TesseractAddedEvent from interface
func getTesseractAddedEvent(t *testing.T, data interface{}) utils.TesseractAddedEvent {
	t.Helper()

	event, ok := data.(utils.TesseractAddedEvent)
	require.True(t, ok)

	return event
}

func getAssetAccountSetupArgs(
	t *testing.T,
	assetDetails common.AssetCreationArgs,
	behaviouralContext []kramaid.KramaID,
	randomContext []kramaid.KramaID,
) common.AssetAccountSetupArgs {
	t.Helper()

	return common.AssetAccountSetupArgs{
		BehaviouralContext: behaviouralContext,
		RandomContext:      randomContext,
		AssetInfo:          &assetDetails,
	}
}

func getTestAssetCreationArgs(t *testing.T, allocationAddr identifiers.Address) common.AssetCreationArgs {
	t.Helper()

	info := tests.GetRandomAssetInfo(t, identifiers.NilAddress)

	if allocationAddr == identifiers.NilAddress {
		allocationAddr = tests.RandomAddress(t)
	}

	return common.AssetCreationArgs{
		Symbol:     info.Symbol,
		Dimension:  hexutil.Uint8(info.Dimension),
		Standard:   hexutil.Uint16(info.Standard),
		IsLogical:  info.IsLogical,
		IsStateful: info.IsStateFul,
		Operator:   info.Operator,
		Allocations: []common.Allocation{
			{
				Address: allocationAddr,
				Amount:  (*hexutil.Big)(big.NewInt(rand.Int63())),
			},
		},
	}
}

func getTestGenesisLogics(t *testing.T) []common.LogicSetupArgs {
	t.Helper()

	manifest := "0x" + common.BytesToHex(tests.ReadManifest(t, "./../compute/manifests/ledger.yaml"))
	calldata := "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29e" +
		"c442dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"

	logic := common.LogicSetupArgs{
		Name: "staking-contract",

		Callsite: "Seeder!",
		Calldata: hexutil.Bytes(common.Hex2Bytes(calldata)),
		Manifest: hexutil.Bytes(common.Hex2Bytes(manifest)),

		BehaviouralContext: tests.RandomKramaIDs(t, 1),
		RandomContext:      nil,
	}

	return []common.LogicSetupArgs{logic}
}

// createMockGenesisFile is a mock function used to create genesis file
func createMockGenesisFile(
	t *testing.T,
	dir string,
	invalidData bool,
	sarga common.AccountSetupArgs,
	accounts []common.AccountSetupArgs,
	assetAccounts []common.AssetAccountSetupArgs,
	logics []common.LogicSetupArgs,
) string {
	t.Helper()

	var (
		data []byte
		err  error
	)

	genesis := &common.GenesisFile{
		SargaAccount:  sarga,
		Accounts:      accounts,
		AssetAccounts: assetAccounts,
		Logics:        logics,
	}

	if invalidData {
		data = []byte{1, 2, 3}
	} else {
		data, err = json.MarshalIndent(genesis, "", " ")
		require.NoError(t, err)
	}

	file, err := os.CreateTemp(dir, "*config.json")
	require.NoError(t, err)

	_, err = file.Write(data)
	require.NoError(t, err)

	return file.Name()
}

func getTestAccountWithAccType(t *testing.T, accType common.AccountType) common.AccountSetupArgs {
	t.Helper()

	ids := tests.RandomKramaIDs(t, 4)

	address := tests.RandomAddress(t)

	if accType == common.SargaAccount {
		address = common.SargaAddress
	}

	return *getAccountSetupArgs(
		t,
		address,
		accType,
		"moi-id",
		ids[:2],
		ids[2:4],
	)
}

func getTestAccountWithAddress(t *testing.T, address identifiers.Address) common.AccountSetupArgs {
	t.Helper()

	ids := tests.RandomKramaIDs(t, 4)

	return *getAccountSetupArgs(
		t,
		address,
		common.RegularAccount,
		"moi-id",
		ids[:2],
		ids[2:4],
	)
}

func getAssetCreationArgs(
	symbol string,
	owner identifiers.Address,
	address []identifiers.Address,
	amount []*big.Int,
) *common.AssetCreationArgs {
	alloc := make([]common.Allocation, len(address))

	for i, addr := range address {
		alloc[i] = common.Allocation{
			Address: addr,
			Amount:  (*hexutil.Big)(amount[i]),
		}
	}

	return &common.AssetCreationArgs{
		Symbol:      symbol,
		Operator:    owner,
		Allocations: alloc,
	}
}

func getAccountSetupArgs(
	t *testing.T,
	address identifiers.Address,
	accType common.AccountType,
	moiID string,
	behNodes []kramaid.KramaID,
	randNodes []kramaid.KramaID,
) *common.AccountSetupArgs {
	t.Helper()

	return &common.AccountSetupArgs{
		Address:            address,
		MoiID:              moiID,
		BehaviouralContext: behNodes,
		RandomContext:      randNodes,
		AccType:            accType,
	}
}

// validation

func validateDeltaGroup(t *testing.T, senatus *MockSenatus, deltaGroup common.DeltaGroup) {
	t.Helper()

	for _, kramaID := range deltaGroup.BehaviouralNodes {
		require.Equal(t, senatus.WalletCount[kramaID], int32(1))
	}

	for _, kramaID := range deltaGroup.RandomNodes {
		require.Equal(t, senatus.WalletCount[kramaID], int32(1))
	}

	for _, kramaID := range deltaGroup.ReplacedNodes {
		require.Equal(t, senatus.WalletCount[kramaID], int32(-1))
	}
}

func checkForTesseractData(
	t *testing.T,
	ts *common.Tesseract,
	c *ChainManager,
	db *MockDB,
	senatus *MockSenatus,
	ixpool *MockIXPool,
	tsAddedResp chan tests.Result,
	shouldCache bool,
) {
	t.Helper()

	// check if tesseract data is added and flushed
	bw := db.batchWriter
	_, found := bw.dbStorage[string(storage.TesseractKey(ts.Hash()))]
	require.True(t, found)

	// check if tesseract stored in cache
	storedTS, ok := c.tesseracts.Get(ts.Hash())

	if shouldCache {
		require.True(t, ok)
		require.Equal(t, ts.GetTesseractWithoutIxns(), storedTS)
	} else {
		require.False(t, ok)
	}

	// check if node inclusivity updated
	for _, s := range ts.Participants() {
		validateDeltaGroup(t, senatus, s.ContextDelta)
	}

	// check if ixpool is reset
	require.True(t, ixpool.IsReset(ts.Hash()))

	// check if tesseract added event is fired
	validateTSAddedEvent(t, tsAddedResp, ts)
}

func checkForParticipantByHeight(
	t *testing.T,
	c *ChainManager,
	addr identifiers.Address,
	participant common.State,
	isPresent bool,
) {
	t.Helper()

	// check if tesseract height key added
	_, err := c.db.GetTesseractHeightEntry(addr, participant.Height)

	if isPresent {
		require.NoError(t, err)

		return
	}

	require.Equal(t, common.ErrKeyNotFound, err)
}

func checkIfAccMetaInfoMatches(
	t *testing.T,
	accMetaInfo *common.AccountMetaInfo,
	addr identifiers.Address,
	height uint64,
	tsHash common.Hash,
	accType common.AccountType,
) {
	t.Helper()

	require.Equal(t, addr, accMetaInfo.Address)
	require.Equal(t, height, accMetaInfo.Height)
	require.Equal(t, tsHash, accMetaInfo.TesseractHash)
	require.Equal(t, accType, accMetaInfo.Type)
}

func checkIfTesseractCachedInCM(t *testing.T, c *ChainManager, withInteractions bool, tsHash common.Hash) {
	t.Helper()

	tesseractData, isCached := c.tesseracts.Get(tsHash)
	if withInteractions { // If fetched without interactions Make sure tesseract added to cache
		require.False(t, isCached) // makesure tesseract not cached if added with interaction

		return
	}

	require.True(t, isCached)

	cachedTS, ok := tesseractData.(*common.Tesseract)
	require.True(t, ok)

	// make sure tesseract in cache doesn't have interactions
	require.Equal(t, 0, len(cachedTS.Interactions()))
}

func validateTSAddedEvent(t *testing.T, tsAddedResp chan tests.Result, ts *common.Tesseract) {
	t.Helper()

	data := tests.WaitForResponse(t, tsAddedResp, utils.TesseractAddedEvent{}) // waits for data from goroutine
	event := getTesseractAddedEvent(t, data)                                   // convert interface type to concrete type
	require.Equal(t, ts, event.Tesseract)
}

func checkForAssetAccounts(t *testing.T, expected, actual []common.AssetAccountSetupArgs) {
	t.Helper()

	require.Equal(t, len(expected), len(actual))

	for i := range expected {
		require.Equal(t, expected[i], actual[i])
	}
}

func checkForLogicAccounts(t *testing.T, expected, actual []common.LogicSetupArgs) {
	t.Helper()

	require.Equal(t, len(expected), len(actual))

	for i := range actual {
		require.Equal(t, expected[i], actual[i])
	}
}

// checkForGenesisTesseract fetches added tesseract and checks if it valid
func checkForGenesisTesseract(
	t *testing.T,
	sm *MockStateManager,
	address identifiers.Address,
) {
	t.Helper()

	found := sm.getFlushedDirtyObject(address)
	require.True(t, found)
}

func checkSargaStorageEntry(t *testing.T, obj *state.Object, addr identifiers.Address) {
	t.Helper()

	val, err := obj.GetStorageEntry(common.SargaLogicID, addr.Bytes())
	require.NoError(t, err)

	genesisInfo := common.AccountGenesisInfo{
		IxHash: common.GenesisIxHash,
	}
	rawGenesisInfo, err := polo.Polorize(genesisInfo)
	assert.NoError(t, err)

	require.Equal(t, val, rawGenesisInfo)
}

func checkSargaObjectAccounts(
	t *testing.T,
	obj *state.Object,
	accounts []common.AccountSetupArgs,
) {
	t.Helper()

	// check if other accounts address inserted in to sarga account storage
	for _, info := range accounts {
		checkSargaStorageEntry(t, obj, info.Address)
	}
}

func checkSargaObjectLogicAccounts(
	t *testing.T,
	obj *state.Object,
	logics []common.LogicSetupArgs,
) {
	t.Helper()

	// check if logics address inserted in to sarga account storage
	for _, logic := range logics {
		checkSargaStorageEntry(t, obj, common.CreateAddressFromString(logic.Name))
	}
}

func checkSargaObjectAssetAccounts(
	t *testing.T,
	obj *state.Object,
	assets []common.AssetAccountSetupArgs,
) {
	t.Helper()

	// check if assert address inserted in to sarga account storage
	for _, asset := range assets {
		checkSargaStorageEntry(t, obj, common.CreateAddressFromString(asset.AssetInfo.Symbol))
	}
}

func validateContextInitialization(
	t *testing.T,
	sm stateManager,
	address identifiers.Address,
	behavioural, random []kramaid.KramaID,
	contextHash common.Hash,
) {
	t.Helper()

	// check if dirty object created
	obj, err := sm.GetDirtyObject(address)
	require.NoError(t, err)

	// check if context created
	_, err = obj.GetDirtyEntry(common.BytesToHex(storage.ContextObjectKey(address, contextHash)))
	require.NoError(t, err)
}

func checkForAssetRegistry(
	t *testing.T,
	so *state.Object,
	assetID identifiers.AssetID,
	expectedAssetDescriptor []byte,
) {
	t.Helper()

	registry, err := so.Registry()
	require.NoError(t, err)

	actualAssetDescriptor, ok := registry.Entries[string(assetID)]
	require.True(t, ok)

	require.Equal(t, expectedAssetDescriptor, actualAssetDescriptor)
}

func checkForAllocations(
	t *testing.T,
	stateObjects map[identifiers.Address]*state.Object,
	assetInfo *common.AssetCreationArgs,
	assetID identifiers.AssetID,
) {
	t.Helper()

	for _, allocation := range assetInfo.Allocations {
		so, ok := stateObjects[allocation.Address]
		require.True(t, ok)

		balances, err := so.Balances()
		require.NoError(t, err)

		bal, ok := balances.AssetMap[assetID]
		require.True(t, ok)

		require.Equal(t, allocation.Amount.ToInt(), bal)
	}
}

func checkForExecutionCleanup(t *testing.T, c *ChainManager, expectedClusterID common.ClusterID) {
	t.Helper()

	mockExec, ok := c.exec.(*MockExec)
	require.True(t, ok)

	require.Equal(t, expectedClusterID, mockExec.clusterID)
}
