package consensus

import (
	"context"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/crypto"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/network/rpc"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
)

type MockServer struct {
	id          id.KramaID
	subscribers map[string]context.Context
	peers       []id.KramaID
	peersLock   sync.RWMutex
}

func NewMockServer() *MockServer {
	return &MockServer{
		subscribers: make(map[string]context.Context),
		peers:       make([]id.KramaID, 0),
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

func (m *MockServer) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	m.subscribers[topic] = ctx

	return nil
}

func (m *MockServer) StartNewRPCServer(protocol protocol.ID) *rpc.Client {
	return nil
}

func (m *MockServer) RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error {
	return nil
}

func (m *MockServer) GetKramaID() id.KramaID {
	return m.id
}

func (m *MockServer) ConnectPeer(kramaID id.KramaID) error {
	for _, peer := range m.peers {
		if peer == kramaID {
			return common.ErrConnectionExists
		}
	}

	m.peers = append(m.peers, kramaID)

	return nil
}

func (m *MockServer) DisconnectPeer(kramaID id.KramaID) error {
	m.peersLock.Lock()
	defer m.peersLock.Unlock()

	indexOf := func(peerID id.KramaID) int {
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
	requests chan Request
}

func processRequests(requestsChan <-chan Request) {
	go func() {
		request := <-requestsChan
		request.responseChan <- Response{}
	}()
}

func (k *MockEngine) Requests() chan Request {
	processRequests(k.requests)

	return k.requests
}

func (k *MockEngine) Logger() hclog.Logger {
	return k.logger
}

type MockStateManager struct {
	accountRegistration map[common.Address]bool
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		accountRegistration: make(map[common.Address]bool),
	}
}

func (ms *MockStateManager) FetchInteractionContext(
	ctx context.Context,
	ix *common.Interaction,
) (map[common.Address]common.Hash, []*common.NodeSet, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetPublicKeys(ctx context.Context, ids ...id.KramaID) (keys [][]byte, err error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetAccountMetaInfo(addr common.Address) (*common.AccountMetaInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetLatestStateObject(addr common.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetNonce(addr common.Address, stateHash common.Hash) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) registerAccount(addr common.Address) {
	ms.accountRegistration[addr] = true
}

func (ms *MockStateManager) IsAccountRegistered(addr common.Address) (bool, error) {
	_, ok := ms.accountRegistration[addr]

	return ok, nil
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

type createKramaEngineParams struct {
	sm             *MockStateManager
	cfg            *config.ConsensusConfig
	smCallback     func(sm *MockStateManager)
	cfgCallback    func(cfg *config.ConsensusConfig)
	serverCallback func(n *MockServer)
	engineCallBack func(k *Engine)
}

func createTestKramaEngine(t *testing.T, params *createKramaEngineParams) *Engine {
	t.Helper()

	var (
		sm     = NewMockStateManager()
		cfg    = createTestConsensusConfig()
		server = NewMockServer()
	)

	if params == nil {
		params = &createKramaEngineParams{}
	}

	if params.sm != nil {
		sm = params.sm
	}

	if params.smCallback != nil {
		params.smCallback(sm)
	}

	if params.cfg != nil {
		cfg = params.cfg
	}

	if params.cfgCallback != nil {
		params.cfgCallback(cfg)
	}

	if params.serverCallback != nil {
		params.serverCallback(server)
	}

	engine, err := NewKramaEngine(
		cfg,
		hclog.NewNullLogger(),
		nil,
		sm,
		server,
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

func createAssetMintIx(t *testing.T, sender, receiver common.Address) *common.Interaction {
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

func createAssetTransferIx(t *testing.T, sender, receiver common.Address) common.Interactions {
	t.Helper()

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(sender, receiver),
	}

	return tests.CreateIxns(t, 1, ixParams)
}

func createTestNodeSet(t *testing.T, n int) *common.NodeSet {
	t.Helper()

	kramaIDs, publicKeys := tests.GetTestKramaIdsWithPublicKeys(t, n)
	nodeset := common.NewNodeSet(kramaIDs, publicKeys)
	nodeset.QuorumSize = n

	for i := 0; i < n; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset
}

func createTestRandomSet(t *testing.T, total, actual int) *common.NodeSet {
	t.Helper()

	kramaIDs, publicKeys := tests.GetTestKramaIdsWithPublicKeys(t, total)
	nodeset := common.NewNodeSet(kramaIDs, publicKeys)
	nodeset.QuorumSize = actual

	for i := 0; i < actual; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset
}

func createNodeSet(
	t *testing.T,
	senderBehaviourSetCount int,
	senderRandomSetCount int,
	receiverBehaviourSetCount int,
	receiverRandomSetCount int,
	randomSetCount int,
	observerSetCount int,
) []*common.NodeSet {
	t.Helper()

	senderBehaviourSet := createTestNodeSet(t, senderBehaviourSetCount)
	senderRandomSet := createTestNodeSet(t, senderRandomSetCount)
	receiverBehaviourSet := createTestNodeSet(t, receiverBehaviourSetCount)
	receiverRandomSet := createTestNodeSet(t, receiverRandomSetCount)

	// create some grace nodes for random set but quorum field will have nodes that are only part of ICS
	randomSet := createTestRandomSet(t, 2*randomSetCount, randomSetCount)
	observerSet := createTestNodeSet(t, observerSetCount)

	testNodeSets := []*common.NodeSet{
		senderBehaviourSet,
		senderRandomSet,
		receiverBehaviourSet,
		receiverRandomSet,
		randomSet,
		observerSet,
	}

	return testNodeSets
}

func createTestClusterState(
	t *testing.T,
	operator id.KramaID,
	selfID id.KramaID,
	nodeset []*common.NodeSet,
	ixs common.Interactions,
	callback func(clusterState *ktypes.ClusterState),
) *ktypes.ClusterState {
	t.Helper()

	clusterState := ktypes.NewICS(
		6,
		nil,
		ixs,
		"cluster-test",
		operator,
		time.Now(),
		selfID,
	)

	clusterState.NodeSet.Nodes = nodeset

	if callback != nil {
		callback(clusterState)
	}

	return clusterState
}

func checkContextDelta(
	t *testing.T,
	sender common.Address,
	receiver common.Address,
	expectedContextDelta common.ContextDelta,
	actualContextDelta common.ContextDelta,
) {
	t.Helper()

	require.Equal(t, expectedContextDelta[sender], actualContextDelta[sender])
	require.Equal(t, expectedContextDelta[receiver], actualContextDelta[receiver])
	require.Equal(t, expectedContextDelta[common.SargaAddress], actualContextDelta[common.SargaAddress])
}

func getRawInteraction(t *testing.T, ixData common.IxData, sign []byte) []byte {
	t.Helper()

	ix, err := common.NewInteraction(ixData, sign)
	require.NoError(t, err)

	rawInteractions, err := polo.Polorize([]*common.Interaction{ix})
	require.NoError(t, err)

	return rawInteractions
}

func getSignature(t *testing.T, kramaID id.KramaID, rawInteractions []byte, vault *crypto.KramaVault) ([]byte, []byte) {
	t.Helper()

	canonicalICSReq := networkmsg.CanonicalICSRequest{
		Operator: string(kramaID),
		IxData:   rawInteractions,
	}

	rawCanonicalICSReq, err := polo.Polorize(canonicalICSReq)
	require.NoError(t, err)

	icsReqSign, err := vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	require.NoError(t, err)

	return rawCanonicalICSReq, icsReqSign
}
