package forage

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/storage/db"

	networkmsg "github.com/sarvalabs/go-moi/network/message"

	"github.com/sarvalabs/go-moi/storage"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
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
	accounts      map[common.Address]AccountSpecificEvents
}

func NewTestSyncer(
	ctx context.Context,
	cfg *config.SyncerConfig,
	node *p2p.Server,
	mux *utils.TypeMux,
	agora syncer.BlockSync,
	db store,
	sm stateManager,
	slots *ktypes.Slots,
	logName string,
	callback func(s *Syncer),
) *Syncer {
	logger := hclog.NewNullLogger()

	s := &Syncer{
		ctx:            ctx,
		network:        node,
		cfg:            cfg,
		mux:            mux,
		agora:          agora,
		db:             db,
		lattice:        newMockLattice(db, logger),
		state:          sm,
		jobWorkerCount: 5,
		jobQueue: &JobQueue{
			jobs: make(map[common.Address]*SyncJob),
			mux:  mux,
		},
		gridStore:         NewGridStore(),
		logger:            logger,
		workerSignal:      make(chan struct{}),
		tesseractRegistry: common.NewHashRegistry(60),
		consensusSlots:    slots,
		lockedAccounts:    make(map[common.Address]common.Hash, 0),
		metrics:           NilMetrics(),
		pendingMsgQueue:   make([]*TesseractInfo, 0),
		pendingMsgChan:    make(chan *TesseractInfo, 10),
		execGrid:          make(map[common.Hash]common.Address),
		tracker:           NewSyncStatusTracker(0),
		workerWaitTime:    10 * time.Millisecond,
	}

	if callback != nil {
		callback(s)
	}

	return s
}

type MockLattice struct {
	lock               sync.RWMutex
	logger             hclog.Logger
	tesseracts         map[string]*common.Tesseract
	db                 store
	executedTesseracts map[string]*common.Tesseract
}

func newMockLattice(db store, logger hclog.Logger) *MockLattice {
	return &MockLattice{
		logger:             logger,
		tesseracts:         make(map[string]*common.Tesseract),
		db:                 db,
		executedTesseracts: make(map[string]*common.Tesseract),
	}
}

func (m *MockLattice) ExecuteAndValidate(ts ...*common.Tesseract) error {
	for _, t := range ts {
		key := t.Address().Hex() + strconv.FormatUint(t.Height(), 10)

		m.executedTesseracts[key] = t
	}

	return nil
}

func (m *MockLattice) AddTesseracts(dirtyStorage map[common.Hash][]byte, tesseracts ...*common.Tesseract) error {
	for _, ts := range tesseracts {
		m.logger.Trace("adding tesseract ", "addr", ts.Address(), "height", ts.Height())

		if _, _, err := m.db.UpdateAccMetaInfo(
			ts.Address(),
			ts.Height(),
			ts.Hash(),
			common.AccTypeFromIxType(ts.Interactions()[0].Type()),
			true,
			true,
		); err != nil {
			return err
		}

		rawIxns, err := ts.Interactions().Bytes()
		if err != nil {
			return err
		}

		err = m.db.SetInteractions(ts.GridHash(), rawIxns)
		if err != nil {
			return err
		}

		m.setTesseractByHeight(ts)

		rawTS, err := ts.Canonical().Bytes()
		if err != nil {
			return err
		}

		if err := m.db.SetTesseract(ts.Hash(), rawTS); err != nil {
			return err
		}

		if err = m.db.SetTesseractHeightEntry(ts.Address(), ts.Height(), ts.Hash()); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockLattice) AddAccountMetaInfo(tesseracts ...*common.Tesseract) error {
	for _, ts := range tesseracts {
		m.logger.Trace("adding account meta info ", "height", ts.Height())

		if _, _, err := m.db.UpdateAccMetaInfo(
			ts.Address(),
			ts.Height(),
			ts.Hash(),
			common.AccTypeFromIxType(ts.Interactions()[0].Type()),
			true,
			true,
		); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockLattice) GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockLattice) setTesseractByHeight(ts *common.Tesseract) {
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
	}()

	key := ts.Address().Hex() + strconv.FormatUint(ts.Height(), 10)
	m.tesseracts[key] = ts
}

func (m *MockLattice) GetTesseractByHeight(
	address common.Address,
	height uint64,
	withInteractions bool,
) (*common.Tesseract, error) {
	m.lock.RLock()
	defer func() {
		m.lock.RUnlock()
	}()

	key := address.Hex() + strconv.FormatUint(height, 10)

	ts, ok := m.tesseracts[key]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	return ts, nil
}

func (m *MockLattice) ValidateTesseract(ts *common.Tesseract, ics *common.ICSNodeSet) error {
	if ts.Height() != 0 {
		if ok := m.db.HasTesseractAt(ts.Address(), ts.Height()-1); !ok {
			m.logger.Trace("prev  tesseract not found ", ts.Address(), ts.Height())

			return common.ErrPreviousTesseractNotFound
		}
	}

	return nil
}

func (m *MockLattice) IsInitialTesseract(ts *common.Tesseract) (bool, error) {
	if ts.Height() == 0 {
		return true, nil
	}

	return false, nil
}

type MockStateManager struct{}

func newMockStateManager() *MockStateManager {
	return &MockStateManager{}
}

func (m MockStateManager) SyncStorageTrees(
	ctx context.Context,
	address common.Address,
	newRoot *common.RootNode,
	logicStorageTreeRoots map[string]*common.RootNode,
) error {
	// TODO implement me
	panic("implement me")
}

func (m MockStateManager) SyncLogicTree(address common.Address, newRoot *common.RootNode) error {
	// TODO implement me
	panic("implement me")
}

func (m MockStateManager) CreateDirtyObject(addr common.Address, accType common.AccountType) *state.Object {
	return nil
}

func (m MockStateManager) GetParticipantContextRaw(
	address common.Address,
	hash common.Hash,
	rawContext map[common.Hash][]byte,
) error {
	return nil
}

func (m MockStateManager) FetchICSNodeSet(ts *common.Tesseract,
	info *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	return &common.ICSNodeSet{
		Nodes: []*common.NodeSet{
			{
				Ids: []kramaid.KramaID{"k1", "k2", "k3"},
			},
		},
	}, nil
}

func (m MockStateManager) GetICSNodeSetFromRawContext(ts *common.Tesseract,
	rawContext map[common.Hash][]byte, clusterInfo *common.ICSClusterInfo,
) (*common.ICSNodeSet, error) {
	// TODO implement me
	panic("implement me")
}

type MockAgora struct {
	sessions   map[cid.CID]syncer.Session
	newSession func(address common.Address) (syncer.Session, error)
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
	address common.Address, stateHash cid.CID,
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
	address common.Address
	blocks  map[cid.CID]*block.Block
}

func newMockSession(addr common.Address) *MockSession {
	return &MockSession{
		address: addr,
		blocks:  make(map[cid.CID]*block.Block),
	}
}

func (m MockSession) ID() common.Address {
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
}

func NewMockReputationEngine() *MockReputationEngine {
	return &MockReputationEngine{
		ntq:      make(map[kramaid.KramaID]float32),
		peerInfo: make(map[peer.ID]*senatus.NodeMetaInfo),
	}
}

func (m *MockReputationEngine) UpdatePeer(key kramaid.KramaID, data *senatus.NodeMetaInfo) error {
	peerID, err := key.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	return m.AddNewPeerWithPeerID(peerID, data)
}

func (m *MockReputationEngine) AddNewPeerWithPeerID(peerID peer.ID, data *senatus.NodeMetaInfo) error {
	m.peerInfo[peerID] = data

	return nil
}

func (m *MockReputationEngine) GetNTQ(id kramaid.KramaID) (float32, error) {
	ntq, ok := m.ntq[id]
	if !ok {
		return -1, common.ErrKeyNotFound
	}

	return ntq, nil
}

func (m *MockReputationEngine) SetNTQ(id kramaid.KramaID, val int) {
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
	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return utils.MultiAddrFromString(peerInfo.Addrs...), nil
	}

	return nil, common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) SetPeerInfo(key peer.ID, peerInfo *senatus.NodeMetaInfo) {
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

		client.AddPeerInfo(info)

		err := client.ConnectAndRegisterPeer(*info)
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

	err = server.StartServer()
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

	acc := common.Account{
		ContextHash: ts.ContextHash(),
	}

	value, err := acc.Bytes()
	require.NoError(t, err)

	for _, s := range syncers {
		err := s.lattice.AddTesseracts(nil, ts)
		require.NoError(t, err)

		err = s.db.CreateEntry(dbKeyFromCID(ts.Address(), cid.AccountCID(ts.StateHash())), value)
		require.NoError(t, err)

		mctx := state.MetaContextObject{}

		mctxVal, err := mctx.Bytes()
		require.NoError(t, err)

		err = s.db.CreateEntry(dbKeyFromCID(ts.Address(), cid.ContextCID(acc.ContextHash)), mctxVal)
		require.NoError(t, err)

		info := common.ICSClusterInfo{}

		rawInfo, err := info.Bytes()
		require.NoError(t, err)

		err = s.db.CreateEntry(ts.ICSHash().Bytes(), rawInfo)
		require.NoError(t, err)

		err = s.db.UpdatePrimarySyncStatus(ts.Address())
		require.NoError(t, err)
	}
}

func storeAccountMetaInfoAndSnapInDB(t *testing.T, ts *common.Tesseract, syncers ...*Syncer) {
	t.Helper()

	acc := common.Account{
		ContextHash: ts.ContextHash(),
	}

	value, err := acc.Bytes()
	require.NoError(t, err)

	for _, s := range syncers {
		mockLattice, ok := s.lattice.(*MockLattice)
		require.True(t, ok)

		err := mockLattice.AddAccountMetaInfo(ts)
		require.NoError(t, err)

		err = s.db.CreateEntry(dbKeyFromCID(ts.Address(), cid.AccountCID(ts.StateHash())), value)
		require.NoError(t, err)

		mctx := state.MetaContextObject{}

		mctxVal, err := mctx.Bytes()
		require.NoError(t, err)

		err = s.db.CreateEntry(dbKeyFromCID(ts.Address(), cid.ContextCID(acc.ContextHash)), mctxVal)
		require.NoError(t, err)

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

	acc := common.Account{
		ContextHash: ts.ContextHash(),
	}

	value, err := acc.Bytes()
	require.NoError(t, err)

	mockSession := newMockSession(ts.Address())

	for _, s := range syncers {
		cID := cid.AccountCID(ts.StateHash())
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

		agora.addSession(mockSession, cid.AccountCID(ts.StateHash()))
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
func broadcastTesseracts(t *testing.T, serverSyncer *Syncer, tesseracts ...*common.Tesseract) {
	t.Helper()

	go func() {
		for i := 0; i < len(tesseracts); i++ {
			var err error

			serverSyncer.logger.Info("broadcast tesseract ", "addr", tesseracts[i].Address(),
				"height", tesseracts[i].Height(), t.Name())

			rawIxns, err := tesseracts[i].Interactions().Bytes()
			require.NoError(t, err)

			info := common.ICSClusterInfo{
				RandomSet: []kramaid.KramaID{serverSyncer.network.GetKramaID()},
			}

			rawInfo, err := info.Bytes()
			require.NoError(t, err)

			msg := &networkmsg.TesseractMessage{
				RawTesseract: make([]byte, 0),
				Sender:       serverSyncer.network.GetKramaID(),
				Ixns:         rawIxns,
				Receipts:     nil,
				Delta: map[common.Hash][]byte{
					tesseracts[i].ICSHash(): rawInfo,
				},
			}

			msg.RawTesseract, err = tesseracts[i].Canonical().Bytes()
			require.NoError(t, err)

			rawData, err := msg.Bytes()
			require.NoError(t, err)

			err = serverSyncer.network.Broadcast(config.TesseractTopic, rawData)
			require.NoError(t, err)

			// introduce delay to send tesseracts in order
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func printEventsReceived(
	logger hclog.Logger,
	events SyncEvents,
	bucketSyncCount,
	systemAccSyncCount int,
	accountLevelSync map[common.Address]map[SyncEventType]int,
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
	addr common.Address,
	startHeight, endHeight int,
	prevHash common.Hash,
	totalParts int32,
	gridHashes ...common.Hash,
) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, 0)

	if len(gridHashes) == 0 {
		gridHashes = tests.GetHashes(t, endHeight-startHeight+1)
	}

	index := 0

	for i := startHeight; i <= endHeight; i++ {
		tesseractParams := &tests.CreateTesseractParams{
			Address: addr,
			Height:  uint64(i),
			Ixns: common.Interactions{ // ixns are needed as we are accessing account type from ixn for initial tesseract
				tests.CreateIX(t, &tests.CreateIxParams{
					IxDataCallback: func(ix *common.IxData) {
						ix.Input.Type = common.IxValueTransfer
					},
				}),
			},
			HeaderCallback: func(header *common.TesseractHeader) {
				header.PrevHash = prevHash
				header.Extra = common.CommitData{
					GridID: &common.TesseractGridID{
						Hash: gridHashes[index], // used to fetch interactions
						Parts: &common.TesseractParts{
							Total: totalParts,
						},
					},
				}
				header.ContextLock = map[common.Address]common.ContextLockInfo{
					addr: {
						ContextHash: tests.RandomHash(t),
					},
				}
			},
			BodyCallback: func(body *common.TesseractBody) {
				body.StateHash = tests.RandomHash(t)
				body.ConsensusProof.ICSHash = tests.RandomHash(t)
			},
		}

		ts := tests.CreateTesseract(t, tesseractParams)
		tesseracts = append(tesseracts, ts)
		prevHash = ts.Hash()

		index++
	}

	return tesseracts
}

func generateTesseractsGridByMap(t *testing.T, addrHeights map[common.Address]int) []*common.Tesseract {
	t.Helper()

	height := 0

	for _, h := range addrHeights {
		height = h

		break
	}

	tesseracts := make([]*common.Tesseract, 0)
	totalParts := int32(len(addrHeights))
	gridHashes := tests.GetHashes(t, height+1)

	for addr, height := range addrHeights {
		tesseracts = append(tesseracts,
			generateTesseracts(t, addr, 0, height, common.NilHash, totalParts, gridHashes...)...)
	}

	return tesseracts
}

func generateTesseractsByMap(t *testing.T, addrHeights map[common.Address]int) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, 0)

	for addr, height := range addrHeights {
		tesseracts = append(tesseracts, generateTesseracts(t, addr, 0, height, common.NilHash, 1)...)
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
	systemAccSync := mux.Subscribe(eventSystemAccounts{})
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
	accountLevelSync := make(map[common.Address]map[SyncEventType]int)

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

func checkIfTesseractsSynced(
	t *testing.T,
	s *Syncer,
	accounts map[common.Address]int,
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
		actualTS, err := s.lattice.GetTesseractByHeight(ts.Address(), ts.Height(), true)
		require.NoError(t, err)
		require.Equal(t, ts, actualTS)

		_, err = s.db.GetInteractions(ts.GridHash())
		require.NoError(t, err)

		if execution {
			l, ok := s.lattice.(*MockLattice)
			require.True(t, ok)

			key := ts.Address().Hex() + strconv.FormatUint(ts.Height(), 10)
			executedTS, ok := l.executedTesseracts[key]
			require.True(t, ok)
			require.Equal(t, executedTS, ts)
		} else {
			_, err = s.db.ReadEntry(dbKeyFromCID(ts.Address(), cid.AccountCID(ts.StateHash())))
			require.NoError(t, err)
		}
	}
}
