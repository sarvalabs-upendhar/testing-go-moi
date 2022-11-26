package krama

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"sync"
	"time"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/sarvalabs/moichain/ixpool"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/moby/locker"
	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/krama/kbft"
	"github.com/sarvalabs/moichain/krama/observer"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/mudra"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/flux"
	"github.com/sarvalabs/moichain/telemetry/tracing"
	"github.com/sarvalabs/moichain/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/blake2b"
)

const (
	ICSRPCProtocol         = protocol.ID("moi/rpc/clusterState")
	ObserverCoeff  float64 = 0.1
	// MinMTQ represents the minimum Modulated Trust Quotient
	MinMTQ float64 = 0.3

	// MaxICSSize represents the maximum size of an ClusterInfo
	MaxICSSize float64 = 10

	RandomNodesDelta float64 = 0.2

	ObserverNodesDelta float64 = 0.2

	ICSTimeOutDuration = 4500 * time.Millisecond

	BehaviouralContextSize = 1
	RandomContextSize      = 1
)

type lattice interface {
	AddKnownHashes(tesseracts []*types.Tesseract)
	AddTesseracts(tesseracts []*types.Tesseract, dirtyStorage map[types.Hash][]byte) error
}

type transport interface {
	InitClusterCommunication(ctx context.Context, slot *ktypes.Slot) error
	RegisterRPCService(serviceID protocol.ID, serviceName string, service interface{}) error
	Call(kramaID id.KramaID, svcName, svcMethod string, args, response interface{}) error
	BroadcastTesseract(msg *ptypes.TesseractMessage) error
}

type state interface {
	FetchInteractionContext(
		ctx context.Context,
		ix *types.Interaction,
	) (
		map[types.Address]types.Hash,
		[]*ktypes.NodeSet,
		error,
	)
	GetPublicKeys(ids ...id.KramaID) (keys [][]byte, err error)
	GetAccountMetaInfo(addr types.Address) (*types.AccountMetaInfo, error)
	IsGenesis(addr types.Address) (bool, error)
	GetLatestStateObject(addr types.Address) (*guna.StateObject, error)
	GetLatestNonce(addr types.Address) (uint64, error)
}

type ixPool interface {
	IncrementWaitTime(addr types.Address, baseTime time.Duration) error
	Executables() ixpool.InteractionQueue
	ResetWithInteractions(ixs types.Interactions)
}

type execution interface {
	CleanupExecutorInstances(id types.ClusterID)
	ExecuteInteractions(
		clusterID types.ClusterID,
		ixs []*types.Interaction,
		contextDelta types.ContextDelta,
	) (types.Receipts, error)
	Revert(clusterID types.ClusterID) error
}

type Response struct {
	requestType int
	err         error
}

type Request struct {
	reqType      int
	ixs          types.Interactions
	msg          *ptypes.ICSRequest
	responseChan chan Response
}

func (r *Request) getClusterID(operator id.KramaID) (types.ClusterID, error) {
	switch r.reqType {
	case 0:
		ixHash, err := r.ixs[0].GetIxHash()
		if err != nil {
			return "", err
		}

		return generateClusterID(operator, ixHash)
	case 1:
		return types.ClusterID(r.msg.ClusterID), nil
	default:
		return "", errors.New("invalid request type")
	}
}

type Engine struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	cfg          *common.ConsensusConfig
	logger       hclog.Logger
	operator     id.KramaID
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
	state state,
	network network,
	exec execution,
	ixPool ixPool,
	val *mudra.KramaVault,
	lattice lattice,
	randomizer *flux.Randomizer,
	metrics *Metrics,
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
		operator:     network.GetKramaID(),
		state:        state,
		slots:        ktypes.NewSlots(cfg.OperatorSlotCount, cfg.ValidatorSlotCount),
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

	return k, k.transport.RegisterRPCService(ICSRPCProtocol, "ICSRPC", NewICSRPCService(k))
}

func (k *Engine) AcquireContextLock(ctx context.Context, clusterID types.ClusterID, request Request) error {
	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "AcquireContextLock")
	defer span.End()
	// Create cluster id using operatorID and IxHash
	k.logger.Info("Creating cluster", "Cluster id", clusterID)

	clusterState := ktypes.NewICS(6, request.ixs, clusterID, k.operator, time.Now())

	// create a slot and try adding it
	newSlot := ktypes.NewSlot(ktypes.OperatorSlot, clusterState)

	if !k.slots.AddSlot(clusterID, newSlot) {
		return types.ErrSlotsFull
	}

	k.metrics.captureAvailableOperatorSlots(-1)

	finalWaitGroup := new(sync.WaitGroup)

	finalWaitGroup.Add(2)

	var (
		contextRandomNodes  []id.KramaID
		operatorRandomNodes []id.KramaID
		observerNodes       []id.KramaID
	)

	// Fetch the context nodes of interaction participants
	contextHashes, nodeSets, err := k.state.FetchInteractionContext(ctx, request.ixs[0])
	if err != nil {
		return err
	}
	// Fetch the committed account info of interaction participants
	clusterState.AccountInfos, err = k.fetchIxAccounts(ctx, request.ixs[0])
	if err != nil {
		return err
	}
	// Generate the contextLock
	lockInfo := GetContextLockInfo(clusterState.AccountInfos, contextHashes)
	clusterState.ContextLock = lockInfo
	// Initiate the cluster communication by subscribing to clusterID
	if err = k.initClusterCommunication(ctx, newSlot); err != nil {
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

	clusterState.ICSReqTime = utils.Now()
	// Construct ICS_Request
	reqMsg, err := k.getICSReqMsg(request.ixs[0], lockInfo, clusterID, clusterState.ICSReqTime)
	if err != nil {
		return err
	}

	// Send ClusterInfo Request to context nodes of both participants
	go k.sendICSRequest(
		ctx,
		ktypes.SenderBehaviourSet,
		finalWaitGroup,
		clusterID,
		nodeSets[ktypes.SenderBehaviourSet],
		reqMsg,
		randomNodesReceiverChan,
	)
	go k.sendICSRequest(
		ctx,
		ktypes.SenderRandomSet,
		finalWaitGroup,
		clusterID,
		nodeSets[ktypes.SenderRandomSet],
		reqMsg,
		randomNodesReceiverChan,
	)

	if !request.ixs[0].ToAddress().IsNil() {
		finalWaitGroup.Add(2)

		go k.sendICSRequest(
			ctx,
			ktypes.ReceiverBehaviourSet,
			finalWaitGroup,
			clusterID,
			nodeSets[ktypes.ReceiverBehaviourSet],
			reqMsg,
			randomNodesReceiverChan,
		)

		go k.sendICSRequest(
			ctx,
			ktypes.ReceiverRandomSet,
			finalWaitGroup,
			clusterID,
			nodeSets[ktypes.ReceiverRandomSet],
			reqMsg,
			randomNodesReceiverChan,
		)
	}

	finalWaitGroup.Wait()
	// Check for context quorum
	if !clusterState.IsContextQuorum() {
		return errors.New("context quorum failed")
	}

	respondedEligibleSetSize, respondedEligibleSet := clusterState.RespondedEligibleSet()
	// Calculate the required number of observer nodes in the ClusterInfo cluster based on observer coefficient
	// value and the actual size of the ClusterInfo cluster including the required observer
	requiredObserverNodes := int(math.Ceil(ObserverCoeff * float64(respondedEligibleSetSize)))
	observerNodesQueryCount := requiredObserverNodes + k.observerNodeDelta(respondedEligibleSetSize)

	actualICSSize := 3*respondedEligibleSetSize + requiredObserverNodes
	// Choose the higher value between the user MTQ and the minimum network MTQ and use
	// that Modulated Trust Quotient to calculate the minimum required ClusterInfo cluster size
	mtqSize := math.Max(MinMTQ, 0.5) // get this from interaction
	requiredICSSize := int(math.Ceil(mtqSize * MaxICSSize))

	k.logger.Info("Required ClusterInfo Size", "Size", requiredICSSize)
	k.logger.Info("Actual ClusterInfo Size", "Size", actualICSSize)

	// Determine the number of required random nodes in the ClusterInfo cluster based on
	// the size of the responded eligible set and the number of additional nodes
	// required to satisfy the ClusterInfo cluster size requirement
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

	if !clusterState.IsOperatorIncluded() {
		operatorRandomNodes = append([]id.KramaID{k.operator}, operatorRandomNodes...) // TODO:Improve this
	}

	exemptedNodes := respondedEligibleSet
	exemptedNodes = append(exemptedNodes, operatorRandomNodes...)

	observerNodes, err = k.getObserverNodes(ctx, observerNodesQueryCount, exemptedNodes)
	if err != nil {
		k.logger.Error("error fetching observer nodes", "error", err)

		return errors.New("unable to retrieve observer nodes")
	}

	finalWaitGroup.Add(2)

	observerKeys, err := k.state.GetPublicKeys(observerNodes...)
	if err != nil {
		return types.ErrKramaIDNotFound
	}

	randomKeys, err := k.state.GetPublicKeys(operatorRandomNodes...)
	if err != nil {
		return types.ErrKramaIDNotFound
	}

	go k.sendICSRequestWithBound(
		ctx,
		ktypes.RandomSet,
		operatorRandomNodesCount,
		finalWaitGroup,
		clusterID,
		operatorRandomNodes,
		randomKeys,
		reqMsg,
	)

	go k.sendICSRequestWithBound(
		ctx,
		ktypes.ObserverSet,
		requiredObserverNodes,
		finalWaitGroup,
		clusterID,
		observerNodes,
		observerKeys,
		reqMsg,
	)

	finalWaitGroup.Wait()

	if !clusterState.IsRandomQuorum(operatorRandomNodesCount, requiredObserverNodes) {
		return errors.New("random quorum failed")
	}

	if err = k.sendICSSuccess(clusterID); err != nil {
		return err
	}

	k.metrics.captureICSCreationTime(clusterState.ICSReqTime)

	return nil
}

func (k *Engine) randomNodeDelta(setSize int) int {
	return (setSize / 2) + 1
}

func (k *Engine) observerNodeDelta(setSize int) int {
	return int(math.Ceil(ObserverNodesDelta * float64(setSize)))
}

func generateClusterID(operator id.KramaID, ixHash types.Hash) (types.ClusterID, error) {
	buffer := ixHash.Bytes()

	peerID, err := operator.PeerID()
	if err != nil {
		return "", types.ErrInvalidKramaID
	}

	rawBytes, err := base58.Decode(peerID)
	if err != nil {
		return "", types.ErrInvalidKramaID
	}

	buffer = append(buffer, rawBytes...)
	clusterHash := blake2b.Sum256(buffer)

	return types.ClusterID(base58.Encode(clusterHash[:])), nil
}

func (k *Engine) Start() {
	go k.minter()

	go k.executionRoutine()

	go func() {
		for {
			select {
			case <-k.ctx.Done():
				k.logger.Info("Closing Krama engine", "reason", "context-closed")

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

func (k *Engine) joinCluster(ctx context.Context, req Request) error {
	ctx, span := tracing.Span(ctx, "Krama.KramaEngine", "joinCluster")
	defer span.End()

	ixHash, err := req.ixs[0].GetIxHash()
	if err != nil {
		return err
	}

	k.logger.Debug(
		"Received an ICS join request",
		"from", req.msg.Operator,
		"cluster-id", req.msg.ClusterID,
		"timestamp", req.msg.Timestamp,
		ixHash.Hex(),
	)

	reqTime := utils.Canonical(time.Unix(0, req.msg.Timestamp))

	if !k.isTimely(reqTime, utils.Now()) {
		return errors.New("invalid time stamp")
	}

	clusterState := ktypes.NewICS(
		6,
		req.ixs,
		types.ClusterID(req.msg.ClusterID),
		id.KramaID(req.msg.Operator),
		reqTime)

	newSlot := ktypes.NewSlot(ktypes.ValidatorSlot, clusterState)
	// Create a slot and try adding it

	clusterState.ContextLock = req.msg.ContextLock

	if !k.slots.AddSlot(types.ClusterID(req.msg.ClusterID), newSlot) {
		return types.ErrSlotsFull
	}

	k.metrics.captureAvailableValidatorSlots(-1)

	clusterState.CurrentRole = ktypes.IcsSetType(req.msg.ContextType)

	contextHashes, nodeSets, err := k.state.FetchInteractionContext(ctx, req.ixs[0])
	if err != nil {
		return err
	}

	clusterState.AccountInfos, err = k.fetchIxAccounts(ctx, req.ixs[0])
	if err != nil {
		return err
	}
	// Check whether the context hashes matches
	for addr, info := range req.msg.ContextLock {
		if contextHashes[addr] != info.ContextHash {
			return types.ErrHashMismatch
		}
	}

	clusterState.ICS.Nodes = nodeSets

	k.logger.Debug("Responding to ICS request", "from", req.msg.Operator, "clusterId", req.msg.ClusterID)

	return k.initClusterCommunication(ctx, newSlot)
}

func (k *Engine) handleReq(req Request) {
	clusterID, err := req.getClusterID(k.operator)
	if err != nil {
		k.logger.Error("Error fetching cluster id", "err", err)
		req.responseChan <- Response{requestType: req.reqType, err: err}

		return
	}

	ctx, span := tracing.Span(
		k.ctx,
		"Krama.KramaEngine",
		"handleReq",
		trace.WithAttributes(attribute.String("clusterID", clusterID.String())),
		trace.WithAttributes(attribute.String("KramaID", string(k.vault.KramaID()))),
	)
	defer span.End()

	k.clusterLocks.Lock(clusterID.String())
	defer func() {
		if err := k.clusterLocks.Unlock(clusterID.String()); err != nil {
			k.logger.Error(fmt.Sprintf("Failed to release cluster lock id-%s", clusterID.String()))
		}
	}()

	if slot := k.slots.GetSlot(clusterID); slot != nil || !k.slots.AreSlotsAvailable(ktypes.SlotType(req.reqType)) {
		k.logger.Debug("Slots not available")
		req.responseChan <- Response{requestType: req.reqType, err: types.ErrSlotsFull}

		return
	}

	if areValid, err := k.validateInteractions(req.ixs); err != nil || !areValid {
		k.logger.Error("Invalid Interaction", "err", err, req)
		req.responseChan <- Response{requestType: req.reqType, err: types.ErrInvalidInteractions}

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

		k.exec.CleanupExecutorInstances(clusterID)
	}()

	switch req.reqType {
	case 0:
		err = k.AcquireContextLock(ctx, clusterID, req)

		req.responseChan <- Response{requestType: 0, err: err}

		if err != nil {
			k.logger.Debug("Error acquiring context lock ", "error", err, "cluster-id", clusterID)
			k.metrics.captureICSCreationFailureCount(1)

			return
		}

		k.logger.Info("Cluster creation successful", clusterID)

	case 1:
		requestTime := time.Now()
		err = k.joinCluster(ctx, req)
		req.responseChan <- Response{requestType: 1, err: err}

		if err != nil {
			k.logger.Info("Error joining cluster", "error", err, "cluster-id", clusterID)
			k.metrics.captureICSParticipationFailureCount(1)

			return
		}

		slot := k.slots.GetSlot(clusterID)
		if slot == nil {
			k.logger.Error("Slot not found")

			return
		}

		timeout := time.After(ICSTimeOutDuration)

		select {
		case success := <-slot.ICSSuccessChan:
			if !success {
				k.metrics.captureICSParticipationFailureCount(1)
				//	k.logger.Info("@@@@@@@@@@@ ClusterInfo Creation successful", clusterID)
				return
			} else {
				k.metrics.captureICSJoiningTime(requestTime)
			}

		case <-timeout:
			k.logger.Info("ICS success timeout", "cluster-id", req.msg.ClusterID)
			k.metrics.captureICSParticipationFailureCount(1)

			return
		}
	}

	k.logger.Trace("Sending execution request")
	// Send execution request
	slot := k.slots.GetSlot(clusterID)
	if slot == nil {
		k.logger.Info("nil slot", "cluster-id", req.msg.ClusterID)

		return
	}

	clusterInfo := slot.CLusterInfo()

	if clusterInfo.CurrentRole == ktypes.ObserverSet {
		log.Println("Observer HashSet", clusterInfo.ID)

		wg := observer.NewWatchDog(ctx, slot)

		wg.StartWatchDog()

		if hash := clusterInfo.ID.Hash(); !hash.IsNil() {
			proofs, err := wg.GenerateProofs()
			if err != nil {
				return
			}

			clusterInfo.AddDirty(hash, proofs)
		} else {
			k.logger.Error("Failed to store watchdog proofs")
		}
	} else {
		executionReqTS := time.Now()
		k.executionReq <- clusterID

		// Wait for execution response
		execResp := <-slot.ExecutionResp
		if execResp.Err != nil {
			k.logger.Info("Error executing interactions ", "error", execResp.Err, "cluster-id", clusterID)
			k.metrics.captureAgreementFailureCount(1)

			return
		}

		k.logger.Trace("Execution finished")
		k.metrics.captureGridGenerationTime(executionReqTS)
		clusterInfo.SetGrid(execResp.Grid)
		k.lattice.AddKnownHashes(execResp.Grid)

		consensusInitTS := time.Now()
		k.metrics.captureClusterSize(float64(clusterInfo.Size()))

		ixHash, err := clusterInfo.Ixs.Hash()
		if err != nil {
			return
		}

		icsEvidence := kbft.NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())
		bft := kbft.NewKBFTService(
			ctx,
			k.operator,
			k.logger.With("cluster-id", clusterID),
			k.cfg,
			slot.BftOutboundChan,
			slot.BftInboundChan,
			k.vault,
			icsEvidence,
			clusterInfo,
			k.wal,
			k.finalizedTesseractHandler,
		)

		if err = bft.Start(); err != nil {
			k.logger.Error("Error consensus failed", "error", err, "cluster-id", clusterInfo.ID)
			k.metrics.captureAgreementFailureCount(1)
			if err := k.exec.Revert(clusterInfo.ID); err != nil {
				log.Panicln(err)
			}

			return
		}

		k.metrics.captureAgreementTime(consensusInitTS)
	}

	k.logger.Info("Interaction finalized", "cluster-id", clusterInfo.ID)
}

func (k *Engine) fetchIxAccounts(ctx context.Context, ix *types.Interaction) (ktypes.AccountInfos, error) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "fetchIxAccounts")
	defer span.End()

	accounts := make(ktypes.AccountInfos)

	if !ix.FromAddress().IsNil() {
		accInfo, err := k.state.GetAccountMetaInfo(ix.FromAddress())
		if err != nil {
			return nil, err
		}

		accounts[ix.FromAddress()] = accInfo
	}

	if !ix.ToAddress().IsNil() {
		isGenesisAccount, err := k.state.IsGenesis(ix.ToAddress())
		if err != nil {
			return nil, err
		}

		if isGenesisAccount {
			genesisAccInfo, err := k.state.GetAccountMetaInfo(guna.GenesisAddress)
			if err != nil {
				return nil, err
			}

			acc := &types.AccountMetaInfo{
				Address:       ix.FromAddress(),
				Type:          types.RegularAccount,
				TesseractHash: types.NilHash,
				LatticeExists: true,
				StateExists:   true,
				Height:        big.NewInt(-1),
			}

			accounts[guna.GenesisAddress] = genesisAccInfo
			accounts[ix.ToAddress()] = acc
		} else {
			accInfo, err := k.state.GetAccountMetaInfo(ix.FromAddress())
			if err != nil {
				return nil, err
			}

			accounts[ix.ToAddress()] = accInfo
		}
	}

	return accounts, nil
}

func (k *Engine) sendICSRequestWithBound(
	ctx context.Context,
	setType ktypes.IcsSetType,
	requiredCount int,
	finalWaitGroup *sync.WaitGroup,
	cID types.ClusterID,
	nodes []id.KramaID,
	keys [][]byte,
	msg ptypes.ICSRequest,
) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "sendICSRequestWithBound")
	defer span.End()

	var wg sync.WaitGroup

	wg.Add(len(nodes))

	currentSlot := k.slots.GetSlot(cID)

	clusterState := currentSlot.CLusterInfo()

	nodeResponses := make([]bool, len(nodes))
	rcount := 0
	msg.ContextType = int32(setType)

	for index, kramaID := range nodes {
		// Check if the counter has reached the min number of required random nodes
		if rcount == requiredCount {
			// Determine the number of un queried nodes and decrement the wait group by that count
			x := len(nodes) - rcount
			wg.Add(-x)

			break
		}

		if kramaID == clusterState.Operator {
			nodeResponses[index] = true
			rcount++

			wg.Done()

			continue
		}
		// Retrieve the peerID from the node's Krama id
		networkID, err := kramaID.PeerID()
		if err != nil {
			k.logger.Error("Error decoding network id from krama id", "error", err)
			wg.Done()

			continue
		}

		peerID, err := peer.Decode(networkID)
		if err != nil {
			log.Println("networkId", networkID, "he")
			k.logger.Error("Unable to decode peer id", "error", err)
			wg.Done()

			continue
		}

		go func(index int, kramaID id.KramaID) {
			icsResponse := new(ptypes.ICSResponse)
			requestTS := time.Now()

			if err := k.transport.Call(
				kramaID,
				"ICSRPC",
				"ICSRequest",
				msg,
				icsResponse,
			); err == nil {
				if icsResponse.Response == 1 {
					// Add the nodeResponses array to capture the success response
					nodeResponses[index] = true
					rcount++
				} else {
					k.logger.Debug(
						"ICS Random nodes request failed",
						"error", err,
						"request status", icsResponse.Response,
						"peer id", peerID,
					)
				}
			}

			//	clusterState.updateResponseTimeMetric(reqTimeStamp)
			k.metrics.captureRequestTurnaroundTime(requestTS)
			// Decrement the wait group
			wg.Done()
		}(index, kramaID)
	}

	wg.Wait()

	idSet := ktypes.NewNodeSet(nodes, keys)

	for index, isAvailable := range nodeResponses {
		if isAvailable {
			idSet.Responses.SetIndex(index, true)
			idSet.Count++
		}
	}

	idSet.QuorumSize = requiredCount

	clusterState.UpdateNodeSet(setType, idSet)
	//	currentSlot.clusterState.IncrementClusterSize(len(nodes))
	finalWaitGroup.Done()
}

func (k *Engine) sendICSRequest(
	ctx context.Context,
	setType ktypes.IcsSetType,
	finalWaitGroup *sync.WaitGroup,
	cID types.ClusterID,
	nodesSet *ktypes.NodeSet,
	msg ptypes.ICSRequest,
	randomNodes chan []id.KramaID,
) {
	_, span := tracing.Span(ctx, "Krama.KramaEngine", "sendICSRequest")
	defer span.End()

	var wg sync.WaitGroup

	defer finalWaitGroup.Done()

	if nodesSet == nil {
		k.logger.Trace("Returning from ICSRequest routine", "set-type", setType)

		return
	}

	wg.Add(len(nodesSet.Ids))

	nodeResponses := make([]bool, len(nodesSet.Ids))
	currentSlot := k.slots.GetSlot(cID)
	clusterState := currentSlot.CLusterInfo()

	msg.ContextType = int32(setType)

	for index, kramaID := range nodesSet.Ids {
		if kramaID == clusterState.Operator {
			nodeResponses[index] = true

			clusterState.IncludeOperator()

			wg.Done()

			continue
		}

		networkID, err := kramaID.PeerID()
		if err != nil {
			k.logger.Error("Error decoding network id from krama id", "error", err)
			wg.Done()

			continue
		}

		peerID, err := peer.Decode(networkID)
		if err != nil {
			log.Println("Network id", networkID)
			k.logger.Error("Unable to decode peer id", "error", err)
			wg.Done()

			continue
		}

		go func(index int, kramaID id.KramaID) {
			icsResponse := new(ptypes.ICSResponse)
			requestTS := time.Now()

			if err := k.transport.Call(
				kramaID,
				"ICSRPC",
				"ICSRequest",
				msg,
				icsResponse,
			); err == nil && icsResponse.Response == 1 {
				// Update the nodeResponses array to capture the success response
				nodeResponses[index] = true
				randomNodes <- types.ToKIPPeerID(icsResponse.RandomNodes)
			} else {
				k.logger.Info(
					"ICSRequest failed",
					"error", err,
					"request-status", icsResponse.Response,
					"peer-id", peerID)
			}

			//	clusterState.updateResponseTimeMetric(reqTimeStamp)
			k.metrics.captureRequestTurnaroundTime(requestTS)

			// Decrement the wait group
			wg.Done()
		}(index, kramaID)
	}

	wg.Wait()
	// idSet := types.NewNodeSet(nodes, keys)

	for index, isAvailable := range nodeResponses {
		if isAvailable {
			nodesSet.Responses.SetIndex(index, true)
			nodesSet.Count++
		}
	}
	// currentSlot.clusterState.IncrementClusterSize(len(nodes))
	clusterState.UpdateNodeSet(setType, nodesSet)
}

func (k *Engine) getICSReqMsg(
	ix *types.Interaction,
	lockInfo map[types.Address]types.ContextLockInfo,
	clusterID types.ClusterID,
	timestamp time.Time,
) (ptypes.ICSRequest, error) {
	icsReqMsg := new(ptypes.ICSRequest)
	Ixs := types.Interactions{ix}

	rawData, err := polo.Polorize(Ixs)
	if err != nil {
		return *icsReqMsg, err
	}

	icsReqMsg.IxData = rawData
	icsReqMsg.ClusterID = string(clusterID)
	icsReqMsg.Operator = string(k.operator)
	icsReqMsg.ContextLock = lockInfo
	icsReqMsg.Timestamp = timestamp.UnixNano()

	return *icsReqMsg, nil
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

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	peers, err := k.randomizer.GetRandomNodes(ctx, count, exemptedNodes)
	if err != nil {
		return nil, err
	}

	return peers, nil
}

/*
func getExclusivePeers(actualSet []id.KramaID, newSet []id.KramaID) []id.KramaID {
	tempMap := make(map[id.KramaID]interface{}, len(actualSet))
	exclusivePeers := make([]id.KramaID, 0)
	for _, id := range actualSet {
		tempMap[id] = nil
	}
	for _, id := range newSet {
		if _, ok := tempMap[id]; !ok {
			exclusivePeers = append(exclusivePeers, id)
		}
	}

	return exclusivePeers
}
*/

func (k *Engine) sendICSSuccess(id types.ClusterID) error {
	slot := k.slots.GetSlot(id)
	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.CLusterInfo()

	msg := clusterState.CreateICSSuccessMsg()

	icsMsg := new(ktypes.ICSMSG)

	rawData, err := polo.Polorize(msg)
	if err != nil {
		return err
	}

	icsMsg.Msg = rawData
	icsMsg.MsgType = ptypes.ICSSUCCESS
	icsMsg.ClusterID = string(id)

	k.logger.Trace("Sending clusterState success message", "cluster id", id)

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
	if err := k.updateContextDelta(slot.ClusterID()); err != nil {
		return nil, err
	}

	clusterState := slot.CLusterInfo()
	clusterState.ComputeICSHash()

	receipts, err := k.exec.ExecuteInteractions(
		clusterState.ID,
		clusterState.Ixs,
		clusterState.GetContextDelta(),
	)
	if err != nil {
		return nil, err
	}
	// store the receipts
	clusterState.SetReceipts(receipts)
	k.logger.Debug("Generating tesseracts", "cluster-id", slot.ClusterID())

	return GenerateTesseracts(clusterState)
}

func (k *Engine) updateContextDelta(clusterID types.ClusterID) error {
	slot := k.slots.GetSlot(clusterID)

	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.CLusterInfo()
	seenAccounts := make(map[types.Address]bool)
	deltaMap := make(types.ContextDelta)

	for _, ix := range clusterState.Ixs {
		senderAddr := ix.FromAddress()
		receiverAddr := ix.ToAddress()

		if !senderAddr.IsNil() && !seenAccounts[senderAddr] {
			senderDeltaGroup := new(types.DeltaGroup)
			senderDeltaGroup.Role = types.Sender
			senderBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
				ktypes.SenderBehaviourSet,
			)

			if senderBehaviourDelta != "" {
				senderDeltaGroup.BehaviouralNodes = append(senderDeltaGroup.BehaviouralNodes, senderBehaviourDelta)
			}

			if replacedNodes != "" {
				senderDeltaGroup.ReplacedNodes = append(senderDeltaGroup.ReplacedNodes, replacedNodes)
			}

			senderRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
				ktypes.SenderRandomSet,
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

			isGenesisAccount, err := k.state.IsGenesis(receiverAddr)
			if err != nil {
				return err
			}

			if isGenesisAccount {
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
					ktypes.ReceiverBehaviourSet,
				)

				if genesisBehaviourDelta != "" {
					genesisDeltaGroup.BehaviouralNodes = append(genesisDeltaGroup.BehaviouralNodes, genesisBehaviourDelta)
				}

				if replacedNodes != "" {
					genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedNodes)
				}

				genesisRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
					ktypes.ReceiverRandomSet,
					1,
				)
				genesisDeltaGroup.RandomNodes = append(genesisDeltaGroup.RandomNodes, genesisRandomDelta...)
				genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedRandomDelta...)
				seenAccounts[guna.GenesisAddress] = true
				deltaMap[guna.GenesisAddress] = genesisDeltaGroup
			} else if clusterState.AccountInfos[receiverAddr].Type == 2 {
				receiverBehaviourDelta, replacedNodes := clusterState.GetBehaviouralContextDelta(
					ktypes.ReceiverBehaviourSet,
				)
				if receiverBehaviourDelta != "" {
					receiverDeltaGroup.BehaviouralNodes = append(receiverDeltaGroup.BehaviouralNodes, receiverBehaviourDelta)
				}
				if replacedNodes != "" {
					receiverDeltaGroup.ReplacedNodes = append(receiverDeltaGroup.ReplacedNodes, replacedNodes)
				}
				receiverRandomDelta, replacedRandomDelta := clusterState.GetRandomContextDelta(
					ktypes.ReceiverRandomSet,
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
	clusterInfo *ktypes.ClusterInfo,
	requiredRandomNodes,
	requiredBehaviouralNodes int,
) (behaviouralNodes []id.KramaID, randomNodes []id.KramaID, err error) {
	// TODO: Need to improve this function
	set := clusterInfo.ICS.Nodes[ktypes.RandomSet]
	count := 0

	for index, kramaID := range set.Ids {
		if set.Responses.GetIndex(index) {
			if index < requiredBehaviouralNodes {
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
	clusterID := tesseracts[0].ClusterID()

	slot := k.slots.GetSlot(clusterID)

	if slot == nil {
		return errors.New("nil slot")
	}

	clusterInfo := slot.CLusterInfo()

	if err := k.lattice.AddTesseracts(tesseracts, clusterInfo.GetDirty()); err != nil {
		return err
	}

	for _, ts := range tesseracts {
		msg := &ptypes.TesseractMessage{
			Tesseract: ts,
			Sender:    k.operator,
			Delta: map[types.Hash][]byte{
				ts.Body.ConsensusProof.ICSHash: clusterInfo.GetDirty()[ts.Body.ConsensusProof.ICSHash],
			},
		}

		if err := k.transport.BroadcastTesseract(msg); err != nil {
			k.logger.Error("Failed to broadcast tesseract", "error", err, "cluster-id", clusterID)
		}
	}

	return nil
}

func GenerateTesseracts(state *ktypes.ClusterInfo) ([]*types.Tesseract, error) {
	ix := state.Ixs[0] // TODO: Improve this
	gasUsed := state.GetGasUsed()
	groupBuffer := make([]byte, 0)
	tesseractGroup := make([]*types.Tesseract, 0)

	if !ix.FromAddress().IsNil() {
		senderTesseract, err := generateTesseract(ix.Hash, ix.FromAddress(), state, gasUsed, 1000)
		if err != nil {
			return nil, err
		}

		tesseractGroup = append(tesseractGroup, senderTesseract)
		groupBuffer = append(groupBuffer, senderTesseract.Header.TesseractHash.Bytes()...)
	}

	if !ix.ToAddress().IsNil() {
		receiverTesseract, err := generateTesseract(ix.Hash, ix.ToAddress(), state, gasUsed, 1000)
		if err != nil {
			return nil, err
		}

		tesseractGroup = append(tesseractGroup, receiverTesseract)
		groupBuffer = append(groupBuffer, receiverTesseract.Header.TesseractHash.Bytes()...)

		if state.AccountInfos.IsGenesis(ix.ToAddress()) {
			genesisTesseract, err := generateTesseract(ix.Hash, guna.GenesisAddress, state, gasUsed, 1000)
			if err != nil {
				return nil, err
			}

			tesseractGroup = append(tesseractGroup, genesisTesseract)
			groupBuffer = append(groupBuffer, genesisTesseract.Header.TesseractHash.Bytes()...)
		}
	}

	groupHash := blake2b.Sum256(groupBuffer)
	for _, v := range tesseractGroup {
		v.Header.GridHash = groupHash
	}

	return tesseractGroup, nil
}

func generateTesseract(
	ixHash types.Hash,
	addr types.Address,
	state *ktypes.ClusterInfo,
	gasUsed,
	gasLimit uint64,
) (*types.Tesseract, error) {
	ixsHash, err := state.Ixs.Hash()
	if err != nil {
		return nil, err
	}

	receiptHash, err := state.Receipts.Hash()
	if err != nil {
		return nil, err
	}

	ts := &types.Tesseract{
		Header: types.TesseractHeader{
			Address:     addr,
			ContextLock: state.ContextLock,
			PrevHash:    state.AccountInfos.GetLatestHash(addr),
			Height:      uint64(state.AccountInfos.GetHeight(addr) + 1),
			AnuUsed:     gasUsed,
			AnuLimit:    gasLimit,
			ClusterID:   string(state.ID),
			Operator:    string(state.Operator),
			Extra: types.CommitData{
				VoteSet:         nil,
				CommitSignature: nil,
			},
		},
		Body: types.TesseractBody{
			StateHash:       state.GetStateHash(ixHash, addr),
			ContextHash:     state.GetContextHash(ixHash, addr),
			ContextDelta:    state.GetContextDelta(),
			InteractionHash: ixsHash,
			ReceiptHash:     receiptHash,
			ConsensusProof: types.PoXCData{
				BinaryHash:   state.BinaryHash,
				IdentityHash: state.IdentityHash,
				ICSHash:      state.ICSHash,
			},
		},
		Ixns: state.Ixs,
	}

	tsBodyHash, err := ts.BodyHash()
	if err != nil {
		return nil, err
	}

	ts.Header.TesseractHash = tsBodyHash

	return ts, nil
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

func (k *Engine) validateInteractions(ixs types.Interactions) (bool, error) {
	for _, ix := range ixs {
		ixHash, err := ix.GetIxHash()
		if err != nil {
			return false, err
		}

		k.logger.Debug(
			"Validating Interaction",
			"Hash", ixHash.Hex(),
			"Nonce", ix.Nonce(),
			"From", ix.FromAddress().Hex(),
			"Type", ix.IxType(),
		)
		/*
			Checks to perform
			1) Verify the nonce
			2) Verify the balances
			3) Verify the account states
		*/
		latestNonce, err := k.state.GetLatestNonce(ix.FromAddress())
		if err != nil {
			return false, err
		}

		// validate nonce
		if ix.Nonce() < latestNonce {
			return false, types.ErrInvalidNonce
		}

		if isValid, err := k.IsIxValid(ix); err != nil || !isValid {
			return isValid, err
		}
	}

	return true, nil
}

// IsIxValid performs validity checks based on the type of interaction
func (k *Engine) IsIxValid(ix *types.Interaction) (bool, error) {
	if ix.FromAddress().IsNil() {
		return false, types.ErrInvalidAddress
	}

	if isGenesis, err := k.state.IsGenesis(ix.FromAddress()); err != nil || isGenesis {
		return false, err
	}

	switch ix.Data.Input.Type {
	case types.ValueTransfer:
		stateObject, err := k.state.GetLatestStateObject(ix.FromAddress())
		if err != nil {
			k.logger.Error("Error fetching stateObject", "addr", ix.FromAddress().Hex())

			return false, err
		}

		for assetID, value := range ix.Data.Input.TransferValue {
			if bal, err := stateObject.BalanceOf(assetID); err != nil || bal.Uint64() < value {
				return false, err
			}
		}
	case types.AssetCreation:
		assetData := ix.Data.Input.Payload.AssetData

		stateObject, err := k.state.GetLatestStateObject(ix.FromAddress())
		if err != nil {
			k.logger.Error("Error fetching stateObject", "addr", ix.FromAddress().Hex())

			return false, err
		}

		logicID, _, err := gtypes.GetLogicID(assetData.Code, false)
		if err != nil {
			return false, err
		}

		assetID, _, _, err := gtypes.GetAssetID(
			ix.FromAddress(),
			uint8(assetData.Dimension),
			assetData.IsFungible,
			assetData.IsMintable,
			assetData.Symbol,
			int64(assetData.TotalSupply),
			logicID,
		)
		if err != nil {
			return false, err
		}

		if _, err = stateObject.BalanceOf(assetID); !errors.Is(err, types.ErrAssetNotFound) {
			return false, err
		}

	default:
		return false, types.ErrInvalidInteractionType
	}

	return true, nil
}

func GetContextLockInfo(
	accounts ktypes.AccountInfos,
	hashes map[types.Address]types.Hash,
) map[types.Address]types.ContextLockInfo {
	lockInfo := make(map[types.Address]types.ContextLockInfo)

	for addr, accInfo := range accounts {
		lockInfo[addr] = types.ContextLockInfo{
			ContextHash:   hashes[addr],
			Height:        accInfo.Height.Uint64(),
			TesseractHash: accInfo.TesseractHash,
		}
	}

	return lockInfo
}
