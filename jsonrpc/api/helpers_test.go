package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log"
	"math/big"
	"math/rand"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/sarvalabs/go-moi/common/hexutil"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/google/uuid"
	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	libp2pTest "github.com/libp2p/go-libp2p/core/test"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
)

type ixData struct {
	ix           *common.Interaction
	tsHash       common.Hash
	participants common.ParticipantsState
	ixIndex      int
}

type MockChainManager struct {
	receipts                   map[common.Hash]*common.Receipt
	tesseractsByHash           map[common.Hash]*common.Tesseract
	tesseractsByHeight         map[string]*common.Tesseract
	latestTesseracts           map[identifiers.Identifier]*common.Tesseract
	ixByTesseract              map[common.Hash]ixData
	ixByHash                   map[common.Hash]ixData
	TSHashByHeight             map[string]common.Hash
	GetInteractionByIxHashHook func() error
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.receipts = make(map[common.Hash]*common.Receipt)
	mockChain.tesseractsByHash = make(map[common.Hash]*common.Tesseract)
	mockChain.tesseractsByHeight = make(map[string]*common.Tesseract)
	mockChain.latestTesseracts = make(map[identifiers.Identifier]*common.Tesseract)
	mockChain.ixByHash = make(map[common.Hash]ixData)
	mockChain.ixByTesseract = make(map[common.Hash]ixData)
	mockChain.TSHashByHeight = make(map[string]common.Hash)

	return mockChain
}

func (c *MockChainManager) GetInteractionAndParticipantsByIxHash(
	ixHash common.Hash,
) (*common.Interaction, common.Hash, common.ParticipantsState, int, error) {
	if c.GetInteractionByIxHashHook != nil {
		return nil, common.NilHash, nil, 0, c.GetInteractionByIxHashHook()
	}

	data, ok := c.ixByHash[ixHash]
	if !ok {
		return nil, common.NilHash, nil, 0, common.ErrFetchingInteraction
	}

	return data.ix, data.tsHash, data.participants, data.ixIndex, nil
}

func (c *MockChainManager) GetInteractionAndParticipantsByTSHash(
	tsHash common.Hash,
	ixIndex int,
) (*common.Interaction, common.ParticipantsState, error) {
	data, ok := c.ixByTesseract[tsHash]
	if !ok {
		return nil, nil, common.ErrFetchingInteraction
	}

	return data.ix, data.participants, nil
}

func (c *MockChainManager) SetTesseractHeightEntry(id identifiers.Identifier, height uint64, hash common.Hash) {
	key := id.Hex() + strconv.FormatUint(height, 10)
	c.TSHashByHeight[key] = hash
}

func (c *MockChainManager) GetTesseractHeightEntry(id identifiers.Identifier, height uint64) (common.Hash, error) {
	key := id.Hex() + strconv.FormatUint(height, 10)

	hash, ok := c.TSHashByHeight[key]
	if !ok {
		return common.NilHash, common.ErrKeyNotFound
	}

	return hash, nil
}

func (c *MockChainManager) SetInteractionDataByTSHash(
	tsHash common.Hash,
	ix *common.Interaction,
	participants common.ParticipantsState,
) {
	c.ixByTesseract[tsHash] = ixData{
		ix:           ix,
		tsHash:       tsHash,
		participants: participants,
	}
}

func (c *MockChainManager) SetInteractionDataByIxHash(
	ix *common.Interaction,
	tsHash common.Hash,
	participants common.ParticipantsState,
	ixIndex int,
) {
	c.ixByHash[ix.Hash()] = ixData{
		ix:           ix,
		tsHash:       tsHash,
		participants: participants,
		ixIndex:      ixIndex,
	}
}

// Chain manager mock functions
func (c *MockChainManager) GetLatestTesseract(id identifiers.Identifier, withIxns bool) (*common.Tesseract, error) {
	ts, ok := c.latestTesseracts[id]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withIxns {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (c *MockChainManager) GetTesseract(
	hash common.Hash,
	withInteractions bool,
	withCommitInfo bool,
) (*common.Tesseract, error) {
	ts, ok := c.tesseractsByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	if !withCommitInfo {
		tsCopy = *tsCopy.GetTesseractWithoutCommitInfo()
	}

	return &tsCopy, nil
}

func (c *MockChainManager) setReceiptByIXHash(ixHash common.Hash, receipt *common.Receipt) {
	c.receipts[ixHash] = receipt
}

func (c *MockChainManager) GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error) {
	if receipt := c.receipts[ixHash]; receipt != nil {
		return receipt, nil
	}

	return nil, common.ErrReceiptNotFound
}

func (c *MockChainManager) setTesseractByHash(
	t *testing.T,
	ts *common.Tesseract,
) {
	t.Helper()

	c.tesseractsByHash[tests.GetTesseractHash(t, ts)] = ts
}

type MockStateManager struct {
	sequenceID              map[identifiers.Identifier]uint64
	storage                 map[common.Hash][]byte
	balances                map[identifiers.Identifier]common.AssetMap
	mandates                map[identifiers.Identifier][]common.AssetMandateOrLockup
	lockups                 map[identifiers.Identifier][]common.AssetMandateOrLockup
	accounts                map[identifiers.Identifier]*common.Account
	consensusNodes          map[identifiers.Identifier][]kramaid.KramaID
	assetDeeds              map[identifiers.AssetID]*common.AssetDescriptor
	logicManifests          map[string][]byte
	logicStorage            map[string]map[string]string // first key denotes logic id, second key denotes storage key
	accMetaInfo             map[identifiers.Identifier]*common.AccountMetaInfo
	logicIDs                map[identifiers.Identifier][]identifiers.LogicID
	deeds                   map[identifiers.Identifier]map[identifiers.Identifier]*common.AssetDescriptor
	fetchIxStateObjectsHook func() error
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)
	mockState.sequenceID = make(map[identifiers.Identifier]uint64)
	mockState.assetDeeds = make(map[identifiers.AssetID]*common.AssetDescriptor)
	mockState.balances = make(map[identifiers.Identifier]common.AssetMap)
	mockState.mandates = make(map[identifiers.Identifier][]common.AssetMandateOrLockup)
	mockState.lockups = make(map[identifiers.Identifier][]common.AssetMandateOrLockup)
	mockState.storage = make(map[common.Hash][]byte)
	mockState.accounts = make(map[identifiers.Identifier]*common.Account)
	mockState.consensusNodes = make(map[identifiers.Identifier][]kramaid.KramaID)
	mockState.logicManifests = make(map[string][]byte)
	mockState.logicStorage = make(map[string]map[string]string)
	mockState.accMetaInfo = make(map[identifiers.Identifier]*common.AccountMetaInfo)
	mockState.logicIDs = make(map[identifiers.Identifier][]identifiers.LogicID)
	mockState.deeds = make(map[identifiers.Identifier]map[identifiers.Identifier]*common.AssetDescriptor)

	return mockState
}

func (s *MockStateManager) GetAccountKeys(id identifiers.Identifier,
	stateHash common.Hash,
) (common.AccountKeys, error) {
	panic("implement me")
}

func (s *MockStateManager) setSequenceID(id identifiers.Identifier, sequenceID uint64) {
	s.sequenceID[id] = sequenceID
}

func (s *MockStateManager) GetSequenceID(id identifiers.Identifier,
	keyID uint64, stateHash common.Hash,
) (uint64, error) {
	seqID, ok := s.sequenceID[id]
	if !ok {
		return 0, common.ErrInvalidSequenceID
	}

	return seqID, nil
}

func (s *MockStateManager) FetchIxStateObjects(ixns common.Interactions,
	hashes map[identifiers.Identifier]common.Hash,
) (*state.Transition, error) {
	if s.fetchIxStateObjectsHook != nil {
		return nil, s.fetchIxStateObjectsHook()
	}

	return nil, nil
}

func (s *MockStateManager) CreateStateObject(id identifiers.Identifier,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetStateObjectByHash(id identifiers.Identifier, hash common.Hash) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) setDeeds(
	t *testing.T, id identifiers.Identifier,
	deeds map[identifiers.Identifier]*common.AssetDescriptor,
) {
	t.Helper()

	s.deeds[id] = deeds
}

func (s *MockStateManager) GetDeeds(
	id identifiers.Identifier,
	stateHash common.Hash,
) (map[identifiers.Identifier]*common.AssetDescriptor, error) {
	deeds, ok := s.deeds[id]
	if !ok {
		return nil, errors.New("deeds not found")
	}

	return deeds, nil
}

func (s *MockStateManager) GetMandates(
	id identifiers.Identifier, hash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	if mandates, ok := s.mandates[id]; ok {
		return mandates, nil
	}

	return []common.AssetMandateOrLockup{}, nil
}

func (s *MockStateManager) GetLockups(
	id identifiers.Identifier, hash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	if lockups, ok := s.lockups[id]; ok {
		return lockups, nil
	}

	return []common.AssetMandateOrLockup{}, nil
}

func (s *MockStateManager) GetAssetInfo(
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*common.AssetDescriptor, error) {
	v, ok := s.assetDeeds[assetID]
	if !ok {
		return nil, common.ErrAssetNotFound
	}

	return v, nil
}

func (s *MockStateManager) addAsset(assetID identifiers.AssetID, descriptor *common.AssetDescriptor) {
	s.assetDeeds[assetID] = descriptor
}

func (s *MockStateManager) GetLogicManifest(logicID identifiers.LogicID, stateHash common.Hash) ([]byte, error) {
	logicManifest, ok := s.logicManifests[hex.EncodeToString(logicID.Bytes())]
	if !ok {
		return logicManifest, errors.New("logic manifest not found")
	}

	return logicManifest, nil
}

func (s *MockStateManager) setLogicIDs(
	t *testing.T,
	id identifiers.Identifier,
	logicIDs []identifiers.LogicID,
) {
	t.Helper()

	s.logicIDs[id] = logicIDs
}

func (s *MockStateManager) GetLogicIDs(
	id identifiers.Identifier,
	stateHash common.Hash,
) ([]identifiers.LogicID, error) {
	logicIDs, ok := s.logicIDs[id]
	if !ok {
		return nil, errors.New("logic IDs not found")
	}

	return logicIDs, nil
}

func (s *MockStateManager) setAccountMetaInfo(
	t *testing.T,
	id identifiers.Identifier,
	accMetaInfo *common.AccountMetaInfo,
) {
	t.Helper()

	s.accMetaInfo[id] = accMetaInfo
}

func (s *MockStateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := s.accMetaInfo[id]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return accMetaInfo, nil
}

func (s *MockStateManager) setStorageEntry(t *testing.T, logicID identifiers.LogicID, storage map[string]string) {
	t.Helper()

	s.logicStorage[hex.EncodeToString(logicID.Bytes())] = storage
}

func (s *MockStateManager) GetPersistentStorageEntry(
	logicID identifiers.LogicID,
	key []byte, _ common.Hash,
) (
	[]byte, error,
) {
	logicStorage, ok := s.logicStorage[hex.EncodeToString(logicID.Bytes())]
	if !ok {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	value, ok := logicStorage[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return []byte(value), nil
}

func (s *MockStateManager) GetEphemeralStorageEntry(
	id identifiers.Identifier,
	logicID identifiers.LogicID,
	key []byte, _ common.Hash,
) (
	[]byte, error,
) {
	logicStorage, ok := s.logicStorage[hex.EncodeToString(logicID.Bytes())]
	if !ok {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	value, ok := logicStorage[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return []byte(value), nil
}

func (s *MockStateManager) GetLatestStateObject(id identifiers.Identifier) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetAccountState(id identifiers.Identifier, stateHash common.Hash) (*common.Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (s *MockStateManager) GetConsensusNodesByHash(id identifiers.Identifier,
	hash common.Hash,
) ([]kramaid.KramaID, error) {
	nodes, ok := s.consensusNodes[id]
	if !ok {
		return nil, common.ErrContextStateNotFound
	}

	return nodes, nil
}

func (s *MockStateManager) GetBalances(id identifiers.Identifier, stateHash common.Hash) (common.AssetMap, error) {
	if _, ok := s.balances[id]; ok {
		return s.balances[id].Copy(), nil
	}

	return nil, common.ErrAccountNotFound
}

func (s *MockStateManager) GetBalance(
	id identifiers.Identifier,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	if _, ok := s.balances[id]; ok {
		if _, ok := s.balances[id][assetID]; ok {
			return s.balances[id][assetID], nil
		}

		return nil, common.ErrAssetNotFound
	}

	return nil, common.ErrAccountNotFound
}

func (s *MockStateManager) IsGenesis(id identifiers.Identifier) (bool, error) {
	if _, ok := s.storage[common.GetHash(id.Bytes())]; ok {
		return true, nil
	}

	return false, nil
}

func (s *MockStateManager) setBalance(id identifiers.Identifier, assetID identifiers.AssetID, balance *big.Int) {
	s.balances[id] = make(common.AssetMap)
	s.balances[id][assetID] = balance
}

func (s *MockStateManager) setConsensusNodes(t *testing.T, id identifiers.Identifier,
	consensusNodes []kramaid.KramaID,
) {
	t.Helper()

	s.consensusNodes[id] = consensusNodes
}

func (s *MockStateManager) setAccount(id identifiers.Identifier, acc common.Account) {
	s.accounts[id] = &acc
}

func (s *MockStateManager) setMandates(id identifiers.Identifier, mandates []common.AssetMandateOrLockup) {
	s.mandates[id] = mandates
}

func (s *MockStateManager) setLockups(id identifiers.Identifier, lockups []common.AssetMandateOrLockup) {
	s.lockups[id] = lockups
}

func (s *MockStateManager) getTDU(id identifiers.Identifier, stateHash common.Hash) common.AssetMap {
	return s.balances[id]
}

func (s *MockStateManager) setLogicManifest(logicID string, logicManifest []byte) {
	s.logicManifests[logicID] = logicManifest
}

type MockExecutionManager struct {
	call map[common.Hash]*common.Receipt
}

func NewMockExecutionManager(t *testing.T) *MockExecutionManager {
	t.Helper()

	exec := new(MockExecutionManager)
	exec.call = make(map[common.Hash]*common.Receipt)

	return exec
}

func (exec *MockExecutionManager) setInteractionCall(ix *common.Interaction, receipt *common.Receipt) {
	exec.call[ix.Hash()] = receipt
}

func (exec *MockExecutionManager) InteractionCall(
	ctx *common.ExecutionContext,
	ix *common.Interaction,
	transition *state.Transition,
) (*common.Receipt, error) {
	receipt, ok := exec.call[ix.Hash()]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return receipt, nil
}

type MockSyncer struct {
	accSyncStatus         map[identifiers.Identifier]*rpcargs.AccSyncStatus
	nodeSyncStatus        *rpcargs.NodeSyncStatus
	pendingNodeSyncStatus *rpcargs.NodeSyncStatus
	syncJobInfo           map[identifiers.Identifier]*rpcargs.SyncJobInfo
}

func NewMockSyncer(t *testing.T) *MockSyncer {
	t.Helper()

	syncer := new(MockSyncer)
	syncer.accSyncStatus = make(map[identifiers.Identifier]*rpcargs.AccSyncStatus)
	syncer.syncJobInfo = make(map[identifiers.Identifier]*rpcargs.SyncJobInfo)

	return syncer
}

func (syncer *MockSyncer) setAccountSyncStatus(id identifiers.Identifier, accSyncStatus *rpcargs.AccSyncStatus) {
	syncer.accSyncStatus[id] = accSyncStatus
}

func (syncer *MockSyncer) GetAccountSyncStatus(id identifiers.Identifier) (*rpcargs.AccSyncStatus, error) {
	if accSyncStatus, ok := syncer.accSyncStatus[id]; ok {
		return accSyncStatus, nil
	}

	return nil, common.ErrAccSyncStatusNotFound
}

func (syncer *MockSyncer) setSyncJobInfo(id identifiers.Identifier, syncJobInfo *rpcargs.SyncJobInfo) {
	syncer.syncJobInfo[id] = syncJobInfo
}

func (syncer *MockSyncer) GetSyncJobInfo(id identifiers.Identifier) (*rpcargs.SyncJobInfo, error) {
	syncJobStatus, ok := syncer.syncJobInfo[id]
	if !ok {
		return nil, common.ErrSyncJobNotFound
	}

	return syncJobStatus, nil
}

func (syncer *MockSyncer) setNodeSyncStatus(nodeSyncStatus *rpcargs.NodeSyncStatus) {
	syncer.nodeSyncStatus = nodeSyncStatus
}

func (syncer *MockSyncer) setPendingNodeSyncStatus(pendingNodeSyncStatus *rpcargs.NodeSyncStatus) {
	syncer.pendingNodeSyncStatus = pendingNodeSyncStatus
}

func (syncer *MockSyncer) GetNodeSyncStatus(includePendingAccounts bool) *rpcargs.NodeSyncStatus {
	if includePendingAccounts {
		return syncer.pendingNodeSyncStatus
	}

	return syncer.nodeSyncStatus
}

type MockIxPool struct {
	interactions       map[common.Hash]*common.Interaction
	nextNonce          map[identifiers.Identifier]uint64
	waitTime           map[identifiers.Identifier]*big.Int
	pending            map[identifiers.Identifier][]*common.Interaction
	queued             map[identifiers.Identifier][]*common.Interaction
	pendingIX          map[common.Hash]*common.Interaction
	addInteractionHook func() []error
}

func NewMockIxPool(t *testing.T) *MockIxPool {
	t.Helper()

	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[common.Hash]*common.Interaction)
	ixpool.nextNonce = make(map[identifiers.Identifier]uint64)
	ixpool.waitTime = make(map[identifiers.Identifier]*big.Int)
	ixpool.pending = make(map[identifiers.Identifier][]*common.Interaction)
	ixpool.queued = make(map[identifiers.Identifier][]*common.Interaction)
	ixpool.pendingIX = make(map[common.Hash]*common.Interaction)

	return ixpool
}

func (mc *MockIxPool) GetSequenceID(id identifiers.Identifier, keyID uint64) (uint64, error) {
	if nextNonce, ok := mc.nextNonce[id]; ok {
		return atomic.LoadUint64(&nextNonce), nil
	}

	return 0, common.ErrAccountNotFound
}

func (mc *MockIxPool) SetPendingIx(ix *common.Interaction) {
	mc.pendingIX[ix.Hash()] = ix
}

func (mc *MockIxPool) GetPendingIx(ixHash common.Hash) (*common.Interaction, bool) {
	ix, ok := mc.pendingIX[ixHash]
	if !ok {
		return nil, false
	}

	return ix, true
}

func (mc *MockIxPool) AddLocalInteractions(ixs common.Interactions) []error {
	if mc.addInteractionHook != nil {
		return mc.addInteractionHook()
	}

	for _, ix := range ixs.IxList() {
		mc.interactions[ix.Hash()] = ix
	}

	return nil
}

func (mc *MockIxPool) GetIxs(id identifiers.Identifier, inclQueued bool) (promoted, enqueued []*common.Interaction) {
	if inclQueued {
		return mc.pending[id], mc.queued[id]
	}

	return mc.pending[id], []*common.Interaction{}
}

func (mc *MockIxPool) GetAllIxs(inclQueued bool) (promoted, enqueued map[identifiers.Identifier][]*common.Interaction) {
	if inclQueued {
		return mc.pending, mc.queued
	}

	return mc.pending, map[identifiers.Identifier][]*common.Interaction{}
}

func (mc *MockIxPool) GetAccountWaitTime(id identifiers.Identifier) (*big.Int, error) {
	if waitTime, ok := mc.waitTime[id]; ok {
		return waitTime, nil
	}

	return nil, common.ErrAccountNotFound
}

func (mc *MockIxPool) GetAllAccountsWaitTime() map[identifiers.Identifier]*big.Int {
	return mc.waitTime
}

func (mc *MockIxPool) setNonce(id identifiers.Identifier, nonce uint64) {
	mc.nextNonce[id] = nonce
}

func (mc *MockIxPool) setWaitTime(id identifiers.Identifier, waitTime int64) {
	mc.waitTime[id] = big.NewInt(waitTime)
}

func (mc *MockIxPool) setIxs(id identifiers.Identifier, pending, queued []*common.Interaction) {
	mc.pending[id] = pending
	mc.queued[id] = queued
}

type MockNetwork struct {
	peers             []kramaid.KramaID
	version           string
	conns             []network.Conn
	inboundConnCount  int64
	outboundConnCount int64
	pubsubTopics      map[string]int
}

func (mn *MockNetwork) GetPeersScores() map[peer.ID]*pubsub.PeerScoreSnapshot {
	// TODO implement me
	panic("implement me")
}

func NewMockNetwork(t *testing.T) *MockNetwork {
	t.Helper()

	mn := new(MockNetwork)
	mn.peers = make([]kramaid.KramaID, 0)
	mn.version = ""
	mn.conns = make([]network.Conn, 0)

	return mn
}

func (mn *MockNetwork) setConns(conns []network.Conn) {
	mn.conns = conns
}

func (mn *MockNetwork) GetConns() []network.Conn {
	return mn.conns
}

func (mn *MockNetwork) GetKramaID() kramaid.KramaID {
	panic("implement me")
}

func (mn *MockNetwork) setPeers(peersList []kramaid.KramaID) {
	mn.peers = peersList
}

func (mn *MockNetwork) GetPeers() []kramaid.KramaID {
	return mn.peers
}

func (mn *MockNetwork) setVersion(version string) {
	mn.version = version
}

func (mn *MockNetwork) GetVersion() string {
	return mn.version
}

func (mn *MockNetwork) setInboundConnCount(inboundConnCount int64) {
	mn.inboundConnCount = inboundConnCount
}

func (mn *MockNetwork) GetInboundConnCount() int64 {
	return mn.inboundConnCount
}

func (mn *MockNetwork) setOutboundConnCount(outboundConnCount int64) {
	mn.outboundConnCount = outboundConnCount
}

func (mn *MockNetwork) GetOutboundConnCount() int64 {
	return mn.outboundConnCount
}

func (mn *MockNetwork) setSubscribedTopics(pubsubTopics map[string]int) {
	mn.pubsubTopics = pubsubTopics
}

func (mn *MockNetwork) GetSubscribedTopics() map[string]int {
	return mn.pubsubTopics
}

type mockStream struct {
	network.MuxedStream
	protocol  protocol.ID
	direction int
}

func (ms *mockStream) ID() string {
	// TODO implement me
	panic("implement me")
}

func (ms *mockStream) Protocol() protocol.ID {
	return ms.protocol
}

func (ms *mockStream) SetProtocol(id protocol.ID) error {
	// TODO implement me
	panic("implement me")
}

func (ms *mockStream) Conn() network.Conn {
	// TODO implement me
	panic("implement me")
}

func (ms *mockStream) Scope() network.StreamScope {
	// TODO implement me
	panic("implement me")
}

func (ms *mockStream) ProtocolID() protocol.ID {
	// TODO implement me
	panic("implement me")
}

func (ms *mockStream) Stat() network.Stats {
	return network.Stats{
		Direction: 1,
	}
}

func createStreams(streamCount int) []network.Stream {
	streams := make([]network.Stream, streamCount)

	for i := 0; i < streamCount; i++ {
		streams[i] = &mockStream{
			protocol:  "/meshsub/1.1.0",
			direction: 1,
		}
	}

	return streams
}

type mockConn struct {
	remotePeerID peer.ID
	streams      []network.Stream
}

func (m mockConn) IsClosed() bool {
	panic("implement me")
}

func (m mockConn) Close() error {
	panic("implement me")
}

func (m mockConn) LocalPeer() peer.ID {
	panic("implement me")
}

func (m mockConn) LocalPrivateKey() libp2pCrypto.PrivKey {
	panic("implement me")
}

func (m *mockConn) SetRemotePeer(id peer.ID) {
	m.remotePeerID = id
}

func (m mockConn) RemotePeer() peer.ID {
	return m.remotePeerID
}

func (m mockConn) RemotePublicKey() libp2pCrypto.PubKey {
	panic("implement me")
}

func (m mockConn) ConnState() network.ConnectionState {
	panic("implement me")
}

func (m mockConn) LocalMultiaddr() multiaddr.Multiaddr {
	panic("implement me")
}

func (m mockConn) RemoteMultiaddr() multiaddr.Multiaddr {
	panic("implement me")
}

func (m mockConn) Stat() network.ConnStats {
	panic("implement me")
}

func (m mockConn) Scope() network.ConnScope {
	panic("implement me")
}

func (m mockConn) ID() string {
	panic("implement me")
}

func (m mockConn) NewStream(ctx context.Context) (network.Stream, error) {
	panic("implement me")
}

func (m *mockConn) SetStreams(streams []network.Stream) {
	m.streams = streams
}

func (m mockConn) GetStreams() []network.Stream {
	return m.streams
}

func createConns(t *testing.T, connCount int, streamCount int) []network.Conn {
	t.Helper()

	peerID, err := libp2pTest.RandPeerID()
	require.NoError(t, err)

	conns := make([]network.Conn, connCount)
	for i := 0; i < connCount; i++ {
		conns[i] = mockConn{
			remotePeerID: peerID,
			streams:      createStreams(streamCount),
		}
	}

	return conns
}

type MockDatabase struct {
	database map[string][]byte
	addrList []identifiers.Identifier
}

func NewMockDatabase(t *testing.T) *MockDatabase {
	t.Helper()

	db := new(MockDatabase)
	db.database = make(map[string][]byte)

	return db
}

func (d *MockDatabase) setDBEntry(key []byte, value []byte) {
	d.database[string(key)] = value
}

func (d *MockDatabase) ReadEntry(key []byte) ([]byte, error) {
	if _, ok := d.database[string(key)]; ok {
		return d.database[string(key)], nil
	}

	return nil, common.ErrKeyNotFound
}

func (d *MockDatabase) setList(t *testing.T, idList []identifiers.Identifier) {
	t.Helper()

	d.addrList = idList
}

func (d *MockDatabase) GetRegisteredAccounts() ([]identifiers.Identifier, error) {
	return d.addrList, nil
}

func (d *MockDatabase) GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *common.DBEntry, error) {
	ch := make(chan *common.DBEntry)

	go func() {
		defer close(ch)

		for key, value := range d.database {
			if bytes.HasPrefix([]byte(key), prefix) {
				dbEntry := &common.DBEntry{
					Key:   []byte(key),
					Value: value,
				}

				select {
				case <-ctx.Done():
					return
				case ch <- dbEntry:
				}
			}
		}
	}()

	return ch, nil
}

func (d *MockDatabase) setNodeMetaInfo(t *testing.T, entries map[peer.ID]*senatus.NodeMetaInfo) {
	t.Helper()

	for key, entry := range entries {
		value, err := entry.Bytes()
		require.NoError(t, err)

		d.setDBEntry(storage.SenatusDBKey(key), value)
	}
}

func createNodeMetaInfo(t *testing.T, count int) ([]peer.ID, map[peer.ID]*senatus.NodeMetaInfo) {
	t.Helper()

	peerIDs := make([]peer.ID, 0)
	kramaIDs := tests.RandomKramaIDs(t, count)
	nodeMetaInfo := make(map[peer.ID]*senatus.NodeMetaInfo)

	for _, kramaID := range kramaIDs {
		peerID, err := kramaID.DecodedPeerID()
		require.NoError(t, err)

		peerIDs = append(peerIDs, peerID)
		nodeMetaInfo[peerID] = &senatus.NodeMetaInfo{
			KramaID:     kramaID,
			RTT:         int64(rand.Intn(500)),
			WalletCount: int32(rand.Intn(5)),
		}
	}

	return peerIDs, nodeMetaInfo
}

func createMandatesOrLockups(t *testing.T) ([]common.AssetMandateOrLockup, []rpcargs.RPCMandateOrLockup) {
	t.Helper()

	assetIDs, _ := tests.CreateTestAssets(t, 3)
	list := make([]common.AssetMandateOrLockup, 0)
	rpcList := make([]rpcargs.RPCMandateOrLockup, 0)

	for _, assetID := range assetIDs {
		id := tests.RandomIdentifier(t)
		amount := big.NewInt(int64(rand.Uint64()))

		list = append(list, common.AssetMandateOrLockup{
			AssetID: assetID,
			ID:      id,
			Amount:  amount,
		})

		rpcList = append(rpcList, rpcargs.RPCMandateOrLockup{
			ID:      id,
			AssetID: assetID.String(),
			Amount:  (*hexutil.Big)(amount),
		})
	}

	return list, rpcList
}

func getTesseractsHashes(t *testing.T, tesseracts []*common.Tesseract) []common.Hash {
	t.Helper()

	count := len(tesseracts)
	hashes := make([]common.Hash, count)

	for i, ts := range tesseracts {
		hashes[i] = getTesseractHash(t, ts)
	}

	return hashes
}

func getTesseractHash(t *testing.T, tesseract *common.Tesseract) common.Hash {
	t.Helper()

	return tesseract.Hash()
}

func getDeeds(
	t *testing.T,
	assetIDs []identifiers.AssetID,
	assetDescriptors []*common.AssetDescriptor,
) (map[identifiers.Identifier]*common.AssetDescriptor, []rpcargs.RPCDeeds) {
	t.Helper()

	count := len(assetIDs)
	deedsMap := make(map[identifiers.Identifier]*common.AssetDescriptor, count)
	deedsEntries := make([]rpcargs.RPCDeeds, 0, count)

	for i := 0; i < count; i++ {
		deedsEntries = append(deedsEntries, rpcargs.RPCDeeds{
			AssetID:   assetIDs[i].String(),
			AssetInfo: *rpcargs.GetRPCAssetDescriptor(assetDescriptors[i]),
		})

		deedsMap[assetIDs[i].AsIdentifier()] = assetDescriptors[i]
	}

	return deedsMap, deedsEntries
}

func getSignatureBytes(t *testing.T, ixData *common.IxData, mnemonic string) []byte {
	t.Helper()

	bz, err := ixData.Bytes()
	require.NoError(t, err)

	sign, err := crypto.GetSignature(bz, mnemonic)
	require.NoError(t, err)

	signBytes, err := hex.DecodeString(sign)
	require.NoError(t, err)

	return signBytes
}

func newTestInteraction(
	t *testing.T,
	ixType common.IxOpType,
	callback func(ixData *common.IxData),
) *common.Interaction {
	t.Helper()

	ixData := &common.IxData{
		FuelLimit: 10000,
		IxOps: []common.IxOpRaw{
			{
				Type:    ixType,
				Payload: tests.CreateTxPayload(t, ixType, tests.RandomIdentifier(t)),
			},
		},
	}

	if callback != nil {
		callback(ixData)
	}

	ixData.Participants = []common.IxParticipant{
		{
			ID:       ixData.Sender.ID,
			LockType: common.MutateLock,
		},
	}

	ix, err := common.NewInteraction(*ixData, nil)
	require.NoError(t, err)

	return ix
}

type tsFilter struct {
	tsChanges []*rpcargs.RPCTesseract
}

type tsByAccFilter struct {
	tsByAccFilterParams identifiers.Identifier
	tsByAccChanges      []*rpcargs.RPCTesseract
}

type pendingIxnsFilter struct {
	pendingIxnsChanges []common.Hash
}

type MockFilterManager struct {
	tsFilter          map[string]tsFilter
	tsByAccFilter     map[string]tsByAccFilter
	pendingIxnsFilter map[string]pendingIxnsFilter

	// used for GetLogs and GetFilterChanges methods
	logFilter map[string]common.Hash
	logs      map[common.Hash][]*rpcargs.RPCLog
}

func NewMockFilterManager(t *testing.T) *MockFilterManager {
	t.Helper()

	mockFilter := new(MockFilterManager)
	mockFilter.tsFilter = make(map[string]tsFilter)
	mockFilter.tsByAccFilter = make(map[string]tsByAccFilter)
	mockFilter.pendingIxnsFilter = make(map[string]pendingIxnsFilter)
	mockFilter.logFilter = make(map[string]common.Hash)
	mockFilter.logs = make(map[common.Hash][]*rpcargs.RPCLog)

	return mockFilter
}

func (f *MockFilterManager) setTSFilter(id string) {
	f.tsFilter[id] = tsFilter{
		tsChanges: nil,
	}
}

func (f *MockFilterManager) getTSFilter(id string) bool {
	_, exists := f.tsFilter[id]

	return exists
}

func (f *MockFilterManager) NewTesseractFilter(ws jsonrpc.ConnManager) string {
	filterID := uuid.New().String()

	f.setTSFilter(filterID)

	return filterID
}

func (f *MockFilterManager) setTSByAccFilter(filterID string, id identifiers.Identifier) {
	f.tsByAccFilter[filterID] = tsByAccFilter{
		tsByAccFilterParams: id,
	}
}

func (f *MockFilterManager) getTSByAccFilter(id string) (tsByAccFilter, bool) {
	resp, exists := f.tsByAccFilter[id]
	if !exists {
		return tsByAccFilter{}, exists
	}

	return resp, exists
}

func (f *MockFilterManager) NewTesseractsByAccountFilter(ws jsonrpc.ConnManager, id identifiers.Identifier) string {
	filterID := uuid.New().String()

	f.setTSByAccFilter(filterID, id)

	return filterID
}

func (f *MockFilterManager) setLogFilter(id string, logQuery *jsonrpc.LogQuery) {
	// use hash of logQuery as key to set and get logs
	hash, err := common.PoloHash(*logQuery)
	if err != nil {
		log.Fatal(err)
	}

	f.logFilter[id] = hash
}

func (f *MockFilterManager) getLogFilter(id string) (common.Hash, bool) {
	resp, exists := f.logFilter[id]
	if !exists {
		return common.NilHash, exists
	}

	return resp, exists
}

func (f *MockFilterManager) NewLogFilter(ws jsonrpc.ConnManager, logQuery *jsonrpc.LogQuery) string {
	filterID := uuid.New().String()

	f.setLogFilter(filterID, logQuery)

	return filterID
}

func (f *MockFilterManager) setIxnsFilter(id string) {
	f.pendingIxnsFilter[id] = pendingIxnsFilter{
		pendingIxnsChanges: nil,
	}
}

func (f *MockFilterManager) getIxnsFilter(id string) bool {
	_, exists := f.pendingIxnsFilter[id]

	return exists
}

func (f *MockFilterManager) PendingIxnsFilter(ws jsonrpc.ConnManager) string {
	filterID := uuid.New().String()

	f.setIxnsFilter(filterID)

	return filterID
}

func (f *MockFilterManager) Uninstall(id string) bool {
	_, exists := f.tsByAccFilter[id]
	if exists {
		delete(f.tsByAccFilter, id)

		return true
	}

	return false
}

func (f *MockFilterManager) setTSByAccFilterChanges(id string, ts []*rpcargs.RPCTesseract) {
	f.tsByAccFilter[id] = tsByAccFilter{
		tsByAccChanges: ts,
	}
}

func (f *MockFilterManager) GetFilterChanges(id string) (interface{}, error) {
	resp, exists := f.tsByAccFilter[id]
	if exists {
		return resp.tsByAccChanges, nil
	}

	return nil, errors.New("unknown subscription type")
}

func (f *MockFilterManager) setLogs(id string, logs []*rpcargs.RPCLog) {
	f.logs[f.logFilter[id]] = logs
}

func (f *MockFilterManager) GetLogsForQuery(query jsonrpc.LogQuery) ([]*rpcargs.RPCLog, error) {
	// use hash of logQuery as key to set and get logs
	hash, err := common.PoloHash(query)
	if err != nil {
		return nil, err
	}

	return f.logs[hash], nil
}

func NumPointer(t *testing.T, input int64) *int64 {
	t.Helper()

	return &input
}

func sortDeeds(deeds []rpcargs.RPCDeeds) {
	sort.Slice(deeds, func(i, j int) bool {
		return deeds[i].AssetID < deeds[j].AssetID
	})
}
