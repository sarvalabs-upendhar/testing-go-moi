package forage

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
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
	accounts      map[identifiers.Identifier]AccountSpecificEvents
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
		ctx:                ctx,
		network:            node,
		cfg:                cfg,
		mux:                mux,
		agora:              agora,
		db:                 db,
		consensus:          krama,
		lattice:            newMockLattice(db, logger),
		state:              newMockStateManager(db),
		accountWorkerCount: 5,
		accountJobQueue: &AccountJobQueue{
			jobs:  make(map[identifiers.Identifier]*AccountSyncJob),
			mux:   mux,
			krama: krama,
		},
		tsJobQueue:          NewTesseractJobQueue(NilMetrics()),
		tsWorkerCount:       5,
		tsWorkerSignal:      make(chan struct{}),
		logger:              logger,
		accountWorkerSignal: make(chan struct{}),
		tesseractRegistry:   common.NewHashRegistry(60),
		consensusSlots:      slots,
		lockedAccounts:      make(map[identifiers.Identifier]struct{}),
		metrics:             NilMetrics(),
		pendingMsgQueue:     make([]*TesseractInfo, 0),
		pendingMsgChan:      make(chan *TesseractInfo, 10),
		execGrid:            make(map[common.Hash]struct{}),
		tracker:             NewSyncStatusTracker(0),
		workerWaitTime:      10 * time.Millisecond,
		syncPeersPresent:    len(cfg.SyncPeers) > 0,
	}

	if callback != nil {
		callback(s)
	}

	return s
}

func NewTestSyncerWithJobQueue(ctx context.Context, mux *utils.TypeMux) *Syncer {
	return &Syncer{
		ctx: ctx,
		accountJobQueue: &AccountJobQueue{
			jobs: make(map[identifiers.Identifier]*AccountSyncJob),
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
	compressor, _ := common.NewZstdCompressor(logger)

	return &Syncer{
		ctx:               ctx,
		network:           node,
		cfg:               cfg,
		db:                db,
		logger:            logger,
		state:             state,
		tesseractRegistry: common.NewHashRegistry(60),
		mux:               &utils.TypeMux{},
		compressor:        compressor,
	}
}

type MockKramaEngine struct {
	logger             hclog.Logger
	db                 store
	executedTesseracts map[string]*common.Tesseract
	activeAccounts     map[identifiers.Identifier]struct{}
	mtx                sync.RWMutex
}

func (m *MockKramaEngine) DeleteLockedTSInfo(ts *common.Tesseract, fromSyncer bool) {
}

func NewMockKramaEngine(db store, logger hclog.Logger) *MockKramaEngine {
	return &MockKramaEngine{
		db:                 db,
		logger:             logger,
		executedTesseracts: make(map[string]*common.Tesseract),
		activeAccounts:     make(map[identifiers.Identifier]struct{}),
	}
}

func (m *MockKramaEngine) GetLockedTSFromDB(tsHash common.Hash) (*common.Tesseract, error) {
	panic("implement me")
}

func (m *MockKramaEngine) GetICSCommittee(ts *common.Tesseract, info *common.CommitInfo,
	systemObject *state.SystemObject,
) (*ktypes.ICSCommittee, error) {
	return &ktypes.ICSCommittee{
		Sets: []*ktypes.NodeSet{
			{
				Infos: []*common.ValidatorInfo{{KramaID: "k1"}, {KramaID: "k2"}, {KramaID: "k3"}},
			},
		},
	}, nil
}

func (m *MockKramaEngine) GetICSCommitteeFromRawContext(
	ts *common.Tesseract,
	rawContext map[string][]byte,
	info *common.CommitInfo,
	systemObject *state.SystemObject,
) (*ktypes.ICSCommittee, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockKramaEngine) AddActiveAccounts(
	lockType common.LockType, clusterID common.ClusterID, ids ...identifiers.Identifier,
) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, id := range ids {
		if _, ok := m.activeAccounts[id]; ok {
			return false
		}
	}

	for _, id := range ids {
		m.activeAccounts[id] = struct{}{}
	}

	return true
}

func (m *MockKramaEngine) ClearActiveAccounts(clusterID common.ClusterID, ids ...identifiers.Identifier) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, id := range ids {
		delete(m.activeAccounts, id)
	}
}

func (m *MockKramaEngine) ValidateTesseract(
	id identifiers.Identifier,
	ts *common.Tesseract,
	ics *ktypes.ICSCommittee,
	allParticipants bool,
) error {
	if !allParticipants && id.IsNil() {
		return common.ErrEmptyID
	}

	var ids []identifiers.Identifier

	if allParticipants {
		ids = ts.AccountIDs()
	} else {
		ids = append(ids, id)
	}

	for _, id := range ids {
		if ts.Height(id) != 0 {
			if ok := m.db.HasAccMetaInfoAt(id, ts.Height(id)-1); !ok {
				m.logger.Trace("prev tesseract not found", "accountID", id, "height", ts.Height(id))

				return common.ErrPreviousTesseractNotFound
			}
		}
	}

	return nil
}

func (m *MockKramaEngine) ExecuteAndValidate(
	ts *common.Tesseract,
	transition *state.Transition,
) error {
	for id, s := range ts.Participants() {
		key := id.Hex() + strconv.FormatUint(s.Height, 10)
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

func (m *MockLattice) GetInteractionsByTSHash(tsHash common.Hash) ([]*common.Interaction, error) {
	panic("implement me")
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
	id identifiers.Identifier,
	dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract,
	transition *state.Transition,
	allParticipants bool,
) error {
	if !allParticipants && id.IsNil() {
		return common.ErrEmptyID
	}

	partcipants := make(common.ParticipantsState)

	if allParticipants {
		partcipants = ts.Participants()
	} else {
		s, ok := ts.State(id)
		if !ok {
			panic(ok)
		}

		partcipants[id] = s
	}

	for id, p := range partcipants {
		m.logger.Trace("adding tesseract ",
			"ts-hash", ts.Hash(),
			"accountID", id,
			"height", p.Height,
		)

		if err := m.db.SetTesseractHeightEntry(id, p.Height, ts.Hash()); err != nil {
			return err
		}

		m.setTesseractByHeight(id, p.Height, ts)
		m.setTesseractHeightEntry(id, p.Height, ts.Hash())

		accountType := common.AccountTypeFromID(ts.AnyAccountID())

		if _, _, err := m.db.UpdateAccMetaInfo(
			id,
			p.Height,
			ts.Hash(),
			ts.StateHash(ts.AnyAccountID()),
			ts.LockedContextHash(ts.AnyAccountID()),
			common.NilHash,
			identifiers.Nil,
			ts.CommitHash(),
			accountType,
			true,
			1,
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

		rawCommitInfo, err := ts.CommitInfo().Bytes()
		if err != nil {
			return err
		}

		if err = m.db.SetCommitInfo(ts.Hash(), rawCommitInfo); err != nil {
			return err
		}

		rawTS, err := ts.Bytes()
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
		for id, s := range ts.Participants() {
			m.logger.Trace("adding account meta info ", "accountID", id, "height", s.Height)

			accountType := common.AccountTypeFromID(ts.AnyAccountID())

			if _, _, err := m.db.UpdateAccMetaInfo(
				id,
				s.Height,
				ts.Hash(),
				ts.StateHash(ts.AnyAccountID()),
				ts.LockedContextHash(ts.AnyAccountID()),
				common.NilHash,
				identifiers.Nil,
				ts.CommitHash(),
				accountType,
				true,
				1,
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
	withCommitInfo bool,
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

func (m *MockLattice) setTesseractByHeight(id identifiers.Identifier, height uint64, ts *common.Tesseract) {
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
	}()

	key := id.Hex() + strconv.FormatUint(height, 10)
	m.tesseractByHeight[key] = ts
}

func (m *MockLattice) GetTesseractByHeight(
	id identifiers.Identifier,
	height uint64,
	withInteractions bool,
	withCommitInfo bool,
) (*common.Tesseract, error) {
	m.lock.RLock()
	defer func() {
		m.lock.RUnlock()
	}()

	key := id.Hex() + strconv.FormatUint(height, 10)

	ts, ok := m.tesseractByHeight[key]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	return ts, nil
}

func (m *MockLattice) setTesseractHeightEntry(id identifiers.Identifier, height uint64, tsHash common.Hash) {
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
	}()

	key := id.Hex() + strconv.FormatUint(height, 10)
	m.tesseractHash[key] = tsHash
}

func (m *MockLattice) GetTesseractHeightEntry(id identifiers.Identifier, height uint64) (common.Hash, error) {
	m.lock.RLock()
	defer func() {
		m.lock.RUnlock()
	}()

	key := id.Hex() + strconv.FormatUint(height, 10)

	tsHash, ok := m.tesseractHash[key]
	if !ok {
		return common.NilHash, common.ErrTSHashNotFound
	}

	return tsHash, nil
}

func (m *MockLattice) IsInitialTesseract(
	ts *common.Tesseract,
	id identifiers.Identifier,
) (bool, error) {
	if ts.Height(id) == 0 {
		return true, nil
	}

	return false, nil
}

type MockStateManager struct {
	db               store
	sealedTesseracts map[common.Hash]bool
	consensusNodes   map[common.Hash][]identifiers.KramaID
	systemObject     *state.SystemObject
}

func newMockStateManager(db store) *MockStateManager {
	return &MockStateManager{
		db:               db,
		sealedTesseracts: make(map[common.Hash]bool),
		consensusNodes:   make(map[common.Hash][]identifiers.KramaID),
		systemObject: state.NewSystemObject(
			state.NewStateObject(
				common.SystemAccountID, nil, nil, nil,
				common.Account{AccType: common.SystemAccount}, state.NilMetrics(), true,
			)),
	}
}

func (m *MockStateManager) GetSystemObject() *state.SystemObject {
	return m.systemObject
}

func (m *MockStateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) RefreshCachedObject(id identifiers.Identifier, sysObj *state.SystemObject) {
}

func (m *MockStateManager) LoadTransitionObjects(
	ps map[identifiers.Identifier]common.ParticipantInfo,
	psState common.ParticipantsState,
) (*state.Transition, error) {
	// Create a new objects map
	objects := make(state.ObjectMap)

	for id, p := range ps {
		if p.IsGenesis {
			objects[id] = m.CreateStateObject(id, true)

			continue
		}

		obj, err := m.GetLatestStateObject(id)
		if err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}

		// copy inorder to avoid modifications to cached object

		objects[id] = obj.Copy()
	}

	return state.NewTransition(nil, objects, nil), nil
}

func (m *MockStateManager) CreateStateObject(id identifiers.Identifier, isGenesis bool,
) *state.Object {
	return state.NewStateObject(id, nil, nil, nil,
		common.Account{AccType: common.AccountTypeFromID(id)}, state.NilMetrics(), isGenesis)
}

func (m *MockStateManager) GetLatestStateObject(id identifiers.Identifier) (*state.Object, error) {
	return state.NewStateObject(id, nil, nil, nil,
		common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false), nil
}

func (m *MockStateManager) SyncStorageTrees(ctx context.Context, newRoot *common.RootNode,
	logicStorageTreeRoots map[string]*common.RootNode, so *state.Object,
) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) SyncAssetTree(newRoot *common.RootNode, so *state.Object) error {
	panic("implement me")
}

func (m *MockStateManager) SyncLogicTree(newRoot *common.RootNode, so *state.Object) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockStateManager) IsInitialTesseract(ts *common.Tesseract, id identifiers.Identifier) (bool, error) {
	if ts.Height(id) == 0 {
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

func (m *MockStateManager) insertConsensusNodes(
	ctxHash common.Hash,
	nodes []identifiers.KramaID,
) {
	m.consensusNodes[ctxHash] = nodes
}

func (m *MockStateManager) GetConsensusNodesByHash(id identifiers.Identifier,
	hash common.Hash,
) ([]identifiers.KramaID, error) {
	c, ok := m.consensusNodes[hash]

	if !ok {
		return nil, common.ErrContextStateNotFound
	}

	return c, nil
}

func (m *MockStateManager) HasParticipantStateAt(id identifiers.Identifier, stateHash common.Hash) bool {
	if _, err := m.db.GetAccount(id, stateHash); err != nil {
		return false
	}

	return true
}

func (m *MockStateManager) CreateDirtyObject(id identifiers.Identifier, accType common.AccountType) *state.Object {
	return nil
}

func (m *MockStateManager) GetParticipantContextRaw(
	id identifiers.Identifier,
	hash common.Hash,
	rawContext map[string][]byte,
) error {
	return nil
}

type MockAgora struct {
	sessions   map[cid.CID]syncer.Session
	newSession func(id identifiers.Identifier) (syncer.Session, error)
}

func newMockAgora() *MockAgora {
	return &MockAgora{
		sessions: make(map[cid.CID]syncer.Session),
	}
}

func (m *MockAgora) addSession(session *MockSession, stateHash cid.CID) {
	m.sessions[stateHash] = session
}

func (m MockAgora) NewSession(ctx context.Context, contextPeers []identifiers.KramaID,
	id identifiers.Identifier, stateHash cid.CID,
) (syncer.Session, error) {
	if m.newSession != nil {
		return m.newSession(id)
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
	id     identifiers.Identifier
	blocks map[cid.CID]*block.Block
}

func newMockSession(id identifiers.Identifier) *MockSession {
	return &MockSession{
		id:     id,
		blocks: make(map[cid.CID]*block.Block),
	}
}

func (m MockSession) ID() identifiers.Identifier {
	return m.id
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
	ntq      map[identifiers.KramaID]float32
	peerInfo map[peer.ID]*senatus.NodeMetaInfo
	mutex    sync.RWMutex
}

func NewMockReputationEngine() *MockReputationEngine {
	return &MockReputationEngine{
		ntq:      make(map[identifiers.KramaID]float32),
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

func (m *MockReputationEngine) GetNTQ(id identifiers.KramaID) (float32, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ntq, ok := m.ntq[id]
	if !ok {
		return -1, common.ErrKeyNotFound
	}

	return ntq, nil
}

func (m *MockReputationEngine) SetNTQ(id identifiers.KramaID, val int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ntq[id] = float32(val)
}

func (m *MockReputationEngine) GetAddress(key identifiers.KramaID) ([]multiaddr.Multiaddr, error) {
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

func (m *MockReputationEngine) GetKramaIDByPeerID(peerID peer.ID) (identifiers.KramaID, error) {
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

	addrs := server.GetAddrs()
	networkID, err := server.GetKramaID().PeerID()
	require.NoError(t, err)

	peerID, err := peer.Decode(networkID)
	require.NoError(t, err)

	return &peer.AddrInfo{
		ID:    peerID,
		Addrs: addrs,
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
// 2. store tesseract using accountID and height as key (helpful in lattice sync)
// 3. store mock session in agora (helpful in sync tesseract)
// 4. store account in db (helpful in snap sync)
// 5. store meta context object in db (helpful in snap sync)
func storeTesseractInDB(t *testing.T, ts *common.Tesseract, syncers ...*Syncer) {
	t.Helper()

	for _, s := range syncers {
		err := s.lattice.AddTesseractWithState(identifiers.Nil, nil, ts, nil, true)
		require.NoError(t, err)

		for id, participant := range ts.Participants() {
			acc := common.Account{
				ContextHash: participant.LockedContext,
			}

			value, err := acc.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(id, cid.AccountCID(ts.StateHash(id))), value)
			require.NoError(t, err)

			mctx := state.MetaContextObject{}

			mctxVal, err := mctx.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(id, cid.ContextCID(acc.ContextHash)), mctxVal)
			require.NoError(t, err)

			err = s.db.UpdatePrimarySyncStatus(id)
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

		for id, participant := range ts.Participants() {
			acc := common.Account{
				ContextHash: participant.LockedContext,
			}

			value, err := acc.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(id, cid.AccountCID(ts.StateHash(id))), value)
			require.NoError(t, err)

			mctx := state.MetaContextObject{}

			mctxVal, err := mctx.Bytes()
			require.NoError(t, err)

			err = s.db.CreateEntry(dbKeyFromCID(id, cid.ContextCID(acc.ContextHash)), mctxVal)
			require.NoError(t, err)
		}
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
		for id, participant := range ts.Participants() {
			acc := common.Account{
				ContextHash: participant.LockedContext,
			}

			value, err := acc.Bytes()
			require.NoError(t, err)

			mockSession := newMockSession(id)

			cID := cid.AccountCID(ts.StateHash(id))
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

			agora.addSession(mockSession, cid.AccountCID(ts.StateHash(id)))
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

			tsInfo := &TesseractInfo{
				tesseract:     tesseracts[i],
				shouldExecute: clientSyncer.cfg.ShouldExecute,
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
	accountLevelSync map[identifiers.Identifier]map[SyncEventType]int,
) {
	logger.Debug("printing expected events")
	logger.Debug(syncerEvents[BucketSync], events.bucketSync)
	logger.Debug(syncerEvents[SystemAcc], events.SystemAccSync)

	for acc := range events.accounts {
		logger.Debug("accountID : ", acc)

		logger.Debug("expected events", ":", events.accounts[acc])
	}

	logger.Debug("")
	logger.Debug("")

	logger.Debug("printing received events")

	logger.Debug(syncerEvents[BucketSync], bucketSyncCount)
	logger.Debug(syncerEvents[SystemAcc], systemAccSyncCount)

	for acc := range events.accounts {
		logger.Debug("accountID : ", acc)

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
	serverID identifiers.KramaID,
	startHeight, endHeight int,
	prevHash common.Hash,
	ids ...identifiers.Identifier,
) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, 0)

	index := 0

	for i := startHeight; i <= endHeight; i++ {
		participants := make(common.ParticipantsState)
		heights := make([]uint64, 0)

		for _, id := range ids {
			participants[id] = common.State{
				StateHash:      tests.RandomHash(t),
				Height:         uint64(i),
				LockedContext:  tests.RandomHash(t),
				TransitiveLink: prevHash,
			}

			heights = append(heights, uint64(i))
		}

		tesseractParams := &tests.CreateTesseractParams{
			IDs:          ids,
			Heights:      heights,
			Participants: participants,
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.ReceiptsHash = tests.RandomHash(t)
				ts.ConsensusInfo.AccountLocks = make(map[identifiers.Identifier]common.LockType)

				for _, id := range ids {
					ts.ConsensusInfo.AccountLocks[id] = common.MutateLock
				}
			},
			// ixns are needed as we are accessing account type from ixn for initial tesseract
			Ixns: common.NewInteractionsWithLeaderCheck(
				false,
				tests.CreateIX(t, &tests.CreateIxParams{
					IxDataCallback: func(ix *common.IxData) {
						for _, id := range ids {
							tests.AddIxOp(t, ix, common.IxAssetAction, common.KMOITokenAssetID, &common.TransferParams{
								Beneficiary: id,
								Amount:      big.NewInt(1),
							})
						}
					},
				}),
			),
			Receipts: map[common.Hash]*common.Receipt{
				tests.RandomHash(t): {
					IxHash: tests.RandomHash(t),
				},
			},
			CommitInfo: &common.CommitInfo{
				Operator:  tests.RandomKramaID(t, 1),
				RandomSet: []common.ValidatorIndex{0},
			},
		}

		ts := tests.CreateTesseract(t, tesseractParams)
		tesseracts = append(tesseracts, ts)
		prevHash = ts.Hash()

		index++
	}

	return tesseracts
}

func generateTesseractsGridByMap(t *testing.T,
	serverID identifiers.KramaID, addrHeights map[identifiers.Identifier]int,
) []*common.Tesseract {
	t.Helper()

	height := 0

	for _, h := range addrHeights {
		height = h

		break
	}

	ids := make([]identifiers.Identifier, 0)

	for id := range addrHeights {
		ids = append(ids, id)
	}

	tesseracts := generateTesseracts(t, serverID, 0, height, common.NilHash, ids...)

	return tesseracts
}

func generateTesseractsByMap(t *testing.T, addrHeights map[identifiers.Identifier]int) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, 0)

	for id, height := range addrHeights {
		tesseracts = append(tesseracts, generateTesseracts(t, "", 0, height, common.NilHash, id)...)
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

func checkIfSyncJobMatches(t *testing.T, expectedJob *AccountSyncJob, syncJob *AccountSyncJob) {
	t.Helper()

	require.Equal(t, expectedJob.db, syncJob.db)
	require.Equal(t, expectedJob.id, syncJob.id)
	require.Equal(t, expectedJob.expectedHeight, syncJob.expectedHeight)
	require.Equal(t, expectedJob.snapDownloaded, syncJob.snapDownloaded)
	require.Equal(t, expectedJob.mode, syncJob.mode)
	require.Equal(t, Pending, syncJob.jobState)

	require.True(t, expectedJob.lastModifiedAt.Equal(syncJob.lastModifiedAt))

	require.NotNil(t, syncJob.bestPeers)
	require.NotNil(t, syncJob.tesseractQueue)
}

func sortIDs(ids []identifiers.Identifier) {
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})
}

func createSyncJobs(t *testing.T, count int, ids []identifiers.Identifier, opts ...Option) []*AccountSyncJob {
	t.Helper()

	jobs := make([]*AccountSyncJob, count)

	for i := 0; i < count; i++ {
		job := &AccountSyncJob{
			id:             ids[i],
			creationTime:   time.Now(),
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
	accountLevelSync := make(map[identifiers.Identifier]map[SyncEventType]int)

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

			accountLevelSync[e.id][SnapSync] += 1

		case event := <-latticeSync.Chan():
			e, ok := event.Data.(eventLatticeSync)
			require.True(t, ok)

			accountLevelSync[e.id][LatticeSync] += 1

		case event := <-tesseractSync.Chan():
			e, ok := event.Data.(eventTesseractSync)
			require.True(t, ok)

			// verify heights are in order
			accountLevelSync[e.id][TesseractSync] += 1
			require.Equal(t, accountLevelSync[e.id][TesseractSync], int(e.height), e.id)

		case event := <-jobDone.Chan():
			e, ok := event.Data.(eventJobDone)
			require.True(t, ok)

			accountLevelSync[e.id][JobDone] += 1

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

					// make sure that all tesseracts of an accountID or account received
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

func checkTSQueue(t *testing.T, nodes ...*Syncer) {
	t.Helper()

	for _, n := range nodes {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), maxTimeout)
			defer cancel()

			_, err := tests.RetryUntilTimeout(ctx, 500*time.Millisecond, func() (interface{}, bool) {
				status := n.GetNodeSyncStatus(true)

				return nil, len(status.PendingTesseractHash) != 0
			})
			require.NoError(t, err)
		}()

		status := n.GetNodeSyncStatus(true)
		require.Equal(t, uint64(0), status.TotalPendingAccounts.ToUint64(), status.PendingAccounts)
	}
}

func checkIfTesseractsSynced(
	t *testing.T,
	s *Syncer,
	accounts map[identifiers.Identifier]int,
	execution bool,
	tesseracts ...*common.Tesseract,
) {
	t.Helper()

	for id, height := range accounts {
		accMetaInfo, err := s.db.GetAccountMetaInfo(id)
		require.NoError(t, err)

		require.Equal(t, height, int(accMetaInfo.Height))
	}

	for _, ts := range tesseracts {
		// check for interactions and receipts
		_, err := s.db.GetInteractions(ts.Hash())
		require.NoError(t, err)

		_, err = s.db.GetReceipts(ts.Hash())
		require.NoError(t, err)

		for id, participant := range ts.Participants() {
			// check if tesseract is added
			actualTS, err := s.lattice.GetTesseractByHeight(id, participant.Height, true, true)
			require.NoError(t, err)

			require.Equal(t, ts.Hash(), actualTS.Hash())

			if execution {
				// check for execution
				key := id.Hex() + strconv.FormatUint(participant.Height, 10)
				executedTS, ok := s.consensus.(*MockKramaEngine).executedTesseracts[key]
				require.True(t, ok)
				require.Equal(t, executedTS.Hash(), ts.Hash())
			} else {
				_, err = s.db.ReadEntry(dbKeyFromCID(id, cid.AccountCID(participant.StateHash)))
				require.NoError(t, err)
			}
		}
	}
}

type MockDB struct {
	accountSyncStatus map[identifiers.Identifier][]byte
	accMetaInfo       map[string]struct{}
	receipts          map[common.Hash][]byte
	interactions      map[common.Hash][]byte
}

func NewMockDB() *MockDB {
	return &MockDB{
		accountSyncStatus: make(map[identifiers.Identifier][]byte),
		accMetaInfo:       make(map[string]struct{}),
		receipts:          make(map[common.Hash][]byte),
		interactions:      make(map[common.Hash][]byte),
	}
}

func (m MockDB) UpdateAccMetaInfo(id identifiers.Identifier, height uint64, tesseractHash common.Hash,
	stateHash, contextHash common.Hash, consensusNodesHash common.Hash, inheritedAccount identifiers.Identifier,
	commitHash common.Hash, accType common.AccountType, shouldUpdateContextSetPosition bool, positionInContextSet int,
) (int32, bool, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetCommitInfo(tsHash common.Hash, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetCommitInfo(tsHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetAccount(id identifiers.Identifier, stateHash common.Hash) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) StreamSnapshot(ctx context.Context, id identifiers.Identifier,
	sinceTS uint64, respChan chan<- common.SnapResponse,
) (uint64, error) {
	// TODO implement me
	panic("implement me")
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

func (m MockDB) SetAccount(id identifiers.Identifier, stateHash common.Hash, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetAccountSyncStatus(id identifiers.Identifier, status *common.AccountSyncStatus) error {
	rawStatus, err := status.Bytes()
	if err != nil {
		return err
	}

	m.accountSyncStatus[id] = rawStatus

	return nil
}

func (m MockDB) CleanupAccountSyncStatus(id identifiers.Identifier) error {
	delete(m.accountSyncStatus, id)

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

func (m MockDB) UpdatePrimarySyncStatus(id identifiers.Identifier) error {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) IsAccountPrimarySyncDone(id identifiers.Identifier) bool {
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
	id identifiers.Identifier,
	sinceTS uint64,
) (*common.Snapshot, error) {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) HasTesseractAt(id identifiers.Identifier, height uint64) bool {
	// TODO implement me
	panic("implement me")
}

func (m MockDB) SetTesseractHeightEntry(id identifiers.Identifier, height uint64, tsHash common.Hash) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockDB) setAccMetaInfoAt(id identifiers.Identifier, height uint64) {
	key := id.Hex() + strconv.FormatUint(height, 10)
	m.accMetaInfo[key] = struct{}{}
}

func (m MockDB) HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool {
	key := id.Hex() + strconv.FormatUint(height, 10)
	_, ok := m.accMetaInfo[key]

	return ok
}

func getDeltaGroup(t *testing.T, consensusNodeCount int, replaceCount int) *common.DeltaGroup {
	t.Helper()

	return &common.DeltaGroup{
		ConsensusNodes: tests.RandomKramaIDs(t, consensusNodeCount),
		ReplacedNodes:  tests.RandomKramaIDs(t, replaceCount),
	}
}

func tesseractParamsWithContextDelta(
	t *testing.T,
	id identifiers.Identifier,
	consensusNodesCount, replacedNodesCount int,
) *tests.CreateTesseractParams {
	t.Helper()

	return &tests.CreateTesseractParams{
		IDs: []identifiers.Identifier{id},
		Participants: common.ParticipantsState{
			id: {
				ContextDelta: getDeltaGroup(t, consensusNodesCount, replacedNodesCount),
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
			p := participants[tesseracts[0].AnyAccountID()]
			p.TransitiveLink = hash
			participants[tesseracts[0].AnyAccountID()] = p
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

		require.Equal(t, expectedReceipt.IxOps[0].IxType, receivedReceipt.IxOps[0].IxType)
		require.Equal(t, expectedReceipt.Status, receivedReceipt.Status)
		require.Equal(t, expectedReceipt.IxHash, receivedReceipt.IxHash)
		require.Equal(t, expectedReceipt.FuelUsed, receivedReceipt.FuelUsed)
		require.Equal(t, expectedReceipt.IxOps[0].Data, receivedReceipt.IxOps[0].Data)
	}
}

func fetchContextFromLattice(
	t *testing.T,
	id identifiers.Identifier,
	ts common.Tesseract,
	s *Syncer,
) []identifiers.KramaID {
	t.Helper()

	peers := make([]identifiers.KramaID, 0)

	for {
		if len(peers) >= 10 {
			break
		}

		delta, _ := ts.GetContextDelta(id)
		peers = append(peers, delta.ConsensusNodes...)

		consensusNodes, err := s.state.GetConsensusNodesByHash(id, ts.LockedContextHash(id))
		if err == nil {
			peers = append(peers, consensusNodes...)

			break
		}

		if ts.TransitiveLink(id).IsNil() {
			break
		}

		t, err := s.lattice.GetTesseract(ts.TransitiveLink(id), false, false)
		if err != nil {
			return nil
		}

		ts = *t
	}

	return peers
}

func compressData(t *testing.T, data []byte) (common.Compressor, []byte) {
	t.Helper()

	compressor, _ := common.NewZstdCompressor(hclog.NewNullLogger())

	compressedData, err := compressor.Compress(data)
	require.NoError(t, err)

	// Adding additional 4 bytes to maintain the actual raw data length
	rawData := make([]byte, len(compressedData)+4)

	binary.BigEndian.PutUint32(rawData[:4], uint32(len(data)))
	copy(rawData[4:], compressedData)

	return compressor, rawData
}

func setValidators(t *testing.T, s *Syncer, validators []identifiers.KramaID) {
	t.Helper()

	for _, kramaID := range validators {
		sysObject := s.state.GetSystemObject()
		err := sysObject.SetValidators([]*common.Validator{
			{
				ID:      0,
				KramaID: kramaID,
			},
		})

		require.NoError(t, err)
	}
}
