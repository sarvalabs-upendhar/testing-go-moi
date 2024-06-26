package forage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-moi/syncer"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
	"github.com/stretchr/testify/require"
)

type SyncEventType int

var (
	SignedData   = []byte{1, 2, 3}
	syncerEvents = map[SyncEventType]string{
		BucketSync:    "BucketSync",
		SystemAcc:     "SystemAccSync",
		SnapSync:      "SnapSync",
		LatticeSync:   "LatticeSync",
		TesseractSync: "TesseractSync",
		JobDone:       "JobDone",
	}
)

const (
	BucketSync = iota
	SystemAcc
	SnapSync
	LatticeSync
	TesseractSync
	JobDone
)

const maxTimeout = 20 * time.Second

type AccountSpecificEvents struct {
	snapSync    int
	latticeSync int
	// used to determine no of tesseract sync events and also helpful in checking if tesseracts are added in order
	startHeight int
	endHeight   int
	JobDone     int
}

func newAccountSpecificEvents(snapSync int, latticeSync int, start, end int) AccountSpecificEvents {
	return AccountSpecificEvents{
		snapSync:    snapSync,
		latticeSync: latticeSync,
		startHeight: start,
		endHeight:   end,
		JobDone:     1, // as there will be one job done per account
	}
}

type SyncEvents struct {
	bucketSync    int
	SystemAccSync int
	accounts      map[identifiers.Address]AccountSpecificEvents
}

func NewTestSyncer(
	ctx context.Context,
	cfg *config.SyncerConfig,
	node *p2p.Server,
	mux *utils.TypeMux,
	agora syncer.BlockSync,
	db store,
	slots *ktypes.Slots,
	logName string,
	callback func(s *Syncer),
) *Syncer {
	logger := hclog.NewNullLogger()

	krama := NewMockKramaEngine(db, logger)

	s := &Syncer{
		ctx:            ctx,
		network:        node,
		cfg:            cfg,
		mux:            mux,
		agora:          agora,
		db:             db,
		krama:          krama,
		lattice:        newMockLattice(db, logger),
		state:          newMockStateManager(db),
		jobWorkerCount: 5,
		jobQueue: &JobQueue{
			jobs:  make(map[identifiers.Address]*SyncJob),
			mux:   mux,
			krama: krama,
		},
		logger:              logger,
		workerSignal:        make(chan struct{}),
		tesseractRegistry:   common.NewHashRegistry(60),
		consensusSlots:      slots,
		lockedAccounts:      make(map[identifiers.Address]struct{}),
		metrics:             NilMetrics(),
		pendingMsgQueue:     make([]*TesseractInfo, 0),
		pendingMsgChan:      make(chan *TesseractInfo, 10),
		execGrid:            make(map[common.Hash]struct{}),
		tracker:             NewSyncStatusTracker(0),
		workerWaitTime:      10 * time.Millisecond,
		trustedPeersPresent: len(cfg.TrustedPeers) > 0,
	}

	if callback != nil {
		callback(s)
	}

	return s
}

func NewTestSyncerWithJobQueue(ctx context.Context, mux *utils.TypeMux) *Syncer {
	return &Syncer{
		ctx: ctx,
		jobQueue: &JobQueue{
			jobs: make(map[identifiers.Address]*SyncJob),
			mux:  mux,
		},
	}
}

func NewTestSyncerForValidation(
	ctx context.Context,
	cfg *config.SyncerConfig,
	node *p2p.Server,
	db store,
	logger hclog.Logger,
	state stateManager,
) *Syncer {
	return &Syncer{
		ctx:               ctx,
		network:           node,
		cfg:               cfg,
		db:                db,
		logger:            logger,
		state:             state,
		tesseractRegistry: common.NewHashRegistry(60),
		mux:               &utils.TypeMux{},
	}
}

type MockKramaEngine struct {
	logger             hclog.Logger
	db                 store
	executedTesseracts map[string]*common.Tesseract
	activeAccounts     map[identifiers.Address]struct{}
	mtx                sync.RWMutex
}

func (m *MockKramaEngine) AddActiveAccount(addr identifiers.Address) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if _, ok := m.activeAccounts[addr]; ok {
		return false
	}

	m.activeAccounts[addr] = struct{}{}

	return true
}

func (m *MockKramaEngine) ClearActiveAccount(addr identifiers.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	delete(m.activeAccounts, addr)
}

func NewMockKramaEngine(db store, logger hclog.Logger) *MockKramaEngine {
	return &MockKramaEngine{
		db:                 db,
		logger:             logger,
		executedTesseracts: make(map[string]*common.Tesseract),
		activeAccounts:     make(map[identifiers.Address]struct{}),
	}
}

func (m *MockKramaEngine) ValidateTesseract(
	addr identifiers.Address,
	ts *common.Tesseract,
	ics *common.ICSNodeSet,
	allParticipants bool,
) error {
	if !allParticipants && addr.IsNil() {
		return common.ErrEmptyAddress
	}

	var addresses []identifiers.Address

	if allParticipants {
		addresses = ts.Addresses()
	} else {
		addresses = append(addresses, addr)
	}

	for _, addr := range addresses {
		if ts.Height(addr) != 0 {
			if ok := m.db.HasAccMetaInfoAt(addr, ts.Height(addr)-1); !ok {
				m.logger.Trace("prev tesseract not found", "addr", addr, "height", ts.Height(addr))

				return common.ErrPreviousTesseractNotFound
			}
		}
	}

	return nil
}

func (m *MockKramaEngine) ExecuteAndValidate(
	ts *common.Tesseract,
	transition *state.Transition,
	ps map[identifiers.Address]common.IxParticipant,
) error {
	for addr, s := range ts.Participants() {
		key := addr.Hex() + strconv.FormatUint(s.Height, 10)
		m.executedTesseracts[key] = ts
	}

	return nil
}

type MockLattice struct {
	lock               sync.RWMutex
	logger             hclog.Logger
	tesseractsByHash   map[common.Hash]*common.Tesseract
	tesseractByHeight  map[string]*common.Tesseract
	tesseractHash      map[string]common.Hash
	db                 store
	executedTesseracts map[string]*common.Tesseract
}

func newMockLattice(db store, logger hclog.Logger) *MockLattice {
	return &MockLattice{
		logger:             logger,
		tesseractsByHash:   make(map[common.Hash]*common.Tesseract),
		tesseractByHeight:  make(map[string]*common.Tesseract),
		tesseractHash:      make(map[string]common.Hash),
		db:                 db,
		executedTesseracts: make(map[string]*common.Tesseract),
	}
}

func (m *MockLattice) AddTesseractWithState(
	addr identifiers.Address,
	dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if !allParticipants && addr.IsNil() {
		return common.ErrEmptyAddress
	}

	partcipants := make(common.ParticipantsState)

	if allParticipants {
		partcipants = ts.Participants()
	} else {
		s, ok := ts.State(addr)
		if !ok {
			panic(ok)
		}

		partcipants[addr] = s
	}

	for addr, p := range partcipants {
		m.logger.Trace("adding tesseract ",
			"ts-hash", ts.Hash(),
			"addr", addr,
			"height", p.Height,
		)

		if err := m.db.SetTesseractHeightEntry(addr, p.Height, ts.Hash()); err != nil {
			return err
		}

		m.setTesseractByHeight(addr, p.Height, ts)
		m.setTesseractHeightEntry(addr, p.Height, ts.Hash())

		if _, _, err := m.db.UpdateAccMetaInfo(
			addr,
			p.Height,
			ts.Hash(),
			ts.StateHash(ts.AnyAddress()),
			ts.LatestContextHash(ts.AnyAddress()),
			common.AccTypeFromIxType(ts.Interactions()[0].Type()),
		); err != nil {
			return err
		}
	}

	if !m.db.HasTesseract(ts.Hash()) {
		rawIxns, err := ts.Interactions().Bytes()
		if err != nil {
			return err
		}

		err = m.db.SetInteractions(ts.Hash(), rawIxns)
		if err != nil {
			return err
		}

		rawReceipts, err := ts.Receipts().Bytes()
		if err != nil {
			return err
		}

		err = m.db.SetReceipts(ts.Hash(), rawReceipts)
		if err != nil {
			return err
		}

		rawTS, err := ts.Canonical().Bytes()
		if err != nil {
			return err
		}

		if err := m.db.SetTesseract(ts.Hash(), rawTS); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockLattice) AddAccountMetaInfo(tesseracts ...*common.Tesseract) error {
	for _, ts := range tesseracts {
		for addr, s := range ts.Participants() {
			m.logger.Trace("adding account meta info ", "addr", addr, "height", s.Height)

			if _, _, err := m.db.UpdateAccMetaInfo(
				addr,
				s.Height,
				ts.Hash(),
				ts.StateHash(ts.AnyAddress()),
				ts.LatestContextHash(ts.AnyAddress()),
				common.AccTypeFromIxType(ts.Interactions()[0].Type()),
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *MockLattice) insertTesseractsByHash(
	t *testing.T,
	tesseracts ...*common.Tesseract,
) {
	t.Helper()

	for _, ts := range tesseracts {
		m.tesseractsByHash[ts.Hash()] = ts
	}
}

func (m *MockLattice) GetTesseract(
	hash common.Hash,
	withInteractions bool,
) (*common.Tesseract, error) {
	ts, ok := m.tesseractsByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (m *MockLattice) setTesseractByHeight(addr identifiers.Address, height uint64, ts *common.Tesseract) {
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
	}()

	key := addr.Hex() + strconv.FormatUint(height, 10)
	m.tesseractByHeight[key] = ts
}

func (m *MockLattice) GetTesseractByHeight(
	address identifiers.Address,
	height uint64,
	withInteractions bool,
) (*common.Tesseract, error) {
	m.lock.RLock()
	defer func() {
		m.lock.RUnlock()
	}()

	key := address.Hex() + strconv.FormatUint(height, 10)

	ts, ok := m.tesseractByHeight[key]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	return ts, nil
}

func (m *MockLattice) setTesseractHeightEntry(address identifiers.Address, height uint64, tsHash common.Hash) {
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
	}()

	key := address.Hex() + strconv.FormatUint(height, 10)
	m.tesseractHash[key] = tsHash
}

func (m *MockLattice) GetTesseractHeightEntry(address identifiers.Address, height uint64) (common.Hash, error) {
	m.lock.RLock()
	defer func() {
		m.lock.RUnlock()
	}()

	key := address.Hex() + strconv.FormatUint(height, 10)

	tsHash, ok := m.tesseractHash[key]
	if !ok {
		return common.NilHash, common.ErrTSHashNotFound
	}

	return tsHash, nil
}

func (m *MockLattice) IsInitialTesseract(
	ts *common.Tesseract,
	addr identifiers.Address,
) (bool, error) {
	if ts.Height(addr) == 0 {
		return true, nil
	}

	return false, nil
}

type Context struct {
	behaviourNodes []kramaid.KramaID
	randomNodes    []kramaid.KramaID
}

type MockStateManager struct {
	db               store
	sealedTesseracts map[common.Hash]bool
	context          map[common.Hash]*Context
}

func (m *MockStateManager) RemoveCachedObject(addr identifiers.Address) {
}

func (m *MockStateManager) GetICSParticipants(
	ixns common.Interactions,
) (map[identifiers.Address]common.IxParticipant, error) {
	return nil, nil
}

func (m *MockStateManager) LoadTransitionObjects(
	ps map[identifiers.Address]common.IxParticipant,
) (*state.Transition, error) {
	// Create a new objects map
	objects := make(state.ObjectMap)

	for addr, p := range ps {
		if p.IsGenesis {
			objects[addr] = m.CreateStateObject(addr, p.AccType, true)

			continue
		}

		obj, err := m.GetLatestStateObject(addr)
		if err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}

		// copy inorder to avoid modifications to cached object

		objects[addr] = obj.Copy()
	}

	return state.NewTransition(objects), nil
}

func newMockStateManager(db store) *MockStateManager {
	return &MockStateManager{
		db:               db,
		sealedTesseracts: make(map[common.Hash]bool),
		context:          make(map[common.Hash]*Context),
	}
}

func (m *MockStateManager) CreateStateObject(address identifiers.Address,
	accountType common.AccountType, isGenesis bool,
) *state.Object {
	return state.NewStateObject(address, nil, nil, nil,
		common.Account{AccType: accountType}, state.NilMetrics(), isGenesis)
}

func (m *MockStateManager) GetLatestStateObject(addr identifiers.Address) (*state.Object, error) {
	return state.NewStateObject(addr, nil, nil, nil,
		common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false), nil
}

func (m *MockStateManager) SyncStorageTrees(ctx context.Context, newRoot *common.RootNode,
	logicStorageTreeRoots map[string]*common.RootNode, so *state.Object,
) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) SyncLogicTree(newRoot *common.RootNode, so *state.Object) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) IsInitialTesseract(ts *common.Tesseract, addr identifiers.Address) (bool, error) {
	if ts.Height(addr) == 0 {
		return true, nil
	}

	return false, nil
}

func (m *MockStateManager) setTSSeal(ts *common.Tesseract) {
	m.sealedTesseracts[ts.Hash()] = true
}

func (m *MockStateManager) removeTSSeal(ts *common.Tesseract) {
	delete(m.sealedTesseracts, ts.Hash())
}

func (m *MockStateManager) IsSealValid(ts *common.Tesseract) (bool, error) {
	value, exists := m.sealedTesseracts[ts.Hash()]
	if !exists {
		return false, errors.New("tesseract seal does not exist")
	}

	return value, nil
}

func (m *MockStateManager) insertContextNodes(
	ctxHash common.Hash,
	behaviouralNodes []kramaid.KramaID,
	randomNodes ...kramaid.KramaID,
) {
	m.context[ctxHash] = &Context{
		behaviourNodes: behaviouralNodes,
		randomNodes:    randomNodes,
	}
}

func (m *MockStateManager) GetContextByHash(address identifiers.Address,
	hash common.Hash,
) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error) {
	c, ok := m.context[hash]

	if !ok {
		return common.NilHash, nil, nil, common.ErrContextStateNotFound
	}

	return hash, c.behaviourNodes, c.randomNodes, nil
}

func (m *MockStateManager) HasParticipantStateAt(addr identifiers.Address, stateHash common.Hash) bool {
	if _, err := m.db.GetAccount(addr, stateHash); err != nil {
		return false
	}

	return true
}

func (m *MockStateManager) CreateDirtyObject(addr identifiers.Address, accType common.AccountType) *state.Object {
	return nil
}

func (m *MockStateManager) GetParticipantContextRaw(
	address identifiers.Address,
	hash common.Hash,
	rawContext map[string][]byte,
) error {
	return nil
}

func (m *MockStateManager) FetchICSNodeSet(ts *common.Tesseract,
	info *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	return &common.ICSNodeSet{
		Sets: []*common.NodeSet{
			{
				Ids: []kramaid.KramaID{"k1", "k2", "k3"},
			},
		},
	}, nil
}

func (m *MockStateManager) GetICSNodeSetFromRawContext(ts *common.Tesseract,
	rawContext map[string][]byte, clusterInfo *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	// TODO implement me
	panic("implement me")
}

type MockAgora struct {
	sessions   map[cid.CID]syncer.Session
	newSession func(address identifiers.Address) (syncer.Session, error)
}

func newMockAgora() *MockAgora {
	return &MockAgora{
		sessions: make(map[cid.CID]syncer.Session),
	}
}

func (m *MockAgora) addSession(session *MockSession, stateHash cid.CID) {
	m.sessions[stateHash] = session
}

func (m MockAgora) NewSession(ctx context.Context, contextPeers []kramaid.KramaID,
	address identifiers.Address, stateHash cid.CID,
) (syncer.Session, error) {
	if m.newSession != nil {
		return m.newSession(address)
	}

	session, ok := m.sessions[stateHash]
	if !ok {
		return nil, errors.New("unable to fetch session")
	}

	return session, nil
}

func (m MockAgora) Start() {
}

func (m MockAgora) Close() {}

type MockSession struct {
	address identifiers.Address
	blocks  map[cid.CID]*block.Block
}

func newMockSession(addr identifiers.Address) *MockSession {
	return &MockSession{
		address: addr,
		blocks:  make(map[cid.CID]*block.Block),
	}
}

func (m MockSession) ID() identifiers.Address {
	return m.address
}

func (m *MockSession) setBlock(cID cid.CID, block *block.Block) {
	m.blocks[cID] = block
}

func (m MockSession) GetBlock(ctx context.Context, cID cid.CID) (*block.Block, error) {
	b, ok := m.blocks[cID]
	if !ok {
		return nil, errors.New(fmt.Sprintf("block not found for %v", cID))
	}

	return b, nil
}

func (m MockSession) GetBlocks(ctx context.Context, cids []cid.CID) chan *block.Block {
	ch := make(chan *block.Block)

	go func() {
		defer close(ch)

		for _, cID := range cids {
			if b, err := m.GetBlock(ctx, cID); err == nil {
				ch <- b
			}

			time.Sleep(10 * time.Millisecond)
		}
	}()

	return ch
}

func (m MockSession) Close() {
}

type CreateServerParams struct {
	ConfigCallback func(c *config.NetworkConfig) // Additional logic that needs to be executed on the configuration
	// Additional logic that needs to be executed on the server before starting
	ServerCallback func(server *p2p.Server)
	Logger         hclog.Logger
	EventMux       *utils.TypeMux
}

type MockReputationEngine struct {
	ntq      map[kramaid.KramaID]float32
	peerInfo map[peer.ID]*senatus.NodeMetaInfo
	mutex    sync.RWMutex
}

func NewMockReputationEngine() *MockReputationEngine {
	return &MockReputationEngine{
		ntq:      make(map[kramaid.KramaID]float32),
		peerInfo: make(map[peer.ID]*senatus.NodeMetaInfo),
	}
}

func (m *MockReputationEngine) UpdatePeer(data *senatus.NodeMetaInfo) error {
	peerID, err := data.KramaID.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	return m.AddNewPeerWithPeerID(peerID, data)
}

func (m *MockReputationEngine) AddNewPeerWithPeerID(peerID peer.ID, data *senatus.NodeMetaInfo) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.peerInfo[peerID] = data

	return nil
}

func (m *MockReputationEngine) GetNTQ(id kramaid.KramaID) (float32, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ntq, ok := m.ntq[id]
	if !ok {
		return -1, common.ErrKeyNotFound
	}

	return ntq, nil
}

func (m *MockReputationEngine) SetNTQ(id kramaid.KramaID, val int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ntq[id] = float32(val)
}

func (m *MockReputationEngine) GetAddress(key kramaid.KramaID) ([]multiaddr.Multiaddr, error) {
	peerID, err := key.DecodedPeerID()
	if err != nil {
		return nil, common.ErrInvalidKramaID
	}

	return m.GetAddressByPeerID(peerID)
}

func (m *MockReputationEngine) GetAddressByPeerID(peerID peer.ID) ([]multiaddr.Multiaddr, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return utils.MultiAddrFromString(peerInfo.Addrs...), nil
	}

	return nil, common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) GetKramaIDByPeerID(peerID peer.ID) (kramaid.KramaID, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return peerInfo.KramaID, nil
	}

	return "", common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) GetRTTByPeerID(peerID peer.ID) (int64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return peerInfo.RTT, nil
	}

	return 0, common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) SetPeerInfo(key peer.ID, peerInfo *senatus.NodeMetaInfo) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.peerInfo[key] = peerInfo
}

func getParamsToCreateMultipleServers(
	t *testing.T,
	count int,
	noDiscovery bool,
) []*CreateServerParams {
	t.Helper()

	var arrayOfParams []*CreateServerParams

	for i := 0; i < count; i++ {
		arrayOfParams = append(arrayOfParams, &CreateServerParams{
			EventMux: &utils.TypeMux{},
			ConfigCallback: func(c *config.NetworkConfig) {
				c.InboundConnLimit = 10
				c.OutboundConnLimit = 10
				c.NoDiscovery = noDiscovery
			},
			ServerCallback: func(s *p2p.Server) {
				m := NewMockReputationEngine()
				m.SetNTQ(s.GetKramaID(), tests.GetRandomNumber(t, 27))
				s.Senatus = m
			},
			Logger: hclog.NewNullLogger(),
		})
	}

	return arrayOfParams
}

type MockVault struct {
	networkPrivateKey crypto.PrivateKey // Private key used in p2p communication
}

func (vault *MockVault) GetNetworkPrivateKey() crypto.PrivateKey {
	return vault.networkPrivateKey
}

func (vault *MockVault) Sign(
	data []byte,
	sigType mudracommon.SigType,
	signOptions ...crypto.SignOption,
) ([]byte, error) {
	return SignedData, nil
}

func (vault *MockVault) SetNetworkPrivateKey(t *testing.T, privKeyBytes []byte) {
	t.Helper()

	networkPrivKey := new(crypto.SECP256K1PrivKey)
	networkPrivKey.UnMarshal(privKeyBytes)
	vault.networkPrivateKey = networkPrivKey
}

// helper functions
func getPeerInfo(t *testing.T, server *p2p.Server) *peer.AddrInfo {
	t.Helper()

	address := server.GetAddrs()
	networkID, err := server.GetKramaID().PeerID()
	require.NoError(t, err)

	peerID, err := peer.Decode(networkID)
	require.NoError(t, err)

	return &peer.AddrInfo{
		ID:    peerID,
		Addrs: address,
	}
}

func connectClientToServers(t *testing.T, client *p2p.Server, servers ...*p2p.Server) {
	t.Helper()

	for _, s := range servers {
		info := getPeerInfo(t, s)

		client.AddPeerInfo(*info)

		err := client.ConnManager.ConnectAndRegisterPeer(
			context.Background(),
			p2p.PeerInfo{AddrInfo: *info},
			s.GetKramaID(),
			50,
		)
		require.NoError(t, err)
	}
}

func createServer(
	t *testing.T,
	nthValidator int,
	params *CreateServerParams,
) *p2p.Server {
	t.Helper()

	if params == nil {
		params = &CreateServerParams{}
	}

	cfg := &config.NetworkConfig{
		MaxPeers:          0, // current we don't limit the no.of peers
		InboundConnLimit:  50,
		OutboundConnLimit: 15,
	}

	cfg.ListenAddresses = tests.GetListenAddresses(t, 1)

	if params.ConfigCallback != nil {
		params.ConfigCallback(cfg)
	}

	kramaID, networkKey := tests.GetKramaIDAndNetworkKey(t, uint32(nthValidator))
	vault := &MockVault{}

	nPriv := new(crypto.SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)
	vault.networkPrivateKey = nPriv

	// Create a new server instance
	server := p2p.NewServer(hclog.NewNullLogger(), kramaID, params.EventMux, cfg, vault, p2p.NilMetrics())

	if params.ServerCallback != nil {
		params.ServerCallback(server)
	}

	err := server.SetupServer()
	require.NoError(t, err)

	err = server.ConnManager.Start()
	require.NoError(t, err)

	return server
}

func createMultipleServers(
	t *testing.T,
	count int,
	paramsMap map[int]*CreateServerParams,
) []*p2p.Server {
	t.Helper()

	servers := make([]*p2p.Server, count)

	if paramsMap == nil {
		paramsMap = map[int]*CreateServerParams{}
	}

	for i := 0; i < count; i++ {
		servers[i] = createServer(t, i, paramsMap[i])
	}

	return servers
}

func closeTestServer(t *testing.T, s *p2p.Server) {
	t.Helper()

	require.NoError(t, s.Close())
}

func closeTestServers(t *testing.T, servers ...*p2p.Server) {
	t.Helper()

	for _, server := range servers {
		closeTestServer(t, server)
	}
}

// 1. store account meta info using bucketID (helpful in bucket sync)
// 2. store tesseract using address and height as key (helpful in lattice sync)
// 3. store mock session in agora (helpful in sync tesseract)
// 4. store account in db (helpful in snap sync)
// 5. store meta context object in db (helpful in snap sync)
func storeTesseractInDB(t *testing.T, ts *common.Tesseract, syncers ...*Syncer) {
	t.Helper()

	for _, s := range syncers {
		err := s.lattice.AddTesseractWithState(identifiers.NilAddress, nil, ts, nil, true)
		require.NoError(t, err)

		info := common.ICSClusterInfo{}

		rawInfo, err := info.Bytes()
		require.NoError(t, err)

		err = s.db.CreateEntry(ts.ICSHash().Bytes(), rawInfo)
		require.NoError(t, err)

		for addr, participant := range ts.Participants() {
			acc := common.Account{
				ContextHash: participant.LatestContext,
			}

			value, err := acc.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(addr, cid.AccountCID(ts.StateHash(addr))), value)
			require.NoError(t, err)

			mctx := state.MetaContextObject{}

			mctxVal, err := mctx.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(addr, cid.ContextCID(acc.ContextHash)), mctxVal)
			require.NoError(t, err)

			err = s.db.UpdatePrimarySyncStatus(addr)
			require.NoError(t, err)
		}
	}
}

func storeAccountMetaInfoAndSnapInDB(t *testing.T, ts *common.Tesseract, syncers ...*Syncer) {
	t.Helper()

	for _, s := range syncers {
		mockLattice, ok := s.lattice.(*MockLattice)
		require.True(t, ok)

		err := mockLattice.AddAccountMetaInfo(ts)
		require.NoError(t, err)

		for addr, participant := range ts.Participants() {
			acc := common.Account{
				ContextHash: participant.LatestContext,
			}

			value, err := acc.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(addr, cid.AccountCID(ts.StateHash(addr))), value)
			require.NoError(t, err)

			mctx := state.MetaContextObject{}

			mctxVal, err := mctx.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(addr, cid.ContextCID(acc.ContextHash)), mctxVal)
			require.NoError(t, err)
		}

		info := common.ICSClusterInfo{}

		rawInfo, err := info.Bytes()
		require.NoError(t, err)

		err = s.db.CreateEntry(ts.ICSHash().Bytes(), rawInfo)
		require.NoError(t, err)
	}
}

func storeAccountMetaInfosAndSnapInDB(t *testing.T, s *Syncer, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		storeAccountMetaInfoAndSnapInDB(t, ts, s)
	}
}

func storeTesseractsInDB(t *testing.T, s *Syncer, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		storeTesseractInDB(t, ts, s)
	}
}

func storeTesseractInSession(t *testing.T, ts *common.Tesseract, syncers ...*Syncer) {
	t.Helper()

	for _, s := range syncers {
		for addr, participant := range ts.Participants() {
			acc := common.Account{
				ContextHash: participant.LatestContext,
			}

			value, err := acc.Bytes()
			require.NoError(t, err)

			mockSession := newMockSession(addr)

			cID := cid.AccountCID(ts.StateHash(addr))
			mockSession.setBlock(cID, block.NewBlock(cID, value))

			s.logger.Info("msession: store account ", cID)

			mctx := state.MetaContextObject{}

			mctxVal, err := mctx.Bytes()
			require.NoError(t, err)

			cID = cid.ContextCID(acc.ContextHash)
			mockSession.setBlock(cID, block.NewBlock(cID, mctxVal))

			s.logger.Info("msession: store context ", cID)

			agora, ok := s.agora.(*MockAgora)
			require.True(t, ok)

			agora.addSession(mockSession, cid.AccountCID(ts.StateHash(addr)))
		}
	}
}

func storeTesseractsInSession(t *testing.T, s *Syncer, tesseracts ...*common.Tesseract) {
	t.Helper()

	for _, ts := range tesseracts {
		storeTesseractInSession(t, ts, s)
	}
}

// Run this code in a separate goroutine to ensure that event verification is
// performed without missing any events on the client.
// Tesseracts are sent sequentially to the client, appearing as if the server is sending them.
func broadcastTesseracts(t *testing.T, clientSyncer, serverSyncer *Syncer, tesseracts ...*common.Tesseract) {
	t.Helper()

	go func() {
		for i := 0; i < len(tesseracts); i++ {
			var err error

			serverSyncer.logger.Info(
				"broadcast tesseract ",
				"ts-hash", tesseracts[i].Hash(),
			)

			info := common.ICSClusterInfo{
				RandomSet: []kramaid.KramaID{serverSyncer.network.GetKramaID()},
			}

			rawInfo, err := info.Bytes()
			require.NoError(t, err)

			tsInfo := &TesseractInfo{
				tesseract:     tesseracts[i],
				shouldExecute: clientSyncer.cfg.ShouldExecute,
				clusterInfo:   &info,
				delta: map[string][]byte{
					tesseracts[i].ICSHash().Hex(): rawInfo,
				},
			}

			err = clientSyncer.msgHandler(&pubsub.Message{
				ValidatorData: tsInfo,
			})
			require.NoError(t, err)
		}
	}()
}

func printEventsReceived(
	logger hclog.Logger,
	events SyncEvents,
	bucketSyncCount,
	systemAccSyncCount int,
	accountLevelSync map[identifiers.Address]map[SyncEventType]int,
) {
	logger.Debug("printing expected events")
	logger.Debug(syncerEvents[BucketSync], events.bucketSync)
	logger.Debug(syncerEvents[SystemAcc], events.SystemAccSync)

	for acc := range events.accounts {
		logger.Debug("address : ", acc)

		logger.Debug("expected events", ":", events.accounts[acc])
	}

	logger.Debug("")
	logger.Debug("")

	logger.Debug("printing received events")

	logger.Debug(syncerEvents[BucketSync], bucketSyncCount)
	logger.Debug(syncerEvents[SystemAcc], systemAccSyncCount)

	for acc := range events.accounts {
		logger.Debug("address : ", acc)

		for i := SnapSync; i <= JobDone; i++ {
			logger.Debug(syncerEvents[SyncEventType(i)], ":", accountLevelSync[acc][SyncEventType(i)])
		}
	}

	logger.Debug("")
}

func defaultSyncerConfig() *config.SyncerConfig {
	return &config.SyncerConfig{
		ShouldExecute:  false,
		SyncMode:       config.DefaultSyncMode,
		EnableSnapSync: true,
	}
}

func generateTesseracts(
	t *testing.T,
	startHeight, endHeight int,
	prevHash common.Hash,
	addresses ...identifiers.Address,
) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, 0)

	index := 0

	for i := startHeight; i <= endHeight; i++ {
		participants := make(common.ParticipantsState)
		heights := make([]uint64, 0)

		for _, addr := range addresses {
			participants[addr] = common.State{
				StateHash:       tests.RandomHash(t),
				Height:          uint64(i),
				PreviousContext: tests.RandomHash(t),
				TransitiveLink:  prevHash,
			}

			heights = append(heights, uint64(i))
		}

		tesseractParams := &tests.CreateTesseractParams{
			Addresses:    addresses,
			Heights:      heights,
			Participants: participants,
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.ConsensusInfo.ICSHash = tests.RandomHash(t)
				ts.ReceiptsHash = tests.RandomHash(t)
			},
			Ixns: common.Interactions{ // ixns are needed as we are accessing account type from ixn for initial tesseract
				tests.CreateIX(t, &tests.CreateIxParams{
					IxDataCallback: func(ix *common.IxData) {
						ix.Input.Type = common.IxValueTransfer
					},
				}),
			},
			Receipts: map[common.Hash]*common.Receipt{
				tests.RandomHash(t): {
					IxHash: tests.RandomHash(t),
				},
			},
		}

		ts := tests.CreateTesseract(t, tesseractParams)
		tesseracts = append(tesseracts, ts)
		prevHash = ts.Hash()

		index++
	}

	return tesseracts
}

func generateTesseractsGridByMap(t *testing.T, addrHeights map[identifiers.Address]int) []*common.Tesseract {
	t.Helper()

	height := 0

	for _, h := range addrHeights {
		height = h

		break
	}

	addresses := make([]identifiers.Address, 0)

	for addr := range addrHeights {
		addresses = append(addresses, addr)
	}

	tesseracts := generateTesseracts(t, 0, height, common.NilHash, addresses...)

	return tesseracts
}

func generateTesseractsByMap(t *testing.T, addrHeights map[identifiers.Address]int) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, 0)

	for addr, height := range addrHeights {
		tesseracts = append(tesseracts, generateTesseracts(t, 0, height, common.NilHash, addr)...)
	}

	return tesseracts
}

func createPersistenceManager(t *testing.T, ctx context.Context) (*storage.PersistenceManager, string) {
	t.Helper()

	dir, err := os.MkdirTemp(os.TempDir(), "test"+strconv.Itoa(tests.GetRandomNumber(t, 1000)))
	require.NoError(t, err)

	db, err := storage.NewPersistenceManager(hclog.NewNullLogger(), &config.DBConfig{
		CleanDB:      true,
		DBFolderPath: dir,
		MaxSnapSize:  1073741824,
	}, db.NilMetrics())
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return db, dir
}

func checkIfSyncJobMatches(t *testing.T, expectedJob *SyncJob, syncJob *SyncJob) {
	t.Helper()

	require.Equal(t, expectedJob.db, syncJob.db)
	require.Equal(t, expectedJob.address, syncJob.address)
	require.Equal(t, expectedJob.expectedHeight, syncJob.expectedHeight)
	require.Equal(t, expectedJob.snapDownloaded, syncJob.snapDownloaded)
	require.Equal(t, expectedJob.mode, syncJob.mode)
	require.Equal(t, Pending, syncJob.jobState)

	require.True(t, expectedJob.lastModifiedAt.Equal(syncJob.lastModifiedAt))

	require.NotNil(t, syncJob.bestPeers)
	require.NotNil(t, syncJob.tesseractQueue)
}

func sortAddresses(addrs []identifiers.Address) {
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i][:], addrs[j][:]) < 0
	})
}

func createSyncJobs(t *testing.T, count int, addrs []identifiers.Address, opts ...Option) []*SyncJob {
	t.Helper()

	jobs := make([]*SyncJob, count)

	for i := 0; i < count; i++ {
		job := &SyncJob{
			address:        addrs[i],
			mode:           common.LatestSync,
			lastModifiedAt: time.Now(),
		}

		for _, opt := range opts {
			opt(job)
		}

		jobs[i] = job
	}

	return jobs
}

func createPersistenceManagers(t *testing.T, ctx context.Context, count int) ([]*storage.PersistenceManager, []string) {
	t.Helper()

	pm := make([]*storage.PersistenceManager, count)
	dirs := make([]string, count)

	for i := 0; i < count; i++ {
		pm[i], dirs[i] = createPersistenceManager(t, ctx)
	}

	return pm, dirs
}

func testingLogger(name string) hclog.Logger {
	if testing.Verbose() {
		return hclog.New(&hclog.LoggerOptions{
			Name:  name,
			Level: hclog.LevelFromString("DEBUG"),
		})
	}

	return hclog.NewNullLogger()
}

func SubscribeAndListenForSyncEvents(
	t *testing.T,
	ctx context.Context,
	logger hclog.Logger,
	mux *utils.TypeMux,
	expectedEvents SyncEvents,
) {
	t.Helper()

	bucketSync := mux.Subscribe(eventBucketSync{})
	systemAccSync := mux.Subscribe(utils.SystemAccountsSyncedEvent{})
	snapSync := mux.Subscribe(eventSnapSync{})
	latticeSync := mux.Subscribe(eventLatticeSync{})
	tesseractSync := mux.Subscribe(eventTesseractSync{})
	jobDone := mux.Subscribe(eventJobDone{})

	defer func() {
		bucketSync.Unsubscribe()
		systemAccSync.Unsubscribe()
		snapSync.Unsubscribe()
		latticeSync.Unsubscribe()
		tesseractSync.Unsubscribe()
		jobDone.Unsubscribe()
	}()

	bucketSyncCount := 0
	systemAccSyncCount := 0
	accountLevelSync := make(map[identifiers.Address]map[SyncEventType]int)

	for acc, event := range expectedEvents.accounts {
		for i := SnapSync; i <= JobDone; i++ {
			accountLevelSync[acc] = make(map[SyncEventType]int)
			// accountLevelSync[acc][TesseractSync] keeps track of the current height of tesseract synced
			accountLevelSync[acc][TesseractSync] = event.startHeight - 1
		}
	}

	for {
		select {
		case <-ctx.Done():
			printEventsReceived(logger, expectedEvents, bucketSyncCount, systemAccSyncCount, accountLevelSync)

			require.FailNow(t, "timed out waiting for events")

		case <-bucketSync.Chan():
			bucketSyncCount += 1

		case <-systemAccSync.Chan():
			systemAccSyncCount += 1

		case event := <-snapSync.Chan():
			e, ok := event.Data.(eventSnapSync)
			require.True(t, ok)

			accountLevelSync[e.address][SnapSync] += 1

		case event := <-latticeSync.Chan():
			e, ok := event.Data.(eventLatticeSync)
			require.True(t, ok)

			accountLevelSync[e.address][LatticeSync] += 1

		case event := <-tesseractSync.Chan():
			e, ok := event.Data.(eventTesseractSync)
			require.True(t, ok)

			// verify heights are in order
			accountLevelSync[e.address][TesseractSync] += 1
			require.Equal(t, accountLevelSync[e.address][TesseractSync], int(e.height), e.address)

		case event := <-jobDone.Chan():
			e, ok := event.Data.(eventJobDone)
			require.True(t, ok)

			accountLevelSync[e.address][JobDone] += 1

		case <-time.After(10 * time.Millisecond):
			eventsReceived := true

			// as of now job done event is not emitted when it is removed
			// but job done event is emitted as soon as job is moved to done state
			// so there can be more than job done events in case of multiple broadcast tesseracts received
			// to solve this emit job event when job removed from syncer
			if bucketSyncCount == expectedEvents.bucketSync && systemAccSyncCount == expectedEvents.SystemAccSync {
				for acc, accountSpecificEvents := range expectedEvents.accounts {
					if accountLevelSync[acc][SnapSync] != accountSpecificEvents.snapSync ||
						accountLevelSync[acc][LatticeSync] != accountSpecificEvents.latticeSync ||
						accountLevelSync[acc][JobDone] < accountSpecificEvents.JobDone {
						eventsReceived = false

						break
					}

					// make sure that all tesseracts of an address or account received
					if accountLevelSync[acc][TesseractSync] !=
						accountSpecificEvents.endHeight {
						eventsReceived = false

						break
					}
				}
			} else {
				eventsReceived = false
			}

			if eventsReceived {
				if logger.IsTrace() {
					logger.Trace("All events received")
					printEventsReceived(logger, expectedEvents, bucketSyncCount, systemAccSyncCount, accountLevelSync)
				}

				return
			}
		}
	}
}

func checkJobQueue(t *testing.T, nodes ...*Syncer) {
	t.Helper()

	for _, n := range nodes {
		status := n.GetNodeSyncStatus(true)
		require.Equal(t, uint64(0), status.TotalPendingAccounts.ToUint64(), status.PendingAccounts)
	}
}

func checkIfTesseractsSynced(
	t *testing.T,
	s *Syncer,
	accounts map[identifiers.Address]int,
	execution bool,
	tesseracts ...*common.Tesseract,
) {
	t.Helper()

	for addr, height := range accounts {
		accMetaInfo, err := s.db.GetAccountMetaInfo(addr)
		require.NoError(t, err)

		require.Equal(t, height, int(accMetaInfo.Height))
	}

	for _, ts := range tesseracts {
		// check for interactions and receipts
		_, err := s.db.GetInteractions(ts.Hash())
		require.NoError(t, err)

		_, err = s.db.GetReceipts(ts.Hash())
		require.NoError(t, err)

		for addr, participant := range ts.Participants() {
			// check if tesseract is added
			actualTS, err := s.lattice.GetTesseractByHeight(addr, participant.Height, true)
			require.NoError(t, err)

			require.Equal(t, ts.Hash(), actualTS.Hash())

			if execution {
				// check for execution
				key := addr.Hex() + strconv.FormatUint(participant.Height, 10)
				executedTS, ok := s.krama.(*MockKramaEngine).executedTesseracts[key]
				require.True(t, ok)
				require.Equal(t, executedTS.Hash(), ts.Hash())
			} else {
				_, err = s.db.ReadEntry(dbKeyFromCID(addr, cid.AccountCID(participant.StateHash)))
				require.NoError(t, err)
			}
		}
	}
}

type MockDB struct {
	accountSyncStatus map[identifiers.Address][]byte
	accMetaInfo       map[string]struct{}
	receipts          map[common.Hash][]byte
	interactions      map[common.Hash][]byte
}

func (m MockDB) GetAccount(addr identifiers.Address, stateHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) StreamSnapshot(ctx context.Context, address identifiers.Address,
	sinceTS uint64, respChan chan<- common.SnapResponse,
) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func NewMockDB() *MockDB {
	return &MockDB{
		accountSyncStatus: make(map[identifiers.Address][]byte),
		accMetaInfo:       make(map[string]struct{}),
		receipts:          make(map[common.Hash][]byte),
		interactions:      make(map[common.Hash][]byte),
	}
}

func (m MockDB) NewBatchWriter() db.BatchWriter {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) CreateEntry(i []byte, i2 []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) UpdateEntry(i []byte, i2 []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) ReadEntry(i []byte) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) Contains(i []byte) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) DeleteEntry(i []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetAccount(addr identifiers.Address, stateHash common.Hash, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetAccountSyncStatus(address identifiers.Address, status *common.AccountSyncStatus) error {
	rawStatus, err := status.Bytes()
	if err != nil {
		return err
	}

	m.accountSyncStatus[address] = rawStatus

	return nil
}

func (m MockDB) CleanupAccountSyncStatus(address identifiers.Address) error {
	delete(m.accountSyncStatus, address)

	return nil
}

func (m MockDB) SetInteractions(tsHash common.Hash, ixns []byte) error {
	m.interactions[tsHash] = ixns

	return nil
}

func (m MockDB) GetInteractions(tsHash common.Hash) ([]byte, error) {
	ixns, ok := m.interactions[tsHash]
	if !ok {
		return nil, common.ErrFetchingInteractions
	}

	return ixns, nil
}

func (m *MockDB) SetReceipts(tsHash common.Hash, receipts []byte) error {
	m.receipts[tsHash] = receipts

	return nil
}

func (m *MockDB) GetReceipts(tsHash common.Hash) ([]byte, error) {
	receipts, ok := m.receipts[tsHash]
	if !ok {
		return nil, common.ErrReceiptNotFound
	}

	return receipts, nil
}

func (m MockDB) StoreAccountSnapShot(snap *common.Snapshot) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetAccountsSyncStatus() ([]*common.AccountSyncStatus, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) DropPrefix(prefix []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) UpdatePrimarySyncStatus(address identifiers.Address) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) IsAccountPrimarySyncDone(address identifiers.Address) bool {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) HasTesseract(tsHash common.Hash) bool {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetTesseract(tsHash common.Hash, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) UpdatePrincipalSyncStatus() error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetBucketCount(bucketNumber uint64) (uint64, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) StreamAccountMetaInfosRaw(ctx context.Context, bucketNumber uint64, response chan []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetRecentUpdatedAccMetaInfosRaw(
	ctx context.Context,
	bucketID uint64,
	sinceTS uint64,
) ([][]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) IsPrincipalSyncDone() (bool, int64) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetAccountSnapshot(
	ctx context.Context,
	address identifiers.Address,
	sinceTS uint64,
) (*common.Snapshot, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) HasTesseractAt(addr identifiers.Address, height uint64) bool {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetTesseractHeightEntry(addr identifiers.Address, height uint64, tsHash common.Hash) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) UpdateAccMetaInfo(
	id identifiers.Address,
	height uint64,
	tesseractHash common.Hash,
	stateHash common.Hash,
	contextHash common.Hash,
	accType common.AccountType,
) (int32, bool, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) setAccMetaInfoAt(addr identifiers.Address, height uint64) {
	key := addr.Hex() + strconv.FormatUint(height, 10)
	m.accMetaInfo[key] = struct{}{}
}

func (m MockDB) HasAccMetaInfoAt(addr identifiers.Address, height uint64) bool {
	key := addr.Hex() + strconv.FormatUint(height, 10)
	_, ok := m.accMetaInfo[key]

	return ok
}

func (m MockDB) UpdateTesseractStatus(addr identifiers.Address, height uint64, tsHash common.Hash) error {
	// TODO implement me
	panic("implement me")
}

func getDeltaGroup(t *testing.T, behaviouralCount int, randomCount int, replaceCount int) *common.DeltaGroup {
	t.Helper()

	return &common.DeltaGroup{
		BehaviouralNodes: tests.RandomKramaIDs(t, behaviouralCount),
		RandomNodes:      tests.RandomKramaIDs(t, randomCount),
		ReplacedNodes:    tests.RandomKramaIDs(t, replaceCount),
	}
}

func tesseractParamsWithContextDelta(
	t *testing.T,
	address identifiers.Address,
	behaviouralCount, randomCount, replacedCount int,
) *tests.CreateTesseractParams {
	t.Helper()

	return &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantsState{
			address: {
				ContextDelta: getDeltaGroup(t, behaviouralCount, randomCount, replacedCount),
			},
		},
	}
}

func createTesseractsWithChain(
	t *testing.T,
	count int,
	paramsMap map[int]*tests.CreateTesseractParams,
) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*tests.CreateTesseractParams{}
	}

	tesseracts[0] = tests.CreateTesseract(t, paramsMap[0])

	for i := 1; i < count; i++ {
		paramsMap[i].ParticipantsCallback = func(participants common.ParticipantsState) {
			hash := tesseracts[i-1].Hash()
			p := participants[tesseracts[0].AnyAddress()]
			p.TransitiveLink = hash
			participants[tesseracts[0].AnyAddress()] = p
		}

		tesseracts[i] = tests.CreateTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func checkForReceipts(t *testing.T, expectedReceipts, receipts common.Receipts) {
	t.Helper()

	for hash, expectedReceipt := range expectedReceipts {
		receivedReceipt, ok := receipts[hash]
		require.True(t, ok)

		require.Equal(t, expectedReceipt.IxType, receivedReceipt.IxType)
		require.Equal(t, expectedReceipt.Status, receivedReceipt.Status)
		require.Equal(t, expectedReceipt.IxHash, receivedReceipt.IxHash)
		require.Equal(t, expectedReceipt.FuelUsed, receivedReceipt.FuelUsed)
		require.Equal(t, expectedReceipt.ExtraData, receivedReceipt.ExtraData)
	}
}

func fetchContextFromLattice(
	t *testing.T,
	address identifiers.Address,
	ts common.Tesseract,
	s *Syncer,
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

		_, behaviour, random, err := s.state.GetContextByHash(address, ts.PreviousContextHash(address))
		if err == nil {
			peers = append(peers, behaviour...)
			peers = append(peers, random...)

			break
		}

		if ts.TransitiveLink(address).IsNil() {
			break
		}

		t, err := s.lattice.GetTesseract(ts.TransitiveLink(address), false)
		if err != nil {
			return nil
		}

		ts = *t
	}

	return peers
}
