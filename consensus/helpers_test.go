package consensus

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute"
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

type MockEngine struct {
	logger   hclog.Logger
	requests chan ktypes.Request
}

func processRequests(requestsChan <-chan ktypes.Request) {
	go func() {
		request := <-requestsChan
		request.ResponseChan <- nil
	}()
}

func (k *MockEngine) Requests() chan ktypes.Request {
	processRequests(k.requests)

	return k.requests
}

func (k *MockEngine) Logger() hclog.Logger {
	return k.logger
}

type MockStateManager struct {
	accountRegistration map[identifiers.Address]bool
	participantsContext map[identifiers.Address]struct {
		contextHash common.Hash
		beSet       *common.NodeSet
		rSet        *common.NodeSet
	}
	accMetaInfo map[identifiers.Address]*common.AccountMetaInfo
}

func newMockStateManager() *MockStateManager {
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

func (ms *MockStateManager) CreateStateObject(
	address identifiers.Address,
	accountType common.AccountType,
) *state.Object {
	// TODO implement me
	panic("implement me")
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

func (ms *MockStateManager) UpdateStateObjects(objs state.ObjectMap) error {
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

type MockExec struct {
	sm                      *MockStateManager
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
	return compute.NewManager(e.sm, hclog.NewNullLogger(), nil, compute.NilMetrics()).SpawnExecutor()
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

func createTestConsensusConfig() *config.ConsensusConfig {
	tmpDir, err := os.MkdirTemp("", "consensus-test")
	if err != nil {
		panic(err)
	}

	return &config.ConsensusConfig{
		DirectoryPath: tmpDir,
	}
}

type testKramaEngineParams struct {
	sm             *MockStateManager
	cfg            *config.ConsensusConfig
	execCallback   func(sm *MockExec)
	smCallback     func(sm *MockStateManager)
	cfgCallback    func(cfg *config.ConsensusConfig)
	serverCallback func(n *MockServer)
	engineCallBack func(k *Engine)
}

func createTestKramaEngine(t *testing.T, params *testKramaEngineParams) *Engine {
	t.Helper()

	var (
		sm     = newMockStateManager()
		cfg    = createTestConsensusConfig()
		server = newMockServer()
		exec   = mockExec(t)
	)

	if params == nil {
		params = &testKramaEngineParams{}
	}

	if params.sm != nil {
		sm = params.sm
	}

	if params.smCallback != nil {
		params.smCallback(sm)
	}

	exec.sm = sm

	if params.cfg != nil {
		cfg = params.cfg
	}

	if params.cfgCallback != nil {
		params.cfgCallback(cfg)
	}

	if params.serverCallback != nil {
		params.serverCallback(server)
	}

	if params.execCallback != nil {
		params.execCallback(exec)
	}

	engine, err := NewKramaEngine(
		cfg,
		hclog.NewNullLogger(),
		nil,
		"",
		sm,
		exec,
		nil,
		nil,
		nil,
		nil,
		nil,
		NilMetrics(),
		nil,
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
		require.Equal(t, len(actualContextDelta.BehaviouralNodes), BehaviouralContextSize)
		require.Equal(t, len(actualContextDelta.RandomNodes), RandomContextSize)

		return
	}

	require.Equal(t, expectedContextDelta, actualContextDelta)
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

func checkForExecutionCleanup(t *testing.T, exec *MockExec, expectedClusterID common.ClusterID) {
	t.Helper()

	require.Equal(t, expectedClusterID, exec.clusterID)
}

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
