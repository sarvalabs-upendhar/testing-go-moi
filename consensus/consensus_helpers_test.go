package consensus

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-polo"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/go-moi/consensus/kbft"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/stretchr/testify/require"

	lru "github.com/hashicorp/golang-lru"
	"github.com/moby/locker"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/consensus/safety"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/ixpool"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/state"
)

const ensureTimeout = time.Millisecond * 300

type mockDB struct {
	commitInfos   map[common.Hash][]byte
	safetyData    map[identifiers.Identifier][]byte
	proposalInfos map[common.Hash][]byte
	accMetaInfos  map[identifiers.Identifier]*common.AccountMetaInfo
}

func newMockDB() *mockDB {
	return &mockDB{
		commitInfos:   make(map[common.Hash][]byte),
		safetyData:    make(map[identifiers.Identifier][]byte),
		proposalInfos: make(map[common.Hash][]byte),
		accMetaInfos:  make(map[identifiers.Identifier]*common.AccountMetaInfo),
	}
}

func (m mockDB) HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool {
	panic("implement me")
}

func (m *mockDB) setAccountMetaInfo(id identifiers.Identifier, acc *common.AccountMetaInfo) {
	m.accMetaInfos[id] = acc
}

func (m mockDB) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	acc, ok := m.accMetaInfos[id]
	if !ok {
		return nil, errors.New("account meta info not found")
	}

	return acc, nil
}

func (m *mockDB) SetSafetyData(id identifiers.Identifier, data []byte) error {
	m.safetyData[id] = data

	return nil
}

func (m *mockDB) SetConsensusProposalInfo(tsHash common.Hash, data []byte) error {
	m.proposalInfos[tsHash] = data

	return nil
}

func (m mockDB) GetSafetyData(id identifiers.Identifier) ([]byte, error) {
	safetyData, ok := m.safetyData[id]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return safetyData, nil
}

func (m mockDB) GetConsensusProposalInfo(tsHash common.Hash) ([]byte, error) {
	proposal, ok := m.proposalInfos[tsHash]
	if !ok {
		return nil, errors.New("safety data not found")
	}

	return proposal, nil
}

func (m *mockDB) setCommitInfo(tsHash common.Hash, info *common.CommitInfo) error {
	data, err := info.Bytes()
	if err != nil {
		return err
	}

	m.commitInfos[tsHash] = data

	return nil
}

func (m mockDB) GetCommitInfo(tsHash common.Hash) ([]byte, error) {
	info, ok := m.commitInfos[tsHash]
	if !ok {
		return nil, errors.New("commit info not found")
	}

	return info, nil
}

func (m *mockDB) DeleteConsensusProposalInfo(tsHash common.Hash) error {
	delete(m.proposalInfos, tsHash)

	return nil
}

func (m mockDB) GetAllConsensusProposalInfo(ctx context.Context) ([][]byte, error) {
	panic("implement me")
}

func (m *mockDB) DeleteSafetyData(id identifiers.Identifier) error {
	delete(m.safetyData, id)

	return nil
}

func (m mockDB) HasTesseract(tsHash common.Hash) bool {
	panic("implement me")
}

type mockStateManager struct {
	db         *mockDB
	registered map[identifiers.Identifier]struct{}
	publicKeys map[identifiers.Identifier][]byte
}

func (m mockStateManager) CreateStateObjectWithAccountType(
	id identifiers.Identifier,
	accType common.AccountType,
	b bool,
) *state.Object {
	// TODO implement me
	panic("implement me")
}

func newMockStateManager(db *mockDB) *mockStateManager {
	return &mockStateManager{
		db:         db,
		registered: make(map[identifiers.Identifier]struct{}),
		publicKeys: make(map[identifiers.Identifier][]byte),
	}
}

func (m mockStateManager) LoadTransitionObjects(ps map[identifiers.Identifier]common.ParticipantInfo,
	psState common.ParticipantsState,
) (*state.Transition, error) {
	return nil, nil
}

func (m mockStateManager) GetLatestContextAndPublicKeys(id identifiers.Identifier,
) (latestContextHash common.Hash, consensusNodesHash common.Hash, vals []*common.ValidatorInfo, err error) {
	panic("implement me")
}

func (m mockStateManager) GetPublicKeys(ids ...identifiers.KramaID) ([][]byte, error) {
	panic("implement me")
}

func (m mockStateManager) RefreshCachedObject(id identifiers.Identifier, sysObj *state.SystemObject) {
	panic("implement me")
}

func (m mockStateManager) GetSystemObject() *state.SystemObject {
	panic("implement me")
}

func (m mockStateManager) CreateSystemObject(id identifiers.Identifier) *state.SystemObject {
	panic("implement me")
}

func (m mockStateManager) GetPublicKey(id identifiers.Identifier, keyID uint64, stateHash common.Hash) ([]byte, error) {
	panic("implement me")
}

func (m mockStateManager) CreateStateObject(identifier identifiers.Identifier, b bool,
) *state.Object {
	panic("implement me")
}

func (m mockStateManager) GetICSSeed(id identifiers.Identifier) ([32]byte, error) {
	panic("implement me")
}

func (m mockStateManager) GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error) {
	return m.db.GetAccountMetaInfo(id)
}

func (m *mockStateManager) registerAccount(id identifiers.Identifier) {
	m.registered[id] = struct{}{}
}

func (m mockStateManager) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	_, ok := m.registered[id]

	return ok, nil
}

func (m mockStateManager) GetLatestStateObject(id identifiers.Identifier) (*state.Object, error) {
	panic("implement me")
}

func (m mockStateManager) GetSequenceID(id identifiers.Identifier,
	keyID uint64, stateHash common.Hash,
) (uint64, error) {
	panic("implement me")
}

func (m mockStateManager) IsInitialTesseract(ts *common.Tesseract, id identifiers.Identifier) (bool, error) {
	panic("implement me")
}

func (m mockStateManager) IsSealValid(ts *common.Tesseract) (bool, error) {
	panic("implement me")
}

func (m mockStateManager) RemoveCachedObject(id identifiers.Identifier) {
	panic("implement me")
}

func (m mockStateManager) GetRegisteredGuardiansCount() (int, error) {
	panic("implement me")
}

func (m mockStateManager) GetGuardianIncentives(id identifiers.KramaID) (uint64, error) {
	panic("implement me")
}

func (m mockStateManager) GetTotalIncentives() (uint64, error) {
	panic("implement me")
}

func (m mockStateManager) GetConsensusNodes(id identifiers.Identifier,
	hash common.Hash,
) (common.NodeList, common.Hash, error) {
	panic("implement me")
}

type mockExec struct{}

func newMockExec() *mockExec {
	return &mockExec{}
}

func (m mockExec) ExecuteInteractions(transition *state.Transition, interactions common.Interactions,
	executionContext *common.ExecutionContext,
) (common.AccountStateHashes, error) {
	panic("implement me")
}

type mockLattice struct {
	icsCommittee map[common.Hash]*types.ICSCommittee // key is ixnsHash
	teseracts    map[common.Hash]*common.Tesseract
}

func newMockLattice() *mockLattice {
	return &mockLattice{
		icsCommittee: make(map[common.Hash]*types.ICSCommittee),
		teseracts:    make(map[common.Hash]*common.Tesseract),
	}
}

func (m mockLattice) AddTesseractWithState(id identifiers.Identifier, dirtyStorage map[common.Hash][]byte,
	ts *common.Tesseract, transition *state.Transition, allParticipants bool,
) error {
	panic("implement me")
}

func (m mockLattice) AddTesseract(cache bool, id identifiers.Identifier, t *common.Tesseract,
	transition *state.Transition, allParticipants bool,
) error {
	panic("implement me")
}

func (m *mockLattice) addTesseract(ts *common.Tesseract) {
	m.teseracts[ts.Hash()] = ts
}

func (m mockLattice) GetTesseract(hash common.Hash,
	withInteractions bool, withCommitInfo bool,
) (*common.Tesseract, error) {
	ts, ok := m.teseracts[hash]
	if !ok {
		return nil, errors.New("ts not found")
	}

	return ts, nil
}

func (m *mockLattice) storeICSCommittee(hash common.Hash, committee *types.ICSCommittee) {
	m.icsCommittee[hash] = committee
}

func (m mockLattice) getICSCommmittee(ts *common.Tesseract, info *common.CommitInfo) (*types.ICSCommittee, error) {
	committee, ok := m.icsCommittee[ts.InteractionsHash()]
	if !ok {
		return nil, errors.New("committee not found")
	}

	return committee, nil
}

type mockRandomizer struct{}

func newMockRandomizer() *mockRandomizer {
	return &mockRandomizer{}
}

func (m mockRandomizer) GetRandomNodes(ctx context.Context, count int,
	avoidPeers []identifiers.KramaID,
) (randomPeers []identifiers.KramaID, err error) {
	panic("implement me")
}

func (m mockRandomizer) DeletePeers(ids []identifiers.KramaID) {
	panic("implement me")
}

type mockCompressor struct{}

func newMockCompressor() *mockCompressor {
	return &mockCompressor{}
}

func (m mockCompressor) Compress(src []byte) ([]byte, error) {
	panic("implement me")
}

func (m mockCompressor) Decompress(src []byte, dstSize int) ([]byte, error) {
	panic("implement me")
}

func (m mockCompressor) Close() {
	panic("implement me")
}

type packet struct {
	msgType  message.MsgType
	voteType common.ConsensusMsgType
}

type receiver struct {
	k       *Engine
	exclude []packet
}

type mockTransport struct {
	mtx         sync.Mutex
	inboundChan chan *types.ICSMSG
	peers       map[identifiers.KramaID]*receiver
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		peers:       make(map[identifiers.KramaID]*receiver),
		inboundChan: make(chan *types.ICSMSG, 100),
	}
}

func (m *mockTransport) register(k *Engine, packets ...packet) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.peers[k.selfID] = &receiver{
		k:       k,
		exclude: packets,
	}
}

func (m *mockTransport) Start() {
	panic("implement me")
}

func (m *mockTransport) Close() {
	panic("implement me")
}

func (m *mockTransport) Messages() <-chan *types.ICSMSG {
	return m.inboundChan
}

func (m *mockTransport) ForwardMsgToEngine(msg *types.ICSMSG) {
	m.inboundChan <- msg
}

func (m *mockTransport) CleanDirectPeer(clusterID common.ClusterID, peers ...identifiers.KramaID) {
	panic("implement me")
}

func (m *mockTransport) RegisterContextRouter(ctx context.Context, operator identifiers.KramaID,
	clusterID common.ClusterID, nodeset *types.ICSCommittee, voteset *types.HeightVoteSet,
) {
	panic("implement me")
}

func (m *mockTransport) ConnectToDirectPeer(ctx context.Context, kramaID identifiers.KramaID,
	clusterID common.ClusterID,
) error {
	return nil
}

func (m *mockTransport) BroadcastTesseract(msg *message.TesseractMsg) error {
	panic("implement me")
}

func shouldForwardMsg(msg *types.ICSMSG, exclude []packet) bool {
	for _, p := range exclude {
		if p.msgType != msg.MsgType {
			continue
		}

		if p.msgType != message.VOTEMSG {
			return false
		}

		v := new(types.Vote)

		if err := v.FromBytes(msg.Payload); err != nil {
			panic("should be vote")
		}

		if v.Type == p.voteType {
			return false
		}
	}

	return true
}

func (m *mockTransport) SendMessage(ctx context.Context, recipient identifiers.KramaID, msg *types.ICSMSG) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	peer, ok := m.peers[recipient]
	if ok && shouldForwardMsg(msg, peer.exclude) {
		peer.k.transport.ForwardMsgToEngine(msg)

		return nil
	}

	return nil
}

func (m *mockTransport) BroadcastMessage(ctx context.Context, msg *types.ICSMSG) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, peer := range m.peers {
		if shouldForwardMsg(msg, peer.exclude) {
			peer.k.transport.ForwardMsgToEngine(msg)
		}
	}
}

func (m *mockTransport) GracefullyCloseContextRouter(clusterID common.ClusterID) {
}

func (m *mockTransport) StartGossip(clusterID common.ClusterID) {
	panic("implement me")
}

type mockRPCClient struct {
	transport *mockTransport
}

func newMockRPCClient(t *mockTransport) *mockRPCClient {
	return &mockRPCClient{
		transport: t,
	}
}

func (m mockRPCClient) MoiCall(ctx context.Context, kramaID identifiers.KramaID,
	svcName, svcMethod string, args, reply interface{}, ttl time.Duration,
) error {
	peer, ok := m.transport.peers[kramaID]
	if !ok {
		return fmt.Errorf("peer not found for %v", kramaID)
	}

	ts, err := peer.k.GetLockedTSFromDB(args.(common.Hash))
	if err != nil {
		return err
	}

	result := reply.(*message.TesseractSyncMsg) //nolint:forcetypeassert

	rawTS, _ := ts.Bytes()
	rawIxns, _ := ts.Interactions().Bytes()
	rawCommitInfo, _ := ts.CommitInfo().Bytes()
	rawReceipts, _ := ts.Receipts().Bytes()

	result.RawTesseract = rawTS
	result.Ixns = rawIxns
	result.Receipts = rawReceipts
	result.CommitInfo = rawCommitInfo

	return nil
}

type mockIxPool struct {
	ixns map[common.Hash]*common.Interaction
}

func newMockIxPool() *mockIxPool {
	return &mockIxPool{
		ixns: make(map[common.Hash]*common.Interaction),
	}
}

func (m *mockIxPool) SetIxns(interactions *common.Interactions) {
	for _, ixn := range interactions.IxList() {
		m.ixns[ixn.Hash()] = ixn
	}
}

func (m mockIxPool) GetIxns(ixHashes common.Hashes) ([]*common.Interaction, bool) {
	ixns := make([]*common.Interaction, 0)

	for _, ixHash := range ixHashes {
		ix, found := m.ixns[ixHash]
		if !found {
			return nil, false
		}

		ixns = append(ixns, ix)
	}

	return ixns, true
}

func (m mockIxPool) IncrementWaitTime(id identifiers.Identifier, baseTime time.Duration) error {
	panic("implement me")
}

func (m mockIxPool) Executables() ixpool.InteractionQueue {
	panic("implement me")
}

func (m mockIxPool) Drop(ix *common.Interaction) {
	panic("implement me")
}

func (m mockIxPool) ProcessableBatches() []*common.IxBatch {
	panic("implement me")
}

func (m mockIxPool) ViewTimeOut() time.Duration {
	return 30 * time.Minute
}

func (m mockIxPool) UpdateCurrentView(view uint64) {
	panic("implement me")
}

type TestVaults [][]*crypto.KramaVault

func (vaults TestVaults) GetVaults(participantIndex int, count int, exclude ...int) []*crypto.KramaVault {
	vals := make([]*crypto.KramaVault, 0, count)

	shouldExclude := func(i int) bool {
		for _, j := range exclude {
			if j == i {
				return true
			}
		}

		return false
	}

	for i, v := range vaults[participantIndex] {
		if shouldExclude(i) {
			continue
		}

		vals = append(vals, v)

		if len(vals) == count {
			return vals
		}
	}

	return vals
}

// createKramaIDAndPrivateKey returns kramaID and private key pair
func createKramaIDAndPrivateKey(t *testing.T, nthValidator uint32) (identifiers.KramaID, *crypto.BLSPrivKey) {
	t.Helper()

	var signKey [32]byte

	_, err := crand.Read(signKey[:]) // fill sign key with random bytes
	require.NoError(t, err)

	// get private key and public key
	privKeyBytes, _, err := tests.GetPrivKeysForTest(t, signKey[:])
	require.NoError(t, err)

	kramaID, err := identifiers.NewKramaID( // Create kramaID from private key , public key
		identifiers.KindGuardian,
		identifiers.KramaIDV0,
		identifiers.NetworkZone0,
		privKeyBytes[32:],
	)
	require.NoError(t, err)

	cPriv := new(crypto.BLSPrivKey)
	cPriv.UnMarshal(privKeyBytes[:32])

	return kramaID, cPriv
}

// createTestNodeSet return nodeset and vaults
// nodeset has nodes info like krama ID and public key
// vaults has node info like krama ID and private key which can be used to sign votes during consensus
func createTestNodeSet(t *testing.T, n int) (*types.NodeSet, []*crypto.KramaVault) {
	t.Helper()

	valset := make([]*crypto.KramaVault, n)
	valInfos := make([]*common.ValidatorInfo, n)

	for i := 0; i < n; i++ {
		kramaID, privateKey := createKramaIDAndPrivateKey(t, 0)

		valInfos[i] = &common.ValidatorInfo{
			ID:        common.ValidatorIndex(i),
			KramaID:   kramaID,
			PublicKey: privateKey.GetPublicKeyInBytes(),
		}

		valset[i] = new(crypto.KramaVault)
		valset[i].SetKramaID(kramaID)
		valset[i].SetConsensusPrivateKey(privateKey)
	}

	nodeset := types.NewNodeSet(valInfos, uint32(n))

	return nodeset, valset
}

// createICSCommittee returns ICSNodes and vaults of given count of specific nodes
func createICSCommittee(
	t *testing.T,
	participants int,
	nodesPerSet int,
	randomNodes int,
) (*types.ICSCommittee, TestVaults) {
	t.Helper()

	vaults := make([][]*crypto.KramaVault, 0, participants+1)

	ics := types.NewICSCommittee()

	for i := 0; i < participants; i++ {
		ns, vals := createTestNodeSet(t, nodesPerSet)

		ics.AppendNodeSet(common.NilHash, ns)

		vaults = append(vaults, vals)
	}

	randomNS, randomVals := createTestNodeSet(t, randomNodes)
	ics.AppendNodeSet(common.NilHash, randomNS)

	vaults = append(vaults, randomVals)

	return ics, vaults
}

func createICSCommitteeFromVaults(
	vaults TestVaults,
	participantIndexes []int,
) *types.ICSCommittee {
	ics := types.NewICSCommittee()

	for _, i := range participantIndexes {
		count := len(vaults[i])

		valInfos := make([]*common.ValidatorInfo, count)

		for j, v := range vaults[i] {
			valInfos[j] = &common.ValidatorInfo{
				ID:        common.ValidatorIndex(j),
				KramaID:   v.KramaID(),
				PublicKey: v.GetConsensusPrivateKey().GetPublicKeyInBytes(),
			}
		}

		ns := types.NewNodeSet(valInfos, uint32(count))
		ics.AppendNodeSet(common.NilHash, ns)
	}

	return ics
}

func fetchAndStoreRandomNodes(ctx context.Context, cs *types.ClusterState) error {
	return nil
}

// buildTSForTest is helper function used by consensus tests
func (k *Engine) buildTSForTest(lockedTS *common.Tesseract, cs *types.ClusterState) (*common.Tesseract, error) {
	voteset := types.NewHeightVoteSet(
		make([]string, 0),
		cs.NewHeights(),
		cs,
		k.logger.With("cluster-id", cs.ClusterID),
	)

	cs.UpdateVoteSet(voteset)

	if lockedTS != nil {
		return k.createNewTSFromLockedTS(cs, lockedTS)
	}

	participants := participantStates(cs, nil)

	fuelUsed := cs.Transition.Receipts().FuelUsed()
	fuelLimit := uint64(1000)

	ixnsHash, err := cs.Ixns().Hash()
	if err != nil {
		return nil, err
	}

	poxt := common.PoXtData{
		Proposer:     k.selfID,
		View:         k.currentView.ID(),
		EvidenceHash: make(map[identifiers.Identifier]common.Hash),
		AccountLocks: cs.Participants.LockInfo(true),
	}

	ts := common.NewTesseract(
		participants,
		ixnsHash,
		common.NilHash,
		big.NewInt(0), // TODO pass appropriate value
		uint64(cs.CurrentView().StartTime().UnixNano()),
		fuelUsed,
		fuelLimit,
		poxt,
		nil,
		cs.SelfKramaID(),
		cs.Ixns(),
		cs.Transition.Receipts(),
		&common.CommitInfo{
			ClusterID:                 cs.ClusterID,
			Operator:                  cs.Operator(),
			RandomSet:                 cs.GetRandomNodes(),
			RandomSetSizeWithoutDelta: cs.Committee().RandomSetSizeWithOutDelta(),
			View:                      cs.CurrentView().ID(),
		},
	)

	return ts, nil
}

func (k *Engine) finalizeTSForTest(tesseract *common.Tesseract) error {
	if err := storeGenesisData(tesseract, k); err != nil {
		return err
	}

	k.DeleteLockedTSInfo(tesseract, false)

	return nil
}

func (k *Engine) getICSCommitteeForTest(ts *common.Tesseract, info *common.CommitInfo,
	systemObject *state.SystemObject,
) (*types.ICSCommittee, error) {
	l := k.lattice.(*mockLattice) //nolint:forcetypeassert

	return l.getICSCommmittee(ts, info)
}

func (k *Engine) ExecuteForTest(ts *common.Tesseract, transition *state.Transition) error {
	return nil
}

func (k *Engine) handlePrepForTest(msg *types.ICSMSG, prepare *types.Prepare) error {
	k.logger.Debug("Handling prepare message", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	if !k.currentView.IsEqualID(prepare.View) {
		if k.currentView.IsNextView(prepare.View) {
			k.enqueueFutureMsg(msg)
		}

		k.logger.Debug("invalid view", "local view", k.currentView.ID(), "remote view", prepare.View)
		// leader view and the local view should match
		return common.ErrInvalidView
	}

	ixs, found := k.pool.GetIxns(prepare.Ixns)
	if !found {
		return common.ErrIxnsNotFound
	}

	k.logger.Debug("Handling prep message", "ixns-count", len(prepare.Ixns), len(ixs))
	ixns := common.NewInteractionsWithLeaderCheck(true, ixs...)

	id := ixns.UniqueIdsWithoutNoLocks()[0]

	k.participantToPrepareMsg[id] = []*metaPrepareMsg{
		{
			msg:         prepare,
			ixns:        &ixns,
			sender:      msg.Sender,
			clusterID:   msg.ClusterID,
			shouldReply: true,
		},
	}

	return nil
}

func (k *Engine) createICSForValidatorForTest(
	ctx context.Context,
	sender identifiers.KramaID,
	proposal *types.ProposalMsg,
	view *types.View,
) error {
	msg := proposal.Proposal()
	ts := msg.Tesseract
	ids := ts.AccountIDs()

	viewInfos, err := k.loadViewInfo(ids)
	if err != nil {
		return err
	}

	ps, err := getParticipantsInfo(k, ids)
	if err != nil {
		return err
	}

	lm := k.lattice.(*mockLattice) //nolint:forcetypeassert

	ics, err := lm.getICSCommmittee(ts, nil)
	if err != nil {
		return err
	}

	tesseract, _ := k.lattice.GetTesseract(ts.Hash(), false, false)

	cs := types.NewICS(
		ts.Interactions(),
		ts.ClusterID(),
		ts.Operator(),
		common.Canonical(time.Unix(0, int64(proposal.Tesseract.Timestamp()))),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(msg.View(), viewTime(0, msg.View(),
			k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		tesseract != nil,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(ts.ClusterID(), types.OperatorSlot, ts.Interactions().Locks())
	if slot == nil {
		return errors.New("slots are full")
	}

	slot.UpdateClusterState(cs)

	voteset := types.NewHeightVoteSet(
		make([]string, 0),
		cs.NewHeights(),
		cs,
		k.logger.With("cluster-id", cs.ClusterID),
	)

	cs.UpdateVoteSet(voteset)

	go k.icsHandler(k.ctx, ts.ClusterID())

	slot.Msgs <- types.ConsensusMessage{
		PeerID:  sender,
		Payload: proposal.Proposal(),
	}

	return nil
}

func NewTestKramaEngine(
	nodeID int,
	db store,
	mux *utils.TypeMux,
	selfID identifiers.KramaID,
	state stateManager,
	exec execution,
	val vault,
	lattice lattice,
	randomizer randomizer,
	transport kramaTransport,
	slots *types.Slots,
	verifier AggregatedSignatureVerifier,
	compressor common.Compressor,
	cache *lru.Cache,
	rpc rpcClient,
	ixPool ixPool,
	opts ...Option,
) (*Engine, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	cfg := &config.ConsensusConfig{
		Precision:      1000 * time.Nanosecond,
		MessageDelay:   5500 * time.Millisecond,
		TimeoutPrepare: 10 * time.Millisecond,
	}

	k := &Engine{
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		cfg:                     cfg,
		logger:                  hclog.NewNullLogger(),
		mux:                     mux,
		consensusMux:            &utils.TypeMux{},
		selfID:                  selfID,
		state:                   state,
		slots:                   slots,
		randomizer:              randomizer,
		transport:               transport,
		exec:                    exec,
		db:                      db,
		lattice:                 lattice,
		executionReq:            make(chan common.ClusterID),
		vault:                   val,
		clusterLocks:            locker.New(),
		metrics:                 NilMetrics(),
		avgICSTime:              cfg.AccountWaitTime,
		icsCloseCh:              make(chan common.ClusterID),
		signatureVerifier:       verifier,
		tsTracker:               make(map[common.Hash]*utils.TSTrackerEvent),
		safety:                  safety.NewConsensusSafety(db, val),
		futureMsg:               make([]*types.ICSMSG, 0, 30),
		compressor:              compressor,
		preparedMsgQueue:        newJobQueue(),
		workerSignal:            make(chan struct{}),
		workerCount:             10,
		workerWaitTime:          DefaultWorkerWaitTime,
		maxRetryCount:           5,
		cache:                   cache,
		rpcClient:               rpc,
		pool:                    ixPool,
		participantToPrepareMsg: make(map[identifiers.Identifier][]*metaPrepareMsg),
		prepareTimeout:          make(chan struct{}),
		currentView:             &types.View{},
	}

	k.currentView.SetDeadline(time.Now())

	for _, opt := range opts {
		opt(k)
	}

	k.metrics.initMetrics(float64(cfg.OperatorSlotCount), float64(cfg.ValidatorSlotCount))

	k.initICS = k.createICSForValidatorForTest
	k.buildProposalTS = k.buildTSForTest
	k.FetchICSCommittee = k.getICSCommitteeForTest
	k.fetchAndStoreStochasticNodes = fetchAndStoreRandomNodes
	k.finalizedTSHandler = k.finalizeTSForTest
	k.ExecuteAndValidateTS = k.ExecuteForTest
	k.handlePrepare = k.handlePrepForTest

	return k, nil
}

func (k *Engine) resetContextForTest() {
	ctx, ctxCancel := context.WithCancel(context.Background())
	k.ctx = ctx
	k.ctxCancel = ctxCancel
}

func (k *Engine) storeICSCommitteeForTest(ixnsHash common.Hash, ics *types.ICSCommittee) error {
	lm := k.lattice.(*mockLattice) //nolint:forcetypeassert

	lm.storeICSCommittee(ixnsHash, ics)

	return nil
}

// registerForTest allows src to send messages to receiver except exclude packets
func (k *Engine) registerForTest(receiver *Engine, exclude ...packet) {
	source := k.transport.(*mockTransport) //nolint:forcetypeassert

	source.register(receiver, exclude...)
}

func createGenesisTS(t *testing.T, ids []identifiers.Identifier) *common.Tesseract {
	t.Helper()

	var (
		ixHashString = "Genesis"
		participants = make(common.ParticipantsState)
	)

	for _, id := range ids {
		participants[id] = common.State{
			Height:         0,
			TransitiveLink: common.NilHash,
			LockedContext:  common.NilHash,
			StateHash:      common.NilHash,
			ContextDelta:   &common.DeltaGroup{},
		}
	}

	interactionsHash := common.GetHash([]byte(ixHashString))

	poxt := common.PoXtData{
		View:     common.GenesisView,
		ICSSeed:  tests.RandomHash(t),
		ICSProof: tests.RandomHash(t).Bytes(),
	}

	ts := common.NewTesseract(
		participants,
		interactionsHash,
		common.NilHash,
		big.NewInt(0),
		0,
		0,
		0,
		poxt,
		nil,
		"",
		common.Interactions{},
		nil,
		&common.CommitInfo{
			View: common.GenesisView,
		},
	)

	ts.SetCommitQc(&common.Qc{
		View:   common.GenesisView,
		TSHash: ts.Hash(),
	})

	return ts
}

//nolint:forcetypeassert
func storeGenesisData(ts *common.Tesseract, engines ...*Engine) error {
	for _, k := range engines {
		db := k.db.(*mockDB)
		sm := k.state.(*mockStateManager)
		l := k.lattice.(*mockLattice)

		l.addTesseract(ts)

		if err := db.setCommitInfo(ts.Hash(), ts.CommitInfo()); err != nil {
			return err
		}

		for id, state := range ts.Participants() {
			sm.registerAccount(id)
			db.setAccountMetaInfo(id, &common.AccountMetaInfo{
				Type:          common.RegularAccount,
				ID:            id,
				Height:        state.Height,
				TesseractHash: ts.Hash(),
			})
		}
	}

	return nil
}

func getParticipantsInfo(k *Engine, ids []identifiers.Identifier) (common.Participants, error) {
	ps := make(common.Participants, len(ids))

	for _, id := range ids {
		info, err := k.state.GetAccountMetaInfo(id)
		if err != nil {
			return nil, err
		}

		ps[id] = &common.Participant{
			ID:            id,
			Height:        info.Height,
			TesseractHash: info.TesseractHash,
		}
	}

	return ps, nil
}

func validatePrepare(t *testing.T, prepare *types.Prepare, view uint64, hashes common.Hashes) {
	t.Helper()

	require.Equal(t, view, prepare.View)
	require.Equal(t, len(hashes), len(prepare.Ixns))

	for i, hash := range prepare.Ixns {
		require.Equal(t, hashes[i], hash)
	}
}

func ensurePrepare(t *testing.T, prepareSub *utils.Subscription, view uint64, hashes common.Hashes) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for NewRound event")
	case msg := <-prepareSub.Chan():
		event, ok := msg.Data.(eventPrepare)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventPrepare, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validatePrepare(t, event.prepare, view, hashes)
	}
}

func validatePrepared(t *testing.T, prepared *types.Prepared, view uint64, viewInfos common.Views) {
	t.Helper()

	require.Equal(t, view, prepared.View)
	require.Equal(t, viewInfos, prepared.Infos)
}

func ensurePrepared(t *testing.T, preparedSub *utils.Subscription, view uint64, viewInfos common.Views) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for prepared event")
	case msg := <-preparedSub.Chan():
		event, ok := msg.Data.(eventPrepared)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventPrepared, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validatePrepared(t, event.prepared, view, viewInfos)
	}
}

func validateProposal(t *testing.T, proposal *types.Proposal, viewInfos []*common.ViewInfo,
	viewInfosCount int, tsHash common.Hash,
) {
	t.Helper()

	for _, infos := range proposal.PrepareQc.PeerViews {
		if reflect.DeepEqual(common.Views(viewInfos), infos) {
			viewInfosCount--
		}
	}

	if viewInfosCount != 0 {
		require.FailNow(t, fmt.Sprintf("view infos are not present %d", viewInfosCount))
	}

	require.Equal(t, tsHash, proposal.Tesseract.Hash())
}

func ensureProposal(t *testing.T, proposalSub *utils.Subscription, view uint64,
	viewInfos []*common.ViewInfo, viewInfosCount int, cs *types.ClusterState,
) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for proposal event")
	case msg := <-proposalSub.Chan():
		event, ok := msg.Data.(kbft.EventProposal)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventProposal, got %T. Wrong subscription channel?",
				msg.Data))
		}

		require.Equal(t, view, event.Proposal.View())
		validateProposal(t, event.Proposal, viewInfos, viewInfosCount, cs.Tesseract().Hash())
	}
}

func ensureSyncEvent(t *testing.T, syncRequestSub *utils.Subscription, ids common.IdentifierList, height uint64) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for proposal event")
	case msg := <-syncRequestSub.Chan():
		event, ok := msg.Data.(utils.SyncRequestEvent)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a SyncRequestEvent, got %T. Wrong subscription channel?",
				msg.Data))
		}

		require.True(t, ids.Has(event.ID))
		require.Equal(t, height, event.Height)
	}
}

func ensureVote(t *testing.T, voteSub *utils.Subscription,
	view uint64, voteType common.ConsensusMsgType, tsHash common.Hash, isQC bool,
) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for vote event")
	case msg := <-voteSub.Chan():
		event, ok := msg.Data.(kbft.EventVote)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventVote, got %T. Wrong subscription channel?",
				msg.Data))
		}

		require.Equal(t, view, event.Vote.View)
		require.Equal(t, voteType, event.Vote.Type)
		require.Equal(t, tsHash, event.Vote.TSHash)
		require.Equal(t, isQC, event.Vote.IsQC)
	}
}

func signVote(
	t *testing.T,
	view uint64,
	id common.ClusterID,
	voteType common.ConsensusMsgType,
	tsHash common.Hash,
	cs *types.ClusterState,
	kramaVault *crypto.KramaVault,
) *types.ICSMSG {
	t.Helper()

	valIndex, _, _ := cs.HasKramaID(kramaVault.KramaID())

	if valIndex == -1 {
		require.NoError(t, common.ErrKramaIDNotFound)
	}

	v := &types.Vote{
		SignerIndex: valIndex,
		TSHash:      tsHash,
		View:        view,
		Type:        voteType,
	}

	rawData, err := v.SignBytes()
	require.NoError(t, err)

	sign, err := kramaVault.Sign(rawData, mudracommon.BlsBLST)
	require.NoError(t, err)

	v.Signature = make([]byte, len(sign))
	copy(v.Signature, sign)

	msg, err := v.Bytes()
	require.NoError(t, err)

	return types.NewICSMsg(kramaVault.KramaID(), id, message.VOTEMSG, msg, false)
}

func signVotes(
	t *testing.T,
	view uint64,
	id common.ClusterID,
	voteType common.ConsensusMsgType,
	tsHash common.Hash,
	cs *types.ClusterState,
	kramaVault ...*crypto.KramaVault,
) []*types.ICSMSG {
	t.Helper()

	msgs := make([]*types.ICSMSG, len(kramaVault))

	for i, kVault := range kramaVault {
		msgs[i] = signVote(t, view, id, voteType, tsHash, cs, kVault)
	}

	return msgs
}

func sendVoteMsg(
	t *testing.T,
	k *Engine,
	view uint64,
	id common.ClusterID,
	voteType common.ConsensusMsgType,
	tsHash common.Hash,
	cs *types.ClusterState,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	if len(kramaVault) == 0 {
		require.FailNow(t, "there are no validators to sign")
	}

	msgs := signVotes(t, view, id, voteType, tsHash, cs, kramaVault...)

	for _, msg := range msgs {
		k.transport.ForwardMsgToEngine(msg)
	}
}

func signPrepared(
	t *testing.T,
	id common.ClusterID,
	view uint64,
	viewInfos []*common.ViewInfo,
	kramaVault *crypto.KramaVault,
) *types.ICSMSG {
	t.Helper()

	responseMsg := &types.Prepared{
		View:  view,
		Infos: viewInfos,
	}

	err := responseMsg.Sign(kramaVault.Sign)
	require.NoError(t, err)

	rawData, err := responseMsg.Bytes()
	require.NoError(t, err)

	return types.NewICSMsg(kramaVault.KramaID(), id, message.PREPARED, rawData, false)
}

func signPreparedMsgs(
	t *testing.T,
	id common.ClusterID,
	view uint64,
	viewInfos []*common.ViewInfo,
	kramaVault ...*crypto.KramaVault,
) []*types.ICSMSG {
	t.Helper()

	msgs := make([]*types.ICSMSG, len(kramaVault))

	for i, kVault := range kramaVault {
		msgs[i] = signPrepared(t, id, view, viewInfos, kVault)
	}

	return msgs
}

func sendPreparedMsg(
	t *testing.T,
	k *Engine,
	id common.ClusterID,
	view uint64,
	viewInfos []*common.ViewInfo,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	if len(kramaVault) == 0 {
		require.FailNow(t, "there are no validators to sign")
	}

	msgs := signPreparedMsgs(t, id, view, viewInfos, kramaVault...)

	for _, msg := range msgs {
		k.transport.ForwardMsgToEngine(msg)
	}
}

func ensureClusterCleanup(t *testing.T, cleanupSub *utils.Subscription, id common.ClusterID) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for cleanup event")
	case msg := <-cleanupSub.Chan():
		event, ok := msg.Data.(eventCleanup)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventCleanup, got %T. Wrong subscription channel?",
				msg.Data))
		}

		require.Equal(t, id, event.clusterID)
	}
}

func ensureNoEventReceived(t *testing.T, eventSub *utils.Subscription) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		return
	case msg := <-eventSub.Chan():
		require.FailNow(t, fmt.Sprintf("expected a no event, got %T",
			msg.Data))
	}
}

func checkForTS(t *testing.T, k *Engine, tsHash common.Hash) {
	t.Helper()

	lm := k.lattice.(*mockLattice) //nolint:forcetypeassert

	_, ok := lm.teseracts[tsHash]
	require.True(t, ok)
}

func newKramaInstance(t *testing.T, index int, vault *crypto.KramaVault) *Engine {
	t.Helper()

	db := newMockDB()
	sm := newMockStateManager(db)
	transport := newMockTransport()
	rpc := newMockRPCClient(transport)

	cache, err := lru.New(1000)
	require.NoError(t, err)

	k, err := NewTestKramaEngine(
		index,
		db,
		&utils.TypeMux{},
		vault.KramaID(),
		sm,
		newMockExec(),
		vault,
		newMockLattice(),
		newMockRandomizer(),
		transport,
		types.NewSlots(1, 1, hclog.NewNullLogger()),
		crypto.VerifyAggregateSignature,
		newMockCompressor(),
		cache,
		rpc,
		newMockIxPool(),
	)
	require.NoError(t, err)

	return k
}

func createKramaInstances(t *testing.T, vaults []*crypto.KramaVault) []*Engine {
	t.Helper()

	k := make([]*Engine, len(vaults))

	for index, v := range vaults {
		k[index] = newKramaInstance(t, index, v)
	}

	return k
}

func startHandler(k *Engine, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	k.handler()
}

func startICSHandler(k *Engine, wg *sync.WaitGroup, ctx context.Context, clusterID common.ClusterID) {
	wg.Add(1)
	defer wg.Done()

	k.icsHandler(ctx, clusterID)
}

func ensureEmptySlots(t *testing.T, k *Engine) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		count := k.slots.Len()

		return nil, count != 0
	})

	require.NoError(t, err, "slot is still present")
}

func exitICSHandler(slot *types.Slot) {
	slot.BftStopChan <- nil
}

func wait(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
		return
	case <-time.After(1 * time.Second):
		require.Error(t, errors.New("timed out waiting for goroutine to finish"))
	}
}

func storeIxns(pool ixPool, ixns *common.Interactions) {
	p := pool.(*mockIxPool) //nolint:forcetypeassert

	p.SetIxns(ixns)
}

// This mock is used for unit tests
type mockTransportUnit struct {
	preparedMsgs map[identifiers.KramaID]*types.ICSMSG
}

func newMockTransportUnit() *mockTransportUnit {
	return &mockTransportUnit{
		preparedMsgs: make(map[identifiers.KramaID]*types.ICSMSG),
	}
}

func (m mockTransportUnit) Start() {
	panic("implement me")
}

func (m mockTransportUnit) Close() {
	panic("implement me")
}

func (m mockTransportUnit) Messages() <-chan *types.ICSMSG {
	panic("implement me")
}

func (m mockTransportUnit) ForwardMsgToEngine(msg *types.ICSMSG) {
	panic("implement me")
}

func (m mockTransportUnit) CleanDirectPeer(clusterID common.ClusterID, peers ...identifiers.KramaID) {
	panic("implement me")
}

func (m mockTransportUnit) RegisterContextRouter(ctx context.Context, operator identifiers.KramaID,
	clusterID common.ClusterID, nodeset *types.ICSCommittee, voteset *types.HeightVoteSet,
) {
	panic("implement me")
}

func (m mockTransportUnit) ConnectToDirectPeer(ctx context.Context,
	kramaID identifiers.KramaID, clusterID common.ClusterID,
) error {
	panic("implement me")
}

func (m mockTransportUnit) BroadcastTesseract(msg *message.TesseractMsg) error {
	panic("implement me")
}

func (m mockTransportUnit) BroadcastMessage(ctx context.Context, msg *types.ICSMSG) {
	panic("implement me")
}

func (m mockTransportUnit) GracefullyCloseContextRouter(clusterID common.ClusterID) {
	panic("implement me")
}

func (m *mockTransportUnit) SendMessage(ctx context.Context, recipient identifiers.KramaID, msg *types.ICSMSG) error {
	m.preparedMsgs[recipient] = msg

	return nil
}

func (m mockTransportUnit) StartGossip(clusterID common.ClusterID) {
	panic("implement me")
}

func insertPrepareMsg(k *Engine, nodeContextParticipants []identifiers.Identifier,
	msg *metaPrepareMsg, ps map[identifiers.Identifier]*common.ParticipantInfo,
) {
	for _, id := range nodeContextParticipants {
		info, ok := ps[id]
		if !ok || info.LockType == common.NoLock {
			continue
		}

		k.participantToPrepareMsg[id] = append(k.participantToPrepareMsg[id], msg)
	}
}

func newTestKramaEngine(v vault) *Engine {
	return &Engine{
		state:                   newMockStateManager(nil),
		transport:               newMockTransportUnit(),
		participantToPrepareMsg: make(map[identifiers.Identifier][]*metaPrepareMsg),
		vault:                   v,
	}
}

func getIDs(msg *types.Prepared) []identifiers.Identifier {
	ids := make([]identifiers.Identifier, 0)

	for _, info := range msg.Infos {
		ids = append(ids, info.ID)
	}

	return ids
}

func ensureIDsMatch(t *testing.T, expected, actual common.IdentifierList) {
	t.Helper()

	sort.Sort(expected)
	sort.Sort(actual)

	require.Equal(t, expected, actual)
}

func ensureResponseMatches(t *testing.T, k *Engine,
	expectedNodesIndexes []int, senders []identifiers.KramaID, ixns []*common.Interaction,
) {
	t.Helper()

	transport := k.transport.(*mockTransportUnit) //nolint:forcetypeassert

	require.Equal(t, len(expectedNodesIndexes), len(transport.preparedMsgs))

	for _, idx := range expectedNodesIndexes {
		icsMsg, ok := transport.preparedMsgs[senders[idx]]
		require.True(t, ok)

		prepared := new(types.Prepared)
		err := prepared.FromBytes(icsMsg.Payload)
		require.NoError(t, err)

		ensureIDsMatch(t, ixns[idx].AccountsWithoutNoLock(), getIDs(prepared))
	}
}

func getHash(sender identifiers.KramaID, viewID uint64) common.Hash {
	polorizer := polo.NewPolorizer()

	polorizer.PolorizeString(string(sender))
	polorizer.PolorizeUint(viewID)

	return common.GetHash(polorizer.Bytes())
}
