package krama

import (
	"context"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/moby/locker"
	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/krama/kbft"
	"gitlab.com/sarvalabs/moichain/krama/observer"
	"gitlab.com/sarvalabs/moichain/krama/types"
	"gitlab.com/sarvalabs/moichain/mudra"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/flux"
	"gitlab.com/sarvalabs/polo/go-polo"
	"golang.org/x/crypto/blake2b"
	"log"
	"math"
	"math/big"
	"time"

	"sync"
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
	AddKnownHashes(tesseracts []*ktypes.Tesseract)
	AddTesseracts(tesseracts []*ktypes.Tesseract, dirtyStorage map[ktypes.Hash][]byte) error
}

type transport interface {
	InitClusterCommunication(ctx context.Context, slot *types.Slot) error
	RegisterRPCService(serviceID protocol.ID, serviceName string, service interface{}) error
	Call(peerID peer.ID, svcName, svcMethod string, args, response interface{}) error
	BroadcastTesseract(msg *ktypes.TesseractMessage) error
}

type state interface {
	FetchInteractionContext(ix *ktypes.Interaction) (map[ktypes.Address]ktypes.Hash, []*ktypes.NodeSet, error)
	GetPublicKeys(ids ...id.KramaID) (keys [][]byte, err error)
	GetAccountMetaInfo(addr ktypes.Address) (*ktypes.AccountMetaInfo, error)
	IsGenesis(addr ktypes.Address) (bool, error)
	GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error)
	GetLatestNonce(addr ktypes.Address) (uint64, error)
}

type ixPool interface {
	IncrementWaitTime(addr ktypes.Address, baseTime time.Duration) error
	Executables() ixpool.InteractionQueue
	ResetWithInteractions(ixs ktypes.Interactions)
}

type execution interface {
	CleanupExecutorInstances(id ktypes.ClusterID)
	ExecuteInteractions(
		clusterID ktypes.ClusterID,
		ixs []*ktypes.Interaction,
		contextDelta ktypes.ContextDelta,
	) (ktypes.Receipts, error)
	Revert(clusterID ktypes.ClusterID) error
}

type Response struct {
	requestType int
	err         error
}

type Request struct {
	reqType      int
	ixs          ktypes.Interactions
	msg          *ktypes.ICSRequest
	responseChan chan Response
}

func (r *Request) getClusterID(operator id.KramaID) (ktypes.ClusterID, error) {
	switch r.reqType {
	case 0:
		return generateClusterID(operator, r.ixs[0].GetIxHash())
	case 1:
		return ktypes.ClusterID(r.msg.ClusterID), nil
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
	slots        *types.Slots
	requests     chan Request
	randomizer   *flux.Randomizer
	transport    transport
	exec         execution
	pool         ixPool
	state        state
	executionReq chan ktypes.ClusterID
	lattice      lattice
	wal          kbft.WAL
	vault        *mudra.KramaVault
	clusterLocks *locker.Locker
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
		slots:        types.NewSlots(cfg.OperatorSlotCount, cfg.ValidatorSlotCount),
		requests:     make(chan Request),
		randomizer:   randomizer,
		transport:    NewKramaTransport(logger, network),
		exec:         exec,
		pool:         ixPool,
		lattice:      lattice,
		executionReq: make(chan ktypes.ClusterID),
		wal:          wal,
		vault:        val,
		clusterLocks: locker.New(),
		avgICSTime:   cfg.AccountWaitTime,
	}

	return k, k.transport.RegisterRPCService(ICSRPCProtocol, "ICSRPC", NewICSRPCService(k))
}

func (k *Engine) AcquireContextLock(ctx context.Context, clusterID ktypes.ClusterID, request Request) (err error) {
	// Create cluster id using operatorID and IxHash
	k.logger.Info("Creating cluster", "Cluster id", clusterID)

	clusterState := types.NewICS(6, request.ixs, clusterID, k.operator, time.Now())

	// create a slot and try adding it
	newSlot := types.NewSlot(types.OperatorSlot, clusterState)

	if !k.slots.AddSlot(clusterID, newSlot) {
		return ktypes.ErrSlotsFull
	}

	finalWaitGroup := new(sync.WaitGroup)

	finalWaitGroup.Add(2)

	var (
		contextRandomNodes  []id.KramaID
		operatorRandomNodes []id.KramaID
		observerNodes       []id.KramaID
	)

	// Fetch the context nodes of interaction participants
	contextHashes, nodeSets, err := k.state.FetchInteractionContext(request.ixs[0])
	if err != nil {
		return err
	}
	// Fetch the committed account info of interaction participants
	clusterState.AccountInfos, err = k.fetchIxAccounts(request.ixs[0])
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

	clusterState.ICSReqTime = kutils.Now()
	// Construct ICS_Request
	reqMsg := k.getICSReqMsg(request.ixs[0], lockInfo, clusterID, clusterState.ICSReqTime)

	// Send ClusterInfo Request to context nodes of both participants
	go k.sendICSRequest(
		ktypes.SenderBehaviourSet,
		finalWaitGroup,
		clusterID,
		nodeSets[ktypes.SenderBehaviourSet],
		reqMsg,
		randomNodesReceiverChan,
	)
	go k.sendICSRequest(
		ktypes.SenderRandomSet,
		finalWaitGroup,
		clusterID,
		nodeSets[ktypes.SenderRandomSet],
		reqMsg,
		randomNodesReceiverChan,
	)

	if request.ixs[0].ToAddress() != ktypes.NilAddress {
		finalWaitGroup.Add(2)

		go k.sendICSRequest(
			ktypes.ReceiverBehaviourSet,
			finalWaitGroup,
			clusterID,
			nodeSets[ktypes.ReceiverBehaviourSet],
			reqMsg,
			randomNodesReceiverChan,
		)

		go k.sendICSRequest(
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

	//contextRandomNodes = getExclusivePeers(respondedEligibleSet, contextRandomNodes)

	operatorRandomNodesCount := totalRandomNodes // - len(contextRandomNodes)
	operatorRandomNodesQueryCount := operatorRandomNodesCount + k.randomNodeDelta(totalRandomNodes)

	//exemptedNodes := append(respondedEligibleSet, contextRandomNodes...)

	operatorRandomNodes, err = k.getRandomNodes(operatorRandomNodesQueryCount, respondedEligibleSet)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve random and observer nodes")
	}

	if !clusterState.IsOperatorIncluded() {
		operatorRandomNodes = append([]id.KramaID{k.operator}, operatorRandomNodes...) // TODO:Improve this
	}

	exemptedNodes := append(respondedEligibleSet, operatorRandomNodes...)

	observerNodes, err = k.getObserverNodes(observerNodesQueryCount, exemptedNodes)
	if err != nil {
		k.logger.Error("error fetching observer nodes", "error", err)

		return errors.New("unable to retrieve observer nodes")
	}

	finalWaitGroup.Add(2)

	observerKeys, err := k.state.GetPublicKeys(observerNodes...)
	if err != nil {
		return ktypes.ErrKramaIDNotFound
	}

	randomKeys, err := k.state.GetPublicKeys(operatorRandomNodes...)
	if err != nil {
		return ktypes.ErrKramaIDNotFound
	}

	go k.sendICSRequestWithBound(
		ktypes.RandomSet,
		operatorRandomNodesCount,
		finalWaitGroup,
		clusterID,
		operatorRandomNodes,
		randomKeys,
		reqMsg,
	)

	go k.sendICSRequestWithBound(
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

	return
}

func (k *Engine) randomNodeDelta(setSize int) int {
	return (setSize / 2) + 1
}
func (k *Engine) observerNodeDelta(setSize int) int {
	return int(math.Ceil(ObserverNodesDelta * float64(setSize)))
}

func generateClusterID(operator id.KramaID, ixHash ktypes.Hash) (ktypes.ClusterID, error) {
	buffer := ixHash.Bytes()

	peerID, err := operator.PeerID()
	if err != nil {
		return "", ktypes.ErrInvalidKramaID
	}

	rawBytes, err := base58.Decode(peerID)
	if err != nil {
		return "", ktypes.ErrInvalidKramaID
	}

	buffer = append(buffer, rawBytes...)
	clusterHash := blake2b.Sum256(buffer)

	return ktypes.ClusterID(base58.Encode(clusterHash[:])), nil
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
	k.logger.Debug(
		"Received an ICS join request",
		"from", req.msg.Operator,
		"cluster-id", req.msg.ClusterID,
		"timestamp", req.msg.Timestamp,
		req.ixs[0].GetIxHash().Hex(),
	)

	reqTime := kutils.Canonical(time.Unix(0, req.msg.Timestamp))

	if !k.isTimely(reqTime, kutils.Now()) {
		return errors.New("invalid time stamp")
	}

	clusterState := types.NewICS(
		6,
		req.ixs,
		ktypes.ClusterID(req.msg.ClusterID),
		id.KramaID(req.msg.Operator),
		reqTime)

	newSlot := types.NewSlot(types.ValidatorSlot, clusterState)
	// Create a slot and try adding it

	clusterState.ContextLock = req.msg.ContextLock

	if !k.slots.AddSlot(ktypes.ClusterID(req.msg.ClusterID), newSlot) {
		return ktypes.ErrSlotsFull
	}

	clusterState.CurrentRole = ktypes.IcsSetType(req.msg.ContextType)

	contextHashes, nodeSets, err := k.state.FetchInteractionContext(req.ixs[0])
	if err != nil {
		return err
	}

	clusterState.AccountInfos, err = k.fetchIxAccounts(req.ixs[0])
	if err != nil {
		return err
	}
	// Check whether the context hashes matches
	for addr, info := range req.msg.ContextLock {
		if contextHashes[addr] != info.ContextHash {
			return ktypes.ErrHashMismatch
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

	k.clusterLocks.Lock(clusterID.String())
	defer func() {
		if err := k.clusterLocks.Unlock(clusterID.String()); err != nil {
			k.logger.Error(fmt.Sprintf("Failed to release cluster lock id-%s", clusterID.String()))
		}
	}()

	if slot := k.slots.GetSlot(clusterID); slot != nil || !k.slots.AreSlotsAvailable(types.SlotType(req.reqType)) {
		k.logger.Debug("Slots not available")
		req.responseChan <- Response{requestType: req.reqType, err: ktypes.ErrSlotsFull}

		return
	}

	if areValid, err := k.validateInteractions(req.ixs); err != nil || !areValid {
		k.logger.Error("Invalid Interaction", "err", err, req)
		req.responseChan <- Response{requestType: req.reqType, err: ktypes.ErrInvalidInteractions}

		return
	}

	ctx, cancelFn := context.WithCancel(k.ctx)
	defer func() {
		// delete the slot from the slots queue
		cancelFn()
		k.slots.CleanupSlot(clusterID)
		k.exec.CleanupExecutorInstances(clusterID)
	}()

	switch req.reqType {
	case 0:
		err = k.AcquireContextLock(ctx, clusterID, req)

		req.responseChan <- Response{requestType: 0, err: err}

		if err != nil {
			k.logger.Debug("Error acquiring context lock ", "error", err, "cluster-id", clusterID)

			return
		}

		k.logger.Info("Cluster creation successful", clusterID)

	case 1:
		err = k.joinCluster(ctx, req)
		req.responseChan <- Response{requestType: 1, err: err}

		if err != nil {
			k.logger.Info("Error joining cluster", "error", err, "cluster-id", clusterID)

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
				//	k.logger.Info("@@@@@@@@@@@ ClusterInfo Creation successful", clusterID)
				return
			}

		case <-timeout:
			k.logger.Info("ICS success timeout", "cluster-id", req.msg.ClusterID)

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

	clusterState := slot.CLusterInfo()

	if clusterState.CurrentRole == ktypes.ObserverSet {
		log.Println("Observer HashSet", clusterState.ID)

		wg := observer.NewWatchDog(ctx, slot)

		wg.StartWatchDog()

		if hash := clusterState.ID.Hash(); hash != ktypes.NilHash {
			clusterState.AddDirty(hash, wg.GenerateProofs())
		} else {
			k.logger.Error("Failed to store watchdog proofs")
		}
	} else {
		k.executionReq <- clusterID
		// Wait for execution response
		execResp := <-slot.ExecutionResp
		if execResp.Err != nil {
			k.logger.Info("Error executing interactions ", "error", execResp.Err, "cluster-id", clusterID)

			return
		}

		k.logger.Trace("Execution finished")

		clusterState.SetGrid(execResp.Grid)
		k.lattice.AddKnownHashes(execResp.Grid)

		exitChan := make(chan error)

		icsEvidence := kbft.NewEvidence(clusterState.Ixs.Hash(), clusterState.Operator, clusterState.Size())
		bft := kbft.NewKBFTService(
			ctx,
			k.operator,
			k.logger.With("cluster-id", clusterID),
			k.cfg,
			slot.BftOutboundChan,
			slot.BftInboundChan,
			k.vault,
			icsEvidence,
			clusterState,
			k.wal,
			exitChan,
			k.finalizedTesseractHandler,
		)

		go bft.Start()

		if err = <-exitChan; err != nil {
			k.logger.Error("Error consensus failed", "error", err, "cluster-id", clusterState.ID)
			if err := k.exec.Revert(clusterState.ID); err != nil {
				log.Fatal(err)
			}

			return
		}
	}

	k.logger.Info("Interaction finalized", "cluster-id", clusterState.ID)
}

func (k *Engine) fetchIxAccounts(ix *ktypes.Interaction) (types.AccountInfos, error) {
	accounts := make(types.AccountInfos)

	if ix.FromAddress() != ktypes.NilAddress {
		accInfo, err := k.state.GetAccountMetaInfo(ix.FromAddress())
		if err != nil {
			return nil, err
		}

		accounts[ix.FromAddress()] = accInfo
	}

	if ix.ToAddress() != ktypes.NilAddress {
		isGenesisAccount, err := k.state.IsGenesis(ix.ToAddress())
		if err != nil {
			return nil, err
		}

		if isGenesisAccount {
			genesisAccInfo, err := k.state.GetAccountMetaInfo(guna.GenesisAddress)
			if err != nil {
				return nil, err
			}

			acc := &ktypes.AccountMetaInfo{
				Address:       ix.FromAddress(),
				Type:          ktypes.RegularAccount,
				TesseractHash: ktypes.NilHash,
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
	setType ktypes.IcsSetType,
	requiredCount int,
	finalWaitGroup *sync.WaitGroup,
	cID ktypes.ClusterID,
	nodes []id.KramaID,
	keys [][]byte,
	msg ktypes.ICSRequest,
) {
	var wg sync.WaitGroup

	wg.Add(len(nodes))

	currentSlot := k.slots.GetSlot(cID)

	clusterState := currentSlot.CLusterInfo()

	nodeResponses := make([]bool, len(nodes))
	rcount := 0
	msg.ContextType = int32(setType)

	for index, kipID := range nodes {
		// Check if the counter has reached the min number of required random nodes
		if rcount == requiredCount {
			// Determine the number of un queried nodes and decrement the wait group by that count
			x := len(nodes) - rcount
			wg.Add(-x)

			break
		}

		if kipID == clusterState.Operator {
			nodeResponses[index] = true
			rcount++

			wg.Done()

			continue
		}
		// Retrieve the peerID from the node's Krama id
		networkID, err := kipID.PeerID()
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

		go func(index int, peerID peer.ID) {
			icsResponse := new(ktypes.ICSResponse)
			//	reqTimeStamp := time.Now()
			if err := k.transport.Call(
				peerID,
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

			// Decrement the wait group
			wg.Done()
		}(index, peerID)
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
	setType ktypes.IcsSetType,
	finalWaitGroup *sync.WaitGroup,
	cID ktypes.ClusterID,
	nodesSet *ktypes.NodeSet,
	msg ktypes.ICSRequest,
	randomNodes chan []id.KramaID,
) {
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

	for index, kipID := range nodesSet.Ids {
		if kipID == clusterState.Operator {
			nodeResponses[index] = true

			clusterState.IncludeOperator()

			wg.Done()

			continue
		}

		networkID, err := kipID.PeerID()
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

		go func(index int, peerID peer.ID) {
			icsResponse := new(ktypes.ICSResponse)
			//reqTimeStamp := time.Now()
			if err := k.transport.Call(
				peerID,
				"ICSRPC",
				"ICSRequest",
				msg,
				icsResponse,
			); err == nil && icsResponse.Response == 1 {
				// Update the nodeResponses array to capture the success response
				nodeResponses[index] = true
				randomNodes <- ktypes.ToKIPPeerID(icsResponse.RandomNodes)
			} else {
				k.logger.Info(
					"ICSRequest failed",
					"error", err,
					"request-status", icsResponse.Response,
					"peer-id", peerID)
			}

			//	clusterState.updateResponseTimeMetric(reqTimeStamp)

			// Decrement the wait group
			wg.Done()
		}(index, peerID)
	}

	wg.Wait()
	//idSet := ktypes.NewNodeSet(nodes, keys)

	for index, isAvailable := range nodeResponses {
		if isAvailable {
			nodesSet.Responses.SetIndex(index, true)
			nodesSet.Count++
		}
	}
	//currentSlot.clusterState.IncrementClusterSize(len(nodes))
	clusterState.UpdateNodeSet(setType, nodesSet)
}
func (k *Engine) getICSReqMsg(
	ix *ktypes.Interaction,
	lockInfo map[ktypes.Address]ktypes.ContextLockInfo,
	clusterID ktypes.ClusterID,
	timestamp time.Time,
) ktypes.ICSRequest {
	icsReqMsg := new(ktypes.ICSRequest)
	Ixs := ktypes.Interactions{ix}
	icsReqMsg.IxData = polo.Polorize(Ixs)
	icsReqMsg.ClusterID = string(clusterID)
	icsReqMsg.Operator = string(k.operator)
	icsReqMsg.ContextLock = lockInfo
	icsReqMsg.Timestamp = timestamp.UnixNano()

	return *icsReqMsg
}
func (k *Engine) getRandomNodes(count int, exemptedNodes []id.KramaID) ([]id.KramaID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)

	defer cancel()

	peers, err := k.randomizer.GetRandomNodes(ctx, count, exemptedNodes)
	if err != nil {
		return nil, err
	}

	return peers, nil
}

func (k *Engine) getObserverNodes(count int, exemptedNodes []id.KramaID) ([]id.KramaID, error) {
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

func (k *Engine) sendICSSuccess(id ktypes.ClusterID) error {
	slot := k.slots.GetSlot(id)
	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.CLusterInfo()

	msg := clusterState.CreateICSSuccessMsg()

	icsMsg := new(ktypes.ICSMSG)
	icsMsg.Msg = polo.Polorize(msg)
	icsMsg.MsgType = ktypes.ICSSUCCESS
	icsMsg.ClusterID = string(id)

	k.logger.Trace("Sending clusterState success message", "cluster id", id)

	slot.OutboundChan <- icsMsg

	clusterState.SuccessMsg = icsMsg

	return nil
}

func (k *Engine) initClusterCommunication(ctx context.Context, slot *types.Slot) error {
	if err := k.transport.InitClusterCommunication(ctx, slot); err != nil {
		return err
	}

	k.startMessageHandlers(ctx, slot)

	return nil
}

func (k *Engine) createProposalGrid(slot *types.Slot) ([]*ktypes.Tesseract, error) {
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

func (k *Engine) updateContextDelta(clusterID ktypes.ClusterID) error {
	slot := k.slots.GetSlot(clusterID)

	if slot == nil {
		return errors.New("nil slot")
	}

	clusterState := slot.CLusterInfo()
	seenAccounts := make(map[ktypes.Address]bool)
	deltaMap := make(ktypes.ContextDelta)

	for _, ix := range clusterState.Ixs {
		senderAddr := ix.FromAddress()
		receiverAddr := ix.ToAddress()

		if senderAddr != ktypes.NilAddress && !seenAccounts[senderAddr] {
			senderDeltaGroup := new(ktypes.DeltaGroup)
			senderDeltaGroup.Role = ktypes.Sender
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

		if receiverAddr != ktypes.NilAddress && !seenAccounts[receiverAddr] {
			receiverDeltaGroup := new(ktypes.DeltaGroup)
			receiverDeltaGroup.Role = ktypes.Receiver

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
				genesisDeltaGroup := new(ktypes.DeltaGroup)
				genesisDeltaGroup.Role = ktypes.Genesis
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
			} else {
				if clusterState.AccountInfos[receiverAddr].Type == 2 {
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
			}

			seenAccounts[receiverAddr] = true
			deltaMap[receiverAddr] = receiverDeltaGroup
		}
	}

	clusterState.UpdateContextDelta(deltaMap)

	return nil
}

func (k *Engine) GetNodes(
	clusterInfo *types.ClusterInfo,
	requiredRandomNodes,
	requiredBehaviouralNodes int,
) (behaviouralNodes []id.KramaID, randomNodes []id.KramaID, err error) {
	//TODO: Need to improve this function
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

func (k *Engine) finalizedTesseractHandler(tesseracts []*ktypes.Tesseract) error {

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
		msg := &ktypes.TesseractMessage{
			Tesseract: ts,
			Sender:    k.operator,
			Delta: map[ktypes.Hash][]byte{
				ts.Body.ConsensusProof.ICSHash: clusterInfo.GetDirty()[ts.Body.ConsensusProof.ICSHash],
			},
		}

		if err := k.transport.BroadcastTesseract(msg); err != nil {
			k.logger.Error("Failed to broadcast tesseract", "error", err, "cluster-id", clusterID)
		}
	}

	return nil
}

func GenerateTesseracts(state *types.ClusterInfo) ([]*ktypes.Tesseract, error) {
	ix := state.Ixs[0] //TODO: Improve this
	gasUsed := state.GetGasUsed()
	groupBuffer := make([]byte, 0)
	tesseractGroup := make([]*ktypes.Tesseract, 0)

	if ix.FromAddress() != ktypes.NilAddress {
		senderTesseract := generateTesseract(ix.Hash, ix.FromAddress(), state, gasUsed, 1000)
		tesseractGroup = append(tesseractGroup, senderTesseract)
		groupBuffer = append(groupBuffer, senderTesseract.Header.TesseractHash.Bytes()...)
	}

	if ix.ToAddress() != ktypes.NilAddress {
		receiverTesseract := generateTesseract(ix.Hash, ix.ToAddress(), state, gasUsed, 1000)
		tesseractGroup = append(tesseractGroup, receiverTesseract)
		groupBuffer = append(groupBuffer, receiverTesseract.Header.TesseractHash.Bytes()...)

		if state.AccountInfos.IsGenesis(ix.ToAddress()) {
			genesisTesseract := generateTesseract(ix.Hash, guna.GenesisAddress, state, gasUsed, 1000)
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
	ixHash ktypes.Hash,
	addr ktypes.Address,
	state *types.ClusterInfo,
	gasUsed,
	gasLimit uint64,
) *ktypes.Tesseract {
	ts := &ktypes.Tesseract{
		Header: ktypes.TesseractHeader{
			Address:     addr,
			ContextLock: state.ContextLock,
			PrevHash:    state.AccountInfos.GetLatestHash(addr),
			Height:      uint64(state.AccountInfos.GetHeight(addr) + 1),
			AnuUsed:     gasUsed,
			AnuLimit:    gasLimit,
			ClusterID:   string(state.ID),
			Operator:    string(state.Operator),
			Extra: ktypes.CommitData{
				VoteSet:         nil,
				CommitSignature: nil,
			},
		},
		Body: ktypes.TesseractBody{
			StateHash:       state.GetStateHash(ixHash, addr),
			ContextHash:     state.GetContextHash(ixHash, addr),
			ContextDelta:    state.GetContextDelta(),
			Interactions:    state.Ixs,
			InteractionHash: state.Ixs.Hash(),
			ReceiptHash:     state.Receipts.Hash(),
			ConsensusProof: ktypes.PoXCData{
				BinaryHash:   state.BinaryHash,
				IdentityHash: state.IdentityHash,
				ICSHash:      state.ICSHash,
			},
		},
	}
	ts.Header.TesseractHash = ts.BodyHash()

	return ts
}

func (k *Engine) executionRoutine() {
	for clusterID := range k.executionReq {
		k.logger.Trace("Processing an execution request")

		go func(id ktypes.ClusterID) {
			slotInfo := k.slots.GetSlot(id)
			grid, err := k.createProposalGrid(slotInfo)
			slotInfo.ExecutionResp <- types.ExecutionResponse{Grid: grid, Err: err}
		}(clusterID)
	}
}

func (k *Engine) Close() {
	defer k.ctxCancel()
}

func (k *Engine) validateInteractions(ixs ktypes.Interactions) (bool, error) {
	for _, ix := range ixs {
		k.logger.Debug(
			"Validating Interaction",
			"Hash", ix.GetIxHash().Hex(),
			"Nonce", ix.Nonce(),
			"From", ix.FromAddress().Hex(),
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
			return false, ktypes.ErrInvalidNonce
		}

		if isValid, err := k.IsIxValid(ix); err != nil {
			k.logger.Error("Invalid Interaction", "error", err)

			return isValid, err
		}
	}

	return true, nil
}

// IsIxValid performs validity checks based on the type of interaction
func (k *Engine) IsIxValid(ix *ktypes.Interaction) (bool, error) {
	if ix.FromAddress() == ktypes.NilAddress {
		return false, ktypes.ErrInvalidAddress
	}

	if isGenesis, err := k.state.IsGenesis(ix.FromAddress()); err != nil || isGenesis {
		return false, err
	}

	switch ix.Data.Input.Type {
	case ktypes.ValueTransfer:
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
	case ktypes.AssetCreation:
		assetData := ix.Data.Input.Payload.AssetData

		stateObject, err := k.state.GetLatestStateObject(ix.FromAddress())
		if err != nil {
			k.logger.Error("Error fetching stateObject", "addr", ix.FromAddress().Hex())

			return false, err
		}

		logicID, _ := ktypes.GetLogicID(assetData.Code, false)
		assetID, _, _ := ktypes.GetAssetID(
			ix.FromAddress(),
			uint8(assetData.Dimension),
			assetData.IsFungible,
			assetData.IsMintable,
			assetData.Symbol,
			int64(assetData.TotalSupply),
			logicID,
		)

		if _, err = stateObject.BalanceOf(assetID); !errors.Is(err, ktypes.ErrAssetNotFound) {
			return false, err
		}

	default:
		return false, ktypes.ErrInvalidInteractionType
	}

	return true, nil
}

func GetContextLockInfo(
	accounts types.AccountInfos,
	hashes map[ktypes.Address]ktypes.Hash,
) map[ktypes.Address]ktypes.ContextLockInfo {
	lockInfo := make(map[ktypes.Address]ktypes.ContextLockInfo)

	for addr, accInfo := range accounts {
		lockInfo[addr] = ktypes.ContextLockInfo{
			ContextHash:   hashes[addr],
			Height:        accInfo.Height.Uint64(),
			TesseractHash: accInfo.TesseractHash,
		}
	}

	return lockInfo
}
