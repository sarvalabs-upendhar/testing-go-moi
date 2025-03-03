package lattice

import (
	"context"
	"encoding/json"
	"math/rand"
	"strconv"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/require"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

// MockDB is an in-memory key-value database used for testing purposes
type MockDB struct {
	dbStorage                   map[string][]byte
	accounts                    map[common.Hash][]byte
	balances                    map[common.Hash][]byte
	accMetaInfos                map[identifiers.Identifier]*common.AccountMetaInfo
	tesseracts                  map[common.Hash]*common.Tesseract
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

func mockDB() *MockDB {
	return &MockDB{
		dbStorage:      make(map[string][]byte),
		accMetaInfos:   make(map[identifiers.Identifier]*common.AccountMetaInfo),
		TSHashByIxHash: make(map[string][]byte),
		tesseracts:     make(map[common.Hash]*common.Tesseract),
	}
}

func (m *MockDB) GetAccountKeys(id identifiers.Identifier, stateHash common.Hash) ([]byte, error) {
	panic("implement me")
}

func (m *MockDB) HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool {
	accMetaInfo, err := m.GetAccountMetaInfo(id)
	if err != nil {
		return false
	}

	if accMetaInfo.Height < height {
		return false
	}

	return true
}

func (m *MockDB) GetDeeds(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	// TODO implement me
	metaInfo, ok := m.accMetaInfos[id]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return metaInfo, nil
}

func (m *MockDB) GetTesseract(hash common.Hash, withInteractions, withCommitInfo bool) (*common.Tesseract, error) {
	// FIXME: check for commitInfo
	ts, ok := m.tesseracts[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract wont't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	if !withCommitInfo {
		tsCopy = *tsCopy.GetTesseractWithoutCommitInfo()
	}

	return &tsCopy, nil
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
	id identifiers.Identifier,
	height uint64,
	tesseractHash common.Hash,
	stateHash common.Hash,
	contextHash common.Hash,
	consensusNodesHash common.Hash,
	inheritedAccount identifiers.Identifier,
	commitHash common.Hash,
	accType common.AccountType,
	shouldUpdateContextSetPosition bool,
	positionInContextSet int,
) (int32, bool, error) {
	if m.updateMetaInfoHook != nil {
		return m.updateMetaInfoHook()
	}

	m.accMetaInfos[id] = &common.AccountMetaInfo{
		ID:                 id,
		Type:               accType,
		Height:             height,
		TesseractHash:      tesseractHash,
		StateHash:          stateHash,
		ContextHash:        contextHash,
		ConsensusNodesHash: consensusNodesHash,
		CommitHash:         commitHash,
	}

	if shouldUpdateContextSetPosition {
		m.accMetaInfos[id].PositionInContextSet = positionInContextSet
	}

	return 8, true, nil
}

func (m *MockDB) GetAccMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, bool) {
	val, ok := m.accMetaInfos[id]

	return val, ok
}

func (m *MockDB) GetRawTesseract(hash common.Hash) ([]byte, error) {
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

func (m *MockDB) GetAccount(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
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

func (m *MockDB) GetTesseractHeightEntry(id identifiers.Identifier, height uint64) ([]byte, error) {
	key := id.Hex() + strconv.Itoa(int(height))

	data, ok := m.dbStorage[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (m *MockDB) SetTesseractHeightEntry(id identifiers.Identifier, height uint64, hash common.Hash) error {
	if m.setTesseractHeightEntryHook != nil {
		return m.setTesseractHeightEntryHook()
	}

	key := id.Hex() + strconv.Itoa(int(height))

	m.dbStorage[key] = hash.Bytes()

	return nil
}

func (m *MockDB) GetBalance(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
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

func (m *MockDB) GetContext(id identifiers.Identifier, contextHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetMerkleTreeEntry(id identifiers.Identifier, prefix storage.PrefixTag, key []byte) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetMerkleTreeEntry(id identifiers.Identifier, prefix storage.PrefixTag, key, value []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetMerkleTreeEntries(
	id identifiers.Identifier,
	prefix storage.PrefixTag,
	entries map[string][]byte,
) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) WritePreImages(id identifiers.Identifier, entries map[common.Hash][]byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetPreImage(id identifiers.Identifier, hash common.Hash) ([]byte, error) {
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

func (m *MockDB) InsertTesseractsInDB(t *testing.T, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		m.tesseracts[ts.Hash()] = ts
	}
}

type mockBatchWriter struct {
	dbStorage map[string][]byte
	dirty     map[string][]byte
	setHook   func() error
	flushHook func() error
}

func (bw *mockBatchWriter) Delete(key []byte) error {
	panic("implement me")
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

type MockIXPool struct {
	reset          map[common.Hash]bool
	removedObjects map[identifiers.Identifier]struct{}
}

func (i *MockIXPool) RemoveCachedObject(id identifiers.Identifier) {
	i.removedObjects[id] = struct{}{}
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
	Sign           []byte
}

func createIX(t *testing.T, params *CreateIxParams) *common.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &common.IxData{
		Participants: []common.IxParticipant{},
	}

	if params.ixDataCallback != nil {
		params.ixDataCallback(data)
	}

	if data.Sender.ID == identifiers.Nil {
		data.Sender.ID = tests.RandomIdentifier(t)
	}

	tests.AppendParticipantsInIxData(t, data)

	if len(params.Sign) == 0 {
		params.Sign = []byte{}
	}

	ix, err := common.NewInteraction(*data, []common.Signature{{
		Signature: make([]byte, 0),
	}})
	require.NoError(t, err)

	return ix
}

func createIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) common.Interactions {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make([]*common.Interaction, count)
	for i := 0; i < count; i++ {
		ixns[i] = createIX(t, paramsMap[i])
	}

	return common.NewInteractionsWithLeaderCheck(false, ixns...)
}

func getIxParamsWithID(t *testing.T, from identifiers.Identifier, to identifiers.Identifier) *CreateIxParams {
	t.Helper()

	return &CreateIxParams{
		ixDataCallback: func(ix *common.IxData) {
			ix.Sender.ID = from
			ix.IxOps = []common.IxOpRaw{
				{
					Type:    common.IxAssetCreate,
					Payload: tests.CreateRawAssetCreatePayload(t),
				},
			}
		},
	}
}

func getIxParamsMapWithIDs(
	t *testing.T,
	from []identifiers.Identifier,
	to []identifiers.Identifier,
) map[int]*CreateIxParams {
	t.Helper()

	ixParams := make(map[int]*CreateIxParams, len(from))

	for i := 0; i < len(from); i++ {
		ixParams[i] = getIxParamsWithID(t, from[i], to[i])
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

type CreateChainParams struct {
	db                   *MockDB
	senatus              *MockSenatus
	ixPool               *MockIXPool
	dbCallback           func(db *MockDB)
	ixPoolCallback       func(ixPool *MockIXPool)
	senatusCallback      func(senatus *MockSenatus)
	networkCallback      func(network *MockNetwork)
	chainManagerCallback func(c *ChainManager)
}

func mockSenatus(t *testing.T) *MockSenatus {
	t.Helper()

	return &MockSenatus{
		WalletCount: make(map[kramaid.KramaID]int32),
	}
}

func mockIXPool() *MockIXPool {
	return &MockIXPool{
		reset:          make(map[common.Hash]bool),
		removedObjects: make(map[identifiers.Identifier]struct{}),
	}
}

func createTestChainManager(t *testing.T, params *CreateChainParams) *ChainManager {
	t.Helper()

	if params == nil {
		params = &CreateChainParams{}
	}

	var (
		db      = mockDB()
		senatus = mockSenatus(t)
		ixPool  = mockIXPool()
		network = mockNetwork(t)
	)

	if params.senatus != nil {
		senatus = params.senatus
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

	if params.senatusCallback != nil {
		params.senatusCallback(senatus)
	}

	if params.networkCallback != nil {
		params.networkCallback(network)
	}

	if params.dbCallback != nil {
		params.dbCallback(db)
	}

	c, err := NewChainManager(
		db,
		hclog.NewNullLogger(),
		&utils.TypeMux{},
		network,
		ixPool,
		mockCache(),
		senatus,
		NilMetrics(),
	)
	require.NoError(t, err)

	if params.chainManagerCallback != nil {
		params.chainManagerCallback(c)
	}

	return c
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

func getDeltaGroup(t *testing.T, consensusNodeCount int, replaceCount int) *common.DeltaGroup {
	t.Helper()

	return &common.DeltaGroup{
		ConsensusNodes: tests.RandomKramaIDs(t, consensusNodeCount),
		ReplacedNodes:  tests.RandomKramaIDs(t, replaceCount),
	}
}

func insertTesseractsInCache(t *testing.T, c *ChainManager, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		c.tesseracts.Add(ts.Hash(), ts)
	}
}

func insertTesseractByHeight(t *testing.T, db store, ts *common.Tesseract) {
	t.Helper()

	for id, s := range ts.Participants() {
		err := db.SetTesseractHeightEntry(id, s.Height, ts.Hash())
		require.NoError(t, err)
	}
}

func getReceipt(ixHash common.Hash) *common.Receipt {
	return &common.Receipt{
		IxHash:   ixHash,
		FuelUsed: rand.Uint64(),
		IxOps: []*common.IxOpResult{
			{
				IxType: 1,
				Data:   make(json.RawMessage, 0),
			},
		},
	}
}

func getIX(t *testing.T) *common.Interaction {
	t.Helper()

	return createIX(
		t,
		getIxParamsWithID(t, tests.RandomIdentifier(t), tests.RandomIdentifier(t)),
	)
}

func getIxAndReceipts(t *testing.T, ixCount int) ([]*common.Interaction, common.Receipts) {
	t.Helper()

	var ixs []*common.Interaction

	receipts := make(map[common.Hash]*common.Receipt, ixCount)

	for i := 0; i < ixCount; i++ {
		ix := getIX(t)
		r := getReceipt(ix.Hash())
		ixs = append(ixs, ix)
		receipts[ix.Hash()] = r
	}

	return ixs, receipts
}

func getHexEntries(t *testing.T, count int) []string {
	t.Helper()

	entries := make([]string, count)

	for i := 0; i < count; i++ {
		entries[i] = tests.RandomHash(t).Hex()
	}

	return entries
}

func getTesseractParamsMapWithIxns(t *testing.T, tsCount int) map[int]*tests.CreateTesseractParams {
	t.Helper()

	tesseractParams := make(map[int]*tests.CreateTesseractParams, tsCount)
	ids := tests.GetIdentifiers(t, 4*tsCount) // for each interaction, sender and receiver ids needed
	ixns := createIxns(t, 2*tsCount, getIxParamsMapWithIDs(t, ids[:2*tsCount], ids[2*tsCount:]))

	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &tests.CreateTesseractParams{
			// allocate two interactions per tesseract
			Ixns: common.NewInteractionsWithLeaderCheck(false, ixns.IxList()[i*2:i*2+2]...),
		}
		tesseractParams[i].CommitInfo = &common.CommitInfo{
			Operator: tests.RandomKramaID(t, 0),
		}
	}

	return tesseractParams
}

func insertReceipts(t *testing.T, db store, tsHash common.Hash, receipts common.Receipts) {
	t.Helper()

	rawData, err := receipts.Bytes()
	require.NoError(t, err)

	err = db.SetReceipts(tsHash, rawData)
	require.NoError(t, err)
}

// getTesseractAddedEvent extracts TesseractAddedEvent from interface
func getTesseractAddedEvent(t *testing.T, data interface{}) utils.TesseractAddedEvent {
	t.Helper()

	event, ok := data.(utils.TesseractAddedEvent)
	require.True(t, ok)

	return event
}

// validation

func validateDeltaGroup(t *testing.T, senatus *MockSenatus, deltaGroup *common.DeltaGroup) {
	t.Helper()

	for _, kramaID := range deltaGroup.ConsensusNodes {
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
	id identifiers.Identifier,
	participant common.State,
	isPresent bool,
) {
	t.Helper()

	// check if tesseract height key added
	_, err := c.db.GetTesseractHeightEntry(id, participant.Height)

	if isPresent {
		require.NoError(t, err)

		return
	}

	require.Equal(t, common.ErrKeyNotFound, err)
}

func checkIfAccMetaInfoMatches(
	t *testing.T,
	accMetaInfo *common.AccountMetaInfo,
	id identifiers.Identifier,
	height uint64,
	tsHash common.Hash,
	accType common.AccountType,
) {
	t.Helper()

	require.Equal(t, id, accMetaInfo.ID)
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
	require.Equal(t, 0, cachedTS.Interactions().Len())
}

func validateTSAddedEvent(t *testing.T, tsAddedResp chan tests.Result, ts *common.Tesseract) {
	t.Helper()

	data := tests.WaitForResponse(t, tsAddedResp, utils.TesseractAddedEvent{}) // waits for data from goroutine
	event := getTesseractAddedEvent(t, data)                                   // convert interface type to concrete type
	require.Equal(t, ts, event.Tesseract)
}
