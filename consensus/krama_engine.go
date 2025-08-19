package consensus

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/moby/locker"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	"github.com/sarvalabs/go-moi/consensus/safety"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/flux"
	"github.com/sarvalabs/go-moi/ixpool"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/telemetry/tracing"
)

const (
	ICSTimeOutDuration = 6 * time.Second
)

type AggregatedSignatureVerifier func(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error)

type lattice interface {
	AddTesseractWithState(
		id identifiers.Identifier,
		dirtyStorage map[common.Hash][]byte,
		ts *common.Tesseract,
		transition *state.Transition,
		allParticipants bool,
	) error
	AddTesseract(
		cache bool,
		id identifiers.Identifier,
		t *common.Tesseract,
		transition *state.Transition,
		allParticipants bool,
	) error
	GetTesseract(
		hash common.Hash,
		withInteractions bool,
		withCommitInfo bool,
	) (*common.Tesseract, error)
}

type kramaTransport interface {
	Start()
	Close()
	Messages() <-chan *ktypes.ICSMSG
	CleanDirectPeer(clusterID common.ClusterID, peers ...identifiers.KramaID)
	RegisterContextRouter(
		ctx context.Context,
		operator identifiers.KramaID,
		clusterID common.ClusterID,
		nodeset *ktypes.ICSCommittee,
		voteset *ktypes.HeightVoteSet,
	)
	ConnectToDirectPeer(ctx context.Context, kramaID identifiers.KramaID, clusterID common.ClusterID) error
	BroadcastTesseract(msg *networkmsg.TesseractMsg) error
	BroadcastMessage(
		ctx context.Context,
		msg *ktypes.ICSMSG,
	)
	GracefullyCloseContextRouter(clusterID common.ClusterID)
	SendMessage(
		ctx context.Context,
		recipient identifiers.KramaID,
		msg *ktypes.ICSMSG,
	) error
	StartGossip(clusterID common.ClusterID)
}

type stateManager interface {
	GetPublicKey(id identifiers.Identifier, KeyID uint64, stateHash common.Hash) ([]byte, error)
	LoadTransitionObjects(
		ps map[identifiers.Identifier]common.ParticipantInfo, psState common.ParticipantsState,
	) (*state.Transition, error)
	CreateStateObject(identifiers.Identifier, common.AccountType, bool) *state.Object
	GetLatestContextAndPublicKeys(id identifiers.Identifier) (
		latestContextHash common.Hash,
		consensusNodesHash common.Hash,
		vals []*common.ValidatorInfo,
		err error,
	)
	GetPublicKeys(ids ...identifiers.KramaID) ([][]byte, error)
	GetICSSeed(id identifiers.Identifier) ([32]byte, error)
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	IsAccountRegistered(id identifiers.Identifier) (bool, error)
	GetLatestStateObject(id identifiers.Identifier) (*state.Object, error)
	GetSequenceID(id identifiers.Identifier, KeyID uint64, stateHash common.Hash) (uint64, error)
	IsInitialTesseract(ts *common.Tesseract, id identifiers.Identifier) (bool, error)
	IsSealValid(ts *common.Tesseract) (bool, error)
	RefreshCachedObject(id identifiers.Identifier, sysObj *state.SystemObject)
	GetConsensusNodes(
		id identifiers.Identifier,
		hash common.Hash,
	) (
		common.NodeList,
		common.Hash,
		error,
	)
	GetSystemObject() *state.SystemObject
	CreateSystemObject(id identifiers.Identifier) *state.SystemObject
}

type ixPool interface {
	IncrementWaitTime(id identifiers.Identifier, baseTime time.Duration) error
	Executables() ixpool.InteractionQueue
	Drop(ix *common.Interaction)
	ProcessableBatches() []*common.IxBatch
	ViewTimeOut() time.Duration
	UpdateCurrentView(view uint64)
}

type execution interface {
	ExecuteInteractions(
		*state.Transition, common.Interactions, *common.ExecutionContext,
	) (common.AccountStateHashes, error)
}

type store interface {
	HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	GetSafetyData(id identifiers.Identifier) ([]byte, error)
	GetCommitInfo(tsHash common.Hash) ([]byte, error)
	SetSafetyData(id identifiers.Identifier, data []byte) error
	SetConsensusProposalInfo(tsHash common.Hash, data []byte) error
	GetConsensusProposalInfo(tsHash common.Hash) ([]byte, error)
	DeleteConsensusProposalInfo(tsHash common.Hash) error
	GetAllConsensusProposalInfo(ctx context.Context) ([][]byte, error)
	DeleteSafetyData(id identifiers.Identifier) error
	HasTesseract(tsHash common.Hash) bool
}

type vault interface {
	GetConsensusPrivateKey() crypto.PrivateKey
	Sign(data []byte, sigType mudraCommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
	KramaID() identifiers.KramaID
}

type Engine struct {
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	cfg                 *config.ConsensusConfig
	mux                 *utils.TypeMux
	logger              hclog.Logger
	selfID              identifiers.KramaID
	slots               *ktypes.Slots
	randomizer          *flux.Randomizer
	transport           kramaTransport
	exec                execution
	pool                ixPool
	db                  store
	state               stateManager
	executionReq        chan common.ClusterID
	lattice             lattice
	wal                 kbft.WAL
	vault               vault
	clusterLocks        *locker.Locker
	metrics             *Metrics
	avgICSTime          time.Duration
	icsCloseCh          chan common.ClusterID
	signatureVerifier   AggregatedSignatureVerifier
	tsTracker           map[common.Hash]*utils.TSTrackerEvent
	view                atomic.Uint64
	viewTimeOutDeadline atomic.Value
	accountLocks        map[identifiers.Identifier]*ktypes.AccConsensusLockInfo
	safety              *safety.ConsensusSafety
	trustedPeersPresent bool
	futureMsg           []*ktypes.ICSMSG
	compressor          common.Compressor
	minimumStake        *big.Int
}

func NewKramaEngine(
	db store,
	cfg *config.ConsensusConfig,
	logger hclog.Logger,
	mux *utils.TypeMux,
	selfID identifiers.KramaID,
	state stateManager,
	exec execution,
	ixPool ixPool,
	val vault,
	lattice lattice,
	randomizer *flux.Randomizer,
	transport kramaTransport,
	metrics *Metrics,
	slots *ktypes.Slots,
	verifier AggregatedSignatureVerifier,
	compressor common.Compressor,
) (*Engine, error) {
	wal, err := kbft.NewWAL(logger, cfg.DirectoryPath)
	if err != nil {
		return nil, errors.Wrap(err, "WAL failed")
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	k := &Engine{
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		cfg:                 cfg,
		logger:              logger.Named("Krama-Engine"),
		mux:                 mux,
		selfID:              selfID,
		state:               state,
		slots:               slots,
		randomizer:          randomizer,
		transport:           transport,
		exec:                exec,
		db:                  db,
		pool:                ixPool,
		lattice:             lattice,
		executionReq:        make(chan common.ClusterID),
		wal:                 wal,
		vault:               val,
		clusterLocks:        locker.New(),
		metrics:             metrics,
		avgICSTime:          cfg.AccountWaitTime,
		icsCloseCh:          make(chan common.ClusterID),
		signatureVerifier:   verifier,
		tsTracker:           make(map[common.Hash]*utils.TSTrackerEvent),
		trustedPeersPresent: len(cfg.TrustedPeers) > 0,
		safety:              safety.NewConsensusSafety(db, val),
		futureMsg:           make([]*ktypes.ICSMSG, 0, 30),
		compressor:          compressor,
		// TODO: Update after the staking flow is implemented
		minimumStake: big.NewInt(0),
	}

	k.metrics.initMetrics(float64(cfg.OperatorSlotCount), float64(cfg.ValidatorSlotCount))

	return k, nil
}

func (k *Engine) enqueueFutureMsg(msg *ktypes.ICSMSG) {
	k.futureMsg = append(k.futureMsg, msg)
}

func (k *Engine) dequeueFutureMsg() {
	k.futureMsg = k.futureMsg[1:]
}

// createICS initializes a new Interaction Consensus Set (ICS) for a given cluster ID and interactions.
// It creates a new slot for the cluster by locking the accounts.
// If the accounts are already locked, it returns the locked cluster ID.
// Otherwise, it locks the accounts, loads the cluster state, and returns the slot.
func (k *Engine) createICS(
	ctx context.Context,
	clusterID common.ClusterID,
	ixns common.Interactions,
	locks map[identifiers.Identifier]common.LockType,
) (*ktypes.Slot, common.ClusterID, error) {
	slot, activeCluster := k.slots.CreateSlotAndLockAccounts(clusterID, ktypes.OperatorSlot, locks)
	if slot == nil {
		return nil, activeCluster, common.ErrSlotsFull
	}

	cs, err := k.loadClusterState(ctx, k.selfID, ixns, clusterID, time.Now())
	if err != nil {
		return nil, "", err
	}

	slot.UpdateClusterState(cs)

	if err = k.validateInteractions(ixns); err != nil {
		return nil, "", err
	}

	k.transport.RegisterContextRouter(ctx, k.selfID, clusterID, cs.Committee(), nil)

	return slot, "", nil
}

// loadClusterState create a clusterState instance with latest participants state and committee information.
func (k *Engine) loadClusterState(
	ctx context.Context,
	operator identifiers.KramaID,
	ixns common.Interactions,
	clusterID common.ClusterID,
	reqTime time.Time,
	qc ...*common.Qc,
) (*ktypes.ClusterState, error) {
	var err error

	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "loadClusterState")
	defer span.End()

	participants, committee, err := k.fetchParticipantsAndCommittee(ctx, ixns.Participants())
	if err != nil {
		k.logger.Error("failed to fetch participants", "error", err)

		return nil, err
	}

	viewInfos, err := k.loadViewInfo(participants.IDs())
	if err != nil {
		k.logger.Error("Failed to load view info", err)

		return nil, err
	}

	clusterState := ktypes.NewICS( //nolint:forcetypeassert
		ixns,
		clusterID,
		operator,
		reqTime,
		k.selfID,
		committee,
		k.state.GetSystemObject(),
		participants,
		viewInfos,
		k.view.Load(),
		k.viewTimeOutDeadline.Load().(time.Time),
	)

	return clusterState, nil
}

func (k *Engine) fetchParticipantsAndCommittee(
	ctx context.Context,
	ps map[identifiers.Identifier]common.ParticipantInfo,
) (
	common.Participants,
	*ktypes.ICSCommittee,
	error,
) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "fetchIxAccounts")
	defer span.End()

	participants := make(map[identifiers.Identifier]*common.Participant)
	ids := make(common.IdentifierList, 0, len(ps))

	for id, info := range ps {
		if _, ok := participants[id]; ok {
			continue
		}

		ids = append(ids, id)

		if !info.IsGenesis {
			accInfo, err := k.state.GetAccountMetaInfo(id)
			if err != nil {
				return nil, nil, err
			}

			participants[id] = &common.Participant{
				AccType:       accInfo.Type,
				ID:            id,
				IsGenesis:     info.IsGenesis,
				Height:        accInfo.Height,
				TesseractHash: accInfo.TesseractHash,
				LockType:      info.LockType,
				IsSigner:      info.IsSigner,
				CommitHash:    accInfo.CommitHash,
			}

			continue
		}

		participants[id] = &common.Participant{
			AccType:       info.AccType,
			ID:            id,
			IsGenesis:     info.IsGenesis,
			Height:        0,
			TesseractHash: common.NilHash,
			LockType:      info.LockType,
			ContextHash:   common.NilHash,
		}
	}

	sort.Sort(ids)

	committee := ktypes.NewICSCommittee()

	position := 0

	for _, id := range ids {
		if participants[id].IsGenesis {
			continue
		}

		contextHash, consensusNodesHash, vals, err := k.state.GetLatestContextAndPublicKeys(id)
		if err != nil {
			return nil, nil, err
		}

		participants[id].ContextHash = contextHash

		existingPosition, ok := committee.GetNodesetPosition(consensusNodesHash)
		if ok {
			participants[id].NodeSetPosition = existingPosition
			participants[id].ConsensusQuorum = committee.ParticipantQuorum(existingPosition)
			participants[id].ConsensusNodesHash = consensusNodesHash
			committee.IncrementPSCount(consensusNodesHash)

			continue
		}

		committee.AppendNodeSet(consensusNodesHash,
			ktypes.NewNodeSet(vals, uint32(len(vals))),
		)

		participants[id].NodeSetPosition = position
		participants[id].ConsensusQuorum = committee.ParticipantQuorum(position)
		participants[id].ConsensusNodesHash = consensusNodesHash

		position++
	}

	return participants, committee, nil
}

// isOperatorEligible checks if the operator is eligible to propose a tesseract for the given interactions.
func (k *Engine) isOperatorEligible(peerID identifiers.KramaID, ixns common.Interactions) bool {
	id := ixns.LeaderCandidateID()

	fmt.Println("Leader candidate info", "id", id, "ixns-size", ixns.Len())

	metaInfo, err := k.state.GetAccountMetaInfo(id)
	if err != nil {
		k.logger.Error("failed to check operator eligibility", "error", err)

		return false
	}

	if id.IsParticipantVariant() {
		metaInfo, err = k.state.GetAccountMetaInfo(metaInfo.InheritedAccount)
		if err != nil {
			k.logger.Error("failed to check operator eligibility", "error", err)

			return false
		}
	}

	if peerID == k.selfID {
		if metaInfo.PositionInContextSet < 0 {
			return false
		}

		currentView := k.view.Load()

		fmt.Println("Current View", currentView, currentView%common.ConsensusNodesSize)

		return currentView%common.ConsensusNodesSize == uint64(metaInfo.PositionInContextSet)
	}

	consensusNodes, _, err := k.state.GetConsensusNodes(id, metaInfo.ContextHash)
	if err != nil {
		k.logger.Error("failed to check operator eligibility", "error", err)

		return false
	}

	return consensusNodes.Contains(peerID)
}

func (k *Engine) updateContextDelta(cs *ktypes.ClusterState) error {
	for _, ps := range cs.Participants {
		// Check 1: We should not update context for accounts with read lock
		// Check 2: We should not update context for a non-signer regular account
		if ps.LockType > common.MutateLock || !ps.IsContextUpdateRequired() {
			continue
		}

		//  Check 3: In debug mode, we should only update context for new accounts
		if k.cfg.EnableDebugMode && !ps.IsGenesis {
			continue
		}

		deltaGroup := new(common.DeltaGroup)

		if ps.IsGenesis {
			// Fetch new nodes for the receiver account
			consensusNodes, err := k.getConsensusNodes(
				cs,
				common.ConsensusNodesSize,
			)
			if err != nil {
				return err
			}

			deltaGroup.ConsensusNodes = append(deltaGroup.ConsensusNodes, consensusNodes...)

			ps.ContextDelta = deltaGroup

			continue
		}
	}

	return nil
}

// getConsensusNodes returns a list of nodes for updating the consensus nodes of an account.
// If trusted peers are available, it returns the trusted peers. Otherwise, it returns the nodes from the random set.
func (k *Engine) getConsensusNodes(
	clusterInfo *ktypes.ClusterState,
	requiredConsensusNodes int,
) (consensusNodes []identifiers.KramaID, err error) {
	if k.trustedPeersPresent {
		peers := clusterInfo.TrustedPeers

		return peers[:requiredConsensusNodes], nil
	}

	// TODO: Need to improve this function
	set := clusterInfo.Committee().RandomSet()
	count := 0

	for index, info := range set.Infos {
		if set.Responses.GetIndex(index) {
			if len(consensusNodes) != requiredConsensusNodes {
				consensusNodes = append(consensusNodes, info.KramaID)
				count++
			}
		}

		if count == requiredConsensusNodes {
			break
		}
	}

	return consensusNodes, nil
}

func (k *Engine) getStochasticNodes(
	ctx context.Context,
	cs *ktypes.ClusterState,
	count int,
	exemptedNodes map[common.ValidatorIndex]struct{},
) ([]common.ValidatorIndex, error) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "getStochasticNodes")
	defer span.End()

	queryInitTime := time.Now()

	indices, err := k.ShuffledList(cs, exemptedNodes)
	if err != nil {
		return nil, err
	}

	k.metrics.captureRandomNodesQueryTime(queryInitTime)

	return indices, nil
}

func (k *Engine) getTrustedPeers(count int) []identifiers.KramaID {
	randomNumbers := utils.GetRandomNumbers(count, len(k.cfg.TrustedPeers))
	peers := make([]identifiers.KramaID, 0, count)

	for id, trustedPeer := range k.cfg.TrustedPeers {
		if _, ok := randomNumbers[id]; ok {
			peers = append(peers, trustedPeer.ID)
		}
	}

	return peers
}

func (k *Engine) createICSForProposal(ctx context.Context, sender identifiers.KramaID, msg *ktypes.Proposal) error {
	k.logger.Debug("Handling proposal message", "cluster-id", msg.ClusterID())
	ts := msg.Tesseract
	// TODO: validate ts timestamp
	slot, _ := k.slots.CreateSlotAndLockAccounts(msg.ClusterID(), ktypes.ValidatorSlot, msg.Locks())
	if slot == nil {
		ps := msg.Tesseract.Participants()
		for id, lockInfo := range k.slots.ActiveAccounts() {
			if _, ok := ps[id]; !ok {
				continue
			}

			for _, info := range lockInfo {
				k.logger.Debug("krama active accounts", "cluster-id", msg.ClusterID(), "id", id, "lock-info", info.String())
			}
		}

		return errors.New("failed to create slot")
	}

	if !k.isOperatorEligible(sender, msg.Tesseract.Interactions()) {
		return common.ErrOperatorNotEligible
	}

	cs, err := k.loadClusterState(ctx, ts.Operator(), ts.Interactions(), ts.ClusterID(), time.Now())
	if err != nil {
		return err
	}

	cs.ExcludeParticipantsFromICS(ts.ExcludedAccounts())

	vals, err := cs.SystemObject.GetValidators(ts.CommitInfo().RandomSet...)
	if err != nil {
		return err
	}

	cs.Committee().AppendNodeSet(common.NilHash, ktypes.NewNodeSet(vals,
		ts.CommitInfo().RandomSetSizeWithoutDelta,
	))

	voteset := ktypes.NewHeightVoteSet(
		make([]string, 0),
		cs.NewHeights(),
		cs,
		k.logger.With("cluster-id", ts.ClusterID()),
	)

	cs.UpdateVoteSet(voteset)
	slot.UpdateClusterState(cs)

	if err = k.validateInteractions(ts.Interactions()); err != nil {
		return err
	}

	go k.icsHandler(ctx, msg.ClusterID())

	slot.Msgs <- ktypes.ConsensusMessage{
		PeerID:  sender,
		Payload: msg,
	}

	return nil
}

func generateTesseract(
	cs *ktypes.ClusterState,
	poxt common.PoXtData,
	hs common.AccountStateHashes,
) (*common.Tesseract, error) {
	participants := participantStates(cs, hs)

	fuelUsed := cs.Transition.Receipts().FuelUsed()
	fuelLimit := uint64(1000)

	ixnsHash, err := cs.Ixns().Hash()
	if err != nil {
		return nil, err
	}

	receiptHash, err := cs.Transition.Receipts().Hash()
	if err != nil {
		return nil, err
	}

	fmt.Println("&&&&&&&&&&&&&&&&&&&&&&&", receiptHash)

	ts := common.NewTesseract(
		participants,
		ixnsHash,
		receiptHash,
		big.NewInt(0), // TODO pass appropriate value
		uint64(cs.ICSReqTime.Unix()),
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
			View:                      cs.CurrentView(),
		},
	)

	return ts, nil
}

func (k *Engine) executionInteractions(cs *ktypes.ClusterState) (common.AccountStateHashes, error) {
	transition, err := k.state.LoadTransitionObjects(cs.Ixns().Participants(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load state transition objects")
	}

	if err = k.updateContextDelta(cs); err != nil {
		return nil, err
	}

	stateHashes, err := k.exec.ExecuteInteractions(
		transition,
		cs.Ixns(),
		cs.ExecutionContext(),
	)
	if err != nil {
		return nil, err
	}

	// Based on the execution outcome, we update which accounts are participating in the ICS.
	// We do this to only lock the accounts for gas fee deduction
	cs.ExcludeParticipantsFromICS(stateHashes.ExcludedAccounts())

	voteset := ktypes.NewHeightVoteSet(
		make([]string, 0),
		cs.NewHeights(),
		cs,
		k.logger.With("cluster-id", cs.ClusterID),
	)

	cs.UpdateVoteSet(voteset)

	// store the transition
	cs.SetStateTransition(transition)

	return stateHashes, nil
}

func (k *Engine) createProposalTesseract(cs *ktypes.ClusterState) (*common.Tesseract, error) {
	stateHashes, err := k.executionInteractions(cs)
	if err != nil {
		return nil, err
	}

	/*
		seed, err := k.lottery.computeICSSeed(cs.Participants)
		if err != nil {
			return nil, err
		}

		newSeed, proof, err := k.lottery.computeVRFOutput(seed)
		if err != nil {
			return nil, err
		}
	*/

	lockInfo := cs.Participants.LockInfo(true)
	lastCommitHash := make(map[identifiers.Identifier]common.Hash)

	for id := range lockInfo {
		lastCommitHash[id] = cs.Participants[id].CommitHash
	}

	poxt := common.PoXtData{
		Proposer:     k.selfID,
		View:         k.view.Load(),
		LastCommit:   lastCommitHash,
		EvidenceHash: make(map[identifiers.Identifier]common.Hash),
		AccountLocks: cs.Participants.LockInfo(true),
		// ICSSeed:      newSeed,
		// ICSProof:     proof,
	}

	k.logger.Debug("Generating tesseracts", "cluster-id", cs.ClusterID)

	return generateTesseract(cs, poxt, stateHashes)
}

func (k *Engine) finalizedTesseractHandler(tesseract *common.Tesseract) error {
	var err error

	if tesseract == nil {
		return errors.New("failed to finalize tesseract")
	}

	clusterID := tesseract.ClusterID()

	slot := k.slots.GetSlot(clusterID)

	if slot == nil {
		return errors.New("nil slot")
	}

	cs := slot.ClusterState()

	if err = k.lattice.AddTesseractWithState(
		identifiers.Nil,
		cs.GetDirty(),
		tesseract,
		cs.Transition,
		true,
	); err != nil {
		return err
	}

	for id := range cs.Participants {
		if err := k.safety.DeleteSafetyData(id); err != nil {
			k.logger.Error("Failed to delete safety data", "err", err, "id", id)
		}
	}

	ixnHashes := make(common.Hashes, 0, tesseract.Interactions().Len())

	for _, ixn := range tesseract.Interactions().IxList() {
		ixnHashes = append(ixnHashes, ixn.Hash())
	}

	msg := &networkmsg.TesseractMsg{
		RawTesseract: make([]byte, 0),
		IxnsHashes:   ixnHashes,
		CommitInfo:   tesseract.CommitInfo(),
		Extra:        make(map[string][]byte),
	}

	for key, value := range cs.GetDirty() {
		msg.Extra[key.String()] = value
	}

	msg.RawTesseract, err = tesseract.Bytes()
	if err != nil {
		return err
	}

	if err = msg.CompressTesseract(k.compressor); err != nil {
		return err
	}

	// only operator broadcasts the tesseract and other cluster nodes broadcast it if the tesseract isn't received
	//  from the operator before expiry time.
	if tesseract.Operator() == k.selfID {
		if err = k.transport.BroadcastTesseract(msg); err != nil {
			k.logger.Error("Failed to broadcast tesseract", "err", err, "cluster-id", clusterID)
		}
	} else {
		if err = k.mux.Post(utils.TSTrackerEvent{
			TSHash:     tesseract.Hash(),
			Msg:        msg,
			ExpiryTime: time.Now().Add(300 * time.Millisecond),
		}); err != nil {
			k.logger.Error("Error posting tesseract tracker event", "ts-hash", tesseract.Hash())
		}
	}

	for _, id := range tesseract.AccountIDs() {
		k.state.RefreshCachedObject(id, cs.Transition.GetSystemObject())
	}

	return nil
}

func (k *Engine) validateInteractions(ixs common.Interactions) error {
	for _, ix := range ixs.IxList() {
		ixHash := ix.Hash()

		k.logger.Debug(
			"Validating interaction",
			"ix-hash", ixHash,
			"sequence-id", ix.SequenceID(),
			"from", ix.SenderID().Hex(),
		)
		/*
			Checks to perform
			1) Verify sequenceID
			2) Verify balances
			3) Verify the account states
		*/
		latestSequenceID, err := k.state.GetSequenceID(ix.SenderID(), ix.SenderKeyID(), common.NilHash)
		if err != nil {
			return err
		}

		// validate sequenceID
		if ix.SequenceID() < latestSequenceID {
			return common.ErrInvalidSequenceID
		}

		if err = k.isIxValid(ix); err != nil {
			return err
		}
	}

	return nil
}

// isIxValid performs validity checks based on the type of interaction
func (k *Engine) isIxValid(ix *common.Interaction) error {
	if ix.SenderID().IsNil() {
		return common.ErrInvalidIdentifier
	}

	if accountRegistered, err := k.state.IsAccountRegistered(ix.SenderID()); err != nil || !accountRegistered {
		return common.ErrAccountNotFound
	}

	senderObject, err := k.state.GetLatestStateObject(ix.SenderID())
	if err != nil {
		return err
	}

	fuelAvailable, err := senderObject.HasSufficientFuel(ix.Cost())
	if err != nil {
		k.logger.Error("Failed to fetch fuel", "err", err)
	}

	if !fuelAvailable {
		return common.ErrInsufficientFunds
	}

	return nil
}

func (k *Engine) verifyTransitions(
	id identifiers.Identifier,
	ts *common.Tesseract,
	allParticipants bool,
) error {
	if ts.ConsensusInfo().View == common.GenesisView {
		return nil
	}

	ids := make([]identifiers.Identifier, 0)

	if allParticipants {
		ids = ts.AccountIDs()
	} else {
		ids = append(ids, id)
	}

	for _, id := range ids {
		initial, err := k.state.IsInitialTesseract(ts, id)
		if err != nil {
			return errors.Wrap(err, "Sarga account not found")
		}

		if initial {
			continue
		}

		if ts.StateHash(id).IsNil() {
			continue
		}

		lockType, ok := ts.ConsensusInfo().AccountLocks[id]
		if ok && lockType > common.MutateLock {
			continue
		}

		parent, err := k.lattice.GetTesseract(ts.TransitiveLink(id), false, false)
		if err != nil {
			k.logger.Error("Failed to fetch parent tesseract", "err", err, "id", id)

			return common.ErrPreviousTesseractNotFound
		}

		// Check Heights
		if parent.Height(id) != ts.Height(id)-1 {
			return common.ErrInvalidHeight
		}
		// TODO: Add more checks
		// Check time stamp
		if ts.Timestamp() < parent.Timestamp() {
			return common.ErrInvalidBlockTime
		}
	}

	return nil
}

func (k *Engine) verifyQc(
	view uint64,
	ics *ktypes.ICSCommittee,
	qc *common.Qc,
) (bool, error) {
	k.logger.Debug("verifying QC", "view", view, "qc", qc)

	var (
		verificationInitTime = time.Now()
		publicKeys           = make([][]byte, 0, qc.SignerIndices.TrueIndicesSize())
		votesCounter         = make([]uint32, ics.Size())
	)

	for _, valIndex := range qc.SignerIndices.GetTrueIndices() {
		nodeSetIndices, _, _, publicKey := ics.GetKramaID(int32(valIndex))
		if nodeSetIndices != nil { // ts.Header.Extra.VoteSet.GetIndex(index)
			publicKeys = append(publicKeys, publicKey)

			for _, index := range nodeSetIndices {
				votesCounter[index]++
			}
		} else {
			k.logger.Debug("Error fetching validator id", "index", valIndex)
		}
	}

	for index := 0; index < ics.Size()-1; index++ {
		if ics.Sets[index].ExcludedFromICS {
			continue
		}

		if votesCounter[index] < ics.ParticipantQuorum(index) {
			return false, common.ErrContextQuorumFailed
		}
	}

	if votesCounter[ics.StochasticSetPosition()] < ics.RandomQuorumSize() {
		return false, common.ErrRandomQuorumFailed
	}

	vote := ktypes.CanonicalVote{
		Type:   qc.Type,
		View:   view,
		TSHash: qc.TSHash,
	}

	rawData, err := vote.Bytes()
	if err != nil {
		return false, err
	}

	verified, err := k.signatureVerifier(rawData, qc.Signature, publicKeys)
	if err != nil {
		return false, err
	}

	k.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (k *Engine) verifySignatures(ts *common.Tesseract, ics *ktypes.ICSCommittee) (bool, error) {
	return k.verifyQc(
		ts.ConsensusInfo().View,
		ics,
		ts.CommitInfo().QC,
	)
}

func (k *Engine) ValidateTesseract(
	id identifiers.Identifier,
	ts *common.Tesseract,
	ics *ktypes.ICSCommittee,
	allParticipants bool,
) error {
	if !allParticipants && k.db.HasAccMetaInfoAt(id, ts.Height(id)) {
		return common.ErrAlreadyKnown
	}

	validSeal, err := k.state.IsSealValid(ts)
	if !validSeal {
		k.logger.Error("Error validating tesseract seal", "err", err)

		return common.ErrInvalidSeal
	}

	if err = k.verifyTransitions(id, ts, allParticipants); err != nil {
		return err
	}

	verified, err := k.verifySignatures(ts, ics)
	if !verified || err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to verify signatures %v %v", id, ts.Height(id)))
	}

	return nil
}

func (k *Engine) ExecuteAndValidate(
	ts *common.Tesseract,
	transition *state.Transition,
) error {
	k.logger.Debug(
		"Executing interactions of grid",
		"ts-hash", ts.Hash(),
		"lock", ts.LockedContext(),
	)

	stateHashes, err := k.exec.ExecuteInteractions(
		transition,
		ts.Interactions(),
		ts.ExecutionContext(),
	)
	if err != nil {
		return err
	}

	if !isReceiptsHashValid(ts, transition.Receipts()) || !areStateHashesValid(ts, stateHashes) {
		return errors.New("failed to validate the tesseract")
	}

	ts.SetReceipts(transition.Receipts())

	return nil
}

func (k *Engine) AddActiveAccounts(
	lockType common.LockType,
	clusterID common.ClusterID,
	ids ...identifiers.Identifier,
) bool {
	return k.slots.AddActiveAccounts(lockType, clusterID, ids...)
}

func (k *Engine) ClearActiveAccounts(clusterID common.ClusterID, ids ...identifiers.Identifier) {
	k.slots.ClearActiveAccounts(clusterID, ids...)

	k.logger.Trace("removed from active accounts", "ids", ids)
}

/*
func (k *Engine) isTimely(reqTime time.Time, currentTime time.Time) bool {
	lowerBound := reqTime.Add(-k.cfg.Precision)
	upperBound := reqTime.Add(k.cfg.MessageDelay).Add(k.cfg.Precision)

	if currentTime.Before(lowerBound) || currentTime.After(upperBound) {
		return false
	}

	return true
}

// verifyOperatorLottery validates the ICS proof provided by an operator with the computed ICS seed.
func (k *Engine) verifyOperatorLottery(
	operator identifiers.KramaID,
	lk common.LotteryKey,
	vrfOutput [32]byte,
	vrfProof []byte,
) error {
	priority, err := k.lottery.VerifySelection(operator, lk.Seed(), vrfOutput, vrfProof)
	if err != nil {
		return err
	}

	k.lottery.AddICSOperatorInfo(lk, operator, priority)

	if ok := k.lottery.IsEligible(lk, operator); !ok {
		for _, v := range k.lottery.GetEligibleOperators(lk) {
			k.logger.Debug("Eligible Proposer", "lottery-key", lk, "krama-id", v.KramaID)
		}

		return common.ErrOperatorNotEligible
	}

	return nil
}


func (k *Engine) runLottery(key common.LotteryKey) ([32]byte, []byte, error) {
	if info, ok := k.lottery.cache.Get(key); ok {
		if sortitionResult, ok := info.(*LotteryResult); ok {
			if sortitionResult.isSelected {
				return sortitionResult.vrfOutput, sortitionResult.vrfProof, nil
			}

			return [32]byte{}, []byte{}, ErrOperatorNotEligible
		}
	}

	icsOutput, icsProof, err := k.lottery.computeVRFOutput(key.Seed())
	if err != nil {
		return [32]byte{}, nil, err
	}

	operatorIncentive, err := k.state.GetGuardianIncentives(k.selfID)
	if err != nil {
		return [32]byte{}, nil, err
	}

	priority, err := k.lottery.Select(operatorIncentive, icsOutput)
	if err != nil {
		k.lottery.cache.Add(key, NewLotteryResult(false, [32]byte{}, []byte{}))

		return [32]byte{}, nil, err
	}

	k.lottery.cache.Add(key, NewLotteryResult(true, icsOutput, icsProof))

	k.lottery.AddICSOperatorInfo(key, k.selfID, priority)

	if ok := k.lottery.IsEligible(key, k.selfID); !ok {
		return [32]byte{}, nil, common.ErrOperatorNotEligible
	}

	return icsOutput, icsProof, nil
}
*/

func (k *Engine) getTS(tsHash common.Hash, kramaID identifiers.KramaID) (*common.Tesseract, error) {
	ts, err := k.lattice.GetTesseract(tsHash, false, true)
	if err == nil {
		return ts, nil
	}

	ts, err = k.safety.GetTesseract(tsHash)
	if err == nil {
		return ts, nil
	}

	// TODO: Should fetch ts from the network using agora

	return nil, err
}

// tsEventTracker adds a received event to the map if its pair doesn't exist
// and removes it if its pair already exists.
// It wakes up once every 300ms, checks if the event has expired, and removes it from the map if it has.
// If the event has the tesseract, it will broadcast as the operator's tesseract didn't reach us.
func (k *Engine) tsEventTracker(eventSub *utils.Subscription) {
	for {
		select {
		case <-k.ctx.Done():
			return

		case <-time.After(300 * time.Millisecond):
			now := time.Now()
			for _, event := range k.tsTracker {
				if now.After(event.ExpiryTime) {
					if event.Msg != nil {
						if err := k.transport.BroadcastTesseract(event.Msg); err != nil {
							k.logger.Error("Error broadcasting tesseract", "ts-hash", event.TSHash, "error", err)
						}
					}

					delete(k.tsTracker, event.TSHash)
				}
			}

		case msg, ok := <-eventSub.Chan():
			if ok {
				event, ok := msg.Data.(utils.TSTrackerEvent)
				if !ok {
					log.Println("Error casting event data to TSTrackerEvent")

					continue
				}

				if _, ok := k.tsTracker[event.TSHash]; ok {
					delete(k.tsTracker, event.TSHash)

					continue
				}

				k.tsTracker[event.TSHash] = &event
			}
		}
	}
}

func (k *Engine) closeICS(clusterID common.ClusterID) {
	k.icsCloseCh <- clusterID
}

func (k *Engine) Start() {
	eventSub := k.mux.Subscribe(utils.TSTrackerEvent{})
	go k.tsEventTracker(eventSub)

	go k.minter()

	go k.handler()

	go k.transport.Start()
}

func (k *Engine) Close() {
	// TODO: uncomment after WAL fix
	// k.wal.Close()
	k.transport.Close()

	defer k.ctxCancel()
}

func areStateHashesValid(ts *common.Tesseract, postExecState common.AccountStateHashes) bool {
	for id, participantState := range ts.Participants() {
		if postExecState.StateHash(id) != participantState.StateHash {
			return false
		}
	}

	return true
}

func isReceiptsHashValid(ts *common.Tesseract, receipts common.Receipts) bool {
	receiptsHash, err := receipts.Hash()
	if err != nil {
		return false
	}

	if ts.ReceiptsHash() != receiptsHash {
		fmt.Println("Receipts hash mismatch", "ts-receipts-hash", ts.ReceiptsHash(), "post-exec-receipts-hash", receiptsHash)
		fmt.Println("Post exec receipts", common.PrintReceipts(receipts))
		fmt.Println("Pre exec receipts", common.PrintReceipts(ts.Receipts()))

		return false
	}

	return true
}

func participantStates(cs *ktypes.ClusterState, ps common.AccountStateHashes) common.ParticipantsState {
	participants := make(common.ParticipantsState, len(cs.Participants))

	for id, p := range cs.Participants {
		participants[id] = common.State{
			Height:         p.NewHeight(),
			TransitiveLink: p.TSHash(),
			LockedContext:  p.ContextHash,
			ContextDelta:   p.ContextDelta,
			StateHash:      ps.StateHash(id),
		}
	}

	return participants
}
