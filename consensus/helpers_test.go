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

func (ms *MockStateManager) GetPublicKeys(ids ...id.KramaID) (keys [][]byte, err error) {
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

func createTestKramaEngine(t *testing.T, sm *MockStateManager) *Engine {
	t.Helper()

	engine, err := NewKramaEngine(
		context.Background(),
		createTestConsensusConfig(),
		hclog.NewNullLogger(),
		nil,
		sm,
		NewMockServer(),
		nil,
		nil,
		nil,
		nil,
		nil,
		NilMetrics(),
		nil,
	)
	require.NoError(t, err)

	return engine
}

func createAssetIx(t *testing.T, sender common.Address) common.Interactions {
	t.Helper()

	payload, err := polo.Polorize(common.AssetCreatePayload{
		Symbol:    "Consensus-Test",
		Supply:    big.NewInt(100),
		Dimension: 1,
	})
	require.NoError(t, err)

	ixn, err := common.NewInteraction(common.IxData{
		Input: common.IxInput{
			Type:      common.IxAssetCreate,
			Nonce:     0,
			Sender:    sender,
			FuelPrice: big.NewInt(1),
			Payload:   payload,
		},
	}, nil)
	require.NoError(t, err)

	return common.Interactions{ixn}
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

func createTestClusterInfo(
	t *testing.T,
	operator id.KramaID,
	selfID id.KramaID,
	nodeset []*common.NodeSet,
	ixs common.Interactions,
	nonRegisteredReceiver bool,
) *ktypes.ClusterState {
	t.Helper()

	clusterInfo := ktypes.NewICS(
		6,
		nil,
		ixs,
		"cluster-test",
		operator,
		time.Now(),
		selfID,
	)

	func(clusterInfo *ktypes.ClusterState) {
		clusterInfo.NodeSet.Nodes = nodeset
		clusterInfo.AccountInfos = make(map[common.Address]*ktypes.AccountInfo)

		if !nonRegisteredReceiver && !ixs[0].Receiver().IsNil() {
			clusterInfo.AccountInfos[ixs[0].Receiver()] = &ktypes.AccountInfo{
				Address: ixs[0].Receiver(),
				AccType: common.AccTypeFromIxType(ixs[0].Type()),
			}
		}
	}(clusterInfo)

	return clusterInfo
}

func createSlot(
	t *testing.T,
	operator id.KramaID,
	selfID id.KramaID,
	sender common.Address,
	receiver common.Address,
	slotType ktypes.SlotType,
	nonRegisteredReceiver bool,
) *ktypes.Slot {
	t.Helper()

	var ixs common.Interactions

	if nonRegisteredReceiver {
		ixParams := tests.GetIxParamsWithAddress(sender, receiver)
		ixs = append(ixs, tests.CreateIX(t, ixParams))
	} else {
		ixs = createAssetIx(t, sender)
	}

	nodeset := createNodeSet(t, 1, 1, 1, 1, 2, 0)
	clusterInfo := createTestClusterInfo(t, operator, selfID, nodeset, ixs, nonRegisteredReceiver)
	slot := ktypes.NewSlot(slotType, clusterInfo)

	return slot
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
