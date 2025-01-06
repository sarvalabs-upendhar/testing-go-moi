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
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
)

type Context struct {
	behaviourNodes []kramaid.KramaID
	randomNodes    []kramaid.KramaID
}

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
	latestTesseracts           map[identifiers.Address]*common.Tesseract
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
	mockChain.latestTesseracts = make(map[identifiers.Address]*common.Tesseract)
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

func (c *MockChainManager) SetTesseractHeightEntry(address identifiers.Address, height uint64, hash common.Hash) {
	key := address.Hex() + strconv.FormatUint(height, 10)
	c.TSHashByHeight[key] = hash
}

func (c *MockChainManager) GetTesseractHeightEntry(address identifiers.Address, height uint64) (common.Hash, error) {
	key := address.Hex() + strconv.FormatUint(height, 10)

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
func (c *MockChainManager) GetLatestTesseract(addr identifiers.Address, withIxns bool) (*common.Tesseract, error) {
	ts, ok := c.latestTesseracts[addr]
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
	sequenceID              map[identifiers.Address]uint64
	storage                 map[common.Hash][]byte
	balances                map[identifiers.Address]common.AssetMap
	mandates                map[identifiers.Address][]common.AssetMandateOrLockup
	lockups                 map[identifiers.Address][]common.AssetMandateOrLockup
	accounts                map[identifiers.Address]*common.Account
	context                 map[identifiers.Address]*Context
	assetDeeds              map[identifiers.AssetID]*common.AssetDescriptor
	logicManifests          map[string][]byte
	logicStorage            map[string]map[string]string // first key denotes logic id, second key denotes storage key
	accMetaInfo             map[identifiers.Address]*common.AccountMetaInfo
	logicIDs                map[identifiers.Address][]identifiers.LogicID
	deeds                   map[identifiers.Address]map[string]*common.AssetDescriptor
	fetchIxStateObjectsHook func() error
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)
	mockState.sequenceID = make(map[identifiers.Address]uint64)
	mockState.assetDeeds = make(map[identifiers.AssetID]*common.AssetDescriptor)
	mockState.balances = make(map[identifiers.Address]common.AssetMap)
	mockState.mandates = make(map[identifiers.Address][]common.AssetMandateOrLockup)
	mockState.lockups = make(map[identifiers.Address][]common.AssetMandateOrLockup)
	mockState.storage = make(map[common.Hash][]byte)
	mockState.accounts = make(map[identifiers.Address]*common.Account)
	mockState.context = make(map[identifiers.Address]*Context)
	mockState.logicManifests = make(map[string][]byte)
	mockState.logicStorage = make(map[string]map[string]string)
	mockState.accMetaInfo = make(map[identifiers.Address]*common.AccountMetaInfo)
	mockState.logicIDs = make(map[identifiers.Address][]identifiers.LogicID)
	mockState.deeds = make(map[identifiers.Address]map[string]*common.AssetDescriptor)

	return mockState
}

func (s *MockStateManager) GetAccountKeys(addrs identifiers.Address,
	stateHash common.Hash,
) (common.AccountKeys, error) {
	panic("implement me")
}

func (s *MockStateManager) setSequenceID(addr identifiers.Address, sequenceID uint64) {
	s.sequenceID[addr] = sequenceID
}

func (s *MockStateManager) GetSequenceID(addr identifiers.Address,
	keyID uint64, stateHash common.Hash,
) (uint64, error) {
	id, ok := s.sequenceID[addr]
	if !ok {
		return 0, common.ErrInvalidSequenceID
	}

	return id, nil
}

func (s *MockStateManager) FetchIxStateObjects(ixns common.Interactions,
	hashes map[identifiers.Address]common.Hash,
) (*state.Transition, error) {
	if s.fetchIxStateObjectsHook != nil {
		return nil, s.fetchIxStateObjectsHook()
	}

	return nil, nil
}

func (s *MockStateManager) CreateStateObject(address identifiers.Address,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetStateObjectByHash(addr identifiers.Address, hash common.Hash) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) IsAccountRegistered(address identifiers.Address) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) setDeeds(
	t *testing.T, addr identifiers.Address,
	deeds map[string]*common.AssetDescriptor,
) {
	t.Helper()

	s.deeds[addr] = deeds
}

func (s *MockStateManager) GetDeeds(
	addr identifiers.Address,
	stateHash common.Hash,
) (map[string]*common.AssetDescriptor, error) {
	deeds, ok := s.deeds[addr]
	if !ok {
		return nil, errors.New("deeds not found")
	}

	return deeds, nil
}

func (s *MockStateManager) GetMandates(
	address identifiers.Address, hash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	if mandates, ok := s.mandates[address]; ok {
		return mandates, nil
	}

	return []common.AssetMandateOrLockup{}, nil
}

func (s *MockStateManager) GetLockups(
	address identifiers.Address, hash common.Hash,
) ([]common.AssetMandateOrLockup, error) {
	if lockups, ok := s.lockups[address]; ok {
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
	logicManifest, ok := s.logicManifests[string(logicID)]
	if !ok {
		return logicManifest, errors.New("logic manifest not found")
	}

	return logicManifest, nil
}

func (s *MockStateManager) setLogicIDs(
	t *testing.T,
	addr identifiers.Address,
	logicIDs []identifiers.LogicID,
) {
	t.Helper()

	s.logicIDs[addr] = logicIDs
}

func (s *MockStateManager) GetLogicIDs(addr identifiers.Address, stateHash common.Hash) ([]identifiers.LogicID, error) {
	logicIDs, ok := s.logicIDs[addr]
	if !ok {
		return nil, errors.New("logic IDs not found")
	}

	return logicIDs, nil
}

func (s *MockStateManager) setAccountMetaInfo(
	t *testing.T,
	address identifiers.Address,
	accMetaInfo *common.AccountMetaInfo,
) {
	t.Helper()

	s.accMetaInfo[address] = accMetaInfo
}

func (s *MockStateManager) GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := s.accMetaInfo[addr]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return accMetaInfo, nil
}

func (s *MockStateManager) setStorageEntry(t *testing.T, logicID identifiers.LogicID, storage map[string]string) {
	t.Helper()

	s.logicStorage[string(logicID)] = storage
}

func (s *MockStateManager) GetPersistentStorageEntry(
	logicID identifiers.LogicID,
	key []byte, _ common.Hash,
) (
	[]byte, error,
) {
	logicStorage, ok := s.logicStorage[string(logicID)]
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
	addr identifiers.Address,
	logicID identifiers.LogicID,
	key []byte, _ common.Hash,
) (
	[]byte, error,
) {
	logicStorage, ok := s.logicStorage[string(logicID)]
	if !ok {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	value, ok := logicStorage[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return []byte(value), nil
}

func (s *MockStateManager) GetLatestStateObject(addr identifiers.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetAccountState(addr identifiers.Address, stateHash common.Hash) (*common.Account, error) {
	account, ok := s.accounts[addr]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (s *MockStateManager) GetContextByHash(address identifiers.Address,
	hash common.Hash,
) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error) {
	context, ok := s.context[address]
	if !ok {
		return common.NilHash, nil, nil, common.ErrContextStateNotFound
	}

	return hash, context.behaviourNodes, context.randomNodes, nil
}

func (s *MockStateManager) GetBalances(addr identifiers.Address, stateHash common.Hash) (common.AssetMap, error) {
	if _, ok := s.balances[addr]; ok {
		return s.balances[addr].Copy(), nil
	}

	return nil, common.ErrAccountNotFound
}

func (s *MockStateManager) GetBalance(
	addr identifiers.Address,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	if _, ok := s.balances[addr]; ok {
		if _, ok := s.balances[addr][assetID]; ok {
			return s.balances[addr][assetID], nil
		}

		return nil, common.ErrAssetNotFound
	}

	return nil, common.ErrAccountNotFound
}

func (s *MockStateManager) IsGenesis(addr identifiers.Address) (bool, error) {
	if _, ok := s.storage[common.GetHash(addr.Bytes())]; ok {
		return true, nil
	}

	return false, nil
}

func (s *MockStateManager) setBalance(addr identifiers.Address, assetID identifiers.AssetID, balance *big.Int) {
	s.balances[addr] = make(common.AssetMap)
	s.balances[addr][assetID] = balance
}

func (s *MockStateManager) setContext(t *testing.T, address identifiers.Address, context *Context) {
	t.Helper()

	s.context[address] = context
}

func (s *MockStateManager) setAccount(addr identifiers.Address, acc common.Account) {
	s.accounts[addr] = &acc
}

func (s *MockStateManager) setMandates(address identifiers.Address, mandates []common.AssetMandateOrLockup) {
	s.mandates[address] = mandates
}

func (s *MockStateManager) setLockups(address identifiers.Address, lockups []common.AssetMandateOrLockup) {
	s.lockups[address] = lockups
}

func (s *MockStateManager) getTDU(addr identifiers.Address, stateHash common.Hash) common.AssetMap {
	return s.balances[addr]
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
	accSyncStatus         map[identifiers.Address]*rpcargs.AccSyncStatus
	nodeSyncStatus        *rpcargs.NodeSyncStatus
	pendingNodeSyncStatus *rpcargs.NodeSyncStatus
	syncJobInfo           map[identifiers.Address]*rpcargs.SyncJobInfo
}

func NewMockSyncer(t *testing.T) *MockSyncer {
	t.Helper()

	syncer := new(MockSyncer)
	syncer.accSyncStatus = make(map[identifiers.Address]*rpcargs.AccSyncStatus)
	syncer.syncJobInfo = make(map[identifiers.Address]*rpcargs.SyncJobInfo)

	return syncer
}

func (syncer *MockSyncer) setAccountSyncStatus(addr identifiers.Address, accSyncStatus *rpcargs.AccSyncStatus) {
	syncer.accSyncStatus[addr] = accSyncStatus
}

func (syncer *MockSyncer) GetAccountSyncStatus(addr identifiers.Address) (*rpcargs.AccSyncStatus, error) {
	if accSyncStatus, ok := syncer.accSyncStatus[addr]; ok {
		return accSyncStatus, nil
	}

	return nil, common.ErrAccSyncStatusNotFound
}

func (syncer *MockSyncer) setSyncJobInfo(addr identifiers.Address, syncJobInfo *rpcargs.SyncJobInfo) {
	syncer.syncJobInfo[addr] = syncJobInfo
}

func (syncer *MockSyncer) GetSyncJobInfo(addr identifiers.Address) (*rpcargs.SyncJobInfo, error) {
	syncJobStatus, ok := syncer.syncJobInfo[addr]
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
	nextNonce          map[identifiers.Address]uint64
	waitTime           map[identifiers.Address]*big.Int
	pending            map[identifiers.Address][]*common.Interaction
	queued             map[identifiers.Address][]*common.Interaction
	pendingIX          map[common.Hash]*common.Interaction
	addInteractionHook func() []error
}

func NewMockIxPool(t *testing.T) *MockIxPool {
	t.Helper()

	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[common.Hash]*common.Interaction)
	ixpool.nextNonce = make(map[identifiers.Address]uint64)
	ixpool.waitTime = make(map[identifiers.Address]*big.Int)
	ixpool.pending = make(map[identifiers.Address][]*common.Interaction)
	ixpool.queued = make(map[identifiers.Address][]*common.Interaction)
	ixpool.pendingIX = make(map[common.Hash]*common.Interaction)

	return ixpool
}

func (mc *MockIxPool) GetSequenceID(addr identifiers.Address, keyID uint64) (uint64, error) {
	if nextNonce, ok := mc.nextNonce[addr]; ok {
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

func (mc *MockIxPool) GetIxs(addr identifiers.Address, inclQueued bool) (promoted, enqueued []*common.Interaction) {
	if inclQueued {
		return mc.pending[addr], mc.queued[addr]
	}

	return mc.pending[addr], []*common.Interaction{}
}

func (mc *MockIxPool) GetAllIxs(inclQueued bool) (promoted, enqueued map[identifiers.Address][]*common.Interaction) {
	if inclQueued {
		return mc.pending, mc.queued
	}

	return mc.pending, map[identifiers.Address][]*common.Interaction{}
}

func (mc *MockIxPool) GetAccountWaitTime(addr identifiers.Address) (*big.Int, error) {
	if waitTime, ok := mc.waitTime[addr]; ok {
		return waitTime, nil
	}

	return nil, common.ErrAccountNotFound
}

func (mc *MockIxPool) GetAllAccountsWaitTime() map[identifiers.Address]*big.Int {
	return mc.waitTime
}

func (mc *MockIxPool) setNonce(addr identifiers.Address, nonce uint64) {
	mc.nextNonce[addr] = nonce
}

func (mc *MockIxPool) setWaitTime(addr identifiers.Address, waitTime int64) {
	mc.waitTime[addr] = big.NewInt(waitTime)
}

func (mc *MockIxPool) setIxs(addr identifiers.Address, pending, queued []*common.Interaction) {
	mc.pending[addr] = pending
	mc.queued[addr] = queued
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
	addrList []identifiers.Address
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

func (d *MockDatabase) setList(t *testing.T, addressList []identifiers.Address) {
	t.Helper()

	d.addrList = addressList
}

func (d *MockDatabase) GetRegisteredAccounts() ([]identifiers.Address, error) {
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
		addr := tests.RandomAddress(t)
		amount := big.NewInt(int64(rand.Uint64()))

		list = append(list, common.AssetMandateOrLockup{
			AssetID: assetID,
			Address: addr,
			Amount:  amount,
		})

		rpcList = append(rpcList, rpcargs.RPCMandateOrLockup{
			Address: addr,
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
) (map[string]*common.AssetDescriptor, []rpcargs.RPCDeeds) {
	t.Helper()

	count := len(assetIDs)
	deedsMap := make(map[string]*common.AssetDescriptor, count)
	deedsEntries := make([]rpcargs.RPCDeeds, 0, count)

	for i := 0; i < count; i++ {
		deedsEntries = append(deedsEntries, rpcargs.RPCDeeds{
			AssetID:   assetIDs[i].String(),
			AssetInfo: rpcargs.GetRPCAssetDescriptor(assetDescriptors[i]),
		})

		deedsMap[string(assetIDs[i])] = assetDescriptors[i]
	}

	return deedsMap, deedsEntries
}

func getContext(t *testing.T, count int) *Context {
	t.Helper()

	return &Context{
		tests.RandomKramaIDs(t, count),
		tests.RandomKramaIDs(t, count),
	}
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

func checkForContext(
	t *testing.T,
	actualContext *Context,
	expectedBehaviouralNodes []string,
	expectedRandomNodes []string,
) {
	t.Helper()

	require.Equal(t, expectedBehaviouralNodes, utils.KramaIDToString(actualContext.behaviourNodes))
	require.Equal(t, expectedRandomNodes, utils.KramaIDToString(actualContext.randomNodes))
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
				Payload: tests.CreateTxPayload(t, ixType, tests.RandomAddress(t)),
			},
		},
	}

	if callback != nil {
		callback(ixData)
	}

	ixData.Participants = []common.IxParticipant{
		{
			Address:  ixData.Sender.Address,
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
	tsByAccFilterParams identifiers.Address
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

func (f *MockFilterManager) setTSByAccFilter(id string, addr identifiers.Address) {
	f.tsByAccFilter[id] = tsByAccFilter{
		tsByAccFilterParams: addr,
	}
}

func (f *MockFilterManager) getTSByAccFilter(id string) (tsByAccFilter, bool) {
	resp, exists := f.tsByAccFilter[id]
	if !exists {
		return tsByAccFilter{}, exists
	}

	return resp, exists
}

func (f *MockFilterManager) NewTesseractsByAccountFilter(ws jsonrpc.ConnManager, addr identifiers.Address) string {
	filterID := uuid.New().String()

	f.setTSByAccFilter(filterID, addr)

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
