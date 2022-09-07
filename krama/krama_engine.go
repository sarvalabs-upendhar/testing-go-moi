package krama

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/krama/ics"
	"gitlab.com/sarvalabs/moichain/krama/kbft"
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

	ICSTimeOutDuration time.Duration = 4000 * time.Millisecond
	MaxSlots           int           = 1

	BehaviouralContextSize = 1
	RandomContextSize      = 1
)

type lattice interface {
	AddKnownHashes(tesseracts []*ktypes.Tesseract)
	AppendTesseracts(
		groupHash ktypes.Hash,
		ts map[ktypes.Address]*ktypes.Tesseract,
		dirtyStorage map[ktypes.Hash][]byte,
	) error
}
type persistence interface {
	CreateAndPublishEntry(key ktypes.Hash, value []byte) error
}
type network interface {
	Unsubscribe(topic string) error
	Broadcast(topic string, data []byte) error
	Subscribe(topic string, handler func(ctx context.Context, msg *pubsub.Message) error) error
	InitNewRPCServer(protocol protocol.ID) *rpc.Client
	RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error
	GetKramaID() id.KramaID
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
	IncrementWaitTime(addr ktypes.Address) error
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

type ExecutionResponse struct {
	err  error
	grid []*ktypes.Tesseract
}
type Response struct {
	reqType int
	err     error
}

type Engine struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	cfg          *common.ConsensusConfig
	logger       hclog.Logger
	operator     id.KramaID
	slots        *Slots
	requests     chan Request
	rpcClient    *rpc.Client
	randomizer   *flux.Randomizer
	server       network
	exec         execution
	pool         ixPool
	state        state
	executionReq chan ktypes.ClusterID
	lattice      lattice
	wal          kbft.WAL
	db           persistence
	vault        *mudra.KramaVault
}

func NewKramaEngine(ctx context.Context,
	cfg *common.ConsensusConfig,
	logger hclog.Logger,
	state state,
	server network,
	exec execution,
	ixPool ixPool,
	val *mudra.KramaVault,
	lattice lattice,
	db persistence,
	randomizer *flux.Randomizer,
) (*Engine, error) {
	wal, err := kbft.NewWAL(ctx, logger, cfg.DirectoryPath)
	if err != nil {
		return nil, errors.Wrap(err, "WAL failed")
	}

	ctx, ctxCancel := context.WithCancel(ctx)
	k := &Engine{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		cfg:       cfg,
		logger:    logger.Named("Krama-Engine"),
		operator:  server.GetKramaID(),
		state:     state,
		slots: &Slots{
			slots:          make(map[ktypes.ClusterID]*slotInfo),
			availableSlots: MaxSlots,
			activeAccounts: make(map[ktypes.Address]ktypes.ClusterID, MaxSlots*2),
		},
		requests:     make(chan Request),
		randomizer:   randomizer,
		server:       server,
		exec:         exec,
		pool:         ixPool,
		lattice:      lattice,
		executionReq: make(chan ktypes.ClusterID),
		wal:          wal,
		db:           db,
		vault:        val,
	}

	return k, k.RegisterRPCService()
}

func (k *Engine) pubSubHandler(ctx context.Context, msg *pubsub.Message) error {
	// Unmarshal the pub sub message into an ClusterInfo message
	icsMsg := new(ktypes.ICSMSG)
	if err := polo.Depolorize(icsMsg, msg.GetData()); err != nil {
		return err
	}

	slot := k.slots.getSlot(ktypes.ClusterID(icsMsg.ClusterID))

	if slot == nil {
		k.logger.Error("Error fetching slot for cluster-id", icsMsg.ClusterID)

		return errors.New("invalid cluster id")
	}

	switch icsMsg.ReqType {
	case ktypes.SUCCESSMSG:
		// Unmarshal into an ICS success message
		successMsg := new(ktypes.ICSSuccess)

		if err := polo.Depolorize(successMsg, icsMsg.Msg); err != nil {
			k.logger.Error("Error unmarshalling ics_success message", "sender", icsMsg.Sender)

			return err
		}

		if slot == nil {
			k.logger.Info(
				"Slot not available",
				"clusterID", successMsg.ClusterID,
				"sender:", icsMsg.Sender,
			)

			return errors.New("invalid cluster id")
		}

		observerPublicKeys, err := k.state.GetPublicKeys(successMsg.ObserverSet...)
		if err != nil {
			return errors.New("failed to retrieve public keys")
		}

		randomPublicKeys, err := k.state.GetPublicKeys(successMsg.RandomSet...)
		if err != nil {
			return errors.New("failed to retrieve public keys")
		}
		// update the cluster state with the latest node set's
		slot.clusterState.ICS.Nodes[ktypes.ObserverSet] = ktypes.NewNodeSet(successMsg.ObserverSet, observerPublicKeys)
		slot.clusterState.ICS.Nodes[ktypes.ObserverSet].QuorumSize = successMsg.QuorumSizes[ktypes.ObserverSet]
		slot.clusterState.ICS.Nodes[ktypes.RandomSet] = ktypes.NewNodeSet(successMsg.RandomSet, randomPublicKeys)
		slot.clusterState.ICS.Nodes[ktypes.RandomSet].QuorumSize = successMsg.QuorumSizes[ktypes.RandomSet]
		slot.clusterState.UpdateClusterSize()

		for j := 0; j < len(slot.clusterState.ICS.Nodes); j++ {
			if successMsg.Responses[j] != nil && successMsg.Responses[j].Size > 0 {
				slot.clusterState.ICS.Nodes[j].Responses = successMsg.Responses[j]
				slot.clusterState.ICS.Nodes[j].Count = slot.clusterState.ICS.Nodes[j].Responses.TrueIndicesSize()
			}
		}

		k.logger.Info(
			"Sending ClusterInfo success signal",
			"Cluster id", icsMsg.ClusterID,
		)

		slot.clusterState.SuccessMsg = icsMsg
		slot.icsSuccess <- true

	default:
		// Forward all other messages to the PoXc InboundMsg  channel
		slot.inboundMsg <- icsMsg
	}

	return nil
}

func (k *Engine) AcquireContextLock(ctx context.Context, clusterID ktypes.ClusterID, request Request) (err error) {
	// Create cluster id using operatorID and IxHash
	k.logger.Info("Creating cluster", "Cluster id", clusterID)

	// create a slot and try adding it
	newSlot := &slotInfo{
		clusterState:  ics.NewICS(6, request.ixs, clusterID, k.operator, time.Now()),
		icsSuccess:    make(chan bool),
		outboundMsg:   make(chan *ktypes.ICSMSG),
		inboundMsg:    make(chan *ktypes.ICSMSG),
		executionResp: make(chan ExecutionResponse),
	}

	if !k.slots.addSlot(clusterID, newSlot) {
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
	newSlot.clusterState.AccountInfos, err = k.fetchIxAccounts(request.ixs[0])
	if err != nil {
		return err
	}
	// Generate the contextLock
	lockInfo := GetContextLockInfo(newSlot.clusterState.AccountInfos, contextHashes)
	newSlot.clusterState.ContextLock = lockInfo
	// Initiate the cluster communication by subscribing to clusterID
	if err = k.initClusterCommunication(ctx, clusterID); err != nil {
		return err
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

	newSlot.clusterState.ICSReqTime = kutils.Now()
	// Construct ICS_Request
	reqMsg := k.getICSReqMsg(request.ixs[0], lockInfo, clusterID, newSlot.clusterState.ICSReqTime)

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
	if !newSlot.clusterState.IsContextQuorum() {
		return errors.New("context quorum failed")
	}

	respondedEligibleSetSize, respondedEligibleSet := newSlot.clusterState.RespondedEligibleSet()
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
		return errors.New("unable to retrieve random and observer nodes from context")
	}

	if !newSlot.clusterState.IsOperatorIncluded() {
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

	if !newSlot.clusterState.IsRandomQuorum(operatorRandomNodesCount, requiredObserverNodes) {
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

	// Create a slot and try adding it
	newSlot := &slotInfo{
		clusterState: ics.NewICS(
			6,
			req.ixs,
			ktypes.ClusterID(req.msg.ClusterID),
			id.KramaID(req.msg.Operator),
			reqTime),
		icsSuccess:    make(chan bool),
		outboundMsg:   make(chan *ktypes.ICSMSG),
		inboundMsg:    make(chan *ktypes.ICSMSG),
		executionResp: make(chan ExecutionResponse),
	}
	newSlot.clusterState.ContextLock = req.msg.ContextLock

	if !k.slots.addSlot(ktypes.ClusterID(req.msg.ClusterID), newSlot) {
		return ktypes.ErrSlotsFull
	}

	newSlot.clusterState.CurrentRole = ktypes.IcsSetType(req.msg.ContextType)

	contextHashes, nodeSets, err := k.state.FetchInteractionContext(req.ixs[0])
	if err != nil {
		return err
	}

	newSlot.clusterState.AccountInfos, err = k.fetchIxAccounts(req.ixs[0])
	if err != nil {
		return err
	}
	// Check whether the context hashes matches
	for addr, info := range req.msg.ContextLock {
		if contextHashes[addr] != info.ContextHash {
			return ktypes.ErrHashMismatch
		}
	}

	newSlot.clusterState.ICS.Nodes = nodeSets

	k.logger.Debug("Responding to ICS request", "from", req.msg.Operator, "clusterId", req.msg.ClusterID)

	return k.initClusterCommunication(ctx, ktypes.ClusterID(req.msg.ClusterID))
}

func (k *Engine) handleReq(req Request) {
	clusterID, err := req.getClusterID(k.operator)
	if err != nil {
		k.logger.Error("Error fetching cluster id", "err", err)
		req.responseChan <- Response{reqType: req.reqType, err: err}

		return
	}

	if slot := k.slots.getSlot(clusterID); slot != nil || !k.slots.areSlotsAvailable() {
		k.logger.Debug("Slots not available")
		req.responseChan <- Response{reqType: req.reqType, err: ktypes.ErrSlotsFull}

		return
	}

	if areValid, err := k.validateInteractions(req.ixs); err != nil || !areValid {
		k.logger.Error("Invalid Interaction", "err", err, req)
		req.responseChan <- Response{reqType: req.reqType, err: ktypes.ErrInvalidInteractions}

		return
	}

	ctx, cancelFn := context.WithCancel(k.ctx)
	defer func() {
		// delete the slot from the slots queue
		cancelFn()
		k.slots.cleanupSlot(clusterID)
		k.exec.CleanupExecutorInstances(clusterID)
	}()

	switch req.reqType {
	case 0:
		err = k.AcquireContextLock(ctx, clusterID, req)

		req.responseChan <- Response{reqType: 0, err: err}

		if err != nil {
			k.logger.Debug("Error acquiring context lock ", "error", err, "cluster-id", clusterID)

			return
		}

		k.logger.Info("Cluster creation successful", clusterID)

	case 1:
		err = k.joinCluster(ctx, req)
		req.responseChan <- Response{reqType: 1, err: err}

		if err != nil {
			k.logger.Info("Error joining cluster", "error", err, "cluster-id", clusterID)

			return
		}

		slot := k.slots.getSlot(clusterID)
		if slot == nil {
			log.Panic("")
		}

		timeout := time.After(ICSTimeOutDuration)

		select {
		case success := <-slot.icsSuccess:
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
	slot := k.slots.getSlot(clusterID)

	if slot.clusterState.CurrentRole == ktypes.ObserverSet {
		messageRouter := kbft.NewMessageRouter(
			ctx,
			slot.inboundMsg,
			slot.outboundMsg,
			nil,
			slot.bft,
			slot.clusterState.ID,
			slot.clusterState.CurrentRole,
		)
		messageRouter.Start()
		metaData := slot.clusterState.GetMetaData(messageRouter.Msgs())
		rawData := polo.Polorize(metaData)
		metaDataHash := ktypes.GetHash(rawData)
		slot.clusterState.AddDirty(metaDataHash, rawData)
	} else {
		k.executionReq <- clusterID
		// Wait for execution response
		execResp := <-slot.executionResp
		if execResp.err != nil {
			k.logger.Info("Error executing interactions ", "error", execResp.err, "cluster-id", clusterID)

			return
		}

		k.logger.Trace("Execution finished")

		slot.clusterState.SetGrid(execResp.grid)
		k.lattice.AddKnownHashes(execResp.grid)
		consensusChan := make(chan ktypes.ConsensusMessage)
		exitChan := make(chan error)

		icsEvidence := kbft.NewEvidence(slot.clusterState.Ixs.Hash(), slot.clusterState.Operator, slot.clusterState.Size())
		slot.bft = kbft.NewKBFTService(
			ctx,
			k.server.GetKramaID(),
			k.logger.With("Cluster-ID", clusterID),
			k.cfg, k.vault,
			k.lattice,
			icsEvidence,
			slot.clusterState,
			k.wal,
			exitChan,
			consensusChan,
		)
		consensusHandler := kbft.NewMessageRouter(
			ctx,
			slot.inboundMsg,
			slot.outboundMsg,
			consensusChan,
			slot.bft,
			slot.clusterState.ID,
			slot.clusterState.CurrentRole,
		)

		go slot.bft.Start()
		go consensusHandler.Start()
		err = <-exitChan
		if err != nil {
			k.logger.Error("Error consensus failed", "error", err, "cluster-id", slot.clusterState.ID)
			if err := k.exec.Revert(slot.clusterState.ID); err != nil {
				log.Fatal(err)
			}

			return
		}
	}

	for key, value := range slot.clusterState.GetDirty() {
		if err := k.db.CreateAndPublishEntry(key, value); err != nil {
			k.logger.Error("Error writing keys to db")
			log.Panic(err) //We panic here, this should not occur at all.
		}
	}

	k.logger.Info("Interaction finalized", "cluster-id", slot.clusterState.ID)
}

func (k *Engine) fetchIxAccounts(ix *ktypes.Interaction) (ics.AccountInfos, error) {
	accounts := make(ics.AccountInfos)

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

	currentSlot := k.slots.getSlot(cID)
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

		if kipID == currentSlot.clusterState.Operator {
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
			k.logger.Error("Unable to decode peer id", "error", err)
			wg.Done()

			continue
		}

		go func(index int, peerID peer.ID) {
			icsResponse := new(ktypes.ICSResponse)
			//	reqTimeStamp := time.Now()
			if err := k.rpcClient.Call(
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

	currentSlot.clusterState.UpdateNodeSet(setType, idSet)
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
	currentSlot := k.slots.getSlot(cID)

	for index, kipID := range nodesSet.Ids {
		if kipID == currentSlot.clusterState.Operator {
			nodeResponses[index] = true

			currentSlot.clusterState.IncludeOperator()

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
			k.logger.Error("Unable to decode peer id", "error", err)
			wg.Done()

			continue
		}

		msg.ContextType = int32(setType)

		go func(index int, peerID peer.ID) {
			icsResponse := new(ktypes.ICSResponse)
			//reqTimeStamp := time.Now()
			if err := k.rpcClient.Call(
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
	currentSlot.clusterState.UpdateNodeSet(setType, nodesSet)
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
	slot := k.slots.getSlot(id)
	msg := slot.clusterState.CreateICSSuccessMsg()

	icsMsg := new(ktypes.ICSMSG)
	icsMsg.Msg = polo.Polorize(msg)
	icsMsg.ReqType = ktypes.SUCCESSMSG
	icsMsg.ClusterID = string(id)

	k.logger.Info("Sending clusterState success message", "cluster id", id)

	if err := k.server.Broadcast(string(id), polo.Polorize(icsMsg)); err != nil {
		return err
	}

	slot.clusterState.SuccessMsg = icsMsg

	return nil
}
func (k *Engine) initClusterCommunication(ctx context.Context, clusterID ktypes.ClusterID) error {
	go func() {
		slot := k.slots.getSlot(clusterID)

		for {
			select {
			case <-ctx.Done():
				if err := k.server.Unsubscribe(string(clusterID)); err != nil {
					log.Panicln(err)
				}

				k.logger.Info("Closing PoXt message handler")

				return
			case msg := <-slot.outboundMsg:
				if err := k.server.Broadcast(string(clusterID), polo.Polorize(msg)); err != nil {
					k.logger.Error("Error broadcasting PoXt Message")
					panic(err)
				}
			}
		}
	}()

	return k.server.Subscribe(string(clusterID), k.pubSubHandler)
}

func (k *Engine) RegisterRPCService() error {
	k.rpcClient = k.server.InitNewRPCServer(ICSRPCProtocol)

	return k.server.RegisterNewRPCService(ICSRPCProtocol, "ICSRPC", NewICSRPCService(k))
}

func (k *Engine) minter() {
	respChan := make(chan Response)

	for {
		if k.slots.areSlotsAvailable() {
			interactionQueue := k.pool.Executables()

			for interactionQueue.Len() > 0 {
				ix, ok := interactionQueue.Pop().(*ktypes.Interaction)
				if !ok {
					k.logger.Error("Error interaction type assertion failed", "hash", ix.GetIxHash())

					continue
				}

				ixs := ktypes.Interactions{ix}

				k.logger.Info("Forwarding request to krama engine")

				k.requests <- Request{reqType: 0, ixs: ixs, msg: nil, responseChan: respChan}
				//Wait for response from krama engine handler
				resp := <-respChan
				if resp.err != nil {
					if errors.Is(resp.err, ktypes.ErrInvalidInteractions) {
						k.pool.ResetWithInteractions(ixs)
					} else {
						if !errors.Is(resp.err, ktypes.ErrSlotsFull) {
							if err := k.pool.IncrementWaitTime(ix.FromAddress()); err != nil {
								k.logger.Error("Error incrementing wait time")
							}
						} else {
							k.logger.Error("ICS creation failed", resp.err)
						}
					}
				}
			}
		}
		select {
		case <-k.ctx.Done():
			return
		case <-time.After(1000 * time.Millisecond):
		}
	}
}

func (k *Engine) createProposalGrid(slot *slotInfo) ([]*ktypes.Tesseract, error) {
	if err := k.updateContextDelta(slot.clusterState.ID); err != nil {
		return nil, err
	}

	slot.clusterState.ComputeICSHash()

	receipts, err := k.exec.ExecuteInteractions(
		slot.clusterState.ID,
		slot.clusterState.Ixs,
		slot.clusterState.GetContextDelta(),
	)
	if err != nil {
		return nil, err
	}
	// store the receipts
	slot.clusterState.SetReceipts(receipts)
	k.logger.Debug("Generating tesseracts", "cluster-id", slot.clusterState.ID)

	return GenerateTesseracts(slot.clusterState)
}

func (k *Engine) updateContextDelta(clusterID ktypes.ClusterID) error {
	slot := k.slots.getSlot(clusterID)
	seenAccounts := make(map[ktypes.Address]bool)
	deltaMap := make(ktypes.ContextDelta)

	for _, ix := range slot.clusterState.Ixs {
		senderAddr := ix.FromAddress()
		receiverAddr := ix.ToAddress()

		if senderAddr != ktypes.NilAddress && !seenAccounts[senderAddr] {
			senderDeltaGroup := new(ktypes.DeltaGroup)
			senderDeltaGroup.Role = ktypes.Sender
			senderBehaviourDelta, replacedNodes := slot.clusterState.GetBehaviouralContextDelta(
				ktypes.SenderBehaviourSet,
			)

			if senderBehaviourDelta != "" {
				senderDeltaGroup.BehaviouralNodes = append(senderDeltaGroup.BehaviouralNodes, senderBehaviourDelta)
				senderDeltaGroup.ReplacedNodes = append(senderDeltaGroup.ReplacedNodes, replacedNodes)
			}

			senderRandomDelta, replacedRandomDelta := slot.clusterState.GetRandomContextDelta(
				ktypes.SenderRandomSet,
				1,
				nil,
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
					slot.clusterState,
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
				genesisBehaviourDelta, replacedNodes := slot.clusterState.GetBehaviouralContextDelta(
					ktypes.ReceiverBehaviourSet,
				)

				if genesisBehaviourDelta != "" {
					genesisDeltaGroup.BehaviouralNodes = append(genesisDeltaGroup.BehaviouralNodes, genesisBehaviourDelta)
					genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedNodes)
				}

				genesisRandomDelta, replacedRandomDelta := slot.clusterState.GetRandomContextDelta(
					ktypes.ReceiverRandomSet,
					1,
					nil,
				)
				genesisDeltaGroup.RandomNodes = append(genesisDeltaGroup.RandomNodes, genesisRandomDelta...)
				genesisDeltaGroup.ReplacedNodes = append(genesisDeltaGroup.ReplacedNodes, replacedRandomDelta...)
				seenAccounts[guna.GenesisAddress] = true
				deltaMap[guna.GenesisAddress] = genesisDeltaGroup
			} else {
				if slot.clusterState.AccountInfos[receiverAddr].Type == 2 {
					receiverBehaviourDelta, replacedNodes := slot.clusterState.GetBehaviouralContextDelta(
						ktypes.ReceiverBehaviourSet,
					)
					if receiverBehaviourDelta != "" {
						receiverDeltaGroup.BehaviouralNodes = append(receiverDeltaGroup.BehaviouralNodes, receiverBehaviourDelta)
						receiverDeltaGroup.ReplacedNodes = append(receiverDeltaGroup.ReplacedNodes, replacedNodes)
					}
					receiverRandomDelta, replacedRandomDelta := slot.clusterState.GetRandomContextDelta(
						ktypes.ReceiverRandomSet,
						1,
						nil,
					)
					receiverDeltaGroup.RandomNodes = append(receiverDeltaGroup.RandomNodes, receiverRandomDelta...)
					receiverDeltaGroup.ReplacedNodes = append(receiverDeltaGroup.ReplacedNodes, replacedRandomDelta...)
				}
			}

			seenAccounts[receiverAddr] = true
			deltaMap[receiverAddr] = receiverDeltaGroup
		}
	}

	slot.clusterState.UpdateContextDelta(deltaMap)

	return nil
}

func (k *Engine) GetNodes(
	clusterInfo *ics.ClusterInfo,
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
func GenerateTesseracts(state *ics.ClusterInfo) ([]*ktypes.Tesseract, error) {
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
		v.Header.GroupHash = groupHash
	}

	return tesseractGroup, nil
}

func generateTesseract(
	ixHash ktypes.Hash,
	addr ktypes.Address,
	state *ics.ClusterInfo,
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
			slotInfo := k.slots.getSlot(id)
			grid, err := k.createProposalGrid(slotInfo)
			slotInfo.executionResp <- ExecutionResponse{grid: grid, err: err}
		}(clusterID)
	}
}

//func (k *KramaEngine) addEvidenceToDB(msgs []*ktypes.ICSMSG) {
//	var buffer bytes.Buffer
//	for _, v := range msgs {
//		buffer.Write(polo.Polorize(v))
//	}
//}

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
	accounts ics.AccountInfos,
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
