package ixpool

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/petar/GoLLRB/llrb"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/state"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
)

const (
	WaitMode = iota
	CostMode
)

const (
	IxSlotSize      = 1 * 1024   // IxSlotSize chosen as 1kB as minimum ixn sizes are around 500 bytes
	IxMaxSize       = 128 * 1024 // 128Kb
	PruningCooldown = 5000 * time.Millisecond
)

const MaxWaitCounter = 10

const (
	BatchIDNotFound = -1
	conflictBatchID = -2
	maxBatches      = 20
)

var (
	ErrSequenceIDTooLow = errors.New("sequenceID too low")
	ErrAlreadyKnown     = errors.New("already known")
	ErrOversizedData    = errors.New("over sized data")
)

type stateManager interface {
	GetPublicKey(id identifiers.Identifier, KeyID uint64, stateHash common.Hash) ([]byte, error)
	GetSequenceID(id identifiers.Identifier, KeyID uint64, stateHash common.Hash) (uint64, error)
	IsAccountRegistered(id identifiers.Identifier) (bool, error)
	IsLogicRegistered(logicID identifiers.LogicID) error
	GetBalance(id identifiers.Identifier, assetID identifiers.AssetID, stateHash common.Hash) (*big.Int, error)
	GetAssetInfo(assetID identifiers.AssetID, hash common.Hash) (*common.AssetDescriptor, error)
	GetLatestStateObject(id identifiers.Identifier) (*state.Object, error)
	RemoveCachedObject(id identifiers.Identifier)
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	GetAccountKeys(id identifiers.Identifier, stateHash common.Hash) (common.AccountKeys, error)
}

type executionManager interface {
	ValidateLogicDeploy(op *common.IxOp) error
	ValidateLogicInvoke(op *common.IxOp, calleracc, logicacc *state.Object) error
	ValidateLogicEnlist(op *common.IxOp, calleracc, logicacc *state.Object) error
}

type p2pServer interface {
	Subscribe(
		ctx context.Context,
		topicName string,
		validator utils.WrappedVal,
		defaultValidator bool,
		handler func(msg *pubsub.Message) error,
	) error
	Broadcast(topicName string, data []byte) error
	GetKramaID() kramaid.KramaID
}

type IxConfig struct {
	Mode       int
	PriceLimit uint64
}

type IxPool struct {
	ctx                context.Context
	ctxCancel          context.CancelFunc
	mu                 sync.RWMutex
	logger             hclog.Logger
	cfg                *config.IxPoolConfig
	msgCache           *ixSaltedCache
	network            p2pServer
	sm                 stateManager
	exec               executionManager
	allIxs             *lookupMap
	close              chan struct{}
	mux                *utils.TypeMux
	accounts           *accountsManager
	gauge              slotGauge // gauge for measuring pool capacity
	pruneCh            chan struct{}
	metrics            *Metrics
	verifier           func(data, signature, pubBytes []byte) (bool, error)
	view               uint64
	genesisTime        time.Time
	consensusNodesHash *lru.Cache // consensusNodesHash holds only primary account's consensus nodes hash
}

func NewIxPool(
	logger hclog.Logger,
	mux *utils.TypeMux,
	node p2pServer,
	sm stateManager,
	exec executionManager,
	cfg *config.IxPoolConfig,
	metrics *Metrics,
	verifier func(data, signature, pubBytes []byte) (bool, error),
	genesisTime uint64,
) (*IxPool, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	var err error

	i := &IxPool{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		cfg:       cfg,
		mux:       mux,
		network:   node,
		sm:        sm,
		exec:      exec,
		allIxs:    NewLookupMap(),
		close:     make(chan struct{}),
		accounts:  newAccountsMap(),
		gauge: slotGauge{
			total:   0,
			max:     cfg.MaxSlots,
			metrics: metrics,
		},
		pruneCh:     make(chan struct{}),
		metrics:     metrics,
		logger:      logger.Named("Ix-Pool"),
		verifier:    verifier,
		genesisTime: time.Unix(int64(genesisTime), 0),
	}

	if cfg.EnableRawIxFiltering {
		i.msgCache = newSaltedCache(int(cfg.IxIncomingFilterMaxSize))
	}

	if i.consensusNodesHash, err = lru.New(2000); err != nil {
		return nil, err
	}

	return i, nil
}

func (i *IxPool) getPrimaryAccountConsensusNodesHash(id identifiers.Identifier) (common.Hash, error) {
	val, isCached := i.consensusNodesHash.Get(id)
	if isCached {
		return val.(common.Hash), nil //nolint:forcetypeassert
	}

	accMetaInfo, err := i.sm.GetAccountMetaInfo(id)
	if err != nil {
		return common.NilHash, err
	}

	i.consensusNodesHash.Add(id, accMetaInfo.ConsensusNodesHash)

	return accMetaInfo.ConsensusNodesHash, nil
}

func (i *IxPool) getConsensusNodesHash(id identifiers.Identifier) (common.Hash, error) {
	if id.IsParticipantVariant() {
		accMetaInfo, err := i.sm.GetAccountMetaInfo(id)
		if err != nil {
			return common.NilHash, err
		}

		return i.getPrimaryAccountConsensusNodesHash(accMetaInfo.InheritedAccount)
	}

	return i.getPrimaryAccountConsensusNodesHash(id)
}

func (i *IxPool) loadConsensusNodesHash(ix *common.Interaction) {
	for id, ps := range ix.Participants() {
		if !id.IsParticipantVariant() && !ps.IsGenesis {
			i.getPrimaryAccountConsensusNodesHash(id)
		}
	}
}

func (i *IxPool) ViewTimeOut() time.Duration {
	return i.cfg.ViewTimeout
}

func (i *IxPool) currentView() uint64 {
	return i.view
}

func (i *IxPool) UpdateCurrentView(view uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.view = view
}

func (i *IxPool) getNextView(view uint64, nodePos uint64) uint64 {
	diffFromStart := view % common.ConsensusNodesSize

	start := view - diffFromStart
	if nodePos >= diffFromStart {
		return start + nodePos
	}

	return start + common.ConsensusNodesSize + nodePos
}

func (i *IxPool) allocateView(view uint64, ixns ...*common.Interaction) {
	for _, ixn := range ixns {
		accMetaInfo, err := i.sm.GetAccountMetaInfo(ixn.LeaderCandidateAcc())
		if err != nil {
			i.logger.Error("account meta info not found", "id", ixn.LeaderCandidateAcc())

			continue
		}

		if ixn.LeaderCandidateAcc().IsParticipantVariant() {
			accMetaInfo, err = i.sm.GetAccountMetaInfo(accMetaInfo.InheritedAccount)
			if err != nil {
				i.logger.Error("account meta info of inherited account not found", "id", ixn.LeaderCandidateAcc())

				continue
			}
		}

		if accMetaInfo.PositionInContextSet == common.NodeNotFound {
			continue
		}

		ixn.SetShouldPropose(true)

		nextView := i.getNextView(view, uint64(accMetaInfo.PositionInContextSet))
		ixn.UpdateAllottedView(nextView)

		i.logger.Trace("Allotted view for ixn", "ixn-hash",
			ixn.Hash(), "position", accMetaInfo.PositionInContextSet,
			"current-view", view, "next-view", nextView, "leader-id", ixn.LeaderCandidateAcc())
	}
}

// GetPendingIx returns the interaction in ixpool for the given interaction hash
func (i *IxPool) GetPendingIx(ixHash common.Hash) (*common.Interaction, bool) {
	return i.allIxs.get(ixHash)
}

func (i *IxPool) GetIxns(ixHashes common.Hashes) ([]*common.Interaction, error) {
	ixns := make([]*common.Interaction, 0, len(ixHashes))

	for _, ixHash := range ixHashes {
		ix, found := i.allIxs.get(ixHash)
		if !found {
			return nil, errors.New(fmt.Sprintf("ixn not found in ixpool %s", ixHash))
		}

		ixns = append(ixns, ix)
	}

	return ixns, nil
}

func (i *IxPool) signalPruning() {
	select {
	case i.pruneCh <- struct{}{}:
	default: // pruning handler is in active or cooldown
	}
}

// getOrCreateAccountQueue fetches the account of the sender if it exists;
// otherwise, it creates a new account and returns it.
func (i *IxPool) getOrCreateAccountQueue(
	sender identifiers.Identifier,
	keyID uint64, sequenceID uint64,
) (*account, *accountQueue) {
	acc := i.accounts.getAccount(sender)

	if acc != nil {
		if accQueue := acc.get(keyID); accQueue != nil {
			return acc, accQueue
		}
	}

	stateSequenceID, err := i.sm.GetSequenceID(sender, keyID, common.NilHash)
	if err != nil {
		stateSequenceID = sequenceID
	}

	accQueue := &accountQueue{
		keyID:          keyID,
		enqueued:       newAccountQueue(),
		promoted:       newAccountQueue(),
		sequenceIDToIX: newSequenceIDToIXMap(),
		nextSequenceID: stateSequenceID,
	}

	if acc == nil {
		i.accounts.accounts[sender] = &account{
			accountQueues: []*accountQueue{accQueue},
			requestTime:   time.Now(),
			waitTime:      time.Now(),
		}

		return i.accounts.accounts[sender], accQueue
	}

	acc.accountQueues = append(acc.accountQueues, accQueue)
	acc.sortAccountQueues()

	return acc, accQueue
}

// validateAndEnqueueIx validates the Interaction, performs checks such as assessing pressure in the gauge,
// and signals for pruning. If the Interaction is a replacement, it is first attempted to be replaced in the
// enqueued queue; if unsuccessful, it is replaced in the promoted queue. Gauge adjustments are made during replacement.
// Finally, if the Interaction can be promoted, it is promoted.
func (i *IxPool) validateAndEnqueueIx(ix *common.Interaction) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	// validate incoming ix
	if err := i.validateIx(ix); err != nil {
		return err
	}

	acc, accQueue := i.getOrCreateAccountQueue(ix.SenderID(), ix.SenderKeyID(), ix.SequenceID())

	// checks if the current gauge size has reached the pressure mark and signals for account pruning if it has
	if i.gauge.highPressure() {
		i.signalPruning()

		if ix.SequenceID() > accQueue.getSequenceID() {
			return common.ErrRejectFutureIx // reject this ix as it will create sequenceID hole in enqueue and gets pruned
		}
	}

	oldIxWithSameSequenceID := accQueue.sequenceIDToIX.get(ix.SequenceID())
	if oldIxWithSameSequenceID != nil {
		if oldIxWithSameSequenceID.Hash() == ix.Hash() {
			return common.ErrAlreadyKnown
		}

		// TODO thrown an error if new interaction gas price is lower than equal to older interaction gas price
		// https://github.com/sarvalabs/go-moi/issues/695
		if oldIxWithSameSequenceID.FuelPrice().Cmp(ix.FuelPrice()) > 0 {
			return common.ErrReplacementUnderpriced
		}
	} else if ix.SequenceID() < accQueue.getSequenceID() {
		return ErrSequenceIDTooLow
	}

	slotsAllocated := slotsRequired(ix)

	var slotsFreed uint64

	if oldIxWithSameSequenceID != nil {
		slotsFreed = slotsRequired(oldIxWithSameSequenceID)
	}

	var slotsIncreased uint64
	if slotsAllocated > slotsFreed {
		slotsIncreased = slotsAllocated - slotsFreed
		if !i.gauge.increaseWithinLimit(slotsIncreased) {
			return common.ErrIXPoolOverFlow
		}
	}

	if ok := i.allIxs.add(ix); !ok {
		if slotsIncreased > 0 {
			i.gauge.decrease(slotsIncreased)
		}

		return ErrAlreadyKnown
	}

	if slotsFreed > slotsAllocated {
		i.gauge.decrease(slotsFreed - slotsAllocated)
	}

	if oldIxWithSameSequenceID != nil {
		i.allIxs.remove(oldIxWithSameSequenceID)

		oldIxSize, _ := oldIxWithSameSequenceID.Size()
		i.metrics.captureIxPoolSize(-1 * float64(oldIxSize))
	}

	ixSize, _ := ix.Size()
	i.metrics.captureIxPoolSize(float64(ixSize))

	// check the counter and reset if required
	if acc.getDelayCounter() >= MaxWaitCounter && time.Now().After(acc.getWaitTime()) {
		acc.resetWaitTimeAndCounter()
	}

	if replacedInPromoted := accQueue.enqueue(ix, oldIxWithSameSequenceID != nil); replacedInPromoted {
		i.allocateView(i.currentView()+1, ix)
	}
	// emit added interactions event
	if err := i.postAddedInteractionEvent(ix); err != nil {
		i.logger.Error("Error sending interaction added event", "err", err)
	}

	i.loadConsensusNodesHash(ix)

	i.logger.Info("added ix to enqueue ", ix.Hash())

	if ix.SequenceID() == accQueue.getSequenceID() {
		i.handlePromoteRequest(accQueue)
	}

	return nil
}

// AddRemoteInteractions validates and adds interactions broadcasted from other peers.
// To avoid spamming, the entire Ixn group is rejected if any single ixn is oversize or has an invalid id/signature.
// Ixn groups are also ignored if the size of the group is greater than 10 and more than 50% of the ixns are invalid.
func (i *IxPool) AddRemoteInteractions(ixs ...*common.Interaction) pubsub.ValidationResult {
	count := 0

	for _, ix := range ixs {
		newIx := *ix //nolint:govet

		if err := i.validateAndEnqueueIx(&newIx); err != nil {
			switch {
			case errors.Is(err, ErrOversizedData),
				errors.Is(err, common.ErrInvalidIdentifier),
				errors.Is(err, common.ErrInvalidIXSignature):
				i.logger.Error("Rejecting ixns", "ix-hash", ix.Hash(), "error", err)

				return pubsub.ValidationReject
			}

			count++
		}
	}

	if ixnCount := len(ixs); ixnCount > 10 && count > ixnCount/2 {
		return pubsub.ValidationIgnore
	}

	return pubsub.ValidationAccept
}

// AddLocalInteractions validates and adds interactions to the interaction pool.
// If flooding is disabled, the interactions are broadcast to the network through gossip sub.
func (i *IxPool) AddLocalInteractions(ixs common.Interactions) []error {
	errs := make([]error, 0, ixs.Len())
	validIxs := common.NewInteractions()

	for _, ix := range ixs.IxList() {
		if err := i.validateAndEnqueueIx(ix); err != nil {
			errs = append(errs, err)
		} else {
			validIxs.Append(ix)
		}
	}

	if !i.cfg.EnableIxFlooding && validIxs.Len() > 0 {
		rawData, err := validIxs.Bytes()
		if err != nil {
			i.logger.Error("unable to polorize ixns", "ixns", validIxs)

			return errs
		}

		if err = i.network.Broadcast(config.IxTopic, rawData); err != nil {
			i.logger.Error("failed to broadcast ixns", "error", err)
		}
	}

	return errs
}

func (i *IxPool) handlePromoteRequest(account *accountQueue) {
	// promote enqueued ixs
	promoted, promotedIxns := account.promote()
	i.metrics.capturePendingIxs(float64(promoted))

	for _, ix := range promotedIxns {
		i.accounts.addToSortedAccounts(ix.SenderID())
	}

	if len(promotedIxns) > 0 {
		i.allocateView(i.currentView()+1, promotedIxns...)

		// emit promoted interactions event
		if err := i.postPromotedInteractionEvent(promotedIxns...); err != nil {
			i.logger.Error("Error sending interaction promoted event", "err", err)
		}
	}
}

func (i *IxPool) RemoveCachedObject(id identifiers.Identifier) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.sm.RemoveCachedObject(id) // invalidate cache
}

func (i *IxPool) ResetWithHeaders(ts *common.Tesseract) {
	ixs := ts.Interactions().IxList()

	if ts != nil && len(ixs) > 0 {
		i.mu.Lock()
		defer i.mu.Unlock()

		// cleanup the lookup queue
		i.allIxs.remove(ixs...)

		// invalidate the consensusNodesHash cache if consensus nodes changes
		for addr, delta := range ts.ContextDelta() {
			if !addr.IsParticipantVariant() && len(delta.ConsensusNodes) > 0 {
				i.consensusNodesHash.Remove(addr)
			}
		}

		processedAccounts := make(map[identifiers.Identifier]uint64)

		for _, ix := range ixs {
			from := ix.Sender()
			// skip already processed accounts
			if _, processed := processedAccounts[from.ID]; processed {
				continue
			}

			// fetch the latest sequenceID from the state
			latestSequenceID, err := i.sm.GetSequenceID(from.ID, from.KeyID, common.NilHash)
			if err != nil {
				latestSequenceID = ix.SequenceID() + 1
			}

			i.logger.Debug("Latest sequenceID in the ixpool", "sequenceID", latestSequenceID, from.ID)
			// update the result map
			processedAccounts[from.ID] = latestSequenceID

			if !i.accounts.exists(from.ID) {
				continue
			}

			cleanup := func(ixns []*common.Interaction) {
				// update pool state
				i.allIxs.remove(ixns...)
				i.gauge.decrease(slotsRequired(ixns...))
			}

			acc, accQueue := i.accounts.getAccountAndAccountQueue(from.ID, from.KeyID)

			// prune promoted
			pruned := accQueue.promoted.prune(latestSequenceID)
			accQueue.sequenceIDToIX.remove(pruned...)

			if len(pruned) > 0 {
				cleanup(pruned)

				// emit pruned promoted interactions event
				if err := i.postPrunedPromotedInteractionEvent(pruned...); err != nil {
					i.logger.Error("Error sending interaction pruned promoted event", "err", err)
				}

				acc.waitLock.Lock()
				i.metrics.captureAccountWaitTime(acc.requestTime, acc.waitTime)
				acc.requestTime = time.Now()
				acc.waitLock.Unlock()
				// update the account waitTime and counter
				acc.resetWaitTimeAndCounter()

				if accQueue.promoted.length() == 0 {
					i.accounts.deleteInSortedAccounts(from.ID)
				}
			}

			i.metrics.capturePendingIxs(float64(-1 * len(pruned)))

			if ixSize, err := getIxsSize(pruned); err == nil {
				i.metrics.captureIxPoolSize(-1 * float64(ixSize))
			}

			if latestSequenceID <= accQueue.getSequenceID() {
				// only the promoted queue needed pruning
				continue
			}

			// prune enqueued
			pruned = accQueue.enqueued.prune(latestSequenceID)
			accQueue.sequenceIDToIX.remove(pruned...)

			if len(pruned) > 0 {
				cleanup(pruned)

				// emit pruned enqueued interactions event
				if err := i.postPrunedEnqueueInteractionEvent(pruned...); err != nil {
					i.logger.Error("Error sending interaction pruned enqueue event", "err", err)
				}
			}

			if ixSize, err := getIxsSize(pruned); err == nil {
				i.metrics.captureIxPoolSize(-1 * float64(ixSize))
			}

			// update next sequenceID
			accQueue.setSequenceID(latestSequenceID)

			if first := accQueue.enqueued.peek(); first != nil && first.SequenceID() == latestSequenceID {
				// first enqueued ix is expected -> signal promotion
				i.handlePromoteRequest(accQueue)
			}
		}
	}
}

func (i *IxPool) Executables() InteractionQueue {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.cfg.Mode == WaitMode {
		return i.accounts.getWaitPrimaries()
	} else if i.cfg.Mode == CostMode {
		return i.accounts.getCostPrimaries()
	}

	return nil
}

// isEligibleForProposal ensures mutual exclusion of participants among batches and also limits consensus nodes hashes
// per batch up to 4
func (i *IxPool) isEligibleForProposal(
	ixn *common.Interaction,
	participantToAcquirer map[identifiers.Identifier]identifiers.Identifier,
	acquirerToConsensusNodeHashes map[identifiers.Identifier]map[common.Hash]struct{},
	batchRegistry *IxBatchRegistry,
) bool {
	var (
		newConsensusNodesHashCount = 0
		leaderAcc                  = ixn.LeaderCandidateAcc()
	)

	existingConsensusNodesHash, ok := acquirerToConsensusNodeHashes[leaderAcc]
	if !ok {
		existingConsensusNodesHash = make(map[common.Hash]struct{})
	}

	for _, participant := range ixn.Participants() {
		if participant.IsGenesis {
			continue
		}

		if acquirer, ok := participantToAcquirer[participant.ID]; ok {
			if leaderAcc != acquirer {
				return false
			}

			continue
		}

		hash, err := i.getConsensusNodesHash(participant.ID)
		if err != nil {
			i.logger.Error("Error getting consensus node hash", "err", err, "id", participant.ID)

			return false
		}

		// add consensus nodes hash to batch registry as they will be useful in counting unique hashes in a batch
		batchRegistry.addConsensusNodesHash(participant.ID, hash)

		// Count new participants
		if _, exists := existingConsensusNodesHash[hash]; !exists {
			newConsensusNodesHashCount++
		}
	}

	// Check if total participants would exceed limit
	if len(existingConsensusNodesHash)+newConsensusNodesHashCount > 4 {
		return false
	}

	if _, ok := acquirerToConsensusNodeHashes[leaderAcc]; !ok {
		acquirerToConsensusNodeHashes[leaderAcc] = make(map[common.Hash]struct{})
	}

	for _, participant := range ixn.Participants() {
		if participant.IsGenesis {
			continue
		}

		// consensus nodes hash are fetched successfully for all participants above, so don't check for error here
		hash, _ := i.getConsensusNodesHash(participant.ID)

		participantToAcquirer[participant.ID] = leaderAcc
		acquirerToConsensusNodeHashes[leaderAcc][hash] = struct{}{}
	}

	return true
}

// ProcessableBatches picks interactions in deterministic fashion to avoid consensus failure
// due to multiple nodes trying to acquire a participant ending up with no quorum.
// It also ensures that interactions are picked only if they can be proposed by this node in the current view
func (i *IxPool) ProcessableBatches() []*common.IxBatch {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.logger.Debug("Processing batches ", "current view-id", i.view)

	batchRegistry := newBatchRegistry()

	participantToAcquirer := make(map[identifiers.Identifier]identifiers.Identifier)
	acquirerToConsensusNodeHashes := make(map[identifiers.Identifier]map[common.Hash]struct{})

	i.accounts.sortedParticipants.AscendGreaterOrEqual(
		&ID{id: identifiers.Nil},
		func(item llrb.Item) bool {
			acc := i.accounts.getAccount(item.(*ID).id) //nolint: forcetypeassert
			for _, accQueue := range acc.accountQueues {
				ixns := common.IxBySequenceID(
					common.NewInteractionsWithLeaderCheck(false, accQueue.promoted.list()...),
				)

				sort.Sort(ixns)

				for _, ixn := range ixns.List() {
					if !i.isEligibleForProposal(ixn, participantToAcquirer, acquirerToConsensusNodeHashes, batchRegistry) {
						break
					}

					if !ixn.ShouldPropose() {
						break
					}

					if ixn.AllottedView() < i.currentView() {
						i.allocateView(i.currentView(), ixn)
					}

					if ixn.AllottedView() > i.currentView() {
						break
					}

					if added := batchRegistry.addIx(ixn); !added {
						break
					}
				}
			}

			return true
		})

	return batchRegistry.selectOptimalBatches()
}

// Pop removes the given interaction from the
// associated promoted queue (account).
// Will update executables with the next primary
// from that account (if any).
func (i *IxPool) Pop(ix *common.Interaction) {
	// fetch the associated account
	acc := i.accounts.getAccountQueue(ix.SenderID(), ix.SenderKeyID())

	i.mu.Lock()
	defer i.mu.Unlock()

	// pop the top most promoted ix
	/*
		TODO://Need to check whether to move ixs

			// update executables
			if ix := account.promoted.peek(); ix != nil {
				i.executableQueue = append(i.executableQueue, ix)
			}
	*/

	ix = acc.promoted.pop()
	if ix != nil {
		i.gauge.decrease(slotsRequired(ix))
	}

	acc.sequenceIDToIX.remove(ix)
}

func (i *IxPool) Drop(ix *common.Interaction) {
	// fetch the associated acc
	acc := i.accounts.getAccountQueue(ix.SenderID(), ix.SenderKeyID())

	if acc != nil {
		sequenceID := ix.SequenceID()
		// fetch the latest sequenceID from the state
		if latestSequenceID, _ := i.sm.GetSequenceID(ix.SenderID(),
			ix.SenderKeyID(), common.NilHash); latestSequenceID > sequenceID {
			i.logger.Debug(
				"Skipping ix drop", "ix-hash", ix.Hash(),
				"ix-sequenceID", ix.SequenceID(), "latest-sequenceID", latestSequenceID,
			)

			return
		}

		noOfDroppedIxs := 0

		i.mu.Lock()
		defer i.mu.Unlock()

		// remove the dropped ixs from the allIxs lookup map and decreases gauge
		cleanup := func(ixs []*common.Interaction) {
			i.allIxs.remove(ixs...)
			i.gauge.decrease(slotsRequired(ixs...))

			noOfDroppedIxs += len(ixs)
		}

		acc.setSequenceID(sequenceID)

		// reset sequenceID to ix
		acc.sequenceIDToIX.reset()

		// drop promoted
		dropped := acc.promoted.clear()
		cleanup(dropped)

		if len(dropped) > 0 {
			// emit dropped interactions event
			if err := i.postDroppedInteractionEvent(dropped...); err != nil {
				i.logger.Error("Error sending interaction dropped event", "err", err)
			}
		}

		i.metrics.capturePendingIxs(float64(-1 * len(dropped)))

		// drop enqueued
		dropped = acc.enqueued.clear()
		cleanup(dropped)

		// drop the acc
		// i.accounts.remove(ix.SenderID()) FIXME: Issue(https://github.com/sarvalabs/go-moi/issues/256)

		i.logger.Debug("Dropped interactions", "count", noOfDroppedIxs,
			"next-sequenceID", sequenceID, "id", ix.SenderID())
	}
}

// IncrementWaitTime updates the waitTime for the given account
func (i *IxPool) IncrementWaitTime(id identifiers.Identifier, baseTime time.Duration) error {
	acc := i.accounts.getAccount(id)
	if acc == nil {
		return common.ErrAccountNotFound
	}

	if acc.getDelayCounter()+1 <= MaxWaitCounter {
		acc.incrementCounter(baseTime)
	} else {
		acc.resetWaitTimeAndCounter()
	}

	return nil
}

func (i *IxPool) verifyParticipantSignatures(
	id identifiers.Identifier,
	rawPayload []byte,
	signatures common.Signatures,
) error {
	accountKeys, err := i.sm.GetAccountKeys(id, common.NilHash)
	if err != nil {
		return err
	}

	count := uint64(len(accountKeys))
	weight := uint64(0)

	for _, sig := range signatures {
		if sig.ID != id {
			continue
		}

		if sig.KeyID >= count {
			return errors.New("invalid key id in signature")
		}

		pk, err := i.sm.GetPublicKey(id, sig.KeyID, common.NilHash)
		if err != nil {
			return err
		}

		if isVerified, err := i.verifier(rawPayload, sig.Signature, pk); !isVerified || err != nil {
			i.logger.Error("Invalid signature", "err", err, "id", id, "keyID", sig.KeyID)

			return common.ErrInvalidIXSignature
		}

		if !accountKeys[sig.KeyID].Revoked {
			weight += accountKeys[sig.KeyID].Weight
		}
	}

	if weight < common.MinWeight {
		return common.ErrInvalidWeight
	}

	return nil
}

func (i *IxPool) hasSenderKeyIDSignature(ix *common.Interaction) bool {
	for _, sig := range ix.Signatures() {
		if sig.ID == ix.SenderID() && sig.KeyID == ix.SenderKeyID() {
			return true
		}
	}

	return false
}

func (i *IxPool) verifySignatures(ix *common.Interaction) error {
	rawPayload, err := ix.PayloadForSignature()
	if err != nil {
		return err
	}

	signatures := ix.Signatures()

	if !i.hasSenderKeyIDSignature(ix) {
		return common.ErrInvalidSenderSignature
	}

	if err = i.verifyParticipantSignatures(ix.SenderID(), rawPayload, signatures); err != nil {
		return errors.Wrap(err, "invalid sender's signature")
	}

	for _, ps := range ix.IxParticipants() {
		if ps.Notary {
			if err := i.verifyParticipantSignatures(ps.ID, rawPayload, signatures); err != nil {
				return errors.Wrap(err, "invalid notary participant signature")
			}
		}
	}

	return nil
}

func (i *IxPool) validateIx(ix *common.Interaction) error {
	// Check the interaction size to overcome DOS Attacks
	ixSize, err := ix.Size()
	if err != nil {
		return err
	}

	if ixSize > IxMaxSize {
		return ErrOversizedData
	}

	if ix.SenderID().IsNil() {
		return common.ErrInvalidIdentifier
	}

	// TODO: Check the signature

	// Reject underpriced interactions
	if ix.IsUnderpriced(i.cfg.PriceLimit) {
		return common.ErrUnderpriced
	}

	// Check sequenceID ordering
	if n, _ := i.sm.GetSequenceID(ix.SenderID(), ix.SenderKeyID(), common.NilHash); n > ix.SequenceID() {
		return ErrSequenceIDTooLow
	}
	/*
		accountBalance, balanceErr := i.lattice.GetBalance(stateRoot, op.From)
		if balanceErr != nil {
			return ErrInvalidAccountState
		}

		// Check if the sender has enough funds to execute the interaction
		if accountBalance.Cmp(ix.Cost()) < 0 {
			return ErrInsufficientFunds
		}
	*/

	moiBal, _ := i.sm.GetBalance(ix.SenderID(), common.KMOITokenAssetID, common.NilHash)

	if moiBal.Cmp(ix.Cost()) < 0 {
		return common.ErrInsufficientFunds
	}

	if err := i.verifySignatures(ix); err != nil {
		return err
	}

	if err = i.validateFunds(ix); err != nil {
		return err
	}

	if err = i.validateOperations(ix); err != nil {
		return err
	}

	return nil
}

func (i *IxPool) validateFunds(ix *common.Interaction) error {
	for _, fund := range ix.Funds() {
		if fund.Amount.Sign() < 0 {
			return common.ErrInvalidValue
		}

		currentBalance, err := i.sm.GetBalance(ix.SenderID(), fund.AssetID, common.NilHash)
		if err != nil {
			return err
		}

		if currentBalance.Cmp(fund.Amount) < 0 {
			return common.ErrInsufficientFunds
		}
	}

	return nil
}

func (i *IxPool) validateOperations(ix *common.Interaction) error {
	for idx, op := range ix.Ops() {
		switch op.Type() {
		case common.IxParticipantCreate:
			return i.validateParticipantCreate(ix, idx)
		case common.IXAccountConfigure:
			return i.validateAccountConfigure(ix, idx)
		case common.IXAccountInherit:
			return i.validateAccountInherit(ix, idx)
		case common.IxAssetCreate:
			return i.validateAssetCreate(ix, idx)
		case common.IxAssetApprove:
			return i.validateAssetApprove(ix, idx)
		case common.IxAssetRevoke:
			return i.validateAssetRevoke(ix, idx)
		case common.IxAssetTransfer:
			return i.validateAssetTransfer(ix, idx)
		case common.IxAssetLockup:
			return i.validateAssetLockup(ix, idx)
		case common.IxAssetRelease:
			return i.validateAssetRelease(ix, idx)
		case common.IxAssetMint, common.IxAssetBurn:
			return i.validateAssetSupply(ix, idx)
		case common.IxLogicDeploy:
			return i.validateLogicDeployPayload(ix, idx)
		case common.IxLogicInvoke:
			return i.validateLogicInvokePayload(ix, idx)
		case common.IxLogicEnlist:
			return i.validateLogicEnlistPayload(ix, idx)
		default:
			return common.ErrInvalidInteractionType
		}
	}

	return nil
}

func (i *IxPool) validateAccountInherit(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAccountInheritPayload()
	if err != nil {
		return err
	}

	return payload.Validate(ix.SenderID())
}

func (i *IxPool) validateAssetCreate(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetCreatePayload()
	if err != nil {
		return err
	}

	return payload.Validate()
}

func (i *IxPool) validateParticipantCreate(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetParticipantCreatePayload()
	if err != nil {
		return err
	}

	return payload.Validate(ix.SenderID())
}

func (i *IxPool) validateAccountConfigure(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAccountConfigurePayload()
	if err != nil {
		return err
	}

	return payload.Validate()
}

func (i *IxPool) validateAssetApprove(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetActionPayload()
	if err != nil {
		return err
	}

	return payload.ValidateAssetApprove(ix.SenderID())
}

func (i *IxPool) validateAssetRevoke(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetActionPayload()
	if err != nil {
		return err
	}

	return payload.ValidateAssetRevoke(ix.SenderID())
}

func (i *IxPool) validateAssetTransfer(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetActionPayload()
	if err != nil {
		return err
	}

	return payload.ValidateAssetTransfer(ix.SenderID())
}

func (i *IxPool) validateAssetLockup(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetActionPayload()
	if err != nil {
		return err
	}

	return payload.ValidateAssetLockup(ix.SenderID())
}

func (i *IxPool) validateAssetRelease(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetActionPayload()
	if err != nil {
		return err
	}

	return payload.ValidateAssetRelease(ix.SenderID())
}

func (i *IxPool) validateAssetSupply(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetSupplyPayload()
	if err != nil {
		return err
	}

	return payload.Validate()
}

func (i *IxPool) validateLogicDeployPayload(ix *common.Interaction, txnID int) error {
	// Obtain logic payload
	payload, err := ix.GetIxOp(txnID).GetLogicPayload()
	if err != nil {
		return err
	}

	if err = payload.ValidateLogicDeploy(); err != nil {
		return err
	}

	if err = i.exec.ValidateLogicDeploy(ix.GetIxOp(txnID)); err != nil {
		return errors.Wrap(err, "failed to validate logic deploy")
	}

	return nil
}

func (i *IxPool) validateLogicInteractPayload(ix *common.Interaction, txnID int) error {
	// Obtain logic payload
	payload, err := ix.GetIxOp(txnID).GetLogicPayload()
	if err != nil {
		return err
	}

	if err = payload.ValidateLogicInteract(); err != nil {
		return err
	}

	// Check if logic is registered
	if err = i.sm.IsLogicRegistered(payload.Logic); err != nil {
		return err
	}

	return nil
}

func (i *IxPool) validateLogicInvokePayload(ix *common.Interaction, txnID int) error {
	if err := i.validateLogicInteractPayload(ix, txnID); err != nil {
		return err
	}

	// Obtain state object of sender
	callerAcc, err := i.sm.GetLatestStateObject(ix.SenderID())
	if err != nil {
		return err
	}

	// Obtain state object of receiver (logic)
	logicAcc, err := i.sm.GetLatestStateObject(ix.GetIxOp(txnID).Target())
	if err != nil {
		return err
	}

	if err := i.exec.ValidateLogicInvoke(ix.GetIxOp(txnID), callerAcc, logicAcc); err != nil {
		return errors.Wrap(err, "failed to validate logic invoke")
	}

	return nil
}

func (i *IxPool) validateLogicEnlistPayload(ix *common.Interaction, txnID int) error {
	if err := i.validateLogicInteractPayload(ix, txnID); err != nil {
		return err
	}

	// Obtain state object of sender
	callerAcc, err := i.sm.GetLatestStateObject(ix.SenderID())
	if err != nil {
		return err
	}

	// Obtain state object of receiver (logic)
	logicAcc, err := i.sm.GetLatestStateObject(ix.GetIxOp(txnID).Target())
	if err != nil {
		return err
	}

	if err := i.exec.ValidateLogicEnlist(ix.GetIxOp(txnID), callerAcc, logicAcc); err != nil {
		return errors.Wrap(err, "failed to validate logic enlist")
	}

	return nil
}

func (i *IxPool) removeSequenceIDHoleAccounts() {
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, acc := range i.accounts.accounts {
		for _, accQueue := range acc.accountQueues {
			ixn := accQueue.enqueued.peek()
			if ixn == nil {
				continue
			}

			// check if the account "enqueue" possesses a sequenceID hole,
			// and if so, remove all interactions from "enqueue" and all associated interactions in allixns map.

			if ixn.SequenceID() == accQueue.getSequenceID() {
				continue
			}

			dropped := accQueue.enqueued.clear()

			accQueue.sequenceIDToIX.remove(dropped...)
			i.allIxs.remove(dropped...)
			i.gauge.decrease(slotsRequired(dropped...))
		}
	}
}

func (i *IxPool) handlePruning() {
	for {
		select {
		case <-i.ctx.Done():
			return
		case <-i.pruneCh:
			i.removeSequenceIDHoleAccounts()
		}

		time.Sleep(PruningCooldown)
	}
}

func (i *IxPool) Close() {
	i.logger.Info("Closing IxPool")
	i.ctxCancel()
}

func (i *IxPool) incomingMsgDupCheck(data []byte) (*common.Hash, bool) {
	var (
		msgKey *common.Hash
		isDup  bool
	)

	if i.msgCache != nil {
		// check for duplicate messages
		// this helps against relaying duplicates
		if msgKey, isDup = i.msgCache.CheckAndPut(data); isDup {
			i.metrics.AddIxRawDupCount(1)

			return msgKey, true
		}
	}

	return msgKey, false
}

// IxValidator decides whether to propagate or reject interactions (ixns) based on validation checks.
// It can forward valid interactions to peers, ignore invalid ones, or punish the sender.
// Validations include checks for:
// - Self-originated ixns
// - Duplicate ixns
// - Zero ixns
// - Excess ixns
// - ixn payload level checks
func (i *IxPool) IxValidator(
	ctx context.Context,
	pid peer.ID,
	msg *pubsub.Message,
) (pubsub.ValidationResult, error) {
	peerID, err := i.network.GetKramaID().DecodedPeerID()
	if err != nil {
		return pubsub.ValidationReject, err
	}

	if msg.GetFrom() == peerID {
		return pubsub.ValidationAccept, nil
	}

	_, shouldDrop := i.incomingMsgDupCheck(msg.GetData())
	if shouldDrop {
		return pubsub.ValidationIgnore, nil
	}

	ixns := new(common.Interactions)

	if err := ixns.FromBytes(msg.GetData()); err != nil {
		return pubsub.ValidationReject, err
	}

	if ixnCount := len(ixns.IxList()); ixnCount == 0 || ixnCount > i.cfg.MaxIxGroupSize {
		i.logger.Error("Rejecting ixns", "peer-id", pid, "count", ixnCount)

		return pubsub.ValidationReject, errors.New("invalid number of ixns")
	}

	return i.AddRemoteInteractions(ixns.IxList()...), nil
}

func (i *IxPool) Start() error {
	i.metrics.initMetrics()

	go i.handlePruning()

	if i.msgCache != nil {
		go i.msgCache.Start(i.ctx, 60*time.Second)
	}

	if !i.cfg.EnableIxFlooding {
		if err := i.network.Subscribe(i.ctx, config.IxTopic, i.IxValidator, false, nil); err != nil {
			return err
		}
	}

	return nil
}

func (i *IxPool) post(ev interface{}) error {
	if i.mux != nil {
		return i.mux.Post(ev)
	}

	return nil
}

func (i *IxPool) postAddedInteractionEvent(ixns ...*common.Interaction) error {
	return i.post(utils.AddedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postPromotedInteractionEvent(ixns ...*common.Interaction) error {
	return i.post(utils.PromotedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postDroppedInteractionEvent(ixns ...*common.Interaction) error {
	return i.post(utils.DroppedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postPrunedEnqueueInteractionEvent(ixns ...*common.Interaction) error {
	return i.post(utils.PrunedEnqueuedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postPrunedPromotedInteractionEvent(ixns ...*common.Interaction) error {
	return i.post(utils.PrunedPromotedInteractionEvent{Ixs: ixns})
}

// helper functions

// getIxsSize aggregates and returns the size of all the interactions.
func getIxsSize(ixs []*common.Interaction) (uint64, error) {
	var ixsSize uint64

	for _, ix := range ixs {
		size, err := ix.Size()
		if err != nil {
			return 0, err
		}

		ixsSize += size
	}

	return ixsSize, nil
}

// getIxParticipants returns the unique participants involved in the interaction
func getIxParticipants(ix *common.Interaction) map[identifiers.Identifier]struct{} {
	participants := make(map[identifiers.Identifier]struct{})

	participants[ix.SenderID()] = struct{}{}

	if !ix.Payer().IsNil() {
		participants[ix.Payer()] = struct{}{}
	}

	for idx, op := range ix.Ops() {
		if op.Type() == common.IxAssetCreate || op.Type() == common.IxLogicDeploy {
			continue
		}

		participants[ix.GetIxOp(idx).Target()] = struct{}{}
	}

	return participants
}
