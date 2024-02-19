package consensus

import (
	"context"
	"math"
	"math/big"
	"time"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/consensus/observer"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"

	"github.com/hashicorp/go-hclog"
	"github.com/moby/locker"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
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
		allParticipants bool,
	) error
}

type kramaTransport interface {
	Start()
	Close()
	Messages() <-chan *ktypes.ICSMSG
	RegisterContextRouter(
		ctx context.Context,
		operator kramaid.KramaID,
		clusterID common.ClusterID,
		nodeset *common.ICSNodeSet,
		voteset *kbft.HeightVoteSet,
	)
	InitClusterConnection(ctx context.Context, clusterID common.ClusterID, isOperator bool)
	BroadcastTesseract(msg *networkmsg.TesseractMsg) error
	BroadcastMessage(
		ctx context.Context,
		msg *ktypes.ICSMSG,
	)
	GracefullyCloseContextRouter(clusterID common.ClusterID)
	SendMessage(
		ctx context.Context,
		peerID, sender kramaid.KramaID,
		clusterID common.ClusterID,
		msgType networkmsg.MsgType,
		rawMsg ktypes.ICSPayload,
	) error
	GetRoundVoteSetBits(clusterID common.ClusterID) (map[int32]*ktypes.VoteBitSet, error)
	StartGossip(clusterID common.ClusterID)
}

type stateManager interface {
	FetchInteractionContext(
		ctx context.Context,
		ix *common.Interaction,
	) (
		map[identifiers.Address]common.Hash,
		[]*common.NodeSet,
		error,
	)
	GetPublicKeys(context context.Context, ids ...kramaid.KramaID) (keys [][]byte, err error)
	GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error)
	IsAccountRegistered(addr identifiers.Address) (bool, error)
	GetLatestStateObject(addr identifiers.Address) (*state.Object, error)
	GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error)
	Cleanup(addr identifiers.Address)
}

type ixPool interface {
	IncrementWaitTime(addr identifiers.Address, baseTime time.Duration) error
	Executables() ixpool.InteractionQueue
	Drop(ix *common.Interaction)
}

type execution interface {
	ExecuteInteractions(common.Interactions, *common.ExecutionContext) (common.Receipts, common.AccStateHashes, error)
	Revert(common.ClusterID) error
	Cleanup(common.ClusterID)
}

type Engine struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	cfg          *config.ConsensusConfig
	mux          *utils.TypeMux
	logger       hclog.Logger
	selfID       kramaid.KramaID
	slots        *ktypes.Slots
	requests     chan ktypes.Request
	randomizer   *flux.Randomizer
	transport    kramaTransport
	exec         execution
	pool         ixPool
	state        stateManager
	executionReq chan common.ClusterID
	lattice      lattice
	wal          kbft.WAL
	vault        *crypto.KramaVault
	clusterLocks *locker.Locker
	metrics      *Metrics
	avgICSTime   time.Duration
	slotCloseCh  chan common.ClusterID
}

func NewKramaEngine(
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
) (*Engine, error) {
	wal, err := kbft.NewWAL(logger, cfg.DirectoryPath)
	if err != nil {
		return nil, errors.Wrap(err, "WAL failed")
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	k := &Engine{
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		cfg:          cfg,
		logger:       logger.Named("Krama-Engine"),
		mux:          mux,
		selfID:       selfID,
		state:        state,
		slots:        slots,
		requests:     make(chan ktypes.Request),
		randomizer:   randomizer,
		transport:    transport,
		exec:         exec,
		pool:         ixPool,
		lattice:      lattice,
		executionReq: make(chan common.ClusterID),
		wal:          wal,
		vault:        val,
		clusterLocks: locker.New(),
		metrics:      metrics,
		avgICSTime:   cfg.AccountWaitTime,
		slotCloseCh:  make(chan common.ClusterID),
	}

	k.metrics.initMetrics(float64(cfg.OperatorSlotCount), float64(cfg.ValidatorSlotCount))

	return k, nil
}

// loadIxnClusterState fetches the account state and returns the interaction cluster state
func (k *Engine) loadIxnClusterState(
	ctx context.Context,
	req ktypes.Request,
	clusterID common.ClusterID,
) (*ktypes.ClusterState, error) {
	var err error

	clusterState := ktypes.NewICS(6, req.Msg, req.Ixs, clusterID, req.Operator, req.ReqTime, k.selfID)
	// Fetch the committed account info of interaction participants
	clusterState.AccountInfos, err = k.fetchIxAccounts(ctx, req.Ixs[0])
	if err != nil {
		return nil, err
	}

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
	)

	// Fetch the context nodes of interaction participants
	contextHashes, nodeSets, err := k.state.FetchInteractionContext(ctx, slot.ClusterState().Ixs[0])
	if err != nil {
		return err
	}

	loadContextLockInfo(slot.ClusterState(), contextHashes)

	contextNodes, contextNodesSize, isOperatorIncluded := getDistinctNodes(k.selfID, nodeSets)

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

	slot.ClusterState().UpdateNodeSet(common.SenderBehaviourSet, nodeSets[common.SenderBehaviourSet])
	slot.ClusterState().UpdateNodeSet(common.SenderRandomSet, nodeSets[common.SenderRandomSet])
	slot.ClusterState().UpdateNodeSet(common.ReceiverBehaviourSet, nodeSets[common.ReceiverBehaviourSet])
	slot.ClusterState().UpdateNodeSet(common.ReceiverRandomSet, nodeSets[common.ReceiverRandomSet])
	slot.ClusterState().UpdateNodeSet(
		common.RandomSet,
		common.NewNodeSet(operatorRandomNodes, randomKeys, operatorRandomNodesCount),
	)
	slot.ClusterState().UpdateNodeSet(
		common.ObserverSet,
		common.NewNodeSet(observerNodes, observerKeys, requiredObserverNodes),
	)

	slot.ClusterState().UpdateClusterSize()

	slot.ClusterState().ICSReqTime = utils.Now()
	// Construct ICS_Request
	reqMsg, err := k.getCanonicalICSReqMsg(slot.ClusterState())
	if err != nil {
		return err
	}

	k.transport.InitClusterConnection(ctx, slot.ClusterID(), true)

	failedReqCount := k.sendICSRequest(ctx, reqMsg, slot.ClusterState().NodeSet)

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

	if !slot.ClusterState().IsRandomQuorum(operatorRandomNodesCount, requiredObserverNodes) {
		return errors.New("random quorum failed")
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

func (k *Engine) joinCluster(ctx context.Context, slot *ktypes.Slot, req ktypes.Request) error {
	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "joinCluster")
	defer span.End()

	k.logger.Debug(
		"Received an ICS join request",
		"from", slot.ClusterState().Operator,
		"timestamp", slot.ClusterState().RequestMsg.Timestamp,
		"cluster-ID", slot.ClusterID(),
		"ix-hash", slot.ClusterState().Ixs[0].Hash(),
	)

	reqTime := utils.Canonical(time.Unix(0, slot.ICSRequestMsg().Timestamp))

	if !k.isTimely(reqTime, utils.Now()) {
		return errors.New("invalid time stamp")
	}

	slot.ClusterState().CurrentRole = common.IcsSetType(slot.ICSRequestMsg().ContextType)

	contextHashes, nodeSets, err := k.state.FetchInteractionContext(ctx, slot.ClusterState().Ixs[0])
	if err != nil {
		return err
	}

	loadContextLockInfo(slot.ClusterState(), contextHashes)

	// Check whether the context hashes matches
	for addr, info := range slot.ICSRequestMsg().ContextLock {
		if slot.ClusterState().AccountInfos.GetHeight(addr) < info.Height {
			if err = k.mux.Post(utils.SyncRequestEvent{
				Address:  addr,
				Height:   info.Height,
				BestPeer: slot.ClusterState().Operator,
			}); err != nil {
				k.logger.Error("Failed to post sync request", "err", err)
			}
		}

		if slot.ClusterState().AccountInfos.GetHeight(addr) != info.Height {
			return common.ErrHeightMismatch
		}

		if info.TesseractHash != slot.ClusterState().AccountInfos.GetLatestHash(addr) {
			return common.ErrHashMismatch
		}
	}

	observerPublicKeys, err := k.state.GetPublicKeys(context.Background(), req.Msg.ObserverSet...)
	if err != nil {
		return errors.New("failed to retrieve public keys")
	}

	randomPublicKeys, err := k.state.GetPublicKeys(context.Background(), req.Msg.RandomSet...)
	if err != nil {
		return errors.New("failed to retrieve public keys")
	}

	// update the cluster state with the latest node set's
	slot.ClusterState().UpdateNodeSet(common.SenderBehaviourSet, nodeSets[common.SenderBehaviourSet])
	slot.ClusterState().UpdateNodeSet(common.SenderRandomSet, nodeSets[common.SenderRandomSet])
	slot.ClusterState().UpdateNodeSet(common.ReceiverBehaviourSet, nodeSets[common.ReceiverBehaviourSet])
	slot.ClusterState().UpdateNodeSet(common.ReceiverRandomSet, nodeSets[common.ReceiverRandomSet])
	slot.ClusterState().UpdateNodeSet(common.ObserverSet, common.NewNodeSet(req.Msg.ObserverSet, observerPublicKeys, 0))
	slot.ClusterState().UpdateNodeSet(common.RandomSet, common.NewNodeSet(req.Msg.RandomSet, randomPublicKeys, 0))
	slot.ClusterState().UpdateClusterSize()

	k.transport.InitClusterConnection(ctx, slot.ClusterID(), false)

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
		trace.WithAttributes(attribute.String("clusterID", clusterID.String())),
	)
	defer span.End()

	if slot := k.slots.GetSlot(clusterID); slot != nil {
		sendResponse(req, nil)

		return
	}

	if !k.slots.AreSlotsAvailable(req.SlotType) {
		sendResponse(req, common.ErrSlotsFull)

		return
	}

	cs, err := k.loadIxnClusterState(ctx, req, clusterID)
	if err != nil {
		sendResponse(req, err)

		return
	}

	// create a slot and try adding it
	newSlot := ktypes.NewSlot(req.SlotType, cs)

	if !k.slots.AddSlot(newSlot) {
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
		k.exec.Cleanup(clusterID)
	}()

	if req.SlotType == ktypes.OperatorSlot {
		k.metrics.captureAvailableOperatorSlots(-1)
	} else {
		k.metrics.captureAvailableValidatorSlots(-1)
	}

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

	defer k.transport.GracefullyCloseContextRouter(clusterID)

	k.transport.RegisterContextRouter(ctx, cs.Operator, clusterID, cs.NodeSet, voteset)

	switch req.SlotType {
	case ktypes.OperatorSlot:
		err = k.acquireContextLock(ctx, newSlot)

		sendResponse(req, err)

		k.metrics.captureICSCreationTime(k.slots.GetSlot(clusterID).ClusterState().ICSReqTime)

		if err != nil {
			k.logger.Error("Error acquiring context lock", "err", err, "cluster-ID", clusterID)
			k.metrics.captureICSCreationFailureCount(1)

			k.sendICSFailure(ctx, clusterID)

			return
		}

		if err = k.sendICSSuccess(ctx, clusterID); err != nil {
			k.logger.Error("Failed to send ics success message", err)

			return
		}

		k.logger.Info("Cluster creation successful", "cluster-ID", clusterID)

	case ktypes.ValidatorSlot:
		requestTime := time.Now()

		err = k.joinCluster(ctx, newSlot, req)

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
	slot := k.slots.GetSlot(clusterID)
	if slot == nil {
		k.logger.Info("Nil slot", "cluster-ID", req.Msg.ClusterID)

		return
	}

	go k.initOutboundMessageHandler(ctx, slot)

	if cs.CurrentRole == common.ObserverSet {
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
			if err = k.exec.Revert(cs.ClusterID); err != nil {
				k.logger.Error("Failed to revert the execution changes", "err", err)
			}

			return
		}

		k.metrics.captureAgreementTime(consensusInitTS)
	}

	k.logger.Info("Interaction finalized", "cluster-ID", cs.ClusterID)
}

func (k *Engine) fetchIxAccounts(ctx context.Context, ix *common.Interaction) (ktypes.AccountInfos, error) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "fetchIxAccounts")
	defer span.End()

	accounts := make(ktypes.AccountInfos)

	if !ix.Sender().IsNil() {
		accInfo, err := k.state.GetAccountMetaInfo(ix.Sender())
		if err != nil {
			return nil, err
		}

		accounts[ix.Sender()] = ktypes.AccountInfoFromAccMetaInfo(accInfo, false)
	}

	if ix.Receiver().IsNil() {
		return accounts, nil
	}

	accountRegistered, err := k.state.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return nil, err
	}

	if accountRegistered {
		accInfo, err := k.state.GetAccountMetaInfo(ix.Receiver())
		if err != nil {
			return nil, err
		}

		accounts[ix.Receiver()] = ktypes.AccountInfoFromAccMetaInfo(accInfo, false)

		return accounts, nil
	}

	genesisAccInfo, err := k.state.GetAccountMetaInfo(common.SargaAddress)
	if err != nil {
		return nil, err
	}

	acc := &ktypes.AccountInfo{
		Address:       ix.Receiver(),
		AccType:       common.AccTypeFromIxType(ix.Type()),
		TesseractHash: common.NilHash,
		Height:        0,
		IsGenesis:     true,
	}

	accounts[common.SargaAddress] = ktypes.AccountInfoFromAccMetaInfo(genesisAccInfo, false)
	accounts[ix.Receiver()] = acc

	return accounts, nil
}

func (k *Engine) getCanonicalICSReqMsg(
	cs *ktypes.ClusterState,
) (ktypes.CanonicalICSRequest, error) {
	canonicalICSReqMsg := new(ktypes.CanonicalICSRequest)

	rawData, err := cs.Ixs.Bytes()
	if err != nil {
		return *canonicalICSReqMsg, err
	}

	canonicalICSReqMsg.IxData = rawData
	canonicalICSReqMsg.ClusterID = cs.ClusterID
	canonicalICSReqMsg.Operator = string(k.selfID)
	canonicalICSReqMsg.ContextLock = cs.ContextLock()
	canonicalICSReqMsg.Timestamp = cs.ICSReqTime.UnixNano()
	canonicalICSReqMsg.RandomSet = cs.NodeSet.Nodes[common.RandomSet].Ids
	canonicalICSReqMsg.ObserverSet = cs.NodeSet.Nodes[common.ObserverSet].Ids

	return *canonicalICSReqMsg, nil
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

	receipts, stateHashes, err := k.exec.ExecuteInteractions(
		clusterState.Ixs,
		clusterState.ExecutionContext(),
	)
	if err != nil {
		return nil, err
	}
	// store the receipts
	clusterState.SetReceipts(receipts)
	clusterState.SetPostExecState(stateHashes)
	k.logger.Debug("Generating tesseracts", "cluster-ID", slot.ClusterID())

	return generateTesseract(clusterState)
}

// Updates the context delta for sender and for the receiver depending on whether the
// receiver account is registered or not
func (k *Engine) updateContextDelta(slot *ktypes.Slot) error {
	if slot == nil {
		return errors.New("nil slot")
	}

	// if debug mode is on, partially update the context delta
	if k.cfg.EnableDebugMode {
		return k.partiallyUpdateContextDelta(slot)
	}

	clusterState := slot.ClusterState()
	seenAccounts := make(map[identifiers.Address]bool)
	deltaMap := make(common.ContextDelta)

	for _, ix := range clusterState.Ixs {
		senderAddr := ix.Sender()
		receiverAddr := ix.Receiver()

		// update context delta for sender
		if !senderAddr.IsNil() && !seenAccounts[senderAddr] {
			senderDeltaGroup := new(common.DeltaGroup)
			senderDeltaGroup.Role = common.Sender
			senderBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
				common.SenderBehaviourSet,
			)

			if senderBehaviourDelta != "" {
				senderDeltaGroup.BehaviouralNodes = append(senderDeltaGroup.BehaviouralNodes, senderBehaviourDelta)
			}

			if replacedNodes != "" {
				senderDeltaGroup.ReplacedNodes = append(senderDeltaGroup.ReplacedNodes, replacedNodes)
			}

			senderRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
				common.SenderRandomSet,
				1,
				clusterState.Operator,
			)

			senderDeltaGroup.RandomNodes = append(senderDeltaGroup.RandomNodes, senderRandomDelta...)
			senderDeltaGroup.ReplacedNodes = append(senderDeltaGroup.ReplacedNodes, replacedRandomDelta...)

			seenAccounts[senderAddr] = true
			deltaMap[senderAddr] = senderDeltaGroup
		}

		// update context delta for receiver
		if !receiverAddr.IsNil() && !seenAccounts[receiverAddr] {
			receiverDeltaGroup := new(common.DeltaGroup)
			receiverDeltaGroup.Role = common.Receiver

			accountRegistered, err := k.state.IsAccountRegistered(receiverAddr)
			if err != nil {
				return err
			}

			// if receiver account is not registered, update context delta for genesis as well as receiver address
			if !accountRegistered {
				// Fetch new nodes for receiver account
				behaviouralNodes, randomNodes, err := k.GetNodes(
					clusterState,
					RandomContextSize,
					BehaviouralContextSize,
				)
				if err != nil {
					return err
				}

				receiverDeltaGroup.RandomNodes = append(receiverDeltaGroup.RandomNodes, randomNodes...)
				receiverDeltaGroup.BehaviouralNodes = append(receiverDeltaGroup.BehaviouralNodes, behaviouralNodes...)

				// Fetch sarga account context delta
				genesisDeltaGroup := new(common.DeltaGroup)
				genesisDeltaGroup.Role = common.Genesis
				genesisBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
					common.ReceiverBehaviourSet,
				)

				if genesisBehaviourDelta != "" {
					genesisDeltaGroup.BehaviouralNodes = append(genesisDeltaGroup.BehaviouralNodes, genesisBehaviourDelta)
				}

				if replacedNodes != "" {
					genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedNodes)
				}

				genesisRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
					common.ReceiverRandomSet,
					1,
					clusterState.Operator,
				)

				genesisDeltaGroup.RandomNodes = append(genesisDeltaGroup.RandomNodes, genesisRandomDelta...)
				genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedRandomDelta...)

				seenAccounts[common.SargaAddress] = true
				deltaMap[common.SargaAddress] = genesisDeltaGroup
			} else if clusterState.AccountInfos[receiverAddr].AccType == common.LogicAccount ||
				clusterState.AccountInfos[receiverAddr].AccType == common.AssetAccount {
				// update context delta for only receiver address if receiver account is registered
				receiverBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
					common.ReceiverBehaviourSet,
				)

				if receiverBehaviourDelta != "" {
					receiverDeltaGroup.BehaviouralNodes = append(receiverDeltaGroup.BehaviouralNodes, receiverBehaviourDelta)
				}

				if replacedNodes != "" {
					receiverDeltaGroup.ReplacedNodes = append(receiverDeltaGroup.ReplacedNodes, replacedNodes)
				}

				receiverRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
					common.ReceiverRandomSet,
					1,
					clusterState.Operator,
				)

				receiverDeltaGroup.RandomNodes = append(receiverDeltaGroup.RandomNodes, receiverRandomDelta...)
				receiverDeltaGroup.ReplacedNodes = append(receiverDeltaGroup.ReplacedNodes, replacedRandomDelta...)
			}

			seenAccounts[receiverAddr] = true
			deltaMap[receiverAddr] = receiverDeltaGroup
		}
	}

	clusterState.UpdateContextDelta(deltaMap)

	return nil
}

// Updates the context delta for the receiver if the receiver account is not registered, and retains the context
// of other participants.
func (k *Engine) partiallyUpdateContextDelta(slot *ktypes.Slot) error {
	clusterState := slot.ClusterState()
	seenAccounts := make(map[identifiers.Address]bool)
	deltaMap := make(common.ContextDelta)

	for _, ix := range clusterState.Ixs {
		senderAddr := ix.Sender()
		receiverAddr := ix.Receiver()

		if !senderAddr.IsNil() && !seenAccounts[senderAddr] {
			senderDeltaGroup := new(common.DeltaGroup)
			senderDeltaGroup.Role = common.Sender
			seenAccounts[senderAddr] = true
			deltaMap[senderAddr] = senderDeltaGroup
		}

		if !receiverAddr.IsNil() && !seenAccounts[receiverAddr] {
			receiverDeltaGroup := new(common.DeltaGroup)
			receiverDeltaGroup.Role = common.Receiver

			accountRegistered, err := k.state.IsAccountRegistered(receiverAddr)
			if err != nil {
				return err
			}

			if !accountRegistered {
				// Fetch new nodes for receiver account
				behaviouralNodes, randomNodes, err := k.GetNodes(
					clusterState,
					RandomContextSize,
					BehaviouralContextSize,
				)
				if err != nil {
					return err
				}

				receiverDeltaGroup.RandomNodes = append(receiverDeltaGroup.RandomNodes, randomNodes...)
				receiverDeltaGroup.BehaviouralNodes = append(receiverDeltaGroup.BehaviouralNodes, behaviouralNodes...)

				genesisDeltaGroup := new(common.DeltaGroup)
				genesisDeltaGroup.Role = common.Genesis
				seenAccounts[common.SargaAddress] = true
				deltaMap[common.SargaAddress] = genesisDeltaGroup
			}

			seenAccounts[receiverAddr] = true
			deltaMap[receiverAddr] = receiverDeltaGroup
		}
	}

	clusterState.UpdateContextDelta(deltaMap)

	return nil
}

func (k *Engine) GetNodes(
	clusterInfo *ktypes.ClusterState,
	requiredRandomNodes,
	requiredBehaviouralNodes int,
) (behaviouralNodes []kramaid.KramaID, randomNodes []kramaid.KramaID, err error) {
	// TODO: Need to improve this function
	set := clusterInfo.NodeSet.Nodes[common.RandomSet]
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

func generateParticipantData(state *ktypes.ClusterState) common.Participants {
	ix := state.Ixs[0] // TODO: Improve this

	participants := make(common.Participants)

	if !ix.Sender().IsNil() {
		addr := ix.Sender()

		participants[addr] = common.State{
			Height:          state.NewHeight(addr),
			TransitiveLink:  state.AccountInfos.GetLatestHash(addr),
			PreviousContext: state.GetPreviousContextHash(addr),
			LatestContext:   state.GetContextHash(addr),
			ContextDelta:    state.ContextDelta(addr),
			StateHash:       state.GetStateHash(addr),
		}
	}

	if !ix.Receiver().IsNil() {
		addr := ix.Receiver()

		participants[addr] = common.State{
			Height:          state.NewHeight(addr),
			TransitiveLink:  state.AccountInfos.GetLatestHash(addr),
			PreviousContext: state.GetPreviousContextHash(addr),
			LatestContext:   state.GetContextHash(addr),
			ContextDelta:    state.ContextDelta(addr),
			StateHash:       state.GetStateHash(addr),
		}

		if state.AccountInfos.IsGenesis(ix.Receiver()) {
			addr := common.SargaAddress

			participants[addr] = common.State{
				Height:          state.NewHeight(addr),
				TransitiveLink:  state.AccountInfos.GetLatestHash(addr),
				PreviousContext: state.GetPreviousContextHash(addr),
				LatestContext:   state.GetContextHash(addr),
				ContextDelta:    state.ContextDelta(addr),
				StateHash:       state.GetStateHash(addr),
			}
		}
	}

	return participants
}

func generateTesseract(state *ktypes.ClusterState) (*common.Tesseract, error) {
	participants := generateParticipantData(state)

	fuelUsed := state.GetFuelUsed()
	fuelLimit := uint64(1000)

	ixnsHash, err := state.Ixs.Hash()
	if err != nil {
		return nil, err
	}

	receiptHash, err := state.Receipts.Hash()
	if err != nil {
		return nil, err
	}

	ts := common.NewTesseract(
		participants,
		ixnsHash,
		receiptHash,
		big.NewInt(0), // TODO pass appropriate value
		state.ICSReqTime.Unix(),
		string(state.Operator),
		fuelUsed,
		fuelLimit,
		generatePoXtData(state),
		nil,
		state.SelfKramaID(),
		state.Ixs,
		state.Receipts,
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

		if err = k.IsIxValid(ix); err != nil {
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

// IsIxValid performs validity checks based on the type of interaction
func (k *Engine) IsIxValid(ix *common.Interaction) error {
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

func loadContextLockInfo(
	cs *ktypes.ClusterState,
	hashes map[identifiers.Address]common.Hash,
) {
	for addr, accInfo := range cs.AccountInfos {
		accInfo.ContextHash = hashes[addr]
	}
}

func sendResponse(req ktypes.Request, err error) {
	req.ResponseChan <- err
}

func getDistinctNodes(operator kramaid.KramaID, nodeSets []*common.NodeSet) ([]kramaid.KramaID, int, bool) {
	nodes := make(map[kramaid.KramaID]struct{})

	for _, nodeSet := range nodeSets {
		if nodeSet == nil {
			continue
		}

		for _, kramaID := range nodeSet.Ids {
			if _, hasKramaID := nodes[kramaID]; hasKramaID {
				continue
			}

			nodes[kramaID] = struct{}{}
		}
	}

	isOperatorIncluded := false
	distinctNodes := make([]kramaid.KramaID, 0, len(nodes))

	for kramaID := range nodes {
		if kramaID == operator {
			isOperatorIncluded = true
		}

		distinctNodes = append(distinctNodes, kramaID)
	}

	return distinctNodes, len(distinctNodes), isOperatorIncluded
}
