package consensus

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/moby/locker"
	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	"github.com/sarvalabs/go-moi/consensus/observer"
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
	Requests() chan Request
	Logger() hclog.Logger
}

type lattice interface {
	AddKnownHashes(tesseracts []*common.Tesseract)
	AddTesseracts(dirtyStorage map[common.Hash][]byte, tesseracts ...*common.Tesseract) error
}

type transport interface {
	InitClusterCommunication(ctx context.Context, slot *ktypes.Slot) error
	RegisterRPCService(serviceID protocol.ID, serviceName string, service interface{}) error
	Call(ctx context.Context, kramaID kramaid.KramaID, svcName, svcMethod string, args, response interface{}) error
	BroadcastTesseract(msg *networkmsg.TesseractMessage) error
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

type Response struct {
	slotType ktypes.SlotType
	err      error
}

type Request struct {
	ixs          common.Interactions
	msg          *networkmsg.CanonicalICSRequest
	operator     kramaid.KramaID
	slotType     ktypes.SlotType
	reqTime      time.Time
	responseChan chan Response
}

func (r *Request) IxHash() common.Hash {
	return r.ixs[0].Hash()
}

func (r *Request) getClusterID() (common.ClusterID, error) {
	switch r.slotType {
	case ktypes.OperatorSlot:
		return generateClusterID()
	case ktypes.ValidatorSlot:
		return common.ClusterID(r.msg.ClusterID), nil
	default:
		return "", errors.New("invalid request type")
	}
}

type Engine struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	cfg          *config.ConsensusConfig
	mux          *utils.TypeMux
	logger       hclog.Logger
	selfID       kramaid.KramaID
	slots        *ktypes.Slots
	requests     chan Request
	randomizer   *flux.Randomizer
	transport    transport
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
}

func NewKramaEngine(
	cfg *config.ConsensusConfig,
	logger hclog.Logger,
	mux *utils.TypeMux,
	state stateManager,
	network network,
	exec execution,
	ixPool ixPool,
	val *crypto.KramaVault,
	lattice lattice,
	randomizer *flux.Randomizer,
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
		selfID:       network.GetKramaID(),
		state:        state,
		slots:        slots,
		requests:     make(chan Request),
		randomizer:   randomizer,
		transport:    NewKramaTransport(logger, network),
		exec:         exec,
		pool:         ixPool,
		lattice:      lattice,
		executionReq: make(chan common.ClusterID),
		wal:          wal,
		vault:        val,
		clusterLocks: locker.New(),
		metrics:      metrics,
		avgICSTime:   cfg.AccountWaitTime,
	}

	k.metrics.initMetrics(float64(cfg.OperatorSlotCount), float64(cfg.ValidatorSlotCount))

	return k, k.transport.RegisterRPCService(config.ICSProtocolRPC, "ICSRPC", NewICSRPCService(k))
}

// loadIxnClusterState fetches the account state and returns the interaction cluster state
func (k *Engine) loadIxnClusterState(
	ctx context.Context,
	req Request,
	clusterID common.ClusterID,
) (*ktypes.ClusterState, error) {
	var err error

	clusterState := ktypes.NewICS(6, req.msg, req.ixs, clusterID, req.operator, req.reqTime, k.selfID)
	// Fetch the committed account info of interaction participants
	clusterState.AccountInfos, err = k.fetchIxAccounts(ctx, req.ixs[0])
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

	finalWaitGroup := new(sync.WaitGroup)

	finalWaitGroup.Add(2)

	var (
		contextRandomNodes  []kramaid.KramaID
		operatorRandomNodes []kramaid.KramaID
		observerNodes       []kramaid.KramaID
	)

	// Fetch the context nodes of interaction participants
	contextHashes, nodeSets, err := k.state.FetchInteractionContext(ctx, slot.ClusterState().Ixs[0])
	if err != nil {
		return err
	}

	loadContextLockInfo(slot.ClusterState(), contextHashes)

	// Initiate the cluster communication by subscribing to clusterID
	if err = k.initClusterCommunication(ctx, slot); err != nil {
		return errors.Wrap(err, "failed to initiate cluster communication")
	}
	// Start routine to capture the random nodes provided by the context nodes
	randomNodesReceiverChan := make(chan []kramaid.KramaID)

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case nodes := <-randomNodesReceiverChan:
				contextRandomNodes = append(contextRandomNodes, nodes...)
			}
		}
	}(ctx)

	slot.ClusterState().ICSReqTime = utils.Now()
	// Construct ICS_Request
	reqMsg, err := k.getCanonicalICSReqMsg(slot.ClusterState())
	if err != nil {
		return err
	}

	// Send ICSRequest to context nodes of both participants
	go k.sendICSRequest(
		ctx,
		common.SenderBehaviourSet,
		finalWaitGroup,
		slot.ClusterID(),
		nodeSets[common.SenderBehaviourSet],
		reqMsg,
		randomNodesReceiverChan,
	)

	go k.sendICSRequest(
		ctx,
		common.SenderRandomSet,
		finalWaitGroup,
		slot.ClusterID(),
		nodeSets[common.SenderRandomSet],
		reqMsg,
		randomNodesReceiverChan,
	)

	if !slot.ClusterState().Ixs[0].Receiver().IsNil() {
		finalWaitGroup.Add(2)

		go k.sendICSRequest(
			ctx,
			common.ReceiverBehaviourSet,
			finalWaitGroup,
			slot.ClusterID(),
			nodeSets[common.ReceiverBehaviourSet],
			reqMsg,
			randomNodesReceiverChan,
		)

		go k.sendICSRequest(
			ctx,
			common.ReceiverRandomSet,
			finalWaitGroup,
			slot.ClusterID(),
			nodeSets[common.ReceiverRandomSet],
			reqMsg,
			randomNodesReceiverChan,
		)
	}

	finalWaitGroup.Wait()
	// Check for context quorum
	if !slot.ClusterState().IsContextQuorum() {
		return errors.New("context quorum failed")
	}

	respondedEligibleSetSize, respondedEligibleSet := slot.ClusterState().RespondedEligibleSet()
	// Calculate the required number of observer nodes in the cluster based on observer coefficient
	// value and the actual size of the cluster including the required observer
	requiredObserverNodes := int(math.Ceil(ObserverCoeff * float64(respondedEligibleSetSize)))
	observerNodesQueryCount := requiredObserverNodes + k.observerNodeDelta(respondedEligibleSetSize)

	actualICSSize := 3*respondedEligibleSetSize + requiredObserverNodes
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

	totalRandomNodes := 2*respondedEligibleSetSize + additionalRandomNodes

	// contextRandomNodes = getExclusivePeers(respondedEligibleSet, contextRandomNodes)

	operatorRandomNodesCount := totalRandomNodes // - len(contextRandomNodes)
	operatorRandomNodesQueryCount := operatorRandomNodesCount + k.randomNodeDelta(totalRandomNodes)

	// exemptedNodes := append(respondedEligibleSet, contextRandomNodes...)

	operatorRandomNodes, err = k.getRandomNodes(ctx, operatorRandomNodesQueryCount, respondedEligibleSet)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve random and observer nodes")
	}

	if !slot.ClusterState().IsOperatorIncluded() {
		operatorRandomNodes = append([]kramaid.KramaID{k.selfID}, operatorRandomNodes...) // TODO:Improve this
	}

	exemptedNodes := respondedEligibleSet
	exemptedNodes = append(exemptedNodes, operatorRandomNodes...)

	observerNodes, err = k.getObserverNodes(ctx, observerNodesQueryCount, exemptedNodes)
	if err != nil {
		k.logger.Error("Error fetching observer nodes", "err", err)

		return errors.New("unable to retrieve observer nodes")
	}

	finalWaitGroup.Add(2)

	observerKeys, err := k.state.GetPublicKeys(context.Background(), observerNodes...)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch the public key of observer nodes.")
	}

	randomKeys, err := k.state.GetPublicKeys(context.Background(), operatorRandomNodes...)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch the public key of random nodes.")
	}

	go k.sendICSRequestWithBound(
		ctx,
		common.RandomSet,
		operatorRandomNodesCount,
		finalWaitGroup,
		slot.ClusterID(),
		operatorRandomNodes,
		randomKeys,
		reqMsg,
	)

	go k.sendICSRequestWithBound(
		ctx,
		common.ObserverSet,
		requiredObserverNodes,
		finalWaitGroup,
		slot.ClusterID(),
		observerNodes,
		observerKeys,
		reqMsg,
	)

	finalWaitGroup.Wait()

	if !slot.ClusterState().IsRandomQuorum(operatorRandomNodesCount, requiredObserverNodes) {
		return errors.New("random quorum failed")
	}

	if err = k.sendICSSuccess(slot.ClusterID()); err != nil {
		return err
	}

	k.metrics.captureICSCreationTime(slot.ClusterState().ICSReqTime)

	return nil
}

func (k *Engine) randomNodeDelta(setSize int) int {
	return int(math.Ceil(RandomNodesDelta * float64(setSize)))
}

func (k *Engine) observerNodeDelta(setSize int) int {
	return int(math.Ceil(ObserverNodesDelta * float64(setSize)))
}

func generateClusterID() (common.ClusterID, error) {
	randHash := make([]byte, 32)

	if _, err := rand.Read(randHash); err != nil {
		return "", err
	}

	return common.ClusterID(base58.Encode(randHash)), nil
}

func (k *Engine) Start() {
	go k.minter()

	go k.executionRoutine()

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

	slot.ClusterState().NodeSet.Nodes = nodeSets

	k.logger.Debug("Responding to ICS request", "from", slot.ClusterState().Operator)

	return k.initClusterCommunication(ctx, slot)
}

func (k *Engine) handleReq(req Request) {
	clusterID, err := req.getClusterID()
	if err != nil {
		req.responseChan <- Response{slotType: req.slotType, err: errors.New("failed to decode clusterID")}

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

	if !k.slots.AreSlotsAvailable(req.slotType) {
		sendResponse(req, common.ErrSlotsFull)

		return
	}

	cs, err := k.loadIxnClusterState(ctx, req, clusterID)
	if err != nil {
		sendResponse(req, err)

		return
	}

	// create a slot and try adding it
	newSlot := ktypes.NewSlot(req.slotType, cs)

	if !k.slots.AddSlot(newSlot) {
		sendResponse(req, common.ErrSlotsFull)

		return
	}

	ctx, cancelFn := context.WithCancel(ctx)
	defer func() {
		// delete the slot from the slots queue
		cancelFn()

		slot := k.slots.GetSlot(clusterID)
		k.slots.CleanupSlot(clusterID)

		if slot != nil {
			if slot.SlotType == ktypes.OperatorSlot {
				k.metrics.captureAvailableOperatorSlots(1)
			} else {
				k.metrics.captureAvailableValidatorSlots(1)
			}
		}

		k.exec.Cleanup(clusterID)
	}()

	if req.slotType == ktypes.OperatorSlot {
		k.metrics.captureAvailableOperatorSlots(-1)
	} else {
		k.metrics.captureAvailableValidatorSlots(-1)
	}

	if err = k.validateInteractions(req.ixs); err != nil {
		k.logger.Error("Invalid interaction", "err", err)

		sendResponse(req, common.ErrInvalidInteractions)

		return
	}

	switch req.slotType {
	case ktypes.OperatorSlot:
		err = k.acquireContextLock(ctx, newSlot)

		sendResponse(req, err)

		if err != nil {
			k.logger.Debug("Error acquiring context lock", "err", err, "cluster-ID", clusterID)
			k.metrics.captureICSCreationFailureCount(1)

			return
		}

		k.logger.Info("Cluster creation successful", "cluster-ID", clusterID)

	case ktypes.ValidatorSlot:
		requestTime := time.Now()

		err = k.joinCluster(ctx, newSlot)
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
		case _, ok := <-slot.ICSSuccessChan:
			if ok {
				k.metrics.captureICSJoiningTime(requestTime)
			}
		case <-time.After(ICSTimeOutDuration):
			k.logger.Info("ICS success timeout", "cluster-ID", req.msg.ClusterID)
			k.metrics.captureICSParticipationFailureCount(1)

			return
		}
	}

	k.logger.Trace("Sending execution request")
	// Send execution request
	slot := k.slots.GetSlot(clusterID)
	if slot == nil {
		k.logger.Info("Nil slot", "cluster-ID", req.msg.ClusterID)

		return
	}

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
		cs.SetGrid(execResp.Grid)
		k.lattice.AddKnownHashes(execResp.Grid)

		consensusInitTS := time.Now()
		k.metrics.captureClusterSize(float64(cs.Size()))

		ixHash, err := cs.Ixs.Hash()
		if err != nil {
			return
		}

		icsEvidence := kbft.NewEvidence(ixHash, cs.Operator, cs.Size())

		bft := kbft.NewKBFTService(
			ctx,
			kbft.MaxBFTimeout,
			k.selfID,
			k.cfg,
			slot.BftOutboundChan,
			slot.BftInboundChan,
			k.vault,
			cs,
			k.finalizedTesseractHandler,
			kbft.WithLogger(k.logger.With("cluster-ID", clusterID)),
			kbft.WithWal(k.wal),
			kbft.WithEvidence(icsEvidence),
		)

		if err = bft.Start(); err != nil {
			k.logger.Error("Consensus failed", "err", err, "cluster-ID", cs.ClusterID)
			k.metrics.captureAgreementFailureCount(1)
			if err := k.exec.Revert(cs.ClusterID); err != nil {
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

func (k *Engine) sendICSRequestWithBound(
	parentContext context.Context,
	setType common.IcsSetType,
	requiredCount int,
	finalWaitGroup *sync.WaitGroup,
	cID common.ClusterID,
	nodes []kramaid.KramaID,
	keys [][]byte,
	msg networkmsg.CanonicalICSRequest,
) {
	ctx, span := tracing.Span(parentContext, "KramaEngine", fmt.Sprintf("ics request set-type %d", setType))
	defer func() {
		span.End()
		finalWaitGroup.Done()
	}()

	var (
		wg            sync.WaitGroup
		nodeResponses = make([]bool, len(nodes))
		slot          = k.slots.GetSlot(cID)
	)

	wg.Add(len(nodes))

	msg.ContextType = int32(setType)

	rawCanonicalICSReq, err := msg.Bytes()
	if err != nil {
		k.logger.Error("Failed to send ICS request", "err", err)

		return
	}

	signature, err := k.vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	if err != nil {
		k.logger.Error("Failed to sign ICS request", "err", err)

		return
	}

	icsRequest := networkmsg.NewICSRequest(rawCanonicalICSReq, signature)

	for index, kramaID := range nodes {
		if kramaID == slot.ClusterState().Operator {
			nodeResponses[index] = true

			wg.Done()

			continue
		}

		go func(index int, kramaID kramaid.KramaID) {
			icsResponse := new(networkmsg.ICSResponse)
			requestTS := time.Now()

			reqCtx, cancelFn := context.WithTimeout(ctx, config.DefaultICSRequestTimeout)
			defer func() {
				k.metrics.captureRequestTurnaroundTime(requestTS)
				cancelFn()
				wg.Done()
			}()

			err := k.transport.Call(
				reqCtx,
				kramaID,
				"ICSRPC",
				"ICSRequest",
				icsRequest,
				icsResponse,
			)
			if err != nil {
				k.logger.Error("ICS request failed", "err", err, "krama-ID", kramaID)

				return
			}

			if icsResponse.StatusCode == networkmsg.Success {
				// Add the nodeResponses array to capture the success response
				nodeResponses[index] = true

				return
			}

			k.logger.Debug(
				"ICS response",
				"status", icsResponse.StatusCode,
				"krama-ID", kramaID,
			)
		}(index, kramaID)
	}

	wg.Wait()

	idSet := common.NewNodeSet(nodes, keys)

	for index, isAvailable := range nodeResponses {
		if isAvailable {
			idSet.Responses.SetIndex(index, true)
			idSet.RespCount++
		}
	}

	idSet.QuorumSize = requiredCount

	slot.ClusterState().UpdateNodeSet(setType, idSet)
}

func (k *Engine) sendICSRequest(
	ctx context.Context,
	setType common.IcsSetType,
	finalWaitGroup *sync.WaitGroup,
	cID common.ClusterID,
	nodesSet *common.NodeSet,
	msg networkmsg.CanonicalICSRequest,
	randomNodes chan []kramaid.KramaID,
) {
	_, span := tracing.Span(ctx, "KramaEngine", fmt.Sprintf("ics request set-type %d", setType))
	defer func() {
		span.End()
		finalWaitGroup.Done()
	}()

	if nodesSet == nil {
		k.logger.Trace("Returning from ICS request routine", "set-type", setType)

		return
	}

	var (
		wg            sync.WaitGroup
		nodeResponses = make([]bool, len(nodesSet.Ids))
		slot          = k.slots.GetSlot(cID)
	)

	wg.Add(len(nodesSet.Ids))

	msg.ContextType = int32(setType)

	rawCanonicalICSReq, err := msg.Bytes()
	if err != nil {
		k.logger.Error("Failed to send ICS request", "err", err)

		return
	}

	signature, err := k.vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	if err != nil {
		k.logger.Error("Failed to sign ICS request", "err", err)

		return
	}

	icsRequest := networkmsg.NewICSRequest(rawCanonicalICSReq, signature)

	for index, kramaID := range nodesSet.Ids {
		if kramaID == slot.ClusterState().Operator {
			nodeResponses[index] = true

			slot.ClusterState().IncludeOperator()

			wg.Done()

			continue
		}

		go func(index int, kramaID kramaid.KramaID) {
			icsResponse := new(networkmsg.ICSResponse)
			requestTS := time.Now()

			reqCtx, cancelFn := context.WithTimeout(ctx, config.DefaultICSRequestTimeout)
			defer func() {
				k.metrics.captureRequestTurnaroundTime(requestTS)

				cancelFn()
				wg.Done()
			}()

			err := k.transport.Call(
				reqCtx,
				kramaID,
				"ICSRPC",
				"ICSRequest",
				icsRequest,
				icsResponse,
			)
			if err != nil {
				k.logger.Error("ICS request failed", "err", err, "krama-ID", kramaID)

				return
			}

			if icsResponse.StatusCode == networkmsg.Success {
				// Update the nodeResponses array to capture the success response
				nodeResponses[index] = true
				randomNodes <- utils.KramaIDFromString(icsResponse.RandomNodes)

				return
			}

			k.logger.Debug(
				"ICS Response",
				"status", icsResponse.StatusCode,
				"krama-ID", kramaID)
		}(index, kramaID)
	}

	wg.Wait()

	for index, isAvailable := range nodeResponses {
		if isAvailable {
			nodesSet.Responses.SetIndex(index, true)
			nodesSet.RespCount++
		}
	}

	slot.ClusterState().UpdateNodeSet(setType, nodesSet)
}

func (k *Engine) getCanonicalICSReqMsg(
	cs *ktypes.ClusterState,
) (networkmsg.CanonicalICSRequest, error) {
	canonicalICSReqMsg := new(networkmsg.CanonicalICSRequest)

	rawData, err := cs.Ixs.Bytes()
	if err != nil {
		return *canonicalICSReqMsg, err
	}

	canonicalICSReqMsg.IxData = rawData
	canonicalICSReqMsg.ClusterID = string(cs.ClusterID)
	canonicalICSReqMsg.Operator = string(k.selfID)
	canonicalICSReqMsg.ContextLock = cs.ContextLock()
	canonicalICSReqMsg.Timestamp = cs.ICSReqTime.UnixNano()

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

func (k *Engine) sendICSSuccess(id common.ClusterID) error {
	slot := k.slots.GetSlot(id)

	if slot == nil {
		return errors.New("nil slot")
	}

	var (
		clusterState = slot.ClusterState()
		msg          = clusterState.CreateICSSuccessMsg()
	)

	rawData, err := msg.Bytes()
	if err != nil {
		return err
	}

	icsMsg := ktypes.NewICSMsg(networkmsg.ICSSUCCESS, string(id), rawData)

	k.logger.Trace("Sending cluster state success message", "cluster-ID", id)

	slot.OutboundChan <- icsMsg

	clusterState.SuccessMsg = icsMsg

	return nil
}

func (k *Engine) initClusterCommunication(ctx context.Context, slot *ktypes.Slot) error {
	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "initClusterCommunication")
	defer span.End()

	if err := k.transport.InitClusterCommunication(ctx, slot); err != nil {
		return err
	}

	k.startMessageHandlers(ctx, slot)

	return nil
}

func (k *Engine) createProposalGrid(slot *ktypes.Slot) ([]*common.Tesseract, error) {
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

	return GenerateTesseracts(clusterState)
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

func (k *Engine) finalizedTesseractHandler(tesseracts []*common.Tesseract) error {
	if len(tesseracts) == 0 {
		return errors.New("failed to finalize tesseracts")
	}

	clusterID := tesseracts[0].ClusterID()

	slot := k.slots.GetSlot(clusterID)

	if slot == nil {
		return errors.New("nil slot")
	}

	clusterInfo := slot.ClusterState()

	if err := k.lattice.AddTesseracts(clusterInfo.GetDirty(), tesseracts...); err != nil {
		return err
	}

	rawIxns, err := tesseracts[0].Interactions().Bytes()
	if err != nil {
		return err
	}

	rawReceipts, err := tesseracts[0].Receipts().Bytes()
	if err != nil {
		return err
	}

	for _, ts := range tesseracts {
		msg := &networkmsg.TesseractMessage{
			RawTesseract: make([]byte, 0),
			Sender:       k.selfID,
			Ixns:         rawIxns,
			Receipts:     rawReceipts,
			Delta: map[common.Hash][]byte{
				ts.ICSHash(): clusterInfo.GetDirty()[ts.ICSHash()],
			},
		}

		msg.RawTesseract, err = ts.Canonical().Bytes()
		if err != nil {
			return err
		}

		if err = k.transport.BroadcastTesseract(msg); err != nil {
			k.logger.Error("Failed to broadcast tesseract", "err", err, "cluster-ID", clusterID)
		}
	}

	return nil
}

func generateBody(
	addr identifiers.Address,
	state *ktypes.ClusterState,
	ixnsHash, receiptHash common.Hash,
) common.TesseractBody {
	return common.TesseractBody{
		StateHash:       state.GetStateHash(addr),
		ContextHash:     state.GetContextHash(addr),
		ContextDelta:    state.GetContextDelta(),
		InteractionHash: ixnsHash,
		ReceiptHash:     receiptHash,
		ConsensusProof: common.PoXtData{
			BinaryHash:   state.BinaryHash,
			IdentityHash: state.IdentityHash,
			ICSHash:      state.ICSHash,
		},
	}
}

func generateTesseract(
	addr identifiers.Address,
	state *ktypes.ClusterState,
	body common.TesseractBody,
	tsBodyHash, gridHash common.Hash,
	fuelUsed, fuelLimit uint64,
	sealer kramaid.KramaID,
) *common.Tesseract {
	header := common.TesseractHeader{
		Address:     addr,
		ContextLock: state.ContextLock(),
		PrevHash:    state.AccountInfos.GetLatestHash(addr),
		Height:      state.NewHeight(addr),
		FuelUsed:    fuelUsed,
		FuelLimit:   fuelLimit,
		ClusterID:   string(state.ClusterID),
		Operator:    string(state.Operator),
		BodyHash:    tsBodyHash,
		GroupHash:   gridHash,
		Extra: common.CommitData{
			VoteSet:         nil,
			CommitSignature: nil,
		},
		Timestamp: state.ICSReqTime.UnixNano(),
	}

	return common.NewTesseract(header, body, state.Ixs, state.Receipts, nil, sealer)
}

func GenerateTesseracts(state *ktypes.ClusterState) ([]*common.Tesseract, error) {
	ix := state.Ixs[0] // TODO: Improve this

	fuelUsed := state.GetFuelUsed()
	fuelLimit := uint64(1000)

	groupBuffer := make([]byte, 0)
	tesseractGroup := make([]*common.Tesseract, 0)

	ixnsHash, err := state.Ixs.Hash()
	if err != nil {
		return nil, err
	}

	receiptHash, err := state.Receipts.Hash()
	if err != nil {
		return nil, err
	}

	var (
		senderBody       common.TesseractBody
		receiverBody     common.TesseractBody
		genesisBody      common.TesseractBody
		senderBodyHash   common.Hash
		receiverBodyHash common.Hash
		genesisBodyHash  common.Hash
	)

	if !ix.Sender().IsNil() {
		senderBody = generateBody(ix.Sender(), state, ixnsHash, receiptHash)

		senderBodyHash, err = senderBody.Hash()
		if err != nil {
			return nil, err
		}

		groupBuffer = append(groupBuffer, senderBodyHash.Bytes()...)
	}

	if !ix.Receiver().IsNil() {
		receiverBody = generateBody(ix.Receiver(), state, ixnsHash, receiptHash)

		receiverBodyHash, err = receiverBody.Hash()
		if err != nil {
			return nil, err
		}

		groupBuffer = append(groupBuffer, receiverBodyHash.Bytes()...)

		if state.AccountInfos.IsGenesis(ix.Receiver()) {
			genesisBody = generateBody(common.SargaAddress, state, ixnsHash, receiptHash)

			genesisBodyHash, err = genesisBody.Hash()
			if err != nil {
				return nil, err
			}

			groupBuffer = append(groupBuffer, genesisBodyHash.Bytes()...)
		}
	}

	groupHash := blake2b.Sum256(groupBuffer)

	if !ix.Sender().IsNil() {
		tesseractGroup = append(tesseractGroup, // append sender tesseract
			generateTesseract(
				ix.Sender(),
				state,
				senderBody,
				senderBodyHash,
				groupHash,
				fuelUsed,
				fuelLimit,
				state.SelfKramaID()),
		)
	}

	if !ix.Receiver().IsNil() {
		tesseractGroup = append(tesseractGroup, // append receiver tesseract
			generateTesseract(
				ix.Receiver(),
				state,
				receiverBody,
				receiverBodyHash,
				groupHash,
				fuelUsed,
				fuelLimit,
				state.SelfKramaID()),
		)

		if state.AccountInfos.IsGenesis(ix.Receiver()) {
			tesseractGroup = append(tesseractGroup, // append sarga tesseract
				generateTesseract(common.SargaAddress,
					state,
					genesisBody,
					genesisBodyHash,
					groupHash,
					fuelUsed,
					fuelLimit,
					state.SelfKramaID(),
				))
		}
	}

	return tesseractGroup, nil
}

func (k *Engine) executionRoutine() {
	for clusterID := range k.executionReq {
		k.logger.Trace("Processing an execution request")

		go func(id common.ClusterID) {
			slotInfo := k.slots.GetSlot(id)
			grid, err := k.createProposalGrid(slotInfo)
			slotInfo.ExecutionResp <- ktypes.ExecutionResponse{Grid: grid, Err: err}
		}(clusterID)
	}
}

func (k *Engine) Close() {
	k.wal.Close()
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
func (k *Engine) Requests() chan Request {
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

func sendResponse(req Request, err error) {
	req.responseChan <- Response{slotType: req.slotType, err: err}
}
