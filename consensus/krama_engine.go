package consensus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"time"

	"github.com/sarvalabs/go-moi/compute"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/consensus/observer"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"

	"github.com/hashicorp/go-hclog"
	"github.com/moby/locker"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/flux"
	"github.com/sarvalabs/go-moi/ixpool"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/telemetry/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	ObserverCoeff float64 = 0.1
	// MinMTQ represents the minimum Modulated Trust Quotient
	MinMTQ float64 = 0.3

	// MaxICSSize represents the maximum size of the cluster
	MaxICSSize float64 = 10

	ObserverNodesDelta float64 = 0.2
	RandomNodesDelta   float64 = 0.3

	ICSTimeOutDuration = 3500 * time.Millisecond

	BehaviouralContextSize = 1
	RandomContextSize      = 1
)

type KramaEngine interface {
	Requests() chan ktypes.Request
	Logger() hclog.Logger
}

type lattice interface {
	AddTesseractWithState(
		addr identifiers.Address,
		dirtyStorage map[common.Hash][]byte,
		ts *common.Tesseract,
		transition *state.Transition,
		allParticipants bool,
	) error
	AddTesseract(
		cache bool,
		addr identifiers.Address,
		t *common.Tesseract,
		transition *state.Transition,
		allParticipants bool,
	) error
	GetTesseract(
		hash common.Hash,
		withInteractions bool,
	) (*common.Tesseract, error)
}

type kramaTransport interface {
	Start()
	Close()
	Messages() <-chan *ktypes.ICSMSG
	CleanDirectPeer(clusterID common.ClusterID, peers ...kramaid.KramaID)
	RegisterContextRouter(
		ctx context.Context,
		operator kramaid.KramaID,
		clusterID common.ClusterID,
		nodeset *common.ICSNodeSet,
		voteset *kbft.HeightVoteSet,
	)
	ConnectToDirectPeer(ctx context.Context, kramaID kramaid.KramaID, clusterID common.ClusterID) error
	InitClusterConnection(ctx context.Context, clusterID common.ClusterID)
	BroadcastTesseract(msg *networkmsg.TesseractMsg) error
	BroadcastMessage(
		ctx context.Context,
		msg *ktypes.ICSMSG,
	)
	GracefullyCloseContextRouter(clusterID common.ClusterID)
	SendMessage(
		peerID, sender kramaid.KramaID,
		clusterID common.ClusterID,
		msgType networkmsg.MsgType,
		rawMsg ktypes.ICSPayload,
	) error
	GetRoundVoteSetBits(clusterID common.ClusterID) (map[int32]*ktypes.VoteBitSet, error)
	StartGossip(clusterID common.ClusterID)
}

type stateManager interface {
	GetICSParticipants(ixns common.Interactions) (map[identifiers.Address]common.IxParticipant, error)
	LoadTransitionObjects(ps map[identifiers.Address]common.IxParticipant) (*state.Transition, error)
	CreateStateObject(identifiers.Address, common.AccountType, bool) *state.Object
	FetchLatestParticipantContext(addr identifiers.Address) (
		latestContextHash common.Hash,
		behaviouralSet, randomSet *common.NodeSet,
		err error,
	)
	GetPublicKeys(context context.Context, ids ...kramaid.KramaID) (keys [][]byte, err error)
	GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error)
	IsAccountRegistered(addr identifiers.Address) (bool, error)
	GetLatestStateObject(addr identifiers.Address) (*state.Object, error)
	GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error)
	IsInitialTesseract(ts *common.Tesseract, addr identifiers.Address) (bool, error)
	IsSealValid(ts *common.Tesseract) (bool, error)
	RemoveCachedObject(addr identifiers.Address)
}

type ixPool interface {
	IncrementWaitTime(addr identifiers.Address, baseTime time.Duration) error
	Executables() ixpool.InteractionQueue
	Drop(ix *common.Interaction)
}

type execution interface {
	ExecuteInteractions(*state.Transition, common.Interactions, *common.ExecutionContext) (common.AccStateHashes, error)
}

type store interface {
	HasAccMetaInfoAt(addr identifiers.Address, height uint64) bool
}

type AggregatedSignatureVerifier func(data []byte, aggSignature []byte, multiplePubKeys [][]byte) (bool, error)

type Engine struct {
	ctx               context.Context
	ctxCancel         context.CancelFunc
	cfg               *config.ConsensusConfig
	mux               *utils.TypeMux
	logger            hclog.Logger
	selfID            kramaid.KramaID
	slots             *ktypes.Slots
	requests          chan ktypes.Request
	randomizer        *flux.Randomizer
	db                store
	transport         kramaTransport
	exec              execution
	pool              ixPool
	state             stateManager
	executionReq      chan common.ClusterID
	lattice           lattice
	wal               kbft.WAL
	vault             *crypto.KramaVault
	clusterLocks      *locker.Locker
	metrics           *Metrics
	avgICSTime        time.Duration
	slotCloseCh       chan common.ClusterID
	signatureVerifier AggregatedSignatureVerifier
}

func NewKramaEngine(
	db store,
	cfg *config.ConsensusConfig,
	logger hclog.Logger,
	mux *utils.TypeMux,
	selfID kramaid.KramaID,
	state stateManager,
	exec execution,
	ixPool ixPool,
	val *crypto.KramaVault,
	lattice lattice,
	randomizer *flux.Randomizer,
	transport kramaTransport,
	metrics *Metrics,
	slots *ktypes.Slots,
	verifier AggregatedSignatureVerifier,
) (*Engine, error) {
	wal, err := kbft.NewWAL(logger, cfg.DirectoryPath)
	if err != nil {
		return nil, errors.Wrap(err, "WAL failed")
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	k := &Engine{
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		cfg:               cfg,
		logger:            logger.Named("Krama-Engine"),
		mux:               mux,
		selfID:            selfID,
		state:             state,
		slots:             slots,
		requests:          make(chan ktypes.Request),
		randomizer:        randomizer,
		db:                db,
		transport:         transport,
		exec:              exec,
		pool:              ixPool,
		lattice:           lattice,
		executionReq:      make(chan common.ClusterID),
		wal:               wal,
		vault:             val,
		clusterLocks:      locker.New(),
		metrics:           metrics,
		avgICSTime:        cfg.AccountWaitTime,
		slotCloseCh:       make(chan common.ClusterID),
		signatureVerifier: verifier,
	}

	k.metrics.initMetrics(float64(cfg.OperatorSlotCount), float64(cfg.ValidatorSlotCount))

	return k, nil
}

// loadIxnClusterState fetches the account state and returns the interaction cluster state
func (k *Engine) loadIxnClusterState(
	ctx context.Context,
	req ktypes.Request,
	clusterID common.ClusterID,
	ixnParticipants map[identifiers.Address]common.IxParticipant,
) (*ktypes.ClusterState, error) {
	var err error

	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "loadIxnClusterState")
	defer span.End()

	participants, nodeSet, err := k.fetchParticipantsAndNodeSet(ctx, ixnParticipants)
	if err != nil {
		return nil, err
	}

	clusterState := ktypes.NewICS(
		req.Msg,
		req.Ixs,
		clusterID,
		req.Operator,
		req.ReqTime,
		k.selfID,
		participants,
		nodeSet,
	)

	return clusterState, nil
}

func (k *Engine) acquireContextLock(ctx context.Context, slot *ktypes.Slot) error {
	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "acquireContextLock")
	defer span.End()
	// Create cluster id using operatorID and IxHash
	k.logger.Debug("Creating cluster", "cluster-ID", slot.ClusterID())

	var (
		operatorRandomNodes []kramaid.KramaID
		observerNodes       []kramaid.KramaID
		err                 error
	)

	contextNodes, contextNodesSize, isOperatorIncluded := getDistinctNodes(k.selfID, slot.ClusterState().NodeSet.Sets)

	// Calculate the required number of observer nodes in the cluster based on observer coefficient
	// value and the actual size of the cluster including the required observer
	requiredObserverNodes := int(math.Ceil(ObserverCoeff * float64(contextNodesSize)))
	observerNodesQueryCount := requiredObserverNodes + k.observerNodeDelta(contextNodesSize)

	actualICSSize := 3*contextNodesSize + requiredObserverNodes
	// Choose the higher value between the user MTQ and the minimum network MTQ and use
	// that Modulated Trust Quotient to calculate the minimum required cluster size
	mtqSize := math.Max(MinMTQ, 0.5) // get this from interaction
	requiredICSSize := int(math.Ceil(mtqSize * MaxICSSize))

	k.logger.Info("Actual cluster size info", "size", actualICSSize)
	k.logger.Info("Required cluster size info", "size", requiredICSSize)
	// Determine the number of required random nodes in the cluster based on
	// the size of the responded eligible set and the number of additional nodes
	// required to satisfy the cluster size requirement
	additionalRandomNodes := 1
	if requiredICSSize > actualICSSize {
		additionalRandomNodes = requiredICSSize - actualICSSize
	}

	totalRandomNodes := 2*contextNodesSize + additionalRandomNodes

	operatorRandomNodesCount := totalRandomNodes
	operatorRandomNodesQueryCount := operatorRandomNodesCount + k.randomNodeDelta(totalRandomNodes)

	operatorRandomNodes, err = k.getRandomNodes(ctx, operatorRandomNodesQueryCount, contextNodes)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve random nodes")
	}

	if !isOperatorIncluded {
		for _, kramaID := range operatorRandomNodes {
			if kramaID == k.selfID {
				isOperatorIncluded = true
			}
		}
	}

	if !isOperatorIncluded {
		operatorRandomNodes = append([]kramaid.KramaID{k.selfID}, operatorRandomNodes...) // TODO:Improve this
	}

	exemptedNodes := contextNodes
	exemptedNodes = append(exemptedNodes, operatorRandomNodes...)

	observerNodes, err = k.getObserverNodes(ctx, observerNodesQueryCount, exemptedNodes)
	if err != nil {
		k.logger.Error("Error fetching observer nodes", "err", err)

		return errors.New("unable to retrieve observer nodes")
	}

	randomKeys, err := k.state.GetPublicKeys(context.Background(), operatorRandomNodes...)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch the public key of random nodes.")
	}

	observerKeys, err := k.state.GetPublicKeys(context.Background(), observerNodes...)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch the public key of observer nodes.")
	}

	slot.ClusterState().UpdateNodeSet(
		slot.ClusterState().NodeSet.RandomSetPosition(),
		common.NewNodeSet(operatorRandomNodes, randomKeys, uint32(operatorRandomNodesCount)),
	)
	slot.ClusterState().UpdateNodeSet(
		slot.ClusterState().NodeSet.ObserverSetPosition(),
		common.NewNodeSet(observerNodes, observerKeys, uint32(requiredObserverNodes)),
	)

	slot.ClusterState().ICSReqTime = utils.Now()
	// Construct ICS_Request
	reqMsg, err := k.getCanonicalICSReqMsg(slot.ClusterState())
	if err != nil {
		return err
	}

	failedReqCount, err := k.sendICSRequest(ctx, reqMsg, slot.ClusterState().NodeSet)
	if err != nil {
		return err
	}

	slot.ClusterState().IncrementICSRespCount(failedReqCount)

	nodes := slot.ClusterState().NodeSet.GetNodes(false)

	utils.RetryUntilTimeout(2000*time.Millisecond, 10*time.Millisecond, func() error {
		// Verify if all nodes have responded
		if slot.ClusterState().GetICSRespCount() < len(nodes)-1 {
			return errors.New("insufficient response count")
		}

		return nil
	})

	k.logger.Info("::::::::Res Count:::::::", slot.ClusterState().GetICSRespCount(), len(nodes))

	if !slot.ClusterState().IsContextQuorum() {
		return errors.New("context quorum failed")
	}

	if !slot.ClusterState().IsRandomQuorum(operatorRandomNodesCount) {
		return errors.New("random quorum failed")
	}

	if !slot.ClusterState().IsObserverQuorum(requiredObserverNodes) {
		return errors.New("observer quorum failed")
	}

	return nil
}

func (k *Engine) signICSRequest(msg ktypes.CanonicalICSRequest) (*ktypes.ICSRequest, error) {
	rawCanonicalICSReq, err := msg.Bytes()
	if err != nil {
		k.logger.Error("Failed to send ICS request", "err", err)

		return nil, err
	}

	signature, err := k.vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	if err != nil {
		k.logger.Error("Failed to sign ICS request", "err", err)

		return nil, err
	}

	return ktypes.NewICSRequest(rawCanonicalICSReq, signature), nil
}

func (k *Engine) verifyICSRequest(
	operator kramaid.KramaID,
	icsReq *ktypes.ICSRequest,
	interactions *common.Interactions,
) error {
	if err := crypto.VerifySignatureUsingKramaID(operator, icsReq.ReqData, icsReq.Signature); err != nil {
		k.logger.Error("Failed to verify ICS request signature", "err", err)

		return err
	}

	for _, ix := range *interactions {
		rawPayload, err := ix.PayloadForSignature()
		if err != nil {
			return err
		}

		isVerified, err := crypto.Verify(rawPayload, ix.Signature(), ix.Sender().Bytes())
		if !isVerified || err != nil {
			return err
		}
	}

	return nil
}

func (k *Engine) randomNodeDelta(setSize int) int {
	return int(math.Ceil(RandomNodesDelta * float64(setSize)))
}

func (k *Engine) observerNodeDelta(setSize int) int {
	return int(math.Ceil(ObserverNodesDelta * float64(setSize)))
}

func (k *Engine) Start() {
	go k.minter()

	go k.executionRoutine()

	go k.handler()

	go func() {
		for {
			select {
			case <-k.ctx.Done():
				k.logger.Info("Closing Krama engine. Reason: context-closed")

				return
			case req := <-k.requests:
				go k.handleReq(req)
			}
		}
	}()

	go k.transport.Start()
}

func (k *Engine) isTimely(reqTime time.Time, currentTime time.Time) bool {
	lowerBound := reqTime.Add(-k.cfg.Precision)
	upperBound := reqTime.Add(k.cfg.MessageDelay).Add(k.cfg.Precision)

	if currentTime.Before(lowerBound) || currentTime.After(upperBound) {
		return false
	}

	return true
}

func (k *Engine) joinCluster(ctx context.Context, slot *ktypes.Slot) error {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "joinCluster")
	defer span.End()

	k.logger.Debug(
		"Received an ICS join request",
		"from", slot.ClusterState().Operator,
		"timestamp", slot.ClusterState().RequestMsg.Timestamp,
		"cluster-ID", slot.ClusterID(),
		"ix-hash", slot.ClusterState().Ixs[0].Hash(),
	)

	reqMsg := slot.ClusterState().RequestMsg

	reqTime := utils.Canonical(time.Unix(0, slot.ICSRequestMsg().Timestamp))

	if !k.isTimely(reqTime, utils.Now()) {
		return errors.New("invalid time stamp")
	}

	slot.ClusterState().IsObserver = utils.ContainsKramaID(slot.ICSRequestMsg().ObserverSet, k.selfID)

	// Check whether the context hashes matches
	for addr, info := range slot.ICSRequestMsg().ContextLock {
		if slot.ClusterState().ParticipantHeight(addr) < info.Height {
			if err := k.mux.Post(utils.SyncRequestEvent{
				Address:  addr,
				Height:   info.Height,
				BestPeer: slot.ClusterState().Operator,
			}); err != nil {
				k.logger.Error("Failed to post sync request", "err", err)
			}
		}

		if slot.ClusterState().ParticipantHeight(addr) != info.Height {
			return common.ErrHeightMismatch
		}

		if info.TesseractHash != slot.ClusterState().ParticipantTSHash(addr) {
			return common.ErrHashMismatch
		}
	}

	observerPublicKeys, err := k.state.GetPublicKeys(context.Background(), reqMsg.ObserverSet...)
	if err != nil {
		return errors.New("failed to retrieve public keys")
	}

	randomPublicKeys, err := k.state.GetPublicKeys(context.Background(), reqMsg.RandomSet...)
	if err != nil {
		return errors.New("failed to retrieve public keys")
	}

	slot.ClusterState().UpdateNodeSet(
		slot.ClusterState().NodeSet.ObserverSetPosition(),
		common.NewNodeSet(slot.ClusterState().RequestMsg.ObserverSet, observerPublicKeys, reqMsg.RequiredObserverSetSize))

	slot.ClusterState().UpdateNodeSet(
		slot.ClusterState().NodeSet.RandomSetPosition(),
		common.NewNodeSet(slot.ClusterState().RequestMsg.RandomSet, randomPublicKeys, reqMsg.RequiredRandomSetSize))

	k.logger.Debug("Responding to ICS request", "from", slot.ClusterState().Operator)

	return nil
}

func (k *Engine) handleReq(req ktypes.Request) {
	clusterID, err := req.GetClusterID()
	if err != nil {
		sendResponse(req, errors.New("failed to decode clusterID"))

		return
	}

	ctx, span := tracing.Span(
		k.ctx,
		"Krama.KramaEngine",
		"handleReq",
		trace.WithAttributes(
			attribute.String("clusterID", clusterID.String()),
			attribute.Int("slotType", int(req.SlotType)),
		),
	)
	defer span.End()

	/*
		1: Check slot availability
		2: Load ICS participants info and create a slot
		3: Load cluster state
		4: Validate Ixns
		5: Create New ICS or Join
		6: Start agreement mechanism
	*/

	if slot := k.slots.GetSlot(clusterID); slot != nil {
		sendResponse(req, nil)

		return
	}

	ps, err := k.state.GetICSParticipants(req.Ixs)
	if err != nil {
		sendResponse(req, err)
	}

	slot := k.slots.CreateSlot(clusterID, req, ps)
	if slot == nil {
		sendResponse(req, common.ErrSlotsFull)

		return
	}

	ctx, cancelFn := context.WithCancel(ctx)
	defer func() {
		// delete the slot from the slots queue
		cancelFn()

		slot := k.slots.GetSlot(clusterID)

		if slot != nil {
			if slot.SlotType == ktypes.OperatorSlot {
				k.metrics.captureAvailableOperatorSlots(1)
			} else {
				k.metrics.captureAvailableValidatorSlots(1)
			}
		}

		k.slotCloseCh <- clusterID
	}()

	if req.SlotType == ktypes.OperatorSlot {
		k.metrics.captureAvailableOperatorSlots(-1)
	} else {
		k.metrics.captureAvailableValidatorSlots(-1)
	}

	cs, err := k.loadIxnClusterState(ctx, req, clusterID, ps)
	if err != nil {
		sendResponse(req, err)

		return
	}

	slot.UpdateClusterState(cs)

	if err = k.validateInteractions(req.Ixs); err != nil {
		k.logger.Error("Invalid interaction", "err", err)

		sendResponse(req, common.ErrInvalidInteractions)

		return
	}

	voteset := kbft.NewHeightVoteSet(
		make([]string, 0),
		cs.NewHeights(),
		cs,
		k.logger.With("cluster-ID", clusterID),
	)

	k.transport.RegisterContextRouter(ctx, cs.Operator, clusterID, cs.NodeSet, voteset)

	defer k.transport.GracefullyCloseContextRouter(clusterID)

	switch req.SlotType {
	case ktypes.OperatorSlot:
		err = k.acquireContextLock(ctx, slot)

		sendResponse(req, err)

		k.metrics.captureICSCreationTime(k.slots.GetSlot(clusterID).ClusterState().ICSReqTime)

		if err != nil {
			k.logger.Error("Error acquiring context lock", "err", err, "cluster-ID", clusterID)
			k.metrics.captureICSCreationFailureCount(1)

			if err = k.sendICSFailure(ctx, clusterID); err != nil {
				k.logger.Error("Failed to send ics failure message", "err", err)
			}

			return
		}

		k.transport.InitClusterConnection(ctx, clusterID)

		if err = k.sendICSSuccess(ctx, clusterID); err != nil {
			k.logger.Error("Failed to send ics success message", err)

			return
		}

		k.logger.Info("Cluster creation successful", "cluster-ID", clusterID)

	case ktypes.ValidatorSlot:
		requestTime := time.Now()

		err = k.joinCluster(ctx, slot)

		sendResponse(req, err)

		if err != nil {
			k.logger.Info("Error joining cluster", "err", err, "cluster-ID", clusterID)
			k.metrics.captureICSParticipationFailureCount(1)

			return
		}

		slot := k.slots.GetSlot(clusterID)
		if slot == nil {
			k.logger.Error("Slot not found")

			return
		}

		k.transport.InitClusterConnection(ctx, slot.ClusterID())

		select {
		case ok := <-slot.ICSSuccessChan:
			if !ok {
				k.metrics.captureICSParticipationFailureCount(1)

				return
			}

			k.metrics.captureICSJoiningTime(requestTime)
		case <-time.After(ICSTimeOutDuration):
			k.logger.Info("ICS success timeout", "cluster-ID", req.Msg.ClusterID)
			k.metrics.captureICSParticipationFailureCount(1)

			return
		}
	}

	// Send execution request
	go k.initOutboundMessageHandler(ctx, slot)

	if cs.IsObserver {
		wg := observer.NewWatchDog(ctx, slot)
		wg.StartWatchDog()

		if hash := cs.ClusterID.Hash(); !hash.IsNil() {
			proofs, err := wg.GenerateProofs()
			if err != nil {
				k.logger.Error("Failed to generate watchdog proofs", "err", err)

				return
			}

			cs.AddDirty(hash, proofs)

			return
		}

		k.logger.Error("Failed to store watchdog proofs")
	} else {
		k.logger.Trace("Sending execution request")

		executionReqTS := time.Now()
		k.executionReq <- clusterID

		// Wait for execution response
		execResp := <-slot.ExecutionResp
		if execResp.Err != nil {
			k.logger.Error("Error executing interactions", "err", execResp.Err, "cluster-ID", clusterID)
			k.metrics.captureAgreementFailureCount(1)

			for _, interaction := range cs.Ixs {
				k.pool.Drop(interaction)
			}

			return
		}

		k.logger.Trace("Execution finished")
		k.metrics.captureGridGenerationTime(executionReqTS)
		cs.SetTesseract(execResp.Tesseract)

		consensusInitTS := time.Now()
		k.metrics.captureClusterSize(float64(cs.Size()))

		ixHash, err := cs.Ixs.Hash()
		if err != nil {
			return
		}

		icsEvidence := kbft.NewEvidence(ixHash, cs.Operator, cs.Size())

		voteset.Reset(cs.NewHeights(), cs)

		go k.transport.StartGossip(clusterID)

		bft := kbft.NewKBFTService(
			ctx,
			kbft.MaxBFTimeout,
			k.selfID,
			k.cfg,
			slot.BftOutboundChan,
			slot.BftInboundChan,
			k.vault,
			cs,
			voteset,
			k.finalizedTesseractHandler,
			kbft.WithLogger(k.logger.With("cluster-ID", clusterID)),
			kbft.WithWal(k.wal),
			kbft.WithEvidence(icsEvidence),
		)

		if err = bft.Start(); err != nil {
			k.logger.Error("Consensus failed", "err", err, "cluster-ID", cs.ClusterID)
			k.metrics.captureAgreementFailureCount(1)

			return
		}

		k.metrics.captureAgreementTime(consensusInitTS)
	}

	k.logger.Info("Interaction finalized", "cluster-ID", cs.ClusterID)
}

func (k *Engine) fetchParticipantsAndNodeSet(
	ctx context.Context,
	ps map[identifiers.Address]common.IxParticipant,
) (
	map[identifiers.Address]*common.Participant,
	*common.ICSNodeSet,
	error,
) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "fetchIxAccounts")
	defer span.End()

	participants := make(map[identifiers.Address]*common.Participant)
	addrs := make(common.Addresses, 0, len(ps))

	nodeSet := common.NewICSNodeSet(2*len(ps) + 2)

	for addr, info := range ps {
		if _, ok := participants[addr]; ok {
			continue
		}

		addrs = append(addrs, addr)

		if !info.IsGenesis {
			accInfo, err := k.state.GetAccountMetaInfo(addr)
			if err != nil {
				return nil, nil, err
			}

			participants[addr] = &common.Participant{
				AccType:       accInfo.Type,
				Address:       addr,
				IsGenesis:     info.IsGenesis,
				Height:        accInfo.Height,
				TesseractHash: accInfo.TesseractHash,
				LockType:      info.LockType,
				IsSigner:      info.IsSigner,
			}

			continue
		}

		participants[addr] = &common.Participant{
			AccType:       info.AccType,
			Address:       addr,
			IsGenesis:     info.IsGenesis,
			Height:        0,
			TesseractHash: common.NilHash,
			LockType:      info.LockType,
			ContextHash:   common.NilHash,
		}
	}

	sort.Sort(addrs)

	for index, addr := range addrs {
		position := index * 2 // we multiply with two, as each participant has two sets

		participants[addr].NodeSetPosition = position

		if participants[addr].IsGenesis {
			continue
		}

		contextHash, bSet, rSet, err := k.state.FetchLatestParticipantContext(addr)
		if err != nil {
			return nil, nil, err
		}

		nodeSet.UpdateNodeSet(position, bSet)
		nodeSet.UpdateNodeSet(position+1, rSet)

		participants[addr].ContextHash = contextHash

		participants[addr].ConsensusQuorum = nodeSet.ParticipantQuorum(participants[addr].NodeSetPosition)
	}

	return participants, nodeSet, nil
}

func (k *Engine) getCanonicalICSReqMsg(
	cs *ktypes.ClusterState,
) (ktypes.CanonicalICSRequest, error) {
	msg := new(ktypes.CanonicalICSRequest)

	rawData, err := cs.Ixs.Bytes()
	if err != nil {
		return *msg, err
	}

	msg.IxData = rawData
	msg.ClusterID = cs.ClusterID
	msg.Operator = string(k.selfID)
	msg.ContextLock = cs.ContextLock()
	msg.Timestamp = cs.ICSReqTime.UnixNano()
	msg.RandomSet = cs.NodeSet.RandomSet().Ids
	msg.ObserverSet = cs.NodeSet.ObserverSet().Ids
	msg.RequiredRandomSetSize = cs.NodeSet.RandomSet().SetSizeWithOutDelta
	msg.RequiredObserverSetSize = cs.NodeSet.ObserverSet().SetSizeWithOutDelta

	return *msg, nil
}

func (k *Engine) getRandomNodes(
	ctx context.Context,
	count int,
	exemptedNodes []kramaid.KramaID,
) ([]kramaid.KramaID, error) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "getRandomNodes")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	queryInitTime := time.Now()

	defer cancel()

	peers, err := k.randomizer.GetRandomNodes(ctx, count, exemptedNodes)
	if err != nil {
		return nil, err
	}

	k.metrics.captureRandomNodesQueryTime(queryInitTime)

	return peers, nil
}

func (k *Engine) getObserverNodes(
	ctx context.Context,
	count int,
	exemptedNodes []kramaid.KramaID,
) ([]kramaid.KramaID, error) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "getObserverNodes")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	peers, err := k.randomizer.GetRandomNodes(ctx, count, exemptedNodes)
	if err != nil {
		return nil, err
	}

	return peers, nil
}

func (k *Engine) createProposalTesseract(slot *ktypes.Slot) (*common.Tesseract, error) {
	if err := k.updateContextDelta(slot); err != nil {
		return nil, err
	}

	clusterState := slot.ClusterState()

	_, err := clusterState.ComputeICSHash()
	if err != nil {
		return nil, err
	}

	transition, err := k.state.LoadTransitionObjects(clusterState.Participants.IxnParticipants())
	if err != nil {
		return nil, errors.Wrap(err, "failed to load state transition objects")
	}

	stateHashes, err := k.exec.ExecuteInteractions(
		transition,
		clusterState.Ixs,
		clusterState.ExecutionContext(),
	)
	if err != nil {
		return nil, err
	}

	// store the transition
	clusterState.SetStateTransition(transition)
	k.logger.Debug("Generating tesseracts", "cluster-ID", slot.ClusterID())

	return generateTesseract(clusterState, stateHashes)
}

func (k *Engine) updateContextDelta(slot *ktypes.Slot) error {
	if slot == nil {
		return errors.New("nil slot")
	}

	cs := slot.ClusterState()

	for _, ps := range cs.Participants {
		// Check 1 : We should not update context for accounts with read lock
		// Check 2 : We should not update context for non signer regular account
		if ps.LockType < common.WriteLock || !ps.IsContextUpdateRequired() {
			continue
		}

		//  Check 3: In debug mode, we should only update context for new accounts
		if k.cfg.EnableDebugMode && !ps.IsGenesis {
			continue
		}

		deltaGroup := new(common.DeltaGroup)

		if ps.IsGenesis {
			// Fetch new nodes for receiver account
			behaviouralNodes, randomNodes, err := k.GetNodes(
				cs,
				RandomContextSize,
				BehaviouralContextSize,
			)
			if err != nil {
				return err
			}

			deltaGroup.RandomNodes = append(deltaGroup.RandomNodes, randomNodes...)
			deltaGroup.BehaviouralNodes = append(deltaGroup.BehaviouralNodes, behaviouralNodes...)

			ps.ContextDelta = deltaGroup

			continue
		}

		senderBehaviourDelta, replacedNodes := cs.GetBehaviouralContextDelta(
			ps.NodeSetPosition,
		)

		if senderBehaviourDelta != "" {
			deltaGroup.BehaviouralNodes = append(deltaGroup.BehaviouralNodes, senderBehaviourDelta)
		}

		if replacedNodes != "" {
			deltaGroup.ReplacedNodes = append(deltaGroup.ReplacedNodes, replacedNodes)
		}

		senderRandomDelta, replacedRandomDelta := cs.GetRandomContextDelta(
			ps.NodeSetPosition+1,
			1,
			cs.Operator,
		)

		deltaGroup.RandomNodes = append(deltaGroup.RandomNodes, senderRandomDelta...)
		deltaGroup.ReplacedNodes = append(deltaGroup.ReplacedNodes, replacedRandomDelta...)

		ps.ContextDelta = deltaGroup
	}

	return nil
}

func (k *Engine) ExecuteAndValidate(
	ts *common.Tesseract,
	transition *state.Transition,
	ps map[identifiers.Address]common.IxParticipant,
) error {
	k.logger.Debug(
		"Executing interactions of grid",
		"ts-hash", ts.Hash(),
		"lock", ts.PreviousContext(),
	)

	stateHashes, err := k.exec.ExecuteInteractions(
		transition,
		ts.Interactions(),
		ts.ExecutionContext(ps),
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

func (k *Engine) GetNodes(
	clusterInfo *ktypes.ClusterState,
	requiredRandomNodes,
	requiredBehaviouralNodes int,
) (behaviouralNodes []kramaid.KramaID, randomNodes []kramaid.KramaID, err error) {
	// TODO: Need to improve this function
	set := clusterInfo.NodeSet.RandomSet()
	count := 0

	for index, kramaID := range set.Ids {
		if set.Responses.GetIndex(index) {
			if len(behaviouralNodes) != requiredBehaviouralNodes {
				behaviouralNodes = append(behaviouralNodes, kramaID)
				count++
			} else {
				randomNodes = append(randomNodes, kramaID)
				count++
			}
		}

		if count == requiredRandomNodes+requiredBehaviouralNodes {
			break
		}
	}

	return
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

	clusterInfo := slot.ClusterState()

	if err := k.lattice.AddTesseractWithState(
		identifiers.NilAddress,
		clusterInfo.GetDirty(),
		tesseract,
		clusterInfo.Transition,
		true,
	); err != nil {
		return err
	}

	ixnHashes := make(common.Hashes, 0, len(tesseract.Interactions()))

	for _, ixn := range tesseract.Interactions() {
		ixnHashes = append(ixnHashes, ixn.Hash())
	}

	msg := &networkmsg.TesseractMsg{
		RawTesseract: make([]byte, 0),
		Extra: map[string][]byte{
			tesseract.ICSHash().String(): clusterInfo.GetDirty()[tesseract.ICSHash()],
		},
		IxnsHashes: ixnHashes,
	}

	msg.RawTesseract, err = tesseract.Canonical().Bytes()
	if err != nil {
		return err
	}

	if err = k.transport.BroadcastTesseract(msg); err != nil {
		k.logger.Error("Failed to broadcast tesseract", "err", err, "cluster-ID", clusterID)
	}

	for _, addr := range tesseract.Addresses() {
		k.state.RemoveCachedObject(addr)
	}

	return nil
}

func generatePoXtData(state *ktypes.ClusterState) common.PoXtData {
	return common.PoXtData{
		EvidenceHash: common.NilHash,
		BinaryHash:   state.BinaryHash,
		IdentityHash: state.IdentityHash,
		ICSHash:      state.ICSHash,
		ClusterID:    state.ClusterID,
		ICSSignature: nil, // TODO calculate and fill this properly
		ICSVoteset:   state.GetICSVoteset(),

		// non canonical fields
		Round:           0,
		CommitSignature: nil,
		BFTVoteSet:      nil,
	}
}

func participantStates(cs *ktypes.ClusterState, ps common.AccStateHashes) common.ParticipantsState {
	participants := make(common.ParticipantsState, len(cs.Participants))

	for addr, p := range cs.Participants {
		participants[addr] = common.State{
			Height:          p.NewHeight(),
			TransitiveLink:  p.TSHash(),
			PreviousContext: p.ContextHash,
			LatestContext:   ps.ContextHash(addr),
			ContextDelta:    p.ContextDelta,
			StateHash:       ps.StateHash(addr),
		}
	}

	return participants
}

func generateTesseract(
	cs *ktypes.ClusterState,
	hs common.AccStateHashes,
) (*common.Tesseract, error) {
	participants := participantStates(cs, hs)

	fuelUsed := cs.Transition.Receipts().FuelUsed()
	fuelLimit := uint64(1000)

	ixnsHash, err := cs.Ixs.Hash()
	if err != nil {
		return nil, err
	}

	receiptHash, err := cs.Transition.Receipts().Hash()
	if err != nil {
		return nil, err
	}

	ts := common.NewTesseract(
		participants,
		ixnsHash,
		receiptHash,
		big.NewInt(0), // TODO pass appropriate value
		uint64(cs.ICSReqTime.Unix()),
		string(cs.Operator),
		fuelUsed,
		fuelLimit,
		generatePoXtData(cs),
		nil,
		cs.SelfKramaID(),
		cs.Ixs,
		cs.Transition.Receipts(),
	)

	return ts, nil
}

func (k *Engine) executionRoutine() {
	for clusterID := range k.executionReq {
		k.logger.Trace("Processing an execution request")

		go func(id common.ClusterID) {
			slotInfo := k.slots.GetSlot(id)
			ts, err := k.createProposalTesseract(slotInfo)
			slotInfo.ExecutionResp <- ktypes.ExecutionResponse{Tesseract: ts, Err: err}
		}(clusterID)
	}
}

func (k *Engine) Close() {
	k.wal.Close()
	k.transport.Close()

	defer k.ctxCancel()
}

func (k *Engine) validateInteractions(ixs common.Interactions) error {
	for _, ix := range ixs {
		ixHash := ix.Hash()

		k.logger.Debug(
			"Validating interaction",
			"ix-hash", ixHash,
			"nonce", ix.Nonce(),
			"from", ix.Sender().Hex(),
			"type", ix.Type(),
		)
		/*
			Checks to perform
			1) Verify the nonce
			2) Verify the balances
			3) Verify the account states
		*/
		latestNonce, err := k.state.GetNonce(ix.Sender(), common.NilHash)
		if err != nil {
			return err
		}

		// validate nonce
		if ix.Nonce() < latestNonce {
			return common.ErrInvalidNonce
		}

		if err = k.isIxValid(ix); err != nil {
			return err
		}
	}

	return nil
}

// Requests returns the request channel of the engine
func (k *Engine) Requests() chan ktypes.Request {
	return k.requests
}

// Logger returns the logger of the engine
func (k *Engine) Logger() hclog.Logger {
	return k.logger
}

// isIxValid performs validity checks based on the type of interaction
func (k *Engine) isIxValid(ix *common.Interaction) error {
	if ix.Sender().IsNil() {
		return common.ErrInvalidAddress
	}

	if accountRegistered, err := k.state.IsAccountRegistered(ix.Sender()); err != nil || !accountRegistered {
		return common.ErrAccountNotFound
	}

	senderObject, err := k.state.GetLatestStateObject(ix.Sender())
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

	switch ix.Type() {
	case common.IxValueTransfer:
		stateObject, err := k.state.GetLatestStateObject(ix.Sender())
		if err != nil {
			k.logger.Error("Error fetching state object", "addr", ix.Sender().Hex())

			return err
		}

		for assetID, value := range ix.TransferValues() {
			if bal, err := stateObject.BalanceOf(assetID); err != nil || bal.Cmp(value) == -1 {
				return errors.New("invalid balance")
			}
		}

	case common.IxAssetCreate:
		payload, err := ix.GetAssetPayload()
		if err != nil {
			k.logger.Error("Error fetching asset payload", "err", err)

			return err
		}

		stateObject, err := k.state.GetLatestStateObject(ix.Sender())
		if err != nil {
			k.logger.Error("Error fetching stateObject", "addr", ix.Sender().Hex())

			return err
		}

		if payload.Create == nil {
			return errors.New("asset create payload is empty")
		}

		assetID := identifiers.NewAssetIDv0(
			payload.Create.IsLogical,
			payload.Create.IsStateFul,
			payload.Create.Dimension,
			uint16(payload.Create.Standard),
			ix.Receiver(),
		)

		if _, err = stateObject.GetRegistryEntry(string(assetID)); err == nil {
			return errors.New("asset already found")
		}

	case common.IxLogicDeploy, common.IxLogicInvoke, common.IxAssetMint, common.IxAssetBurn:
		return nil
	default:
		return common.ErrInvalidInteractionType
	}

	return nil
}

func (k *Engine) setupSargaAccount(
	sarga *common.AccountSetupArgs,
	accounts []common.AccountSetupArgs,
	assets []common.AssetAccountSetupArgs,
	logics []common.LogicSetupArgs,
) (*state.Object, error) {
	stateObject := k.state.CreateStateObject(common.SargaAddress, common.SargaAccount, true)

	if _, err := stateObject.CreateContext(sarga.BehaviouralContext, sarga.RandomContext); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := stateObject.CreateStorageTreeForLogic(common.SargaLogicID); err != nil {
		return nil, errors.Wrap(err, "failed to create storage tree")
	}

	if err := stateObject.AddAccountGenesisInfo(common.SargaAddress, common.GenesisIxHash); err != nil {
		return nil, err
	}

	for _, account := range accounts {
		// Add account to sarga storage tree
		if err := stateObject.AddAccountGenesisInfo(account.Address, common.GenesisIxHash); err != nil {
			return nil, err
		}
	}

	for _, logic := range logics {
		// Add logic account to sarga
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAddressFromString(logic.Name),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	for _, assetAcc := range assets {
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAddressFromString(assetAcc.AssetInfo.Symbol),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	return stateObject, nil
}

func (k *Engine) setupNewAccount(info common.AccountSetupArgs) (*state.Object, error) {
	stateObject := k.state.CreateStateObject(info.Address, info.AccType, true)

	if _, err := stateObject.CreateContext(info.BehaviouralContext, info.RandomContext); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	return stateObject, nil
}

func (k *Engine) setupGenesisLogics(
	transition map[identifiers.Address]*state.Object,
	logics []common.LogicSetupArgs,
) ([]common.Hash, error) {
	hashes := make([]common.Hash, len(logics))

	for _, logic := range logics {
		logicAddr := common.CreateAddressFromString(logic.Name)

		if !common.ContainsAddress(common.GenesisLogicAddrs, logicAddr) {
			k.logger.Error("Mismatch of contract address", "logic-name", logic.Name)

			return nil, errors.New("generated address does not exist in predefined contract address")
		}

		// Create state object for the logic
		logicState := k.state.CreateStateObject(logicAddr, common.LogicAccount, true)

		// Create a dummy state object for the deployer
		// NOTE: This is a dummy object we create at genesis deployment with the 0x00..00 address
		// to act as a placeholder account for the execution environment's sender state driver.
		deployerState := k.state.CreateStateObject(identifiers.NilAddress, common.RegularAccount, true)

		behaviouralCtx := logic.BehaviouralContext
		randomCtx := logic.RandomContext

		_, err := logicState.CreateContext(behaviouralCtx, randomCtx)
		if err != nil {
			return nil, errors.Wrap(err, "context initiation failed in genesis")
		}

		// Create a new execution context
		ctx := &common.ExecutionContext{
			CtxDelta: nil,
			Cluster:  "genesis",
			Time:     k.cfg.GenesisTimestamp,
		}

		// Create a new IxLogicDeploy interaction with the logic payload
		ix, _ := common.NewInteraction(common.IxData{Input: common.IxInput{
			Type: common.IxLogicDeploy,
			Payload: func() []byte {
				payload := &common.LogicPayload{
					Callsite: logic.Callsite,
					Calldata: logic.Calldata,
					Manifest: logic.Manifest.Bytes(),
				}

				encoded, _ := payload.Bytes()

				return encoded
			}(),
		}}, nil)

		// Deploy the genesis logic and check for errors
		_, receipt, err := compute.DeployLogic(ctx, ix, logicState, deployerState, compute.NewEventStream(ix.LogicID()))
		if err != nil {
			k.logger.Error("Unable to deploy logic for", "logic-name", logic.Name)

			return nil, errors.Wrap(err, "deployment failed for logic")
		}

		if receipt.Error != nil {
			return nil, errors.Errorf("deployment call failed: %#x", receipt.Error)
		}

		// Update the dirty objects map with the logic state object
		transition[logicState.Address()] = logicState

		// Obtain the logic ID from the call receipt
		logicID := receipt.LogicID
		k.logger.Info("Deployed genesis contract",
			"logic-name", logic.Name,
			"logic-ID", logicID.String(),
		)
	}

	return hashes, nil
}

func (k *Engine) setupAssetAccounts(
	transition map[identifiers.Address]*state.Object,
	assetAccs []common.AssetAccountSetupArgs,
) error {
	for _, assetAccount := range assetAccs {
		accAddress := common.CreateAddressFromString(assetAccount.AssetInfo.Symbol)

		transition[accAddress] = k.state.CreateStateObject(accAddress, common.AssetAccount, true)

		_, err := transition[accAddress].CreateContext(assetAccount.BehaviouralContext, assetAccount.RandomContext)
		if err != nil {
			return err
		}

		assetID, err := transition[accAddress].CreateAsset(accAddress, assetAccount.AssetInfo.AssetDescriptor())
		if err != nil {
			return err
		}

		if assetAccount.AssetInfo.Operator != identifiers.NilAddress {
			if _, ok := transition[assetAccount.AssetInfo.Operator]; !ok {
				return errors.New("operator account not found")
			}

			_, err = transition[assetAccount.AssetInfo.Operator].CreateAsset(
				accAddress, assetAccount.AssetInfo.AssetDescriptor())
			if err != nil {
				return err
			}
		}

		for _, allocation := range assetAccount.AssetInfo.Allocations {
			if _, ok := transition[allocation.Address]; !ok {
				return errors.New("allocation address not found in state objects")
			}

			transition[allocation.Address].AddBalance(assetID, allocation.Amount.ToInt())
		}
	}

	return nil
}

func (k *Engine) validateAccountCreationInfo(accs ...common.AccountSetupArgs) error {
	for _, acc := range accs {
		if acc.Address == common.SargaAddress {
			return common.ErrInvalidAddress
		}
		// check for address validity
		err := utils.ValidateAccountType(acc.AccType)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("invalid genesis account creation info %s", acc.Address))
		}
	}

	return nil
}

func (k *Engine) validateSargaAccountCreationInfo(acc common.AccountSetupArgs) error {
	if acc.Address != common.SargaAddress {
		return common.ErrInvalidAddress
	}

	return nil
}

func (k *Engine) validateAssetAccountCreationArgs(assetAccounts ...common.AssetAccountSetupArgs) error {
	for _, acc := range assetAccounts {
		if len(acc.AssetInfo.Allocations) == 0 {
			return errors.New("empty allocations")
		}
	}

	return nil
}

func (k *Engine) validateLogicCreationArgs(logicAccounts ...common.LogicSetupArgs) error {
	for _, acc := range logicAccounts {
		if len(acc.Manifest) == 0 {
			return errors.New("invalid manifest")
		}
	}

	return nil
}

func (k *Engine) parseGenesisFile() (
	*common.AccountSetupArgs,
	[]common.AccountSetupArgs,
	[]common.AssetAccountSetupArgs,
	[]common.LogicSetupArgs,
	error,
) {
	genesisData := new(common.GenesisFile)

	data, err := os.ReadFile(k.cfg.GenesisFilePath)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisData); err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to parse genesis file")
	}

	err = k.validateSargaAccountCreationInfo(genesisData.SargaAccount)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "invalid sarga account info")
	}

	err = k.validateAccountCreationInfo(genesisData.Accounts...)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if err = k.validateAssetAccountCreationArgs(genesisData.AssetAccounts...); err != nil {
		return nil, nil, nil, nil, err
	}

	if err = k.validateLogicCreationArgs(genesisData.Logics...); err != nil {
		return nil, nil, nil, nil, err
	}

	return &genesisData.SargaAccount, genesisData.Accounts, genesisData.AssetAccounts, genesisData.Logics, nil
}

func (k *Engine) addGenesisTesseract(
	addresses []identifiers.Address,
	stateHashes, contextHashes []common.Hash,
	transition state.ObjectMap,
) error {
	tesseract := createGenesisTesseract(addresses, stateHashes, contextHashes, k.cfg.GenesisTimestamp)

	if err := k.lattice.AddTesseract(
		true,
		identifiers.NilAddress,
		tesseract,
		state.NewTransition(transition),
		true,
	); err != nil {
		return errors.Wrap(err, "error adding genesis tesseract")
	}

	return nil
}

func (k *Engine) SetupGenesis() error {
	transition := make(state.ObjectMap)

	sargaAccount, genesisAccounts, assetAccounts, logics, err := k.parseGenesisFile()
	if err != nil {
		return errors.Wrap(err, "failed to parse genesis file")
	}

	if _, err = k.state.GetAccountMetaInfo(sargaAccount.Address); err == nil {
		k.logger.Info("!!!!!!....Skipping Genesis....!!!!!!")

		return nil
	}

	sargaObject, err := k.setupSargaAccount(sargaAccount, genesisAccounts, assetAccounts, logics)
	if err != nil {
		return errors.Wrap(err, "failed to setup sarga account")
	}

	transition[sargaObject.Address()] = sargaObject

	for _, v := range genesisAccounts {
		if transition[v.Address], err = k.setupNewAccount(v); err != nil {
			return errors.Wrap(err, "failed to setup genesis account")
		}
	}

	if _, err = k.setupGenesisLogics(transition, logics); err != nil {
		return errors.Wrap(err, "failed to setup genesis logic")
	}

	if err = k.setupAssetAccounts(transition, assetAccounts); err != nil {
		return errors.Wrap(err, "failed to setup asset accounts")
	}

	count := len(transition)
	addresses := make([]identifiers.Address, 0, count)
	stateHashes := make([]common.Hash, 0, count)
	contextHashes := make([]common.Hash, 0, count)

	for _, stateObject := range transition {
		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		addresses = append(addresses, stateObject.Address())
		stateHashes = append(stateHashes, stateHash)
		contextHashes = append(contextHashes, stateObject.ContextHash())
	}

	if err = k.addGenesisTesseract(addresses, stateHashes, contextHashes, transition); err != nil {
		return err
	}

	return nil
}

func (k *Engine) verifySignatures(ts *common.Tesseract, ics *common.ICSNodeSet) (bool, error) {
	var (
		verificationInitTime = time.Now()
		consensusInfo        = ts.ConsensusInfo()
		publicKeys           = make([][]byte, 0, consensusInfo.BFTVoteSet.TrueIndicesSize())
		votesCounter         = make([]uint32, ts.ParticipantCount()+1)
	)

	for _, valIndex := range ts.BFTVoteSet().GetTrueIndices() {
		nodeSetIndices, _, _, publicKey := ics.GetKramaID(int32(valIndex))
		if nodeSetIndices != nil { // ts.Header.Extra.VoteSet.GetIndex(index)
			publicKeys = append(publicKeys, publicKey)

			for _, index := range nodeSetIndices {
				votesCounter[index/2]++
			}
		} else {
			k.logger.Debug("Error fetching validator address", "index", valIndex)
		}
	}

	for index := range ts.Addresses() {
		if votesCounter[index] < ics.ParticipantQuorum(index*2) {
			return false, common.ErrQuorumFailed
		}
	}

	if votesCounter[len(ts.Addresses())] < ics.RandomQuorumSize() {
		return false, common.ErrQuorumFailed
	}

	vote := ktypes.CanonicalVote{
		Type:   ktypes.PRECOMMIT,
		Round:  consensusInfo.Round,
		TSHash: ts.Hash(),
	}

	rawData, err := vote.Bytes()
	if err != nil {
		return false, err
	}

	verified, err := k.signatureVerifier(rawData, consensusInfo.CommitSignature, publicKeys)
	if err != nil {
		return false, err
	}

	k.metrics.captureSignatureVerificationTime(verificationInitTime)

	return verified, nil
}

func (k *Engine) verifyTransitions(
	addr identifiers.Address,
	ts *common.Tesseract,
	allParticipants bool,
) error {
	if ts.ClusterID() == "genesis" {
		return nil
	}

	addresses := make([]identifiers.Address, 0)

	if allParticipants {
		addresses = ts.Addresses()
	} else {
		addresses = append(addresses, addr)
	}

	for _, addr := range addresses {
		initial, err := k.state.IsInitialTesseract(ts, addr)
		if err != nil {
			return errors.Wrap(err, "Sarga account not found")
		}

		if !initial {
			parent, err := k.lattice.GetTesseract(ts.TransitiveLink(addr), false)
			if err != nil {
				k.logger.Error("Failed to fetch parent tesseract", "err", err, "addr", addr)

				return common.ErrPreviousTesseractNotFound
			}

			// Check Heights
			if parent.Height(addr) != ts.Height(addr)-1 {
				return common.ErrInvalidHeight
			}
			// TODO: Add more checks
			// Check time stamp
			if ts.Timestamp() < parent.Timestamp() {
				return common.ErrInvalidBlockTime
			}
		}
	}

	return nil
}

func (k *Engine) ValidateTesseract(
	addr identifiers.Address,
	ts *common.Tesseract,
	ics *common.ICSNodeSet,
	allParticipants bool,
) error {
	if k.db.HasAccMetaInfoAt(addr, ts.Height(addr)) {
		return common.ErrAlreadyKnown
	}

	validSeal, err := k.state.IsSealValid(ts)
	if !validSeal {
		k.logger.Error("Error validating tesseract seal", "err", err)

		return common.ErrInvalidSeal
	}

	if err = k.verifyTransitions(addr, ts, allParticipants); err != nil {
		return err
	}

	verified, err := k.verifySignatures(ts, ics)
	if !verified || err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to verify signatures %v %v", addr, ts.Height(addr)))
	}

	return nil
}

func (k *Engine) AddActiveAccount(addr identifiers.Address) bool {
	return k.slots.AddActiveAccount(addr)
}

func (k *Engine) ClearActiveAccount(addr identifiers.Address) {
	k.slots.ClearActiveAccount(addr)

	k.logger.Trace("removed from active accounts", "address", addr)
}

func sendResponse(req ktypes.Request, err error) {
	req.ResponseChan <- err
}

func getDistinctNodes(operator kramaid.KramaID, nodeSets []*common.NodeSet) ([]kramaid.KramaID, int, bool) {
	nodes := make(map[kramaid.KramaID]struct{})
	isOperatorIncluded := false

	for _, nodeSet := range nodeSets {
		if nodeSet == nil {
			continue
		}

		for _, kramaID := range nodeSet.Ids {
			if _, hasKramaID := nodes[kramaID]; hasKramaID {
				continue
			}

			if kramaID == operator {
				isOperatorIncluded = true
			}

			nodes[kramaID] = struct{}{}
		}
	}

	distinctNodes := make([]kramaid.KramaID, 0, len(nodes))

	for kramaID := range nodes {
		distinctNodes = append(distinctNodes, kramaID)
	}

	return distinctNodes, len(distinctNodes), isOperatorIncluded
}

func areStateHashesValid(ts *common.Tesseract, postExecState common.AccStateHashes) bool {
	for addr, participantState := range ts.Participants() {
		if postExecState.StateHash(addr) != participantState.StateHash {
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
		return false
	}

	return true
}
