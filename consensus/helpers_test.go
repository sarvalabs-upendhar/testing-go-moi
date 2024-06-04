package consensus

import (
	"context"
	"encoding/json"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/rpc"
	"github.com/sarvalabs/go-moi/state"
)

type MockServer struct {
	id          kramaid.KramaID
	subscribers map[string]context.Context
	peers       []kramaid.KramaID
	peersLock   sync.RWMutex
}

func newMockServer() *MockServer {
	return &MockServer{
		subscribers: make(map[string]context.Context),
		peers:       make([]kramaid.KramaID, 0),
		peersLock:   sync.RWMutex{},
	}
}

func (m *MockServer) Broadcast(topic string, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockServer) Unsubscribe(topic string) error {
	delete(m.subscribers, topic)

	return nil
}

func (m *MockServer) Subscribe(
	ctx context.Context,
	topic string,
	validator utils.WrappedVal,
	defaultValidator bool,
	handler func(msg *pubsub.Message) error,
) error {
	m.subscribers[topic] = ctx

	return nil
}

func (m *MockServer) StartNewRPCServer(protocol protocol.ID, tag string) *rpc.Client {
	return nil
}

func (m *MockServer) RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error {
	return nil
}

func (m *MockServer) GetKramaID() kramaid.KramaID {
	return m.id
}

func (m *MockServer) ConnectPeerByKramaID(kramaID kramaid.KramaID) error {
	for _, peer := range m.peers {
		if peer == kramaID {
			return common.ErrConnectionExists
		}
	}

	m.peers = append(m.peers, kramaID)

	return nil
}

func (m *MockServer) DisconnectPeerByKramaID(kramaID kramaid.KramaID) error {
	m.peersLock.Lock()
	defer m.peersLock.Unlock()

	indexOf := func(peerID kramaid.KramaID) int {
		for i, peer := range m.peers {
			if peer == kramaID {
				return i
			}
		}

		return -1
	}

	if index := indexOf(kramaID); index != -1 {
		m.peers = append(m.peers[:index], m.peers[index+1:]...)
	}

	return nil
}

func mockCache() *lru.Cache {
	cache, _ := lru.New(1200)

	return cache
}

type MockExec struct {
	accountHashes           common.AccStateHashes
	executeInteractionsHook func() (common.AccStateHashes, error)
}

func NewMockExec() *MockExec {
	return new(MockExec)
}

// mock execution implementation
func (e *MockExec) ExecuteInteractions(
	ts *state.Transition,
	ixns common.Interactions,
	exec *common.ExecutionContext,
) (common.AccStateHashes, error) {
	if e.executeInteractionsHook != nil {
		return e.executeInteractionsHook()
	}

	return e.accountHashes, nil
}

type MockStateManager struct {
	accountRegistration map[identifiers.Address]bool
	participantsContext map[identifiers.Address]struct {
		contextHash common.Hash
		beSet       *common.NodeSet
		rSet        *common.NodeSet
	}
	accMetaInfo map[identifiers.Address]*common.AccountMetaInfo

	GetAccountMetaInfoHook func() error
	IsInitialTesseractHook func() error
	isSealValid            func() bool
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		accMetaInfo:         make(map[identifiers.Address]*common.AccountMetaInfo),
		accountRegistration: make(map[identifiers.Address]bool),
		participantsContext: make(map[identifiers.Address]struct {
			contextHash common.Hash
			beSet       *common.NodeSet
			rSet        *common.NodeSet
		}),
	}
}

func (ms *MockStateManager) GetICSParticipants(
	ixns common.Interactions,
) (map[identifiers.Address]common.IxParticipant, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) LoadTransitionObjects(
	ps map[identifiers.Address]common.IxParticipant,
) (*state.Transition, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) CreateStateObject(address identifiers.Address,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	stateObject := state.NewStateObject(address,
		mockCache(),
		nil,
		nil,
		common.Account{AccType: accountType},
		state.NilMetrics(),
		isGenesis,
	)

	return stateObject
}

func (ms *MockStateManager) IsInitialTesseract(ts *common.Tesseract, addr identifiers.Address) (bool, error) {
	if ms.IsInitialTesseractHook != nil {
		return false, ms.IsInitialTesseractHook()
	}

	if ts.Height(ts.AnyAddress()) == 0 {
		return true, nil
	}

	return false, nil
}

func (ms *MockStateManager) IsSealValid(ts *common.Tesseract) (bool, error) {
	if ms.isSealValid != nil {
		return ms.isSealValid(), nil
	}

	return true, nil
}

func (ms *MockStateManager) RemoveCacheObject(addr identifiers.Address) {
	panic("implement me")
}

func (ms *MockStateManager) addNodeSet(
	t *testing.T,
	addr identifiers.Address,
	contextHash common.Hash,
	beSet, rSet *common.NodeSet,
) {
	t.Helper()

	ms.participantsContext[addr] = struct {
		contextHash common.Hash
		beSet       *common.NodeSet
		rSet        *common.NodeSet
	}{contextHash: contextHash, beSet: beSet, rSet: rSet}
}

func (ms *MockStateManager) addAccMetaInfo(t *testing.T, addr identifiers.Address, info *common.AccountMetaInfo) {
	t.Helper()

	ms.accMetaInfo[addr] = info
}

func (ms *MockStateManager) GetDirtyObject(address identifiers.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) CreateDirtyObject(
	address identifiers.Address,
	accountType common.AccountType,
) *state.Object {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetEmptyStateObject() *state.Object {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetStateObjectByHash(
	addr identifiers.Address,
	hash common.Hash,
) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) FetchInteractionContext(
	ctx context.Context,
	ix *common.Interaction,
) (map[identifiers.Address]common.Hash, []*common.NodeSet, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) FetchLatestParticipantContext(addr identifiers.Address) (
	latestContextHash common.Hash,
	behaviouralSet, randomSet *common.NodeSet,
	err error,
) {
	info, ok := ms.participantsContext[addr]
	if !ok {
		return common.NilHash, nil, nil, common.ErrContextStateNotFound
	}

	return info.contextHash, info.beSet, info.rSet, nil
}

func (ms *MockStateManager) Cleanup(addr identifiers.Address) {}

func (ms *MockStateManager) GetPublicKeys(ctx context.Context, ids ...kramaid.KramaID) (keys [][]byte, err error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error) {
	if ms.GetAccountMetaInfoHook != nil {
		return nil, ms.GetAccountMetaInfoHook()
	}

	info, ok := ms.accMetaInfo[addr]
	if !ok {
		return nil, common.ErrFetchingAccMetaInfo
	}

	return info, nil
}

func (ms *MockStateManager) GetLatestStateObject(addr identifiers.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) IsAccountRegistered(addr identifiers.Address) (bool, error) {
	_, ok := ms.accountRegistration[addr]

	return ok, nil
}

type MockDB struct {
	accMetaInfo map[identifiers.Address]bool
}

func NewMockDB() *MockDB {
	return &MockDB{
		accMetaInfo: make(map[identifiers.Address]bool),
	}
}

func (m *MockDB) setAccMetaInfoAt(addr identifiers.Address) {
	m.accMetaInfo[addr] = true
}

func (m *MockDB) HasAccMetaInfoAt(addr identifiers.Address, height uint64) bool {
	_, ok := m.accMetaInfo[addr]

	return ok
}

type MockChainManager struct {
	tesseractByAddress map[identifiers.Address]*common.Tesseract
	tesseractByHash    map[common.Hash]*common.Tesseract

	addTesseractHook func() error
	GetTesseractHook func() error
}

func NewMockChainManager() *MockChainManager {
	return &MockChainManager{
		tesseractByAddress: make(map[identifiers.Address]*common.Tesseract),
		tesseractByHash:    make(map[common.Hash]*common.Tesseract),
	}
}

func (m *MockChainManager) AddTesseractWithState(
	addr identifiers.Address,
	dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	panic("implement me")
}

func (m *MockChainManager) AddTesseract(
	cache bool, addr identifiers.Address,
	t *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if m.addTesseractHook != nil {
		return m.addTesseractHook()
	}

	for _, addr := range t.Addresses() {
		m.tesseractByAddress[addr] = t
	}

	return nil
}

func (m *MockChainManager) insertTesseracts(tesseracts ...*common.Tesseract) {
	for _, ts := range tesseracts {
		m.tesseractByHash[ts.Hash()] = ts
	}
}

func (m *MockChainManager) GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	if m.GetTesseractHook != nil {
		return nil, m.GetTesseractHook()
	}

	ts, ok := m.tesseractByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	return ts, nil
}

func createTestConsensusConfig() *config.ConsensusConfig {
	tmpDir, err := os.MkdirTemp("", "consensus-test")
	if err != nil {
		panic(err)
	}

	return &config.ConsensusConfig{
		DirectoryPath: tmpDir,
	}
}

// MockAggregateSignVerifier returns true if first byte is true else return false
func mockAggregateSignVerifier(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error) {
	if aggSignature[0] == 1 {
		return true, nil
	}

	return false, common.ErrSignatureVerificationFailed
}

type createKramaEngineParams struct {
	db             *MockDB
	sm             *MockStateManager
	cm             *MockChainManager
	cfg            *config.ConsensusConfig
	smCallback     func(sm *MockStateManager)
	dbCallback     func(db *MockDB)
	cmCallback     func(cm *MockChainManager)
	cfgCallback    func(cfg *config.ConsensusConfig)
	serverCallback func(n *MockServer)
	engineCallBack func(k *Engine)
	execCallback   func(exec *MockExec)
}

func createTestKramaEngine(t *testing.T, params *createKramaEngineParams) *Engine {
	t.Helper()

	var (
		sm     = NewMockStateManager()
		cfg    = createTestConsensusConfig()
		server = newMockServer()
		cm     = NewMockChainManager()
		db     = NewMockDB()
		exec   = NewMockExec()
	)

	if params == nil {
		params = &createKramaEngineParams{}
	}

	if params.sm != nil {
		sm = params.sm
	}

	if params.cm != nil {
		cm = params.cm
	}

	if params.db != nil {
		db = params.db
	}

	if params.smCallback != nil {
		params.smCallback(sm)
	}

	if params.cmCallback != nil {
		params.cmCallback(cm)
	}

	if params.dbCallback != nil {
		params.dbCallback(db)
	}

	if params.cfg != nil {
		cfg = params.cfg
	}

	if params.execCallback != nil {
		params.execCallback(exec)
	}

	if params.cfgCallback != nil {
		params.cfgCallback(cfg)
	}

	if params.serverCallback != nil {
		params.serverCallback(server)
	}

	engine, err := NewKramaEngine(
		db,
		cfg,
		hclog.NewNullLogger(),
		nil,
		"",
		sm,
		exec,
		nil,
		nil,
		cm,
		nil,
		nil,
		NilMetrics(),
		nil,
		mockAggregateSignVerifier,
	)
	require.NoError(t, err)

	if params.engineCallBack != nil {
		params.engineCallBack(engine)
	}

	return engine
}

/*
These utility functions are required for extending the tests down the line
func createAssetMintIx(t *testing.T, sender, receiver identifiers.Address) *common.Interaction {
	t.Helper()

	assetMintPayload := common.AssetMintOrBurnPayload{
		Asset: tests.GetRandomAssetID(t, receiver),
	}

	rawAssetMintPayload, err := assetMintPayload.Bytes()
	require.NoError(t, err)

	ixParams := &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Input = common.IxInput{
				Type:      common.IxAssetMint,
				Nonce:     0,
				Sender:    sender,
				FuelPrice: big.NewInt(1),
				Payload:   rawAssetMintPayload,
			}
		},
	}

	return tests.CreateIX(t, ixParams)
}

func createAssetTransferIx(t *testing.T, sender, receiver identifiers.Address) common.Interactions {
	t.Helper()

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(sender, receiver),
	}

	return tests.CreateIxns(t, 1, ixParams)
}
*/

func createTestNodeSet(t *testing.T, n int) *common.NodeSet {
	t.Helper()

	kramaIDs, publicKeys := tests.GetTestKramaIdsWithPublicKeys(t, n)
	nodeset := common.NewNodeSet(kramaIDs, publicKeys, uint32(n))

	for i := 0; i < n; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset
}

func createTestRandomSet(t *testing.T, total, actual int) *common.NodeSet {
	t.Helper()

	kramaIDs, publicKeys := tests.GetTestKramaIdsWithPublicKeys(t, total)
	nodeset := common.NewNodeSet(kramaIDs, publicKeys, uint32(actual))

	for i := 0; i < actual; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset
}

func createNodeSet(
	t *testing.T,
	participantsCount int,
	nodesPerSet int,
) *common.ICSNodeSet {
	t.Helper()

	ns := common.NewICSNodeSet(2*participantsCount + 1)

	for i := 0; i < participantsCount; i++ {
		ns.UpdateNodeSet(i, createTestNodeSet(t, nodesPerSet))
		ns.UpdateNodeSet(i+1, createTestNodeSet(t, nodesPerSet))
	}

	// create some grace nodes for random set but quorum field will have nodes that are only part of ICS
	randomSet := createTestRandomSet(t, 2*participantsCount*nodesPerSet+5, 2*participantsCount*nodesPerSet)
	observerSet := createTestNodeSet(t, nodesPerSet)

	ns.UpdateNodeSet(ns.RandomSetPosition(), randomSet)
	ns.UpdateNodeSet(ns.ObserverSetPosition(), observerSet)

	return ns
}

func createTestClusterState(
	t *testing.T,
	operator kramaid.KramaID,
	selfID kramaid.KramaID,
	nodeset *common.ICSNodeSet,
	ixs common.Interactions,
	ps map[identifiers.Address]*common.Participant,
	callback func(clusterState *ktypes.ClusterState),
) *ktypes.ClusterState {
	t.Helper()

	clusterState := ktypes.NewICS(
		nil,
		ixs,
		"cluster-test",
		operator,
		time.Now(),
		selfID,
		ps,
		nodeset,
	)

	if callback != nil {
		callback(clusterState)
	}

	return clusterState
}

func checkContextDelta(
	t *testing.T,
	isGenesisAccount bool,
	expectedContextDelta *common.DeltaGroup,
	actualContextDelta *common.DeltaGroup,
) {
	t.Helper()

	if isGenesisAccount {
		require.NotNil(t, actualContextDelta)
		require.Equal(t, len(actualContextDelta.BehaviouralNodes), BehaviouralContextSize)
		require.Equal(t, len(actualContextDelta.RandomNodes), RandomContextSize)

		return
	}

	require.Equal(t, expectedContextDelta, actualContextDelta)
}

func tesseractParamsWithCommitSign(commitSign []byte) *createTesseractParams {
	return &createTesseractParams{
		TSDataCallback: func(ts *tests.TesseractData) {
			ts.ConsensusInfo.CommitSignature = commitSign
		},
	}
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

type createTesseractParams struct {
	Addresses            []identifiers.Address
	Heights              []uint64
	Participants         common.ParticipantsState
	participantsCallback func(participants common.ParticipantsState)
	TSDataCallback       func(ts *tests.TesseractData)

	Ixns     common.Interactions
	Receipts common.Receipts
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
		params.Participants = make(common.ParticipantsState)
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

func getPublicKeys(t *testing.T, count int) [][]byte {
	t.Helper()

	var pk [][]byte

	for i := 0; i < count; i++ {
		addr := tests.RandomAddress(t).Bytes()
		pk = append(pk, addr)
	}

	return pk
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

func createTesseractsWithChain(t *testing.T, count int, paramsMap map[int]*createTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	tesseracts[0] = createTesseract(t, paramsMap[0])

	for i := 1; i < count; i++ {
		paramsMap[i].participantsCallback = func(participants common.ParticipantsState) {
			hash := tesseracts[i-1].Hash()
			p := participants[tesseracts[0].AnyAddress()]
			p.TransitiveLink = hash
			participants[tesseracts[0].AnyAddress()] = p
		}

		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
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

func getTestGenesisLogics(t *testing.T) []common.LogicSetupArgs {
	t.Helper()

	manifestFile, err := engineio.NewManifestFromFile("./../compute/manifests/tokenledger.yaml")
	require.NoError(t, err)

	manifestEncoded, err := manifestFile.Encode(common.POLO)
	require.NoError(t, err)

	manifest := "0x" + common.BytesToHex(manifestEncoded)
	calldata := "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29e" +
		"c442dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"

	logic := common.LogicSetupArgs{
		Name: "staking-contract",

		Callsite: "Seeder",
		Calldata: hexutil.Bytes(common.Hex2Bytes(calldata)),
		Manifest: hexutil.Bytes(common.Hex2Bytes(manifest)),

		BehaviouralContext: tests.RandomKramaIDs(t, 1),
		RandomContext:      nil,
	}

	return []common.LogicSetupArgs{logic}
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

func getICSNodeset(t *testing.T, participantCount, nodesCount int) *common.ICSNodeSet {
	t.Helper()

	ics := common.NewICSNodeSet(2*participantCount + 2)

	for i := 0; i < 2*participantCount+2; i++ {
		ics.UpdateNodeSet(
			i,
			common.NewNodeSet(
				tests.RandomKramaIDs(t, nodesCount),
				getPublicKeys(t, nodesCount),
				uint32(nodesCount),
			))
	}

	return ics
}

func getReceipt(ixHash common.Hash) *common.Receipt {
	return &common.Receipt{
		IxType:    1,
		IxHash:    ixHash,
		FuelUsed:  rand.Uint64(),
		ExtraData: make(json.RawMessage, 0),
	}
}

func getIxParamsWithAddress(from identifiers.Address, to identifiers.Address) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
			ix.Input.Type = common.IxValueTransfer
		},
	}
}

func getIX(t *testing.T) *common.Interaction {
	t.Helper()

	return tests.CreateIX(
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

func checkForLogicAccounts(t *testing.T, expected, actual []common.LogicSetupArgs) {
	t.Helper()

	require.Equal(t, len(expected), len(actual))

	for i := range actual {
		require.Equal(t, expected[i], actual[i])
	}
}

func checkForAssetAccounts(t *testing.T, expected, actual []common.AssetAccountSetupArgs) {
	t.Helper()

	require.Equal(t, len(expected), len(actual))

	for i := range expected {
		require.Equal(t, expected[i], actual[i])
	}
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
	obj *state.Object,
	address identifiers.Address,
	contextHash common.Hash,
) {
	t.Helper()

	// check if context created
	_, err := obj.GetDirtyEntry(common.BytesToHex(storage.ContextObjectKey(address, contextHash)))
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

/*
func getRawInteraction(t *testing.T, ixData common.IxData, sign []byte) []byte {
	t.Helper()

	ix, err := common.NewInteraction(ixData, sign)
	require.NoError(t, err)

	rawInteractions, err := polo.Polorize([]*common.Interaction{ix})
	require.NoError(t, err)

	return rawInteractions
}

func getSignature(t *testing.T, kramaID kramaid.KramaID, rawIxns []byte, vault *crypto.KramaVault) ([]byte, []byte) {
	t.Helper()

	canonicalICSReq := ktypes.CanonicalICSRequest{
		Operator: string(kramaID),
		IxData:   rawIxns,
	}

	rawCanonicalICSReq, err := polo.Polorize(canonicalICSReq)
	require.NoError(t, err)

	icsReqSign, err := vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	require.NoError(t, err)

	return rawCanonicalICSReq, icsReqSign
}
*/

func checkNodeSetForParticipant(t *testing.T, state *MockStateManager, ps common.Participants, ns *common.ICSNodeSet) {
	t.Helper()

	for addr, p := range ps {
		if p.IsGenesis {
			continue
		}
		// fetch the context info from mock sm
		storedContext := state.participantsContext[addr]
		// this index access will validate nodeSetPositions
		beSet := ns.Sets[p.NodeSetPosition]
		rSet := ns.Sets[p.NodeSetPosition+1]
		// validate nodeSets
		require.Equal(t, storedContext.beSet, beSet)
		require.Equal(t, storedContext.rSet, rSet)
		// validate context hash
		require.Equal(t, storedContext.contextHash, p.ContextHash)
		// sum of setSizeWithOutDelta should be equal to context quourum
		require.Equal(t,
			(storedContext.beSet.SetSizeWithOutDelta+storedContext.rSet.SetSizeWithOutDelta)*2/3+1,
			p.ConsensusQuorum,
		)
	}
}

func checkParticipantInfo(
	t *testing.T,
	state *MockStateManager,
	ixnParticipant map[identifiers.Address]common.IxParticipant,
	ps common.Participants,
) {
	t.Helper()

	for addr, p := range ps {
		if ixnParticipant[addr].IsGenesis {
			require.Equal(t, p.LockType, ixnParticipant[addr].LockType)
			require.Equal(t, p.ContextHash, common.NilHash)
			require.Equal(t, p.TesseractHash, common.NilHash)
			require.Equal(t, p.IsGenesis, ixnParticipant[addr].IsGenesis)

			continue
		}
		// fetch meta info from mock sm
		storedMetaInfo := state.accMetaInfo[addr]

		require.Equal(t, p.AccType, storedMetaInfo.Type)
		require.Equal(t, p.IsGenesis, ixnParticipant[addr].IsGenesis)
		require.Equal(t, p.Height, storedMetaInfo.Height)
		require.Equal(t, p.TesseractHash, storedMetaInfo.TesseractHash)
		require.Equal(t, p.LockType, ixnParticipant[addr].LockType)
		require.Equal(t, p.IsSigner, ixnParticipant[addr].IsSigner)
	}
}
