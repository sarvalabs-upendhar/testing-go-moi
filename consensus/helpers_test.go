package consensus

/*
import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"

	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-polo"

	cryptocommon "github.com/sarvalabs/go-moi/crypto/common"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto"

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
	accountRegistration    map[identifiers.Identifier]bool
	stateObjects           map[identifiers.Identifier]*state.Object
	publicKeys             map[kramaid.KramaID][]byte
	icsSeed                map[identifiers.Identifier][32]byte
	accMetaInfo            map[identifiers.Identifier]*common.AccountMetaInfo
	GetAccountMetaInfoHook func() error
	IsInitialTesseractHook func() error
	isSealValid            func() bool
}

func (ms *MockStateManager) GetLatestContextAndPublicKeys(id identifiers.Identifier) (latestContextHash common.Hash,
	behaviouralSet, randomSet []kramaid.KramaID, bePublicKeys, beRandomKeys [][]byte, err error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetConsensusNodes(id identifiers.Identifier,
hash common.Hash) (common.NodeList, common.NodeList, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetRegisteredGuardiansCount() (int, error) {
	obj, ok := ms.stateObjects[common.GuardianLogicAddr]
	if !ok {
		return 0, common.ErrObjectNotFound
	}

	storageReader := state.NewLogicStorageObject(common.GuardianLogicID, obj)

	return guardianregistry.GetGuardiansCount(storageReader)
}

func (ms *MockStateManager) GetGuardianIncentives(id kramaid.KramaID) (uint64, error) {
	obj, ok := ms.stateObjects[common.GuardianLogicAddr]
	if !ok {
		return 0, common.ErrObjectNotFound
	}

	storageReader := state.NewLogicStorageObject(common.GuardianLogicID, obj)

	return guardianregistry.GetGuardianIncentive(storageReader, id)
}

func (ms *MockStateManager) GetTotalIncentives() (uint64, error) {
	obj, ok := ms.stateObjects[common.GuardianLogicAddr]
	if !ok {
		return 0, common.ErrObjectNotFound
	}

	storageReader := state.NewLogicStorageObject(common.GuardianLogicID, obj)

	return guardianregistry.GetTotalIncentives(storageReader)
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		accMetaInfo:         make(map[identifiers.Identifier]*common.AccountMetaInfo),
		accountRegistration: make(map[identifiers.Identifier]bool),
		stateObjects:        make(map[identifiers.Identifier]*state.Object),
		publicKeys:          make(map[kramaid.KramaID][]byte),
		icsSeed:             make(map[identifiers.Identifier][32]byte),
	}
}

func (ms *MockStateManager) LoadTransitionObjects(
	ps map[identifiers.Identifier]common.ParticipantInfo,
) (*state.Transition, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) CreateStateObject(id identifiers.Identifier,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	stateObject := state.NewStateObject(id,
		mockCache(),
		nil,
		nil,
		common.Account{AccType: accountType},
		state.NilMetrics(),
		isGenesis,
	)

	return stateObject
}

func (ms *MockStateManager) IsInitialTesseract(ts *common.Tesseract, id identifiers.Identifier) (bool, error) {
	if ms.IsInitialTesseractHook != nil {
		return false, ms.IsInitialTesseractHook()
	}

	if ts.Height(ts.AnyAccountID()) == 0 {
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

func (ms *MockStateManager) RemoveCachedObject(id identifiers.Identifier) {
	panic("implement me")
}

func (ms *MockStateManager) addAccMetaInfo(t *testing.T, id identifiers.Identifier, info *common.AccountMetaInfo) {
	t.Helper()

	ms.accMetaInfo[id] = info
}

func (ms *MockStateManager) GetDirtyObject(id identifiers.Identifier) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) CreateDirtyObject(
	id identifiers.Identifier,
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
	id identifiers.Identifier,
	hash common.Hash,
) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) FetchInteractionContext(
	ctx context.Context,
	ix *common.Interaction,
) (map[identifiers.Identifier]common.Hash, []*ktypes.NodeSet, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) Cleanup(id identifiers.Identifier) {}

func (ms *MockStateManager) GetPublicKeys(ctx context.Context, ids ...kramaid.KramaID) (keys [][]byte, err error) {
	pubKeys := make([][]byte, 0)

	for _, id := range ids {
		if publicKey, ok := ms.publicKeys[id]; ok {
			pubKeys = append(pubKeys, publicKey)
		}
	}

	if len(pubKeys) == 0 {
		return nil, common.ErrPublicKeyNotFound
	}

	return pubKeys, nil
}

func (ms *MockStateManager) setPublicKey(kramaID kramaid.KramaID, pubKey []byte) {
	ms.publicKeys[kramaID] = pubKey
}

func (ms *MockStateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	if ms.GetAccountMetaInfoHook != nil {
		return nil, ms.GetAccountMetaInfoHook()
	}

	info, ok := ms.accMetaInfo[id]
	if !ok {
		return nil, common.ErrFetchingAccMetaInfo
	}

	return info, nil
}

func (ms *MockStateManager) GetLatestStateObject(id identifiers.Identifier) (*state.Object, error) {
	if stateObject, ok := ms.stateObjects[id]; ok {
		return stateObject, nil
	}

	return nil, errors.New("state object doesn't exist")
}

func (ms *MockStateManager) setLatestStateObject(id identifiers.Identifier, so *state.Object) {
	ms.stateObjects[id] = so
}

func (ms *MockStateManager) GetNonce(id identifiers.ID, stateHash common.Hash) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetICSSeed(id identifiers.Identifier) ([32]byte, error) {
	if icsSeed, ok := ms.icsSeed[id]; ok {
		return icsSeed, nil
	}

	return [32]byte{}, errors.New("seed not found")
}

func (ms *MockStateManager) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	_, ok := ms.accountRegistration[id]

	return ok, nil
}

type MockVault struct {
	kramaID          kramaid.KramaID
	consensusPrivKey crypto.PrivateKey // Private key used in consensus
}

func newMockVault() *MockVault {
	return &MockVault{}
}

func (mv *MockVault) KramaID() kramaid.KramaID {
	return mv.kramaID
}

func (mv *MockVault) setKramaID(kramaID kramaid.KramaID) {
	mv.kramaID = kramaID
}

func (mv *MockVault) GetConsensusPrivateKey() crypto.PrivateKey {
	return mv.consensusPrivKey
}

func (mv *MockVault) setConsensusPrivateKey(privKey []byte) {
	cPriv := new(crypto.BLSPrivKey)
	cPriv.UnMarshal(privKey)

	mv.consensusPrivKey = cPriv
}

func (mv *MockVault) Sign(data []byte, sigType cryptocommon.SigType,
signOptions ...crypto.SignOption) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

type MockDB struct {
	accMetaInfo map[identifiers.Identifier]bool
}

func (m *MockDB) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetSafetyData(id identifiers.Identifier) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetCommitInfo(tsHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetSafetyData(id identifiers.Identifier, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) SetConsensusProposalInfo(tsHash common.Hash, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetConsensusProposalInfo(tsHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) DeleteConsensusProposalInfo(tsHash common.Hash) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) GetAllConsensusProposalInfo(ctx context.Context) ([][]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) DeleteSafetyData(id identifiers.Identifier) error {
	// TODO implement me
	panic("implement me")
}

func NewMockDB() *MockDB {
	return &MockDB{
		accMetaInfo: make(map[identifiers.Identifier]bool),
	}
}

func (m *MockDB) setAccMetaInfoAt(id identifiers.Identifier) {
	m.accMetaInfo[id] = true
}

func (m *MockDB) HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool {
	_, ok := m.accMetaInfo[id]

	return ok
}

type MockChainManager struct {
	tesseractByAddress map[identifiers.Identifier]*common.Tesseract
	tesseractByHash    map[common.Hash]*common.Tesseract

	addTesseractHook func() error
	GetTesseractHook func() error
}

func NewMockChainManager() *MockChainManager {
	return &MockChainManager{
		tesseractByAddress: make(map[identifiers.Identifier]*common.Tesseract),
		tesseractByHash:    make(map[common.Hash]*common.Tesseract),
	}
}

func (m *MockChainManager) AddTesseractWithState(
	id identifiers.Identifier,
	dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	panic("implement me")
}

func (m *MockChainManager) AddTesseract(
	cache bool, id identifiers.Identifier,
	t *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if m.addTesseractHook != nil {
		return m.addTesseractHook()
	}

	for _, id := range t.AccountIDs() {
		m.tesseractByAddress[id] = t
	}

	return nil
}

func (m *MockChainManager) insertTesseracts(tesseracts ...*common.Tesseract) {
	for _, ts := range tesseracts {
		m.tesseractByHash[ts.Hash()] = ts
	}
}

func (m *MockChainManager) GetTesseract(hash common.Hash, withInteractions bool,
	withCommitInfo bool) (*common.Tesseract, error) {
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
	vault          *MockVault
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
		vault  = newMockVault()
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

	if params.vault != nil {
		vault = params.vault
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
		vault.KramaID(),
		sm,
		exec,
		nil,
		vault,
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


These utility functions are required for extending the tests down the line
func createAssetMintIx(t *testing.T, sender, receiver identifiers.Identifier) *common.Interaction {
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
				SequenceID:     0,
				SenderID:    sender,
				FuelPrice: big.NewInt(1),
				Payload:   rawAssetMintPayload,
			}
		},
	}

	return tests.CreateIX(t, ixParams)
}

func createAssetTransferIx(t *testing.T, sender, receiver identifiers.Identifier) common.Interactions {
	t.Helper()

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(sender, receiver),
	}

	return tests.CreateIxns(t, 1, ixParams)
}


func createTestNodeSet(t *testing.T, n int) *ktypes.NodeSet {
	t.Helper()

	kramaIDs, publicKeys := tests.GetTestKramaIdsWithPublicKeys(t, n)
	nodeset := ktypes.NewNodeSet(kramaIDs, publicKeys, uint32(n))

	for i := 0; i < n; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset
}

func createTestRandomSet(t *testing.T, total, actual int) *ktypes.NodeSet {
	t.Helper()

	kramaIDs, publicKeys := tests.GetTestKramaIdsWithPublicKeys(t, total)
	nodeset := ktypes.NewNodeSet(kramaIDs, publicKeys, uint32(actual))

	for i := 0; i < actual; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset
}

func createNodeSet(
	t *testing.T,
	participantsCount int,
	nodesPerSet int,
) *ktypes.ICSCommittee {
	t.Helper()

	ns := ktypes.NewICSCommittee(2*participantsCount + 1)

	for i := 0; i < participantsCount; i++ {
		ns.UpdateNodeSet(i, createTestNodeSet(t, nodesPerSet))
		ns.UpdateNodeSet(i+1, createTestNodeSet(t, nodesPerSet))
	}

	// create some grace nodes for random set but quorum field will have nodes that are only part of ICS
	randomSet := createTestRandomSet(t, 2*participantsCount*nodesPerSet+5, 2*participantsCount*nodesPerSet)

	ns.UpdateNodeSet(ns.StochasticSetPosition(), randomSet)

	return ns
}

func createTestClusterState(
	t *testing.T,
	operator kramaid.KramaID,
	selfID kramaid.KramaID,
	nodeset *ktypes.ICSCommittee,
	ixs common.Interactions,
	ps map[identifiers.Identifier]*common.Participant,
	callback func(clusterState *ktypes.ClusterState),
) *ktypes.ClusterState {
	t.Helper()

	clusterState := ktypes.NewICS(
		ixs,
		"cluster-test",
		operator,
		time.Now(),
		selfID,
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
		require.Equal(t, len(actualContextDelta.RandomNodes), StochasticSetSize)

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

		CommitInfo: &common.CommitInfo{
			QC: &common.Qc{
				SignerIndices: voteSet.Copy(),
			},
		},

		// non canonical fields
		Seal:   nil,
		SealBy: "",
	}
}

type createTesseractParams struct {
	AccountIDs            []identifiers.Identifier
	Heights              []uint64
	Participants         common.ParticipantsState
	participantsCallback func(participants common.ParticipantsState)
	TSDataCallback       func(ts *tests.TesseractData)

	Ixns       common.Interactions
	Receipts   common.Receipts
	CommitInfo *common.CommitInfo
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

	if len(params.AccountIDs) == 0 {
		id := tests.RandomAddress(t)
		params.AccountIDs = []identifiers.Identifier{id}
		params.Participants[id] = common.State{}

		if len(params.Heights) != 0 {
			params.Participants[id] = common.State{
				Height: params.Heights[0],
			}
		}
	}

	if params.Ixns.Len() != 0 {
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
		tsData.FuelUsed,
		tsData.FuelLimit,
		tsData.ConsensusInfo,
		tsData.Seal,
		tsData.SealBy,
		params.Ixns,
		params.Receipts,
		params.CommitInfo,
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
		id := tests.RandomAddress(t).Bytes()
		pk = append(pk, id)
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

func createTesseractsWithChain(t *testing.T, count int,
	paramsMap map[int]*createTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*createTesseractParams{}
	}

	tesseracts[0] = createTesseract(t, paramsMap[0])

	for i := 1; i < count; i++ {
		paramsMap[i].participantsCallback = func(participants common.ParticipantsState) {
			hash := tesseracts[i-1].Hash()
			p := participants[tesseracts[0].AnyAccountID()]
			p.TransitiveLink = hash
			participants[tesseracts[0].AnyAccountID()] = p
		}

		tesseracts[i] = createTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func getAccountSetupArgs(
	t *testing.T,
	id identifiers.Identifier,
	accType common.AccountType,
	moiID string,
	behNodes []kramaid.KramaID,
	randNodes []kramaid.KramaID,
) *common.AccountSetupArgs {
	t.Helper()

	return &common.AccountSetupArgs{
		Identifier:            id,
		MoiID:              moiID,
		BehaviouralContext: behNodes,
		RandomContext:      randNodes,
		AccType:            accType,
	}
}

func getTestAccountWithAccType(t *testing.T, accType common.AccountType) common.AccountSetupArgs {
	t.Helper()

	ids := tests.RandomKramaIDs(t, 4)

	id := tests.RandomAddress(t)

	if accType == common.SargaAccount {
		id = common.SargaAddress
	}

	return *getAccountSetupArgs(
		t,
		id,
		accType,
		"moi-id",
		ids[:2],
		ids[2:4],
	)
}

func getTestGenesisLogics(t *testing.T) []common.LogicSetupArgs {
	t.Helper()

	manifestFile, err := engineio.NewManifestFromFile("./../compute/exlogics/tokenledger/tokenledger.yaml")
	require.NoError(t, err)

	manifestEncoded, err := manifestFile.Encode(common.POLO)
	require.NoError(t, err)

	manifest := "0x" + common.BytesToHex(manifestEncoded)
	calldata := "0x0d6f0665b6019502737570706c790305f5e10073796d626f6c064d4f49"

	logic := common.LogicSetupArgs{
		Name: "staking-contract",

		Callsite: "Seed",
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

func getTestAssetCreationArgs(t *testing.T, allocationID identifiers.Identifier) common.AssetCreationArgs {
	t.Helper()

	info := tests.GetRandomAssetInfo(t, identifiers.Nil)

	if allocationID == identifiers.Nil {
		allocationID = tests.RandomAddress(t)
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
				Identifier: allocationID,
				Amount:  (*hexutil.Big)(big.NewInt(rand.Int63())),
			},
		},
	}
}

func getTestAccountWithAddress(t *testing.T, id identifiers.Identifier) common.AccountSetupArgs {
	t.Helper()

	ids := tests.RandomKramaIDs(t, 4)

	return *getAccountSetupArgs(
		t,
		id,
		common.RegularAccount,
		"moi-id",
		ids[:2],
		ids[2:4],
	)
}

func getAssetCreationArgs(
	symbol string,
	owner identifiers.Identifier,
	id []identifiers.Identifier,
	amount []*big.Int,
) *common.AssetCreationArgs {
	alloc := make([]common.Allocation, len(id))

	for i, id := range id {
		alloc[i] = common.Allocation{
			Identifier: id,
			Amount:  (*hexutil.Big)(amount[i]),
		}
	}

	return &common.AssetCreationArgs{
		Symbol:      symbol,
		Operator:    owner,
		Allocations: alloc,
	}
}

func getICSNodeset(t *testing.T, participantCount, nodesCount int) *ktypes.ICSCommittee {
	t.Helper()

	ics := ktypes.NewICSCommittee(2*participantCount + 2)

	for i := 0; i < 2*participantCount+2; i++ {
		ics.UpdateNodeSet(
			i,
			ktypes.NewNodeSet(
				tests.RandomKramaIDs(t, nodesCount),
				getPublicKeys(t, nodesCount),
				uint32(nodesCount),
			))
	}

	return ics
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

func getIxParamsWithAddress(
t *testing.T,
from identifiers.Identifier,
to identifiers.Identifier) *tests.CreateIxParams {
	t.Helper()

	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.SenderID = from
			ix.IxOps = []common.IxOpRaw{
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, to),
				},
			}
		},
	}
}

func getIX(t *testing.T) *common.Interaction {
	t.Helper()

	return tests.CreateIX(
		t,
		getIxParamsWithAddress(t, tests.RandomAddress(t), tests.RandomAddress(t)),
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

func checkSargaStorageEntry(t *testing.T, obj *state.Object, id identifiers.Identifier) {
	t.Helper()

	val, err := obj.GetStorageEntry(common.SargaLogicID, id.Bytes())
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

	// check if other accounts id inserted in to sarga account storage
	for _, info := range accounts {
		checkSargaStorageEntry(t, obj, info.Identifier)
	}
}

func checkSargaObjectLogicAccounts(
	t *testing.T,
	obj *state.Object,
	logics []common.LogicSetupArgs,
) {
	t.Helper()

	// check if logics id inserted in to sarga account storage
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

	// check if assert id inserted in to sarga account storage
	for _, asset := range assets {
		checkSargaStorageEntry(t, obj, common.CreateAddressFromString(asset.AssetInfo.Symbol))
	}
}

func validateContextInitialization(
	t *testing.T,
	obj *state.Object,
	id identifiers.Identifier,
	contextHash common.Hash,
) {
	t.Helper()

	// check if context created
	_, err := obj.GetDirtyEntry(common.BytesToHex(storage.ContextObjectKey(id, contextHash)))
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
	stateObjects map[identifiers.Identifier]*state.Object,
	assetInfo *common.AssetCreationArgs,
	assetID identifiers.AssetID,
) {
	t.Helper()

	for _, allocation := range assetInfo.Allocations {
		so, ok := stateObjects[allocation.Identifier]
		require.True(t, ok)

		balances, err := so.Balances()
		require.NoError(t, err)

		bal, ok := balances.AssetMap[assetID]
		require.True(t, ok)

		require.Equal(t, allocation.Amount.ToInt(), bal)
	}
}


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
		Proposer: string(kramaID),
		IxData:   rawIxns,
	}

	rawCanonicalICSReq, err := polo.Polorize(canonicalICSReq)
	require.NoError(t, err)

	icsReqSign, err := vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	require.NoError(t, err)

	return rawCanonicalICSReq, icsReqSign
}


func checkNodeSetForParticipant(t *testing.T, state *MockStateManager, ps common.Participants,
ns *ktypes.ICSCommittee) {
	t.Helper()

	for id, p := range ps {
		if p.IsGenesis {
			continue
		}
		// fetch the context info from mock sm
		storedContext := state.participantsContext[id]
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
	ixnParticipant map[identifiers.Identifier]common.ParticipantInfo,
	ps common.Participants,
) {
	t.Helper()

	for id, p := range ps {
		if ixnParticipant[id].IsGenesis {
			require.Equal(t, p.LockType, ixnParticipant[id].LockType)
			require.Equal(t, p.ContextHash, common.NilHash)
			require.Equal(t, p.TesseractHash, common.NilHash)
			require.Equal(t, p.IsGenesis, ixnParticipant[id].IsGenesis)

			continue
		}
		// fetch meta info from mock sm
		storedMetaInfo := state.accMetaInfo[id]

		require.Equal(t, p.AccType, storedMetaInfo.Type)
		require.Equal(t, p.IsGenesis, ixnParticipant[id].IsGenesis)
		require.Equal(t, p.Height, storedMetaInfo.Height)
		require.Equal(t, p.TesseractHash, storedMetaInfo.TesseractHash)
		require.Equal(t, p.LockType, ixnParticipant[id].LockType)
		require.Equal(t, p.IsSigner, ixnParticipant[id].IsSigner)
	}
}

func createTestLotteryKey(t *testing.T, ixHash common.Hash, seeds ...[32]byte) common.LotteryKey {
	t.Helper()

	finalSeed := seeds[0]

	for i := 1; i < len(seeds); i++ {
		for j := 0; j < len(seeds[i]); j++ {
			finalSeed[j] ^= seeds[i][j]
		}
	}

	return common.NewLotteryKey(ixHash, finalSeed)
}

func checkForOperatorInfo(
	t *testing.T,
	engine *Engine,
	lk common.LotteryKey,
	kramaID kramaid.KramaID,
	priority uint64,
) {
	t.Helper()

	info, ok := engine.lottery.ixOperators.Get(lk)
	require.True(t, ok)

	operators := info.(ICSOperators) //nolint

	for _, info := range operators {
		if info.KramaID == kramaID && info.Priority == priority {
			return
		}
	}

	require.Fail(t, "kramaID is not found")
}

func createSortitionVault(t *testing.T, isEligible bool) *MockVault {
	t.Helper()

	mockVault := newMockVault()

	if isEligible {
		mockVault.setKramaID("3WvfmcZCP85fghMBc7NWvYzNgAk5b6XQLTULBLJmVRHZy8Mo8Je3.16Uiu2HAkupwrGKxG7s55qtQ" +
			"9rHRb5RRSQ4VdBTxickMEwFmTGMjA")
		mockVault.setConsensusPrivateKey([]byte{
			40, 74, 193, 90, 79, 247, 13, 74, 32, 66, 190, 54, 82, 18, 11, 133, 235, 58, 50, 58,
			250, 42, 252, 220, 83, 113, 51, 250, 253, 130, 148, 73,
		})

		return mockVault
	}

	mockVault.setKramaID("3WyR9BoaWznXD19kNW3pwMvQUybuU4sbkwRwftB39F8gD4qRCnHM.16Uiu2HAkvMe9u6B7MM3JnstkCPpgtqZ" +
		"N2B3Upq5cSeFKL251QTqK")
	mockVault.setConsensusPrivateKey([]byte{
		68, 157, 28, 23, 108, 108, 250, 232, 100, 61, 64, 128, 0, 112, 65, 242, 248, 87, 225, 227, 213,
		100, 64, 60, 17, 186, 217, 114, 222, 33, 39, 194,
	})

	return mockVault
}

func updateTotalIncentive(t *testing.T, so *state.Object, count uint64) {
	t.Helper()

	_, err := so.GetStorageTree(common.GuardianLogicID)
	if err != nil {
		err := so.CreateStorageTreeForLogic(common.GuardianLogicID)
		require.NoError(t, err)
	}

	key := pisa.GenerateStorageKey(guardianregistry.SlotTotalIncentives)

	value, err := polo.Polorize(count)
	require.NoError(t, err)

	err = so.SetStorageEntry(common.GuardianLogicID, key, value)
	require.NoError(t, err)
}

func updateTotalIncentives(t *testing.T, sm *MockStateManager, totalIncentive uint64) {
	t.Helper()

	so, ok := sm.stateObjects[common.GuardianLogicAddr]
	if !ok {
		so = state.NewStateObject(
			common.GuardianLogicAddr, nil, tests.NewTestTreeCache(),
			nil, common.Account{}, state.NilMetrics(), false,
		)
	}

	updateTotalIncentive(t, so, totalIncentive)

	sm.setLatestStateObject(common.GuardianLogicAddr, so)
}

func updateGuardianIncentive(t *testing.T, sm *MockStateManager, id kramaid.KramaID, incentive uint64) {
	t.Helper()

	so, ok := sm.stateObjects[common.GuardianLogicAddr]
	if !ok {
		so = state.NewStateObject(
			common.GuardianLogicAddr, nil, tests.NewTestTreeCache(),
			nil, common.Account{}, state.NilMetrics(), false,
		)
	}

	_, err := so.GetStorageTree(common.GuardianLogicID)
	if err != nil {
		err := so.CreateStorageTreeForLogic(common.GuardianLogicID)
		require.NoError(t, err)
	}

	encoded, _ := polo.Polorize(id)
	hashed := blake2b.Sum256(encoded)

	// Generate a storage access key for Registry.Guardians[kramaID].PubKey
	key := pisa.GenerateStorageKey(guardianregistry.SlotGuardians, pisa.MapKey(hashed), pisa.ClsFld(2), pisa.ClsFld(0))

	value, err := polo.Polorize(incentive)
	require.NoError(t, err)

	err = so.SetStorageEntry(common.GuardianLogicID, key, value)
	require.NoError(t, err)

	sm.setLatestStateObject(common.GuardianLogicAddr, so)
}
*/
