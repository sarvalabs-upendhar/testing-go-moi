package ixpool

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/state"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
)

const (
	WaitMode = iota
	CostMode
)

const (
	IxSlotSize        = 1 * 1024   // IxSlotSize chosen as 1kB as minimum ixn sizes are around 500 bytes
	IxMaxSize         = 128 * 1024 // 128Kb
	PruningCooldown   = 5000 * time.Millisecond
	TotalContextNodes = 6
)

const MaxWaitCounter = 10

const (
	BatchIDNotFound = -1
	conflictBatchID = -2
	maxBatches      = 20
)

var (
	ErrNonceTooLow   = errors.New("nonce too low")
	ErrAlreadyKnown  = errors.New("already known")
	ErrOversizedData = errors.New("over sized data")
)

type stateManager interface {
	GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error)
	IsAccountRegistered(addr identifiers.Address) (bool, error)
	IsLogicRegistered(logicID identifiers.LogicID) error
	GetBalance(addrs identifiers.Address, assetID identifiers.AssetID, stateHash common.Hash) (*big.Int, error)
	GetAssetInfo(assetID identifiers.AssetID, hash common.Hash) (*common.AssetDescriptor, error)
	GetLatestStateObject(addr identifiers.Address) (*state.Object, error)
	RemoveCachedObject(addr identifiers.Address)
	GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error)
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
	ctx         context.Context
	ctxCancel   context.CancelFunc
	mu          sync.RWMutex
	logger      hclog.Logger
	cfg         *config.IxPoolConfig
	msgCache    *ixSaltedCache
	network     p2pServer
	sm          stateManager
	exec        executionManager
	allIxs      *lookupMap
	close       chan struct{}
	sealing     bool
	mux         *utils.TypeMux
	accounts    *accountsMap
	gauge       slotGauge // gauge for measuring pool capacity
	pruneCh     chan struct{}
	metrics     *Metrics
	verifier    func(data, signature, pubBytes []byte) (bool, error)
	view        uint64
	genesisTime time.Time
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
) *IxPool {
	ctx, ctxCancel := context.WithCancel(context.Background())
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
		sealing:   false,
		accounts:  new(accountsMap),
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

	return i
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
	diffFromStart := view % TotalContextNodes

	start := view - diffFromStart
	if nodePos >= diffFromStart {
		return start + nodePos
	}

	return start + TotalContextNodes + nodePos
}

func (i *IxPool) allocateView(view uint64, ixns ...*common.Interaction) {
	for _, ixn := range ixns {
		acc, _ := i.sm.GetAccountMetaInfo(ixn.LeaderCandidateAcc())
		if acc.PositionInContextSet == common.NodeNotFound {
			continue
		}

		ixn.SetShouldPropose(true)

		nextView := i.getNextView(view, uint64(acc.PositionInContextSet))
		ixn.UpdateAllottedView(nextView)

		i.logger.Trace("Allotted view for ixn", "ixn-hash",
			ixn.Hash(), "position", acc.PositionInContextSet,
			"current-view", view, "next-view", nextView, "leader-addr", ixn.LeaderCandidateAcc())
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

// getOrCreateAccount fetches the account of the sender if it exists;
// otherwise, it creates a new account and returns it.
func (i *IxPool) getOrCreateAccount(ix *common.Interaction) *account {
	if acc := i.accounts.get(ix.Sender()); acc != nil {
		return acc
	}

	return i.createAccountOnce(ix.Sender(), ix.Nonce())
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

	acc := i.getOrCreateAccount(ix)

	// checks if the current gauge size has reached the pressure mark and signals for account pruning if it has
	if i.gauge.highPressure() {
		i.signalPruning()

		if ix.Nonce() > acc.getNonce() {
			return common.ErrRejectFutureIx // reject this ix as it will create nonce hole in enqueue and gets pruned
		}
	}

	oldIxWithSameNonce := acc.nonceToIX.get(ix.Nonce())
	if oldIxWithSameNonce != nil {
		if oldIxWithSameNonce.Hash() == ix.Hash() {
			return common.ErrAlreadyKnown
		}

		// TODO thrown an error if new interaction gas price is lower than equal to older interaction gas price
		// https://github.com/sarvalabs/go-moi/issues/695
		if oldIxWithSameNonce.FuelPrice().Cmp(ix.FuelPrice()) > 0 {
			return common.ErrReplacementUnderpriced
		}
	} else if ix.Nonce() < acc.getNonce() {
		return ErrNonceTooLow
	}

	slotsAllocated := slotsRequired(ix)

	var slotsFreed uint64

	if oldIxWithSameNonce != nil {
		slotsFreed = slotsRequired(oldIxWithSameNonce)
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

	if oldIxWithSameNonce != nil {
		i.allIxs.remove(oldIxWithSameNonce)

		oldIxSize, _ := oldIxWithSameNonce.Size()
		i.metrics.captureIxPoolSize(-1 * float64(oldIxSize))
	}

	ixSize, _ := ix.Size()
	i.metrics.captureIxPoolSize(float64(ixSize))

	acc.enqueue(ix, oldIxWithSameNonce != nil)

	// emit added interactions event
	if err := i.postAddedInteractionEvent(ix); err != nil {
		i.logger.Error("Error sending interaction added event", "err", err)
	}

	i.logger.Info("added ix to enqueue ", ix.Hash())

	if ix.Nonce() == acc.getNonce() {
		i.handlePromoteRequest(acc)
	}

	return nil
}

// AddRemoteInteractions validates and adds interactions broadcasted from other peers.
// To avoid spamming, the entire Ixn group is rejected if any single ixn is oversize or has an invalid addr/signature.
// Ixn groups are also ignored if the size of the group is greater than 10 and more than 50% of the ixns are invalid.
func (i *IxPool) AddRemoteInteractions(ixs ...*common.Interaction) pubsub.ValidationResult {
	count := 0

	for _, ix := range ixs {
		newIx := *ix //nolint:govet

		if err := i.validateAndEnqueueIx(&newIx); err != nil {
			switch {
			case errors.Is(err, ErrOversizedData),
				errors.Is(err, common.ErrInvalidAddress),
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

func (i *IxPool) handlePromoteRequest(account *account) {
	// promote enqueued ixs
	promoted, promotedIxns := account.promote()
	i.metrics.capturePendingIxs(float64(promoted))

	if len(promotedIxns) > 0 {
		i.allocateView(i.currentView()+1, promotedIxns...)

		// emit promoted interactions event
		if err := i.postPromotedInteractionEvent(promotedIxns...); err != nil {
			i.logger.Error("Error sending interaction promoted event", "err", err)
		}
	}
}

// createAccountOnce creates an account and
// ensures it is only initialized once.
func (i *IxPool) createAccountOnce(newAddr identifiers.Address, nonce uint64) *account {
	// fetch nonce from the latest state
	stateNonce, err := i.sm.GetNonce(newAddr, common.NilHash)
	if err != nil {
		stateNonce = nonce
	}

	// initialize the account
	return i.accounts.initOnce(newAddr, stateNonce)
}

func (i *IxPool) RemoveCachedObject(addr identifiers.Address) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.sm.RemoveCachedObject(addr) // invalidate cache
}

func (i *IxPool) ResetWithHeaders(ts *common.Tesseract) {
	ixs := ts.Interactions().IxList()

	if ts != nil && len(ixs) > 0 {
		i.mu.Lock()
		defer i.mu.Unlock()

		// cleanup the lookup queue
		i.allIxs.remove(ixs...)

		processedAccounts := make(map[identifiers.Address]uint64)

		for _, ix := range ixs {
			from := ix.Sender()
			// skip already processed accounts
			if _, processed := processedAccounts[from]; processed {
				continue
			}

			// fetch the latest nonce from the state
			latestNonce, err := i.sm.GetNonce(from, common.NilHash)
			if err != nil {
				latestNonce = ix.Nonce() + 1
			}

			i.logger.Debug("Latest nonce in the ixpool", "nonce", latestNonce)
			// update the result map
			processedAccounts[from] = latestNonce

			if !i.accounts.exists(from) {
				continue
			}

			cleanup := func(ixns []*common.Interaction) {
				// update pool state
				i.allIxs.remove(ixns...)
				i.gauge.decrease(slotsRequired(ixns...))
			}

			account := i.accounts.get(from)

			// prune promoted
			pruned := account.promoted.prune(latestNonce)
			account.nonceToIX.remove(pruned...)

			if len(pruned) > 0 {
				cleanup(pruned)

				// emit pruned promoted interactions event
				if err := i.postPrunedPromotedInteractionEvent(pruned...); err != nil {
					i.logger.Error("Error sending interaction pruned promoted event", "err", err)
				}

				account.waitLock.Lock()
				i.metrics.captureAccountWaitTime(account.requestTime, account.waitTime)
				account.requestTime = time.Now()
				account.waitLock.Unlock()
				// update the account waitTime and counter
				account.resetWaitTimeAndCounter()
			}

			i.metrics.capturePendingIxs(float64(-1 * len(pruned)))

			if ixSize, err := getIxsSize(pruned); err == nil {
				i.metrics.captureIxPoolSize(-1 * float64(ixSize))
			}

			if latestNonce <= account.getNonce() {
				// only the promoted queue needed pruning
				continue
			}

			// prune enqueued
			pruned = account.enqueued.prune(latestNonce)
			account.nonceToIX.remove(pruned...)

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

			// update next nonce
			account.setNonce(latestNonce)

			if first := account.enqueued.peek(); first != nil && first.Nonce() == latestNonce {
				// first enqueued ix is expected -> signal promotion
				i.handlePromoteRequest(account)
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

func (i *IxPool) ProcessableBatches() []*common.IxBatch {
	i.mu.Lock()
	defer i.mu.Unlock()

	batchRegistry := newBatchRegistry()

	i.accounts.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(identifiers.Address)
		if !ok {
			return false
		}

		account := i.accounts.get(addressKey)

		ixns := common.IxByNonce(common.NewInteractionsWithLeaderCheck(false, account.promoted.list()...))

		sort.Sort(ixns)

		for _, ixn := range ixns.List() {
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
	account := i.accounts.get(ix.Sender())

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

	ix = account.promoted.pop()
	if ix != nil {
		i.gauge.decrease(slotsRequired(ix))
	}

	account.nonceToIX.remove(ix)
}

func (i *IxPool) Drop(ix *common.Interaction) {
	// fetch the associated account
	account := i.accounts.get(ix.Sender())

	if account != nil {
		nonce := ix.Nonce()
		// fetch the latest nonce from the state
		if latestNonce, _ := i.sm.GetNonce(ix.Sender(), common.NilHash); latestNonce > nonce {
			i.logger.Debug(
				"Skipping ix drop", "ix-hash", ix.Hash(),
				"ix-nonce", ix.Nonce(), "latest-nonce", latestNonce,
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

		account.setNonce(nonce)

		// reset nonce to ix
		account.nonceToIX.reset()

		// drop promoted
		dropped := account.promoted.clear()
		cleanup(dropped)

		if len(dropped) > 0 {
			// emit dropped interactions event
			if err := i.postDroppedInteractionEvent(dropped...); err != nil {
				i.logger.Error("Error sending interaction dropped event", "err", err)
			}
		}

		i.metrics.capturePendingIxs(float64(-1 * len(dropped)))

		// drop enqueued
		dropped = account.enqueued.clear()
		cleanup(dropped)

		// drop the account
		// i.accounts.remove(ix.Sender()) FIXME: Issue(https://github.com/sarvalabs/go-moi/issues/256)

		i.logger.Debug("Dropped interactions", "count", noOfDroppedIxs, "next-nonce", nonce, "addr", ix.Sender())
	}
}

// IncrementWaitTime updates the waitTime for the given account
func (i *IxPool) IncrementWaitTime(addr identifiers.Address, baseTime time.Duration) error {
	acc := i.accounts.get(addr)
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

func (i *IxPool) validateIx(ix *common.Interaction) error {
	// Check the interaction size to overcome DOS Attacks
	ixSize, err := ix.Size()
	if err != nil {
		return err
	}

	if ixSize > IxMaxSize {
		return ErrOversizedData
	}

	if ix.Sender().IsNil() {
		return common.ErrInvalidAddress
	}

	// TODO: Check the signature

	// Reject underpriced interactions
	if ix.IsUnderpriced(i.cfg.PriceLimit) {
		return common.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := i.sm.GetNonce(ix.Sender(), common.NilHash); n > ix.Nonce() {
		return ErrNonceTooLow
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

	moiBal, _ := i.sm.GetBalance(ix.Sender(), common.KMOITokenAssetID, common.NilHash)

	if moiBal.Cmp(ix.Cost()) < 0 {
		return common.ErrInsufficientFunds
	}

	rawPayload, err := ix.PayloadForSignature()
	if err != nil {
		return err
	}

	isVerified, err := i.verifier(rawPayload, ix.Signature(), ix.Sender().Bytes())
	if !isVerified || err != nil {
		return common.ErrInvalidIXSignature
	}

	if err = i.validateFunds(ix); err != nil {
		return err
	}

	if err = i.validateTransactions(ix); err != nil {
		return err
	}

	return nil
}

func (i *IxPool) validateFunds(ix *common.Interaction) error {
	for _, fund := range ix.Funds() {
		if fund.Amount.Sign() < 0 {
			return common.ErrInvalidValue
		}

		currentBalance, err := i.sm.GetBalance(ix.Sender(), fund.AssetID, common.NilHash)
		if err != nil {
			return err
		}

		if currentBalance.Cmp(fund.Amount) < 0 {
			return common.ErrInsufficientFunds
		}
	}

	return nil
}

func (i *IxPool) validateTransactions(ix *common.Interaction) error {
	for idx, op := range ix.Ops() {
		switch op.Type() {
		case common.IxParticipantCreate:
			return i.validateParticipantRegister(ix, idx)
		case common.IxAssetCreate:
			return i.validateAssetCreate(ix, idx)
		case common.IxAssetTransfer:
			return i.validateAssetTransfer(ix, idx)
		case common.IxAssetMint:
			return i.validateAssetMint(ix, idx)
		case common.IxAssetBurn:
			return i.validateAssetBurn(ix, idx)
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

func (i *IxPool) validateAssetCreate(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetCreatePayload()
	if err != nil {
		return err
	}

	// asset standard should be mas1 or mas2
	if payload.Standard != common.MAS1 && payload.Standard != common.MAS0 {
		return common.ErrInvalidAssetStandard
	}

	// supply should be one if asset standard is mas1
	if payload.Standard == common.MAS1 {
		if payload.Supply == nil || payload.Supply.Uint64() != 1 {
			return common.ErrInvalidAssetSupply
		}
	}

	return nil
}

func (i *IxPool) validateParticipantRegister(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetParticipantCreatePayload()
	if err != nil {
		return err
	}

	if payload.Address.IsNil() {
		return common.ErrInvalidAddress
	}

	if registered, err := i.sm.IsAccountRegistered(payload.Address); err != nil || registered {
		return common.ErrAlreadyRegistered
	}

	if payload.Amount.Sign() < 0 {
		return common.ErrInvalidValue
	}

	currentBalance, err := i.sm.GetBalance(ix.Sender(), common.KMOITokenAssetID, common.NilHash)
	if err != nil {
		return err
	}

	if currentBalance.Cmp(payload.Amount) < 0 {
		return common.ErrInsufficientFunds
	}

	return nil
}

func (i *IxPool) validateAssetTransfer(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetActionPayload()
	if err != nil {
		return err
	}

	if payload.Beneficiary.IsNil() {
		return common.ErrInvalidAddress
	}

	if ix.Sender() == payload.Beneficiary {
		return common.ErrInvalidIxParticipants
	}

	// Reject genesis account interaction
	if payload.Beneficiary == common.SargaAddress {
		return common.ErrGenesisAccount
	}

	if registered, err := i.sm.IsAccountRegistered(payload.Beneficiary); err != nil || !registered {
		return common.ErrBeneficiaryNotRegistered
	}

	if payload.Amount.Sign() < 0 {
		return common.ErrInvalidValue
	}

	currentBalance, err := i.sm.GetBalance(ix.Sender(), payload.AssetID, common.NilHash)
	if err != nil {
		return err
	}

	if currentBalance.Cmp(payload.Amount) < 0 {
		return common.ErrInsufficientFunds
	}

	return nil
}

func (i *IxPool) validateAssetMint(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetSupplyPayload()
	if err != nil {
		return err
	}

	assetID, err := payload.AssetID.Identifier()
	if err != nil {
		return err
	}

	// can not mint asset standard mas1
	if common.AssetStandard(assetID.Standard()) == common.MAS1 {
		return common.ErrMintNonFungibleToken
	}

	assetInfo, err := i.sm.GetAssetInfo(payload.AssetID, common.NilHash)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// only operator can mint asset
	if assetInfo.Operator != ix.Sender() {
		return common.ErrOperatorMismatch
	}

	return nil
}

func (i *IxPool) validateAssetBurn(ix *common.Interaction, txnID int) error {
	payload, err := ix.GetIxOp(txnID).GetAssetSupplyPayload()
	if err != nil {
		return err
	}

	// make sure asset exists
	assetInfo, err := i.sm.GetAssetInfo(payload.AssetID, common.NilHash)
	if err != nil {
		return common.ErrAssetNotFound
	}

	currentBal, err := i.sm.GetBalance(ix.Sender(), payload.AssetID, common.NilHash)
	if err != nil {
		return err
	}

	// cannot burn amount greater than current balance
	if currentBal.Cmp(payload.Amount) < 0 {
		return common.ErrInsufficientFunds
	}

	// only operator can burn asset
	if assetInfo.Operator != ix.Sender() {
		return common.ErrOperatorMismatch
	}

	return nil
}

func (i *IxPool) validateLogicDeployPayload(ix *common.Interaction, txnID int) error {
	// Obtain logic payload
	payload, err := ix.GetIxOp(txnID).GetLogicPayload()
	if err != nil {
		return err
	}

	// Manifest cannot be empty for logic deploy
	if len(payload.Manifest) == 0 {
		return common.ErrEmptyManifest
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

	// Callsite cannot be empty
	if len(payload.Callsite) == 0 {
		return common.ErrEmptyCallSite
	}

	// LogicID cannot be empty
	if len(payload.Logic) == 0 {
		return common.ErrMissingLogicID
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
	callerAcc, err := i.sm.GetLatestStateObject(ix.Sender())
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
	callerAcc, err := i.sm.GetLatestStateObject(ix.Sender())
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

func (i *IxPool) removeNonceHoleAccounts() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.accounts.Range(
		func(key, value any) bool {
			acc, _ := value.(*account)

			ixn := acc.enqueued.peek()
			if ixn == nil {
				return true
			}

			// check if the account "enqueue" possesses a nonce hole,
			// and if so, remove all interactions from "enqueue" and all associated interactions in allixns map.

			if ixn.Nonce() == acc.getNonce() {
				return true
			}

			dropped := acc.enqueued.clear()

			acc.nonceToIX.remove(dropped...)
			i.allIxs.remove(dropped...)
			i.gauge.decrease(slotsRequired(dropped...))

			return true
		})
}

func (i *IxPool) handlePruning() {
	for {
		select {
		case <-i.ctx.Done():
			return
		case <-i.pruneCh:
			i.removeNonceHoleAccounts()
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
// It can forward valid transactions to peers, ignore invalid ones, or punish the sender.
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
func getIxParticipants(ix *common.Interaction) map[identifiers.Address]struct{} {
	participants := make(map[identifiers.Address]struct{})

	participants[ix.Sender()] = struct{}{}

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
