package krama

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/moby/locker"
	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/moichain/common"

	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/ixpool"
	"github.com/sarvalabs/moichain/krama/kbft"
	"github.com/sarvalabs/moichain/krama/observer"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/mudra"
	mudraCommon "github.com/sarvalabs/moichain/mudra/common"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/flux"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/telemetry/tracing"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
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

type lattice interface {
	AddKnownHashes(tesseracts []*types.Tesseract)
	AddTesseracts(dirtyStorage map[types.Hash][]byte, tesseracts ...*types.Tesseract) error
}

type transport interface {
	InitClusterCommunication(ctx context.Context, slot *ktypes.Slot) error
	RegisterRPCService(serviceID protocol.ID, serviceName string, service interface{}) error
	Call(ctx context.Context, kramaID id.KramaID, svcName, svcMethod string, args, response interface{}) error
	BroadcastTesseract(msg *ptypes.TesseractMessage) error
}

type state interface {
	FetchInteractionContext(
		ctx context.Context,
		ix *types.Interaction,
	) (
		map[types.Address]types.Hash,
		[]*types.NodeSet,
		error,
	)
	GetPublicKeys(ids ...id.KramaID) (keys [][]byte, err error)
	GetAccountMetaInfo(addr types.Address) (*types.AccountMetaInfo, error)
	IsAccountRegistered(addr types.Address) (bool, error)
	GetLatestStateObject(addr types.Address) (*guna.StateObject, error)
	GetNonce(addr types.Address, stateHash types.Hash) (uint64, error)
}

type ixPool interface {
	IncrementWaitTime(addr types.Address, baseTime time.Duration) error
	Executables() ixpool.InteractionQueue
	Drop(ix *types.Interaction)
}

type execution interface {
	ExecuteInteractions(types.ClusterID, types.Interactions, types.ContextDelta) (types.Receipts, error)
	Revert(types.ClusterID) error
	Cleanup(types.ClusterID)
}

type Response struct {
	slotType ktypes.SlotType
	err      error
}

type Request struct {
	ixs          types.Interactions
	msg          *ptypes.CanonicalICSRequest
	operator     id.KramaID
	slotType     ktypes.SlotType
	reqTime      time.Time
	responseChan chan Response
}

func (r *Request) IxHash() types.Hash {
	return r.ixs[0].Hash()
}

func (r *Request) getClusterID() (types.ClusterID, error) {
	switch r.slotType {
	case ktypes.OperatorSlot:
		return generateClusterID()
	case ktypes.ValidatorSlot:
		return types.ClusterID(r.msg.ClusterID), nil
	default:
		return "", errors.New("invalid request type")
	}
}

type Engine struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	cfg          *common.ConsensusConfig
	mux          *utils.TypeMux
	logger       hclog.Logger
	selfID       id.KramaID
	slots        *ktypes.Slots
	requests     chan Request
	randomizer   *flux.Randomizer
	transport    transport
	exec         execution
	pool         ixPool
	state        state
	executionReq chan types.ClusterID
	lattice      lattice
	wal          kbft.WAL
	vault        *mudra.KramaVault
	clusterLocks *locker.Locker
	metrics      *Metrics
	avgICSTime   time.Duration
}

func NewKramaEngine(ctx context.Context,
	cfg *common.ConsensusConfig,
	logger hclog.Logger,
	mux *utils.TypeMux,
	state state,
	network network,
	exec execution,
	ixPool ixPool,
	val *mudra.KramaVault,
	lattice lattice,
	randomizer *flux.Randomizer,
	metrics *Metrics,
	slots *ktypes.Slots,
) (*Engine, error) {
	wal, err := kbft.NewWAL(ctx, logger, cfg.DirectoryPath)
	if err != nil {
		return nil, errors.Wrap(err, "WAL failed")
	}

	ctx, ctxCancel := context.WithCancel(ctx)
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
		executionReq: make(chan types.ClusterID),
		wal:          wal,
		vault:        val,
		clusterLocks: locker.New(),
		metrics:      metrics,
		avgICSTime:   cfg.AccountWaitTime,
	}

	k.metrics.initMetrics(float64(cfg.OperatorSlotCount), float64(cfg.ValidatorSlotCount))

	return k, k.transport.RegisterRPCService(common.ICSProtocolRPC, "ICSRPC", NewICSRPCService(k))
}

// loadIxnClusterState fetches the account state and returns the interaction cluster state
func (k *Engine) loadIxnClusterState(
	ctx context.Context,
	req Request,
	clusterID types.ClusterID,
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
		contextRandomNodes  []id.KramaID
		operatorRandomNodes []id.KramaID
		observerNodes       []id.KramaID
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
	randomNodesReceiverChan := make(chan []id.KramaID)

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
		types.SenderBehaviourSet,
		finalWaitGroup,
		slot.ClusterID(),
		nodeSets[types.SenderBehaviourSet],
		reqMsg,
		randomNodesReceiverChan,
	)

	go k.sendICSRequest(
		ctx,
		types.SenderRandomSet,
		finalWaitGroup,
		slot.ClusterID(),
		nodeSets[types.SenderRandomSet],
		reqMsg,
		randomNodesReceiverChan,
	)

	if !slot.ClusterState().Ixs[0].Receiver().IsNil() {
		finalWaitGroup.Add(2)

		go k.sendICSRequest(
			ctx,
			types.ReceiverBehaviourSet,
			finalWaitGroup,
			slot.ClusterID(),
			nodeSets[types.ReceiverBehaviourSet],
			reqMsg,
			randomNodesReceiverChan,
		)

		go k.sendICSRequest(
			ctx,
			types.ReceiverRandomSet,
			finalWaitGroup,
			slot.ClusterID(),
			nodeSets[types.ReceiverRandomSet],
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
		operatorRandomNodes = append([]id.KramaID{k.selfID}, operatorRandomNodes...) // TODO:Improve this
	}

	exemptedNodes := respondedEligibleSet
	exemptedNodes = append(exemptedNodes, operatorRandomNodes...)

	observerNodes, err = k.getObserverNodes(ctx, observerNodesQueryCount, exemptedNodes)
	if err != nil {
		k.logger.Error("Error fetching observer nodes", "err", err)

		return errors.New("unable to retrieve observer nodes")
	}

	finalWaitGroup.Add(2)

	observerKeys, err := k.state.GetPublicKeys(observerNodes...)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch the public key of observer nodes.")
	}

	randomKeys, err := k.state.GetPublicKeys(operatorRandomNodes...)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch the public key of random nodes.")
	}

	go k.sendICSRequestWithBound(
		ctx,
		types.RandomSet,
		operatorRandomNodesCount,
		finalWaitGroup,
		slot.ClusterID(),
		operatorRandomNodes,
		randomKeys,
		reqMsg,
	)

	go k.sendICSRequestWithBound(
		ctx,
		types.ObserverSet,
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

func generateClusterID() (types.ClusterID, error) {
	randHash := make([]byte, 32)

	if _, err := rand.Read(randHash); err != nil {
		return "", err
	}

	return types.ClusterID(base58.Encode(randHash)), nil
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

	slot.ClusterState().CurrentRole = types.IcsSetType(slot.ICSRequestMsg().ContextType)

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
			return types.ErrHeightMismatch
		}

		if info.TesseractHash != slot.ClusterState().AccountInfos.GetLatestHash(addr) {
			return types.ErrHashMismatch
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
		sendResponse(req, types.ErrSlotsFull)

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
		sendResponse(req, types.ErrSlotsFull)

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

		sendResponse(req, types.ErrInvalidInteractions)

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

	if cs.CurrentRole == types.ObserverSet {
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
			k.logger.Info("Error executing interactions", "err", execResp.Err, "cluster-ID", clusterID)
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

func (k *Engine) fetchIxAccounts(ctx context.Context, ix *types.Interaction) (ktypes.AccountInfos, error) {
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

	genesisAccInfo, err := k.state.GetAccountMetaInfo(types.SargaAddress)
	if err != nil {
		return nil, err
	}

	acc := &ktypes.AccountInfo{
		Address:       ix.Receiver(),
		AccType:       types.AccTypeFromIxType(ix.Type()),
		TesseractHash: types.NilHash,
		Height:        0,
		IsGenesis:     true,
	}

	accounts[types.SargaAddress] = ktypes.AccountInfoFromAccMetaInfo(genesisAccInfo, false)
	accounts[ix.Receiver()] = acc

	return accounts, nil
}

func (k *Engine) sendICSRequestWithBound(
	parentContext context.Context,
	setType types.IcsSetType,
	requiredCount int,
	finalWaitGroup *sync.WaitGroup,
	cID types.ClusterID,
	nodes []id.KramaID,
	keys [][]byte,
	msg ptypes.CanonicalICSRequest,
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

	signature, err := k.vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, mudra.UsingNetworkKey())
	if err != nil {
		k.logger.Error("Failed to sign ICS request", "err", err)

		return
	}

	icsRequest := ptypes.NewICSRequest(rawCanonicalICSReq, signature)

	for index, kramaID := range nodes {
		if kramaID == slot.ClusterState().Operator {
			nodeResponses[index] = true

			wg.Done()

			continue
		}
		// Retrieve the peerID from the node's Krama id
		networkID, err := kramaID.PeerID()
		if err != nil {
			k.logger.Error("Error decoding network ID from krama ID", "err", err)
			wg.Done()

			continue
		}

		peerID, err := peer.Decode(networkID)
		if err != nil {
			k.logger.Error("Unable to decode peer ID", "err", err)
			wg.Done()

			continue
		}

		go func(index int, kramaID id.KramaID) {
			icsResponse := new(ptypes.ICSResponse)
			requestTS := time.Now()

			if err = k.transport.Call(
				ctx,
				kramaID,
				"ICSRPC",
				"ICSRequest",
				icsRequest,
				icsResponse,
			); err == nil {
				if icsResponse.StatusCode == ptypes.Success {
					// Add the nodeResponses array to capture the success response
					nodeResponses[index] = true
				} else {
					k.logger.Debug(
						"ICS random nodes request failed",
						"err", err,
						"status", icsResponse.StatusCode,
						"peer-ID", peerID,
					)
				}
			} else {
				k.logger.Error("ICS random nodes request failed", "err", err)
			}

			//	clusterState.updateResponseTimeMetric(reqTimeStamp)
			k.metrics.captureRequestTurnaroundTime(requestTS)
			// Decrement the wait group
			wg.Done()
		}(index, kramaID)
	}

	wg.Wait()

	idSet := types.NewNodeSet(nodes, keys)

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
	setType types.IcsSetType,
	finalWaitGroup *sync.WaitGroup,
	cID types.ClusterID,
	nodesSet *types.NodeSet,
	msg ptypes.CanonicalICSRequest,
	randomNodes chan []id.KramaID,
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

	signature, err := k.vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, mudra.UsingNetworkKey())
	if err != nil {
		k.logger.Error("Failed to sign ICS request", "err", err)

		return
	}

	icsRequest := ptypes.NewICSRequest(rawCanonicalICSReq, signature)

	for index, kramaID := range nodesSet.Ids {
		if kramaID == slot.ClusterState().Operator {
			nodeResponses[index] = true

			slot.ClusterState().IncludeOperator()

			wg.Done()

			continue
		}

		networkID, err := kramaID.PeerID()
		if err != nil {
			k.logger.Error("Error decoding network ID from krama ID", "err", err)
			wg.Done()

			continue
		}

		peerID, err := peer.Decode(networkID)
		if err != nil {
			k.logger.Error("Unable to decode peer ID", "err", err)
			wg.Done()

			continue
		}

		go func(index int, kramaID id.KramaID) {
			icsResponse := new(ptypes.ICSResponse)
			requestTS := time.Now()

			if err := k.transport.Call(
				ctx,
				kramaID,
				"ICSRPC",
				"ICSRequest",
				icsRequest,
				icsResponse,
			); err == nil && icsResponse.StatusCode == ptypes.Success {
				// Update the nodeResponses array to capture the success response
				nodeResponses[index] = true
				randomNodes <- utils.KramaIDFromString(icsResponse.RandomNodes)
			} else {
				k.logger.Info(
					"ICS request failed",
					"err", err,
					"status", icsResponse.StatusCode,
					"peer-ID", peerID)
			}

			//	clusterState.updateResponseTimeMetric(reqTimeStamp)
			k.metrics.captureRequestTurnaroundTime(requestTS)

			// Decrement the wait group
			wg.Done()
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
) (ptypes.CanonicalICSRequest, error) {
	canonicalICSReqMsg := new(ptypes.CanonicalICSRequest)

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

func (k *Engine) getRandomNodes(ctx context.Context, count int, exemptedNodes []id.KramaID) ([]id.KramaID, error) {
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

func (k *Engine) getObserverNodes(ctx context.Context, count int, exemptedNodes []id.KramaID) ([]id.KramaID, error) {
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

func (k *Engine) sendICSSuccess(id types.ClusterID) error {
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

	icsMsg := ktypes.NewICSMsg(ptypes.ICSSUCCESS, string(id), rawData)

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

func (k *Engine) createProposalGrid(slot *ktypes.Slot) ([]*types.Tesseract, error) {
	if err := k.updateContextDelta(slot); err != nil {
		return nil, err
	}

	clusterState := slot.ClusterState()

	_, err := clusterState.ComputeICSHash()
	if err != nil {
		return nil, err
	}

	receipts, err := k.exec.ExecuteInteractions(
		clusterState.ClusterID,
		clusterState.Ixs,
		clusterState.GetContextDelta(),
	)
	if err != nil {
		return nil, err
	}
	// store the receipts
	clusterState.SetReceipts(receipts)
	k.logger.Debug("Generating tesseracts", "cluster-ID", slot.ClusterID())

	return GenerateTesseracts(clusterState)
}

func (k *Engine) updateContextDelta(slot *ktypes.Slot) error {
	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.ClusterState()
	seenAccounts := make(map[types.Address]bool)
	deltaMap := make(types.ContextDelta)

	for _, ix := range clusterState.Ixs {
		senderAddr := ix.Sender()
		receiverAddr := ix.Receiver()

		if !senderAddr.IsNil() && !seenAccounts[senderAddr] {
			senderDeltaGroup := new(types.DeltaGroup)
			senderDeltaGroup.Role = types.Sender
			senderBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
				types.SenderBehaviourSet,
			)

			if senderBehaviourDelta != "" {
				senderDeltaGroup.BehaviouralNodes = append(senderDeltaGroup.BehaviouralNodes, senderBehaviourDelta)
			}

			if replacedNodes != "" {
				senderDeltaGroup.ReplacedNodes = append(senderDeltaGroup.ReplacedNodes, replacedNodes)
			}

			senderRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
				types.SenderRandomSet,
				1,
				clusterState.Operator,
			)
			senderDeltaGroup.RandomNodes = append(senderDeltaGroup.RandomNodes, senderRandomDelta...)
			senderDeltaGroup.ReplacedNodes = append(senderDeltaGroup.ReplacedNodes, replacedRandomDelta...)
			seenAccounts[senderAddr] = true
			deltaMap[senderAddr] = senderDeltaGroup
		}

		if !receiverAddr.IsNil() && !seenAccounts[receiverAddr] {
			receiverDeltaGroup := new(types.DeltaGroup)
			receiverDeltaGroup.Role = types.Receiver

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

				// Fetch sarga account context delta
				genesisDeltaGroup := new(types.DeltaGroup)
				genesisDeltaGroup.Role = types.Genesis
				genesisBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
					types.ReceiverBehaviourSet,
				)

				if genesisBehaviourDelta != "" {
					genesisDeltaGroup.BehaviouralNodes = append(genesisDeltaGroup.BehaviouralNodes, genesisBehaviourDelta)
				}

				if replacedNodes != "" {
					genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedNodes)
				}

				genesisRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
					types.ReceiverRandomSet,
					1,
				)
				genesisDeltaGroup.RandomNodes = append(genesisDeltaGroup.RandomNodes, genesisRandomDelta...)
				genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedRandomDelta...)
				seenAccounts[types.SargaAddress] = true
				deltaMap[types.SargaAddress] = genesisDeltaGroup
			} else if clusterState.AccountInfos[receiverAddr].AccType == types.LogicAccount ||
				clusterState.AccountInfos[receiverAddr].AccType == types.AssetAccount {
				receiverBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
					types.ReceiverBehaviourSet,
				)
				if receiverBehaviourDelta != "" {
					receiverDeltaGroup.BehaviouralNodes = append(receiverDeltaGroup.BehaviouralNodes, receiverBehaviourDelta)
				}
				if replacedNodes != "" {
					receiverDeltaGroup.ReplacedNodes = append(receiverDeltaGroup.ReplacedNodes, replacedNodes)
				}
				receiverRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
					types.ReceiverRandomSet,
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

func (k *Engine) GetNodes(
	clusterInfo *ktypes.ClusterState,
	requiredRandomNodes,
	requiredBehaviouralNodes int,
) (behaviouralNodes []id.KramaID, randomNodes []id.KramaID, err error) {
	// TODO: Need to improve this function
	set := clusterInfo.NodeSet.Nodes[types.RandomSet]
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

func (k *Engine) finalizedTesseractHandler(tesseracts []*types.Tesseract) error {
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
		msg := &ptypes.TesseractMessage{
			RawTesseract: make([]byte, 0),
			Sender:       k.selfID,
			Ixns:         rawIxns,
			Receipts:     rawReceipts,
			Delta: map[types.Hash][]byte{
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
	addr types.Address,
	state *ktypes.ClusterState,
	ixHash, ixnsHash, receiptHash types.Hash,
) types.TesseractBody {
	return types.TesseractBody{
		StateHash:       state.GetStateHash(ixHash, addr),
		ContextHash:     state.GetContextHash(ixHash, addr),
		ContextDelta:    state.GetContextDelta(),
		InteractionHash: ixnsHash,
		ReceiptHash:     receiptHash,
		ConsensusProof: types.PoXCData{
			BinaryHash:   state.BinaryHash,
			IdentityHash: state.IdentityHash,
			ICSHash:      state.ICSHash,
		},
	}
}

func generateTesseract(
	addr types.Address,
	state *ktypes.ClusterState,
	body types.TesseractBody,
	tsBodyHash, gridHash types.Hash,
	fuelUsed, fuelLimit *big.Int,
	sealer id.KramaID,
) *types.Tesseract {
	header := types.TesseractHeader{
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
		Extra: types.CommitData{
			VoteSet:         nil,
			CommitSignature: nil,
		},
		Timestamp: state.ICSReqTime.UnixNano(),
	}

	return types.NewTesseract(header, body, state.Ixs, state.Receipts, nil, sealer)
}

func GenerateTesseracts(state *ktypes.ClusterState) ([]*types.Tesseract, error) {
	ix := state.Ixs[0] // TODO: Improve this
	fuelUsed := state.GetFuelUsed()
	groupBuffer := make([]byte, 0)
	tesseractGroup := make([]*types.Tesseract, 0)

	ixnsHash, err := state.Ixs.Hash()
	if err != nil {
		return nil, err
	}

	receiptHash, err := state.Receipts.Hash()
	if err != nil {
		return nil, err
	}

	var (
		senderBody       types.TesseractBody
		receiverBody     types.TesseractBody
		genesisBody      types.TesseractBody
		senderBodyHash   types.Hash
		receiverBodyHash types.Hash
		genesisBodyHash  types.Hash
	)

	if !ix.Sender().IsNil() {
		senderBody = generateBody(ix.Sender(), state, ix.Hash(), ixnsHash, receiptHash)

		senderBodyHash, err = senderBody.Hash()
		if err != nil {
			return nil, err
		}

		groupBuffer = append(groupBuffer, senderBodyHash.Bytes()...)
	}

	if !ix.Receiver().IsNil() {
		receiverBody = generateBody(ix.Receiver(), state, ix.Hash(), ixnsHash, receiptHash)

		receiverBodyHash, err = receiverBody.Hash()
		if err != nil {
			return nil, err
		}

		groupBuffer = append(groupBuffer, receiverBodyHash.Bytes()...)

		if state.AccountInfos.IsGenesis(ix.Receiver()) {
			genesisBody = generateBody(types.SargaAddress, state, ix.Hash(), ixnsHash, receiptHash)

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
				big.NewInt(1000),
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
				big.NewInt(1000),
				state.SelfKramaID()),
		)

		if state.AccountInfos.IsGenesis(ix.Receiver()) {
			tesseractGroup = append(tesseractGroup, // append sarga tesseract
				generateTesseract(types.SargaAddress,
					state,
					genesisBody,
					genesisBodyHash,
					groupHash,
					fuelUsed,
					big.NewInt(1000),
					state.SelfKramaID(),
				))
		}
	}

	return tesseractGroup, nil
}

func (k *Engine) executionRoutine() {
	for clusterID := range k.executionReq {
		k.logger.Trace("Processing an execution request")

		go func(id types.ClusterID) {
			slotInfo := k.slots.GetSlot(id)
			grid, err := k.createProposalGrid(slotInfo)
			slotInfo.ExecutionResp <- ktypes.ExecutionResponse{Grid: grid, Err: err}
		}(clusterID)
	}
}

func (k *Engine) Close() {
	defer k.ctxCancel()
}

func (k *Engine) validateInteractions(ixs types.Interactions) error {
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
		latestNonce, err := k.state.GetNonce(ix.Sender(), types.NilHash)
		if err != nil {
			return err
		}

		// validate nonce
		if ix.Nonce() < latestNonce {
			return types.ErrInvalidNonce
		}

		if err = k.IsIxValid(ix); err != nil {
			return err
		}
	}

	return nil
}

// IsIxValid performs validity checks based on the type of interaction
func (k *Engine) IsIxValid(ix *types.Interaction) error {
	if ix.Sender().IsNil() {
		return types.ErrInvalidAddress
	}

	if accountRegistered, err := k.state.IsAccountRegistered(ix.Sender()); err != nil || !accountRegistered {
		return errors.New("account not found")
	}

	senderObject, err := k.state.GetLatestStateObject(ix.Sender())
	if err != nil {
		return err
	}

	fuelAvailable, err := senderObject.HasFuel(new(big.Int).Add(ix.MOITokenValue(), ix.FuelLimit()))
	if err != nil {
		k.logger.Error("Failed to fetch fuel", "err", err)
	}

	if !fuelAvailable {
		return types.ErrInsufficientFunds
	}

	switch ix.Type() {
	case types.IxValueTransfer:
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

	case types.IxAssetCreate:
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

		assetID := types.NewAssetIDv0(
			payload.Create.IsLogical,
			payload.Create.IsStateFul,
			payload.Create.Dimension,
			payload.Create.Standard,
			ix.Receiver(),
		)

		if _, err = stateObject.GetRegistryEntry(assetID.String()); err == nil {
			return errors.New("asset already found")
		}

	case types.IxLogicDeploy, types.IxLogicInvoke, types.IxAssetMint, types.IxAssetBurn:
		return nil
	default:
		return types.ErrInvalidInteractionType
	}

	return nil
}

func loadContextLockInfo(
	cs *ktypes.ClusterState,
	hashes map[types.Address]types.Hash,
) {
	for addr, accInfo := range cs.AccountInfos {
		accInfo.ContextHash = hashes[addr]
	}
}

func sendResponse(req Request, err error) {
	req.responseChan <- Response{slotType: req.slotType, err: err}
}
