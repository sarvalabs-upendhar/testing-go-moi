package lattice

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-polo"

	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
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

type result struct {
	data interface{}
	err  error
}

// MockDB is an in-memory key-value database used for testing purposes
type MockDB struct {
	dbStorage                   map[string][]byte
	accounts                    map[common.Hash][]byte
	balances                    map[common.Hash][]byte
	accMetaInfos                map[common.Address]*common.AccountMetaInfo
	gridHashByIxHash            map[string][]byte
	gridHashByTSHash            map[string][]byte
	tesseractParts              map[string][]byte
	updateMetaInfoHook          func() (int32, bool, error)
	setTesseractHook            func() error
	setInteractionsHook         func() error
	setReceiptsHook             func() error
	setTesseractHeightEntryHook func() error
	createEntryHook             func() error
	setTSGridLookupHook         func() error
}

func (m *MockDB) GetAssetRegistry(addr common.Address, registryHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetAccountMetaInfo(id common.Address) (*common.AccountMetaInfo, error) {
	// TODO implement me
	metaInfo, ok := m.accMetaInfos[id]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return metaInfo, nil
}

type testTSArgs struct {
	cache           bool
	stateExists     bool
	tesseractExists bool
}

func mockDB() *MockDB {
	return &MockDB{
		dbStorage:        make(map[string][]byte),
		accMetaInfos:     make(map[common.Address]*common.AccountMetaInfo),
		gridHashByIxHash: make(map[string][]byte),
		gridHashByTSHash: make(map[string][]byte),
		tesseractParts:   make(map[string][]byte),
	}
}

func (m *MockDB) SetIXGridLookup(ixHash common.Hash, gridHash common.Hash) error {
	m.gridHashByIxHash[ixHash.String()] = gridHash.Bytes()

	return nil
}

func (m *MockDB) GetIXGridLookup(ixHash common.Hash) ([]byte, error) {
	gridHash, ok := m.gridHashByIxHash[ixHash.String()]
	if !ok {
		return nil, common.ErrGridHashNotFound
	}

	return gridHash, nil
}

func (m *MockDB) GetTesseractParts(ixHash common.Hash) ([]byte, error) {
	parts, ok := m.tesseractParts[ixHash.String()]
	if !ok {
		return nil, common.ErrTesseractPartsNotFound
	}

	return parts, nil
}

func (m *MockDB) SetTesseractParts(gridHash common.Hash, parts []byte) error {
	m.tesseractParts[gridHash.String()] = parts

	return nil
}

func (m *MockDB) SetTSGridLookup(tsHash common.Hash, gridHash common.Hash) error {
	if m.setTSGridLookupHook != nil {
		return m.setTSGridLookupHook()
	}

	m.gridHashByTSHash[tsHash.String()] = gridHash.Bytes()

	return nil
}

func (m *MockDB) GetTSGridLookup(tsHash common.Hash) ([]byte, error) {
	gridHash, ok := m.gridHashByTSHash[tsHash.String()]
	if !ok {
		return nil, common.ErrGridHashNotFound
	}

	return gridHash, nil
}

func (m *MockDB) GetInteractionsLookup(ixHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetInteractionsLookup(ixHash common.Hash, interactionsHash common.Hash) error {
	// TODO implement me
	panic("implement me")
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
	id common.Address,
	height uint64,
	tesseractHash common.Hash,
	accType common.AccountType,
	latticeExists bool,
	tesseractExists bool,
) (int32, bool, error) {
	if m.updateMetaInfoHook != nil {
		return m.updateMetaInfoHook()
	}

	m.accMetaInfos[id] = &common.AccountMetaInfo{
		Address:       id,
		Type:          accType,
		Height:        height,
		TesseractHash: tesseractHash,
		StateExists:   tesseractExists,
		LatticeExists: latticeExists,
	}

	return 8, true, nil
}

func (m *MockDB) GetAccMetaInfo(id common.Address) (*common.AccountMetaInfo, bool) {
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

func (m *MockDB) GetInteractions(gridHash common.Hash) ([]byte, error) {
	if ix, ok := m.dbStorage[gridHash.String()]; ok {
		return ix, nil
	}

	return nil, common.ErrKeyNotFound
}

func (m *MockDB) GetAccount(addr common.Address, hash common.Hash) ([]byte, error) {
	account, ok := m.accounts[hash]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (m *MockDB) SetInteractions(gridHash common.Hash, data []byte) error {
	if m.setInteractionsHook != nil {
		return m.setInteractionsHook()
	}

	m.dbStorage[gridHash.String()] = data

	return nil
}

func (m *MockDB) GetTesseractHeightEntry(addr common.Address, height uint64) ([]byte, error) {
	key := addr.Hex() + strconv.Itoa(int(height))

	data, ok := m.dbStorage[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetTesseractHeightEntry(addr common.Address, height uint64, hash common.Hash) error {
	if m.setTesseractHeightEntryHook != nil {
		return m.setTesseractHeightEntryHook()
	}

	key := addr.Hex() + strconv.Itoa(int(height))

	m.dbStorage[key] = hash.Bytes()

	return nil
}

func (m *MockDB) GetBalance(addr common.Address, hash common.Hash) ([]byte, error) {
	balance, ok := m.balances[hash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return balance, nil
}

func (m *MockDB) GetReceipts(gridHash common.Hash) ([]byte, error) {
	if m.setReceiptsHook != nil {
		return nil, m.setReceiptsHook()
	}

	data, ok := m.dbStorage[gridHash.String()]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetReceipts(gridHash common.Hash, data []byte) error {
	if m.setReceiptsHook != nil {
		return m.setReceiptsHook()
	}

	m.dbStorage[gridHash.String()] = data

	return nil
}

func (m *MockDB) GetContext(addr common.Address, contextHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetMerkleTreeEntry(address common.Address, prefix storage.Prefix, key []byte) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetMerkleTreeEntry(address common.Address, prefix storage.Prefix, key, value []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetMerkleTreeEntries(address common.Address, prefix storage.Prefix, entries map[string][]byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) WritePreImages(address common.Address, entries map[common.Hash][]byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetPreImage(address common.Address, hash common.Hash) ([]byte, error) {
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
	// TODO implement me
	panic("implement me")
}

type MockNetwork struct {
	kramaID id.KramaID
}

// mock network implementation

func (n *MockNetwork) SetKramaID(id id.KramaID) {
	n.kramaID = id
}

func (n *MockNetwork) GetKramaID() id.KramaID {
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

func (n *MockNetwork) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	// TODO implement me
	panic("implement me")
}

type MockExec struct {
	receipts                map[common.Hash]common.Receipts
	revertHook              func() error
	executeInteractionsHook func() (common.Receipts, error)
	clusterID               common.ClusterID
}

func (e *MockExec) Cleanup(clusterID common.ClusterID) {
	e.clusterID = clusterID
}

func (e *MockExec) SpawnExecutor() *compute.IxExecutor {
	sm := mockStateManager()

	return compute.NewManager(sm, hclog.NewNullLogger(), nil).SpawnExecutor()
}

func mockExec(t *testing.T) *MockExec {
	t.Helper()

	return &MockExec{
		receipts: make(map[common.Hash]common.Receipts),
	}
}

// mock execution implementation
func (e *MockExec) ExecuteInteractions(
	ixs common.Interactions,
	clusterID common.ClusterID,
	contextDelta common.ContextDelta,
) (common.Receipts, error) {
	if e.executeInteractionsHook != nil {
		return e.executeInteractionsHook()
	}

	if val, ok := e.receipts[ixs[0].Hash()]; ok {
		return val, nil
	}

	return nil, common.ErrInvalidInteractions
}

func (e *MockExec) Revert(clusterID common.ClusterID) error {
	if e.revertHook != nil {
		return e.revertHook()
	}

	return nil
}

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type AccHash struct {
	contextHash common.Hash
	stateHash   common.Hash
}

type MockStateManager struct {
	dirtyObjects        map[common.Address]*state.Object
	objects             map[common.Address]*state.Object
	context             map[common.Hash]*Context
	publicKeys          map[id.KramaID][]byte
	latestTesseracts    map[common.Address]*common.Tesseract
	dbTesseracts        map[common.Hash]*common.Tesseract
	registeredAccounts  map[common.Address]*AccHash
	cleanUp             map[common.Address]bool
	accountTypes        map[common.Address]common.AccountType
	flushedDirtyObjects map[common.Address]bool

	flushHook               func() error
	newAccountHook          func() (common.Hash, common.Hash, error)
	accountRegistrationHook func(hash common.Hash) (bool, error)
	createDirtyObjectHook   func() *state.Object
}

func (sm *MockStateManager) GetLatestStateObject(addr common.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetLogicIDs(addr common.Address, hash common.Hash) ([]common.LogicID, error) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) Revert(object *state.Object) error {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) GetNodeSet(ids []id.KramaID) (*common.NodeSet, error) {
	pks := make([][]byte, 0, len(ids))

	for _, v := range ids {
		pk, ok := sm.publicKeys[v]
		if !ok {
			return nil, common.ErrKeyNotFound
		}

		pks = append(pks, pk)
	}

	return common.NewNodeSet(ids, pks), nil
}

func mockStateManager() *MockStateManager {
	return &MockStateManager{
		dirtyObjects:        make(map[common.Address]*state.Object),
		objects:             make(map[common.Address]*state.Object),
		context:             make(map[common.Hash]*Context),
		publicKeys:          make(map[id.KramaID][]byte),
		latestTesseracts:    make(map[common.Address]*common.Tesseract),
		dbTesseracts:        make(map[common.Hash]*common.Tesseract),
		registeredAccounts:  make(map[common.Address]*AccHash),
		cleanUp:             make(map[common.Address]bool),
		accountTypes:        make(map[common.Address]common.AccountType),
		flushedDirtyObjects: make(map[common.Address]bool),
	}
}

func (sm *MockStateManager) isCleanup(addrs common.Address) bool {
	if _, ok := sm.cleanUp[addrs]; ok {
		return true
	}

	return false
}

func (sm *MockStateManager) CreateDirtyObject(addr common.Address, accType common.AccountType) *state.Object {
	if sm.createDirtyObjectHook != nil {
		return sm.createDirtyObjectHook()
	}

	if obj, ok := sm.dirtyObjects[addr]; ok {
		return obj
	}

	obj := state.NewStateObject(addr, mockCache(), new(state.Journal), mockDB(), common.Account{AccType: accType})
	sm.dirtyObjects[addr] = obj.Copy()

	return sm.dirtyObjects[addr]
}

func (sm *MockStateManager) GetDirtyObject(addr common.Address) (*state.Object, error) {
	return sm.dirtyObjects[addr], nil
}

func (sm *MockStateManager) setAccType(address common.Address, accountType common.AccountType) {
	sm.accountTypes[address] = accountType
}

func (sm *MockStateManager) GetAccTypeUsingStateObject(address common.Address) (common.AccountType, error) {
	accType, ok := sm.accountTypes[address]
	if !ok {
		return 0, errors.New("account type not found")
	}

	return accType, nil
}

func (sm *MockStateManager) DeleteStateObject(addr common.Address) {
	// TODO implement me
	panic("implement me")
}

func (sm *MockStateManager) FlushDirtyObject(addrs common.Address) error {
	if sm.flushHook != nil {
		return sm.flushHook()
	}

	sm.flushedDirtyObjects[addrs] = true

	return nil
}

func (sm *MockStateManager) getFlushedDirtyObject(addrs common.Address) bool {
	_, ok := sm.flushedDirtyObjects[addrs]

	return ok
}

func (sm *MockStateManager) GetLatestTesseract(addr common.Address, withInteractions bool) (*common.Tesseract, error) {
	ts, ok := sm.latestTesseracts[addr]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	copyTS := *ts // copy, so that stored tesseract wont't be modified

	if !withInteractions {
		copyTS = *copyTS.GetTesseractWithoutIxns()
	}

	return &copyTS, nil
}

func (sm *MockStateManager) insertContextNodes(
	ctxHash common.Hash,
	behaviouralNodes []id.KramaID,
	randomNodes ...id.KramaID,
) {
	sm.context[ctxHash] = &Context{
		behaviourNodes: behaviouralNodes,
		randomNodes:    randomNodes,
	}
}

func (sm *MockStateManager) GetContextByHash(
	addr common.Address,
	hash common.Hash,
) (
	common.Hash,
	[]id.KramaID,
	[]id.KramaID,
	error,
) {
	c, ok := sm.context[hash]

	if !ok {
		return common.NilHash, nil, nil, common.ErrContextStateNotFound
	}

	return hash, c.behaviourNodes, c.randomNodes, nil
}

func (sm *MockStateManager) GetPublicKeys(id ...id.KramaID) ([][]byte, error) {
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

func (sm *MockStateManager) Cleanup(addrs common.Address) {
	sm.cleanUp[addrs] = true
}

func (sm *MockStateManager) insertRegisteredAcc(addr common.Address) {
	sm.registeredAccounts[addr] = &AccHash{}
}

func (sm *MockStateManager) IsAccountRegistered(addr common.Address) (bool, error) {
	if sm.accountRegistrationHook != nil {
		return sm.accountRegistrationHook(common.NilHash)
	}

	_, ok := sm.registeredAccounts[addr]

	return ok, nil
}

func (sm *MockStateManager) IsAccountRegisteredAt(addr common.Address, tesseractHash common.Hash) (bool, error) {
	if sm.accountRegistrationHook != nil {
		return sm.accountRegistrationHook(tesseractHash)
	}

	_, ok := sm.registeredAccounts[addr]

	return ok, nil
}

func (sm *MockStateManager) FetchContextLock(ts *common.Tesseract) (*common.ICSNodeSet, error) {
	ics := common.NewICSNodeSet(6)

	contextLock, _ := ts.ContextLockByAddress(ts.Address())

	_, behaviourSet, randomSet, err := sm.GetContextByHash(common.NilAddress, contextLock.ContextHash)
	if err != nil {
		return nil, err
	}

	// fetching public keys
	behaviouralPK, err := sm.GetPublicKeys(behaviourSet...)
	if err != nil {
		return nil, err
	}

	randomPK, err := sm.GetPublicKeys(randomSet...)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(common.SenderBehaviourSet, common.NewNodeSet(behaviourSet, behaviouralPK))
	ics.UpdateNodeSet(common.SenderRandomSet, common.NewNodeSet(randomSet, randomPK))

	return ics, nil
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
		sm.latestTesseracts[ts.Address()] = ts
	}
}

func (sm *MockStateManager) InsertTesseractsInDB(t *testing.T, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		sm.dbTesseracts[ts.Hash()] = ts
	}
}

func (sm *MockStateManager) setPublicKey(id id.KramaID, pk []byte) {
	sm.publicKeys[id] = pk
}

type MockIXPool struct {
	reset map[common.Hash]bool
}

type MockSenatus struct {
	WalletCount           map[id.KramaID]int32
	UpdateWalletCountHook func() error
}

func (s *MockSenatus) UpdateWalletCount(peerID id.KramaID, delta int32) error {
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

func createCommitdataWithRandomGridHash(t *testing.T) common.CommitData {
	t.Helper()

	return common.CommitData{
		GridID: &common.TesseractGridID{
			Hash:  tests.RandomHash(t),
			Parts: &common.TesseractParts{},
		},
	}
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

func getIxParamsWithAddress(from common.Address, to common.Address) *CreateIxParams {
	return &CreateIxParams{
		ixDataCallback: func(ix *common.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
			ix.Input.Type = common.IxValueTransfer
		},
	}
}

func getIxParamsMapWithAddresses(
	from []common.Address,
	to []common.Address,
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

func getTestTesseractGrid(t *testing.T) *common.TesseractGridID {
	t.Helper()

	return &common.TesseractGridID{
		Hash: tests.RandomHash(t),
		Parts: &common.TesseractParts{
			Grid: make(map[common.Address]common.TesseractHeightAndHash),
		},
	}
}

func mockContextLock() map[common.Address]common.ContextLockInfo {
	return make(map[common.Address]common.ContextLockInfo)
}

func mockContextDelta() common.ContextDelta {
	return make(map[common.Address]*common.DeltaGroup)
}

func mockConsensusProof() common.PoXtData {
	return common.PoXtData{}
}

func mockCommitData() common.CommitData {
	return common.CommitData{}
}

func mockNetwork(t *testing.T) *MockNetwork {
	t.Helper()

	return &MockNetwork{}
}

func defaultCommitData() common.CommitData {
	commitData := mockCommitData()
	// vote-set is a bit array
	voteSet := common.ArrayOfBits{
		Size:     1,                 // represents node tsCount
		Elements: make([]uint64, 1), // each element holds eight votes
	}

	voteSet.Size = 5         // there are 5 ics nodes
	voteSet.Elements[0] = 31 // first 5 ics nodes voted yes

	commitData.VoteSet = voteSet.Copy()
	commitData.GridID = &common.TesseractGridID{
		Parts: &common.TesseractParts{},
	}

	return commitData
}

type createTesseractParams struct {
	address           common.Address
	height            uint64
	ixns              common.Interactions
	receipts          common.Receipts
	seal              []byte
	sealer            id.KramaID
	headerCallback    func(header *common.TesseractHeader)
	makeChainCallback func(header *common.TesseractHeader)
	bodyCallback      func(body *common.TesseractBody)
}

func createTesseract(t *testing.T, params *createTesseractParams) *common.Tesseract {
	t.Helper()

	if params == nil {
		params = &createTesseractParams{}
	}

	if params.address == common.NilAddress {
		params.address = tests.RandomAddress(t)
	}

	var interactionHash common.Hash

	if params.ixns != nil {
		hash, err := params.ixns.Hash()
		require.NoError(t, err)

		interactionHash = hash
	}

	header := &common.TesseractHeader{
		Address:     params.address,
		Height:      params.height,
		ContextLock: mockContextLock(),
		Extra:       defaultCommitData(),
	}
	body := &common.TesseractBody{
		ContextDelta:    mockContextDelta(),
		ConsensusProof:  mockConsensusProof(),
		InteractionHash: interactionHash,
	}

	if params.headerCallback != nil {
		params.headerCallback(header)
	}

	if params.bodyCallback != nil {
		params.bodyCallback(body)
	}

	if params.makeChainCallback != nil {
		params.makeChainCallback(header)
	}

	return common.NewTesseract(*header, *body, params.ixns, params.receipts, params.seal, params.sealer)
}

func createTesseracts(t *testing.T, count int, paramsMap map[int]*createTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	for i := 0; i < count; i++ {
		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func createTesseractsWithChain(t *testing.T, count int, paramsMap map[int]*createTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	tesseracts[0] = createTesseract(t, paramsMap[0])

	for i := 1; i < count; i++ {
		paramsMap[i].makeChainCallback = func(header *common.TesseractHeader) {
			hash := tesseracts[i-1].Hash()
			header.PrevHash = hash
		}

		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

// FIXME: move this method
func getAddresses(t *testing.T, count int) []common.Address {
	t.Helper()

	addresses := make([]common.Address, count)
	for i := 0; i < count; i++ {
		addresses[i] = tests.RandomAddress(t)
	}

	return addresses
}

func tesseractParamsWithICSClusterInfo(
	t *testing.T,
	ixns common.Interactions,
	icsInfo *common.ICSClusterInfo,
) *createTesseractParams {
	t.Helper()

	rawData, err := icsInfo.Bytes()
	require.NoError(t, err)

	return &createTesseractParams{
		ixns:           ixns,
		headerCallback: tests.HeaderCallbackWithGridHash(t),
		bodyCallback: func(body *common.TesseractBody) {
			body.ReceiptHash = tests.RandomHash(t)
			body.ConsensusProof.ICSHash = common.GetHash(rawData)
		},
	}
}

func tesseractParamsWithGridInfo(
	t *testing.T,
	address common.Address,
	stateHash, receiptHash common.Hash,
	clusterInfo *common.ICSClusterInfo,
	ixns []*common.Interaction,
	gridSize int32,
	clusterID common.ClusterID,
) *createTesseractParams {
	t.Helper()

	rawBytes, err := clusterInfo.Bytes()
	require.NoError(t, err)

	return &createTesseractParams{
		address: address,
		ixns:    ixns,
		headerCallback: func(header *common.TesseractHeader) {
			header.Extra.GridID = getTestTesseractGrid(t)
			header.Extra.GridID.Parts.Total = gridSize
			header.ClusterID = clusterID.String()
		},
		bodyCallback: func(body *common.TesseractBody) {
			body.ConsensusProof.ICSHash = common.GetHash(rawBytes)
			body.StateHash = stateHash
			body.ReceiptHash = receiptHash
			body.InteractionHash = tests.RandomHash(t) // make sure that nil hash won't be inserted as key
		},
	}
}

/*
func tesseractParamsForExecution(

	t *testing.T,
	address types.Address,
	contextHash, stateHash types.Hash,
	ixns []*types.Interaction,
	receipts types.Receipts,
	clusterInfo *types.ICSClusterInfo,
	gridSize int32,

	) *createTesseractParams {
		t.Helper()

		rawBytes, err := clusterInfo.Bytes()
		require.NoError(t, err)

		return &createTesseractParams{
			address: address,
			ixns:    ixns,
			headerCallback: func(header *types.TesseractHeader) {
				header.ContextLock[address] = types.ContextLockInfo{ContextHash: contextHash}
				header.Extra.GridID = getTestTesseractGrid(t)
				header.Extra.GridID.Parts.Total = gridSize
				header.Extra.CommitSignature = validCommitSign
			},
			bodyCallback: func(body *types.TesseractBody) {
				body.StateHash = stateHash
				body.ReceiptHash = getReceiptHash(t, receipts)
				body.InteractionHash = tests.RandomHash(t) // make sure that nil hash won't be inserted as key
				body.ContextDelta[address] = getDeltaGroup(t, 2, 2, 0)
				body.ConsensusProof.ICSHash = types.GetHash(rawBytes)
			},
		}
	}
*/
func tesseractParamsWithContextDelta(
	t *testing.T,
	address common.Address,
	behaviouralCount, randomCount, replacedCount int,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		address: address,
		bodyCallback: func(body *common.TesseractBody) {
			body.ContextDelta[address] = getDeltaGroup(t, behaviouralCount, randomCount, replacedCount)
		},
	}
}

/*
func tesseractParamsWithContextHash(
	t *testing.T,
	address types.Address,
	ctxHash types.Hash,
	sign []byte,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		address: address,
		headerCallback: func(header *types.TesseractHeader) {
			header.ContextLock[address] = types.ContextLockInfo{ContextHash: ctxHash}
			header.Extra.CommitSignature = sign
		},
		bodyCallback: func(body *types.TesseractBody) {
			body.ContextDelta[address] = getDeltaGroup(t, 1, 1, 1)
		},
	}
}
*/

func tesseractParamsWithReceiptHash(
	t *testing.T,
	receiptHash common.Hash,
	groupHash common.Hash,
	clusterID common.ClusterID,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		headerCallback: func(header *common.TesseractHeader) {
			header.GroupHash = groupHash
			header.ClusterID = clusterID.String()
		},
		bodyCallback: func(body *common.TesseractBody) {
			body.ReceiptHash = receiptHash
		},
	}
}

func tesseractParamsWithStateHash(
	t *testing.T,
	stateHash common.Hash,
	clusterID common.ClusterID,
) *createTesseractParams {
	t.Helper()

	return &createTesseractParams{
		headerCallback: func(header *common.TesseractHeader) {
			header.ClusterID = clusterID.String()
		},
		bodyCallback: func(body *common.TesseractBody) {
			body.StateHash = stateHash
		},
	}
}

func getTSParamsMapWithStateHash(t *testing.T, paramsCount int) map[int]*createTesseractParams {
	t.Helper()

	tsParamsMap := make(map[int]*createTesseractParams)

	for i := 0; i < paramsCount; i++ {
		j := i // we initialize new variable every time, to persist the value of cluster id when call back is called
		tsParamsMap[i] = &createTesseractParams{
			headerCallback: func(header *common.TesseractHeader) {
				header.ClusterID = "cluster-" + strconv.Itoa(j)
			},
			bodyCallback: func(body *common.TesseractBody) {
				body.StateHash = tests.RandomHash(t)
			},
		}
	}

	return tsParamsMap
}

func getHeaderCallbackWithCommitSign(commitSign []byte) func(header *common.TesseractHeader) {
	return func(header *common.TesseractHeader) {
		header.Extra.CommitSignature = commitSign
	}
}

func tesseractParamsWithCommitSign(commitSign []byte) *createTesseractParams {
	return &createTesseractParams{
		headerCallback: getHeaderCallbackWithCommitSign(commitSign),
	}
}

func smCallbackWithRegisteredAcc(address common.Address) func(sm *MockStateManager) {
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
	addresses := getAddresses(t, 4*tsCount) // for each interaction, sender and receiver addresses needed
	ixns := createIxns(t, 2*tsCount, getIxParamsMapWithAddresses(addresses[:2*tsCount], addresses[2*tsCount:]))

	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &createTesseractParams{
			ixns: ixns[i*2 : i*2+2], // allocate two interactions per tesseract
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
		WalletCount: make(map[id.KramaID]int32),
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

	ctx := context.Background()

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
		ctx,
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

func fetchContextFromLattice(t *testing.T, ts common.Tesseract, c *ChainManager) []id.KramaID {
	t.Helper()

	var (
		address = ts.Address()
		peers   = make([]id.KramaID, 0)
	)

	for {
		if len(peers) >= 10 {
			break
		}

		delta, _ := ts.GetContextDeltaByAddress(address)
		peers = append(peers, delta.BehaviouralNodes...)
		peers = append(peers, delta.RandomNodes...)

		contextLock, _ := ts.ContextLockByAddress(address)

		_, behaviour, random, err := c.sm.GetContextByHash(address, contextLock.ContextHash)
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.PrevHash().IsNil() {
			break
		}

		t, err := c.GetTesseract(ts.PrevHash(), false)
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

func getDeltaGroup(t *testing.T, behaviouralCount int, randomCount int, replaceCount int) *common.DeltaGroup {
	t.Helper()

	return &common.DeltaGroup{
		Role:             1,
		BehaviouralNodes: tests.GetTestKramaIDs(t, behaviouralCount),
		RandomNodes:      tests.GetTestKramaIDs(t, randomCount),
		ReplacedNodes:    tests.GetTestKramaIDs(t, replaceCount),
	}
}

func getContextLockInfo(contextHash common.Hash, tsHash common.Hash, height uint64) common.ContextLockInfo {
	return common.ContextLockInfo{
		ContextHash:   contextHash,
		Height:        height,
		TesseractHash: tsHash,
	}
}

func setContextDelta(body *common.TesseractBody, address common.Address, delta *common.DeltaGroup) {
	body.ContextDelta[address] = delta
}

func setContextLock(header *common.TesseractHeader, address common.Address, info common.ContextLockInfo) {
	header.ContextLock[address] = info
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

	err := db.SetTesseractHeightEntry(ts.Address(), ts.Height(), ts.Hash())
	require.NoError(t, err)
}

func getICSNodeset(t *testing.T, count int) *common.ICSNodeSet {
	t.Helper()

	ics := common.NewICSNodeSet(6)

	senderBehaviourSet := tests.GetTestKramaIDs(t, count)
	senderRandomSet := tests.GetTestKramaIDs(t, count)
	receiverBehaviourSet := tests.GetTestKramaIDs(t, count)
	receiverRandomSet := tests.GetTestKramaIDs(t, count)
	randomNodes := tests.GetTestKramaIDs(t, count)

	ics.UpdateNodeSet(common.SenderBehaviourSet, common.NewNodeSet(senderBehaviourSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(common.SenderRandomSet, common.NewNodeSet(senderRandomSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(common.ReceiverBehaviourSet, common.NewNodeSet(receiverBehaviourSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(common.ReceiverRandomSet, common.NewNodeSet(receiverRandomSet, getPublicKeys(t, count)))
	ics.UpdateNodeSet(common.RandomSet, common.NewNodeSet(randomNodes, getPublicKeys(t, count)))

	return ics
}

func getTestDirtyEntriesWithClusterInfo(
	t *testing.T,
	clusterInfo *common.ICSClusterInfo,
	count int,
) map[common.Hash][]byte {
	t.Helper()

	d := make(map[common.Hash][]byte, count)

	for i := 0; i < count; i++ {
		d[tests.RandomHash(t)] = tests.RandomAddress(t).Bytes()
	}

	if clusterInfo != nil {
		rawData, err := clusterInfo.Bytes()
		require.NoError(t, err)

		d[common.GetHash(rawData)] = rawData
	}

	return d
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
		FuelUsed:  big.NewInt(rand.Int63()),
		Hashes:    make(common.ReceiptAccHashes),
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

func getIxAndReceiptsWithStateHash(
	t *testing.T,
	hashes common.ReceiptAccHashes,
	ixCount int,
) ([]*common.Interaction, common.Receipts) {
	t.Helper()

	var (
		ixs      []*common.Interaction
		receipts = make(map[common.Hash]*common.Receipt, ixCount)
	)

	for i := 0; i < ixCount; i++ {
		ix, r := getIxAndReceipt(t)
		r.Hashes = hashes

		ixs = append(ixs, ix)

		receipts[ix.Hash()] = r
	}

	return ixs, receipts
}

func insertReceipts(t *testing.T, db store, gridHash common.Hash, receipts common.Receipts) {
	t.Helper()

	rawData, err := receipts.Bytes()
	require.NoError(t, err)

	err = db.SetReceipts(gridHash, rawData)
	require.NoError(t, err)
}

func getReceiptHash(t *testing.T, receipts common.Receipts) common.Hash {
	t.Helper()

	hash, err := receipts.Hash()
	require.NoError(t, err)

	return hash
}

func getHashes(t *testing.T, count int, nilHash bool) []common.Hash {
	t.Helper()

	var hashes []common.Hash

	for i := 0; i < count; i++ {
		if nilHash {
			hashes = append(hashes, common.NilHash)
		} else {
			hashes = append(hashes, tests.RandomHash(t))
		}
	}

	return hashes
}

func getTestClusterInfo(t *testing.T, nodeCount int) *common.ICSClusterInfo {
	t.Helper()

	info := new(common.ICSClusterInfo)
	info.RandomSet = tests.GetTestKramaIDs(t, nodeCount)
	info.ObserverSet = tests.GetTestKramaIDs(t, nodeCount)

	return info
}

// signs tesseract and stores signed bytes in seal also stores the public key associated with krama id
func signTesseract(t *testing.T, sm *MockStateManager, ts *common.Tesseract) {
	t.Helper()

	rawData, err := ts.Bytes()
	require.NoError(t, err)
	ids := tests.GetTestKramaIDs(t, 1)

	seal, pk := tests.SignBytes(t, rawData) // calculate the seal of tesseract
	ts.SetSeal(seal)                        // store seal in tesseract
	ts.SetSealer(ids[0])

	sm.setPublicKey(ids[0], pk) // store the public key for the signed sender
}

func setAccountType(sm *MockStateManager, accType common.AccountType, tesseracts ...*common.Tesseract) {
	for _, ts := range tesseracts {
		sm.setAccType(ts.Address(), accType)
	}
}

// waitForResponse waits for response on respChannel
// and checks if datatype of data received on channel is equal to datatype of data received as argument
func waitForResponse(t *testing.T, respChan chan result, data interface{}) interface{} {
	t.Helper()

	res := <-respChan
	require.NoError(t, res.err)

	require.Equal(t, reflect.TypeOf(res.data), reflect.TypeOf(data))

	return res.data
}

// handleMuxEvents sends the data to resp channel if it receives data on subscription channel
// sends time out error when context is closed
func handleMuxEvents(ctx context.Context, s *utils.Subscription, resp chan result) {
	for {
		select {
		case <-ctx.Done():
			resp <- result{data: nil, err: common.ErrTimeOut}

			return
		case data := <-s.Chan():
			resp <- result{data: data.Data, err: nil}

			return
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
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
	behaviouralContext []id.KramaID,
	randomContext []id.KramaID,
) common.AssetAccountSetupArgs {
	t.Helper()

	return common.AssetAccountSetupArgs{
		BehaviouralContext: behaviouralContext,
		RandomContext:      randomContext,
		AssetInfo:          &assetDetails,
	}
}

func getTestAssetCreationArgs(t *testing.T, allocationAddr common.Address) common.AssetCreationArgs {
	t.Helper()

	info := tests.GetRandomAssetInfo(t, common.NilAddress)

	if allocationAddr == common.NilAddress {
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

	manifest := "0x" + common.BytesToHex(tests.ReadManifest(t, "./../compute/manifests/erc20.json"))
	calldata := "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29e" +
		"c442dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"

	logic := common.LogicSetupArgs{
		Name: "staking-contract",

		Callsite: "Seeder!",
		Calldata: hexutil.Bytes(common.Hex2Bytes(calldata)),
		Manifest: hexutil.Bytes(common.Hex2Bytes(manifest)),

		BehaviouralContext: tests.GetTestKramaIDs(t, 1),
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

	file, err := ioutil.TempFile(dir, "*config.json")
	require.NoError(t, err)

	_, err = file.Write(data)
	require.NoError(t, err)

	return file.Name()
}

func getTestAccountWithAccType(t *testing.T, accType common.AccountType) common.AccountSetupArgs {
	t.Helper()

	ids := tests.GetTestKramaIDs(t, 4)

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

func getTestAccountWithAddress(t *testing.T, address common.Address) common.AccountSetupArgs {
	t.Helper()

	ids := tests.GetTestKramaIDs(t, 4)

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
	owner common.Address,
	address []common.Address,
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
	address common.Address,
	accType common.AccountType,
	moiID string,
	behNodes []id.KramaID,
	randNodes []id.KramaID,
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

func validateNodeInclusivity(t *testing.T, senatus *MockSenatus, ctxDelta common.ContextDelta) {
	t.Helper()

	for _, deltaGroup := range ctxDelta {
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
}

func checkForTSInCache(t *testing.T, c *ChainManager, ts *common.Tesseract, exists bool) {
	t.Helper()

	if exists { // check if cache has both tesseract address and hash
		hash, ok := c.tesseracts.Get(ts.Address())
		require.True(t, ok)
		require.Equal(t, ts.Hash(), hash)

		cachedTS, ok := c.tesseracts.Get(ts.Hash())
		require.True(t, ok)
		require.Equal(t, ts.GetTesseractWithoutIxns(), cachedTS)

		return
	}

	_, ok := c.tesseracts.Get(ts.Address()) // make sure tesseract address not present in cache
	require.False(t, ok)
}

func checkForCanonicalTSInDB(t *testing.T, c *ChainManager, expectedTS *common.Tesseract) {
	t.Helper()

	// check if tesseract matches
	rawData, err := c.db.GetTesseract(expectedTS.Hash())
	require.NoError(t, err)

	canonicalTS := new(common.CanonicalTesseract)
	err = canonicalTS.FromBytes(rawData)
	require.NoError(t, err)

	require.Equal(t, expectedTS.Canonical(), canonicalTS)
}

func checkForIxnsInDB(t *testing.T, c *ChainManager, expectedTS *common.Tesseract) {
	t.Helper()

	gridHash := expectedTS.GridHash()

	// check if tesseract matches
	rawData, err := c.db.GetTesseract(gridHash)
	require.NoError(t, err)

	actualIxns := new(common.Interactions)
	err = actualIxns.FromBytes(rawData)
	require.NoError(t, err)

	require.Equal(t, expectedTS.Interactions(), *actualIxns)
}

func checkForTesseractByHeight(t *testing.T, c *ChainManager, ts *common.Tesseract) {
	t.Helper()

	// check if tesseract height key added
	_, err := c.db.GetTesseractHeightEntry(ts.Address(), ts.Height())
	require.NoError(t, err)
}

func checkIfAccMetaInfoMatches(
	t *testing.T,
	accMetaInfo *common.AccountMetaInfo,
	ts *common.Tesseract,
	accType common.AccountType,
	latticeExists bool,
	stateExists bool,
) {
	t.Helper()

	require.Equal(t, ts.Address(), accMetaInfo.Address)
	require.Equal(t, ts.Height(), accMetaInfo.Height)
	require.Equal(t, ts.Hash(), accMetaInfo.TesseractHash)
	require.Equal(t, accType, accMetaInfo.Type)
	require.Equal(t, latticeExists, accMetaInfo.LatticeExists)
	require.Equal(t, stateExists, accMetaInfo.StateExists)
}

func checkIfTesseractAdded(
	t *testing.T,
	c *ChainManager,
	db *MockDB,
	sm *MockStateManager,
	senatus *MockSenatus,
	ixPool *MockIXPool,
	args testTSArgs,
	count int,
	latticeExists bool,
	accType common.AccountType,
	ts *common.Tesseract,
) {
	t.Helper()

	// cache should be checked first
	// otherwise tesseract may get added to cache while doing other validations
	checkForTSInCache(t, c, ts, args.cache)
	checkForCanonicalTSInDB(t, c, ts)
	checkForIxnsInDB(t, c, ts)
	validateNodeInclusivity(t, senatus, ts.ContextDelta())
	checkForTesseractByHeight(t, c, ts)

	if args.stateExists {
		found := sm.getFlushedDirtyObject(ts.Address())
		require.True(t, found)
	}

	// check if acc meta info inserted
	accMetaInfo, found := db.GetAccMetaInfo(ts.Address())
	require.True(t, found)

	checkIfAccMetaInfoMatches(
		t,
		accMetaInfo,
		ts,
		accType,
		latticeExists,
		args.tesseractExists,
	)

	require.True(t, sm.isCleanup(ts.Address())) // check if dirty objects cleaned up
	require.True(t, ixPool.IsReset(ts.Hash()))  // check if interactions are reset
}

/*
func checkIfICSNodeSetMatches(
	t *testing.T,
	ics *common.ICSNodeSet,
	senderBehaviourSet *common.NodeSet,
	senderRandomSet *common.NodeSet,
	randomSet *common.NodeSet,
) {
	t.Helper()

	require.Equal(t,
		ics.Nodes[common.SenderBehaviourSet],
		senderBehaviourSet,
	)
	require.Equal(t,
		ics.Nodes[common.SenderRandomSet],
		senderRandomSet,
	)
	require.Equal(t,
		ics.Nodes[common.RandomSet],
		randomSet,
	)
}
*/

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

/*
func checkForClusterInfo(t *testing.T, db db, clusterInfo *types.ICSClusterInfo) {
	t.Helper()

	rawInfo, err := clusterInfo.Bytes()
	require.NoError(t, err)

	fetchedData, err := db.ReadEntry(types.GetHash(rawInfo).Bytes())
	require.NoError(t, err)

	require.Equal(t, rawInfo, fetchedData)
}
*/

func checkForDirtyEntries(t *testing.T, db store, dirtyEntries map[common.Hash][]byte) {
	t.Helper()

	// check if dirty entries inserted in db
	for k, v := range dirtyEntries {
		val, err := db.ReadEntry(k.Bytes())
		require.NoError(t, err)
		require.Equal(t, v, val)
	}
}

func checkForAddedTesseracts(
	t *testing.T,
	c *ChainManager,
	ts []*common.Tesseract,
	accType common.AccountType,
) {
	t.Helper()

	checkIfTesseractsAdded(
		t,
		accType,
		false,
		true,
		c.db.(*MockDB), //nolint:forcetypeassert
		ts...,
	)
}

func checkForOrphanTSInCache(t *testing.T, c *ChainManager, expectedTS *common.Tesseract) {
	t.Helper()

	// check if cache has both tesseract address and hash
	actualTS, ok := c.orphanTesseracts.Get(expectedTS.Hash())
	require.True(t, ok)
	require.Equal(t, expectedTS, actualTS)
}

/*
func fetchContextWithNodes(t *testing.T, c *ChainManager, ts *types.Tesseract, randomset []id.KramaID) []id.KramaID {
	t.Helper()

	fetchContext := fetchContextFromLattice(t, *ts, c)

	return append(fetchContext, randomset...)
}


func checkIfTSSyncEventsMatch(
	t *testing.T,
	ts *types.Tesseract,
	event utils.TesseractSyncEvent,
	randomContext []id.KramaID,
) {
	t.Helper()

	require.Equal(t, ts, event.Tesseract)
	require.Equal(t, randomContext, event.Context)
}

func validateTSSyncEvent(
	t *testing.T,
	c *ChainManager,
	ts *types.Tesseract,
	resp chan result,
	info *types.ICSClusterInfo,
) {
	t.Helper()

	eventData := waitForResponse(t, resp, utils.TesseractSyncEvent{}) // waits for eventData from goroutine
	checkIfTSSyncEventsMatch(
		t,
		ts,
		getTesseractSyncEvent(t, eventData), // convert interface type to concrete type
		fetchContextWithNodes(t, c, ts, info.RandomSet),
	)
}
*/

func validateTSAddedEvent(t *testing.T, tsAddedResp chan result, ts *common.Tesseract) {
	t.Helper()

	data := waitForResponse(t, tsAddedResp, utils.TesseractAddedEvent{}) // waits for data from goroutine
	event := getTesseractAddedEvent(t, data)                             // convert interface type to concrete type
	require.Equal(t, ts, event.Tesseract)
}

func checkIfTesseractsAdded(
	t *testing.T,
	accType common.AccountType,
	latticeExists, stateExists bool,
	db *MockDB,
	ts ...*common.Tesseract,
) {
	t.Helper()

	for i := 0; i < len(ts); i++ {
		// check if acc meta info inserted
		accMetaInfo, found := db.GetAccMetaInfo(ts[i].Address())
		require.True(t, found)

		checkIfAccMetaInfoMatches(
			t,
			accMetaInfo,
			ts[i],
			accType, // check accType to make sure tesseract added with state
			latticeExists,
			stateExists,
		)
	}
}

/*

func checkForAccountCreation(t *testing.T, accountInfo AccountInfo, accSetupArgs gtypes.AccountSetupArgs) {
	t.Helper()

	require.Equal(t, accountInfo.AccountType, accSetupArgs.AccType)
	require.Equal(t, accountInfo.Address, accSetupArgs.Address.Hex())
	require.Equal(t, accountInfo.MOIId, accSetupArgs.MoiID)
	require.Equal(t, accountInfo.BehaviourContext, utils.KramaIDToString(accSetupArgs.BehaviouralContext))
	require.Equal(t, accountInfo.RandomContext, utils.KramaIDToString(accSetupArgs.RandomContext))

	for i, assetDetails := range accountInfo.AssetDetails {
		require.Equal(t, types.AssetKind(assetDetails.Type), accSetupArgs.Assets[i].Type)
		require.Equal(t, assetDetails.Symbol, accSetupArgs.Assets[i].Symbol)
		require.Equal(t, assetDetails.Owner, accSetupArgs.Assets[i].Owner.Hex())
		require.Equal(t, assetDetails.TotalSupply, accSetupArgs.Assets[i].Supply.Uint64())
		require.Equal(t, assetDetails.Dimension, accSetupArgs.Assets[i].Dimension)
		require.Equal(t, assetDetails.Decimals, accSetupArgs.Assets[i].Decimals)
		require.Equal(t, assetDetails.IsFungible, accSetupArgs.Assets[i].IsFungible)
		require.Equal(t, assetDetails.IsMintable, accSetupArgs.Assets[i].IsMintable)
		require.Equal(t, assetDetails.IsTransferable, accSetupArgs.Assets[i].IsTransferable)
		require.Equal(t, assetDetails.LogicID, accSetupArgs.Assets[i].LogicID.String())
	}

	for _, balance := range accountInfo.Balances {
		amount, ok := accSetupArgs.Balances[types.AssetID(strings.TrimPrefix(balance.AssetID, "0x"))]
		require.True(t, ok)
		require.Equal(t, balance.Amount, amount.Int64())
	}
}
*/

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
	c *ChainManager,
	address common.Address,
) {
	t.Helper()

	hashData, ok := c.tesseracts.Get(address) // fetch tesseract hash from cache
	require.True(t, ok)

	tsHash, ok := hashData.(common.Hash)
	require.True(t, ok)

	ok = c.db.HasTesseract(tsHash)
	require.True(t, ok)

	ts, err := c.GetTesseract(tsHash, false)
	require.NoError(t, err)

	require.Equal(t, address, ts.Address())
}

func checkSargaStorageEntry(t *testing.T, obj *state.Object, addr common.Address) {
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
	address common.Address,
	behavioural, random []id.KramaID,
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
	assetID common.AssetID,
	expectedAssetDescriptor []byte,
) {
	t.Helper()

	registry, err := so.Registry()
	require.NoError(t, err)

	actualAssetDescriptor, ok := registry.Entries[assetID.String()]
	require.True(t, ok)

	require.Equal(t, expectedAssetDescriptor, actualAssetDescriptor)
}

func checkForAllocations(
	t *testing.T,
	stateObjects map[common.Address]*state.Object,
	assetInfo *common.AssetCreationArgs,
	assetID common.AssetID,
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
