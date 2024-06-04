package ixpool

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/state"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
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
	ixSlotSize      = 1 * 1024   // ixSlotSize chosen as 1kB as minimum ixn sizes are around 500 bytes
	ixMaxSize       = 128 * 1024 // 128Kb
	pruningCooldown = 5000 * time.Millisecond
)

const MaxWaitCounter = 10

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
	RemoveCacheObject(addr identifiers.Address)
}

type executionManager interface {
	ValidateLogicInvoke(receiverObject *state.Object, ix *common.Interaction) error
	ValidateLogicDeploy(ix *common.Interaction, manifest []byte) error
}

type IxConfig struct {
	Mode       int
	PriceLimit uint64
}

type IxPool struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	mu        sync.RWMutex
	logger    hclog.Logger
	cfg       *config.IxPoolConfig
	sm        stateManager
	exec      executionManager
	allIxs    *lookupMap
	close     chan struct{}
	sealing   bool
	mux       *utils.TypeMux
	accounts  *accountsMap
	gauge     slotGauge // gauge for measuring pool capacity
	pruneCh   chan struct{}
	metrics   *Metrics
	verifier  func(data, signature, pubBytes []byte) (bool, error)
}

func NewIxPool(
	logger hclog.Logger,
	mux *utils.TypeMux,
	sm stateManager,
	exec executionManager,
	cfg *config.IxPoolConfig,
	metrics *Metrics,
	verifier func(data, signature, pubBytes []byte) (bool, error),
) *IxPool {
	ctx, ctxCancel := context.WithCancel(context.Background())
	i := &IxPool{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		cfg:       cfg,
		mux:       mux,
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
		pruneCh:  make(chan struct{}),
		metrics:  metrics,
		logger:   logger.Named("Ix-Pool"),
		verifier: verifier,
	}

	return i
}

// GetPendingIx returns the interaction in ixpool for the given interaction hash
func (i *IxPool) GetPendingIx(ixHash common.Hash) (*common.Interaction, bool) {
	return i.allIxs.get(ixHash)
}

func (i *IxPool) GetIxns(ixHashes common.Hashes) (common.Interactions, error) {
	ixns := make(common.Interactions, 0, len(ixHashes))

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
	if err := i.postAddedInteractionEvent(common.Interactions{ix}); err != nil {
		i.logger.Error("Error sending interaction added event", "err", err)
	}

	i.logger.Info("added ix to enqueue ", ix.Hash())

	if ix.Nonce() == acc.getNonce() {
		i.handlePromoteRequest(acc)
	}

	return nil
}

func (i *IxPool) AddInteractions(ixs common.Interactions) []error {
	errs := make([]error, 0, len(ixs))

	for _, ix := range ixs {
		if err := i.validateAndEnqueueIx(ix); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (i *IxPool) handlePromoteRequest(account *account) {
	// promote enqueued ixs
	promoted, promotedIxns := account.promote()
	i.metrics.capturePendingIxs(float64(promoted))

	if len(promotedIxns) > 0 {
		// emit promoted interactions event
		if err := i.postPromotedInteractionEvent(promotedIxns); err != nil {
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

	i.sm.RemoveCacheObject(addr) // invalidate cache
}

func (i *IxPool) ResetWithHeaders(ts *common.Tesseract) {
	ixs := ts.Interactions()

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

			cleanup := func(ixns common.Interactions) {
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
				if err := i.postPrunedPromotedInteractionEvent(pruned); err != nil {
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

			if ixSize, err := GetIxsSize(pruned); err == nil {
				i.metrics.captureIxPoolSize(-1 * float64(ixSize))
			}

			if latestNonce <= account.getNonce() {
				// only the promoted queue needed pruning
				return
			}

			// prune enqueued
			pruned = account.enqueued.prune(latestNonce)
			account.nonceToIX.remove(pruned...)

			if len(pruned) > 0 {
				cleanup(pruned)

				// emit pruned enqueued interactions event
				if err := i.postPrunedEnqueueInteractionEvent(pruned); err != nil {
					i.logger.Error("Error sending interaction pruned enqueue event", "err", err)
				}
			}

			if ixSize, err := GetIxsSize(pruned); err == nil {
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
		cleanup := func(ixs common.Interactions) {
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
			if err := i.postDroppedInteractionEvent(dropped); err != nil {
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

	if ixSize > ixMaxSize {
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
		accountBalance, balanceErr := i.lattice.GetBalance(stateRoot, tx.From)
		if balanceErr != nil {
			return ErrInvalidAccountState
		}

		// Check if the sender has enough funds to execute the interaction
		if accountBalance.Cmp(ix.Cost()) < 0 {
			return ErrInsufficientFunds
		}
	*/

	moiBal, err := i.sm.GetBalance(ix.Sender(), common.KMOITokenAssetID, common.NilHash)
	if err != nil {
		i.logger.Error("Error fetching balance", "sender", ix.Sender(), "err", err)

		return common.ErrInsufficientFunds
	}

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

	switch ix.Type() {
	case common.IxAssetCreate:
		return i.validateAssetCreate(ix)
	case common.IxValueTransfer:
		return i.validateValueTransfer(ix)
	case common.IxAssetMint:
		return i.validateAssetMint(ix)
	case common.IxAssetBurn:
		return i.validateAssetBurn(ix)
	case common.IxLogicDeploy:
		return i.validateLogicDeployPayload(ix)
	case common.IxLogicInvoke:
		return i.validateLogicInvokePayload(ix)
	default:
		return common.ErrInvalidInteractionType
	}
}

func (i *IxPool) validateAssetCreate(ix *common.Interaction) error {
	payload, err := ix.GetAssetPayload()
	if err != nil {
		return err
	}

	// asset standard should be mas1 or mas2
	if payload.Create.Standard != common.MAS1 && payload.Create.Standard != common.MAS0 {
		return common.ErrInvalidAssetStandard
	}

	// supply should be one if asset standard is mas1
	if payload.Create.Standard == common.MAS1 {
		if payload.Create.Supply == nil || payload.Create.Supply.Uint64() != 1 {
			return common.ErrInvalidAssetSupply
		}
	}

	return nil
}

func (i *IxPool) validateValueTransfer(ix *common.Interaction) error {
	if len(ix.TransferValues()) == 0 {
		return common.ErrEmptyTransferValues
	}

	for assetID, v := range ix.TransferValues() {
		if v.Sign() < 0 {
			return common.ErrInvalidValue
		}

		currentBalance, err := i.sm.GetBalance(ix.Sender(), assetID, common.NilHash)
		if err != nil {
			return err
		}

		if currentBalance.Cmp(v) < 0 {
			return common.ErrInsufficientFunds
		}
	}

	return nil
}

func (i *IxPool) validateAssetMint(ix *common.Interaction) error {
	assetPayload, err := ix.GetAssetPayload()
	if err != nil {
		return err
	}

	assetID, err := assetPayload.Mint.Asset.Identifier()
	if err != nil {
		return err
	}

	// can not mint asset standard mas1
	if common.AssetStandard(assetID.Standard()) == common.MAS1 {
		return common.ErrMintNonFungibleToken
	}

	assetInfo, err := i.sm.GetAssetInfo(assetPayload.Mint.Asset, common.NilHash)
	if err != nil {
		return common.ErrAssetNotFound
	}

	// only operator can mint asset
	if assetInfo.Operator != ix.Sender() {
		return common.ErrOperatorMismatch
	}

	return nil
}

func (i *IxPool) validateAssetBurn(ix *common.Interaction) error {
	assetPayload, err := ix.GetAssetPayload()
	if err != nil {
		return err
	}

	// make sure asset exists
	assetInfo, err := i.sm.GetAssetInfo(assetPayload.Mint.Asset, common.NilHash)
	if err != nil {
		return common.ErrAssetNotFound
	}

	currentBal, err := i.sm.GetBalance(ix.Sender(), assetPayload.Mint.Asset, common.NilHash)
	if err != nil {
		return err
	}

	// cannot burn amount greater than current balance
	if currentBal.Cmp(assetPayload.Mint.Amount) < 0 {
		return common.ErrInsufficientFunds
	}

	// only operator can burn asset
	if assetInfo.Operator != ix.Sender() {
		return common.ErrOperatorMismatch
	}

	return nil
}

func (i *IxPool) validateLogicDeployPayload(ix *common.Interaction) error {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return err
	}

	// manifest cannot be empty
	if len(payload.Manifest) == 0 {
		return common.ErrEmptyManifest
	}

	if err := i.exec.ValidateLogicDeploy(ix, payload.Manifest); err != nil {
		return errors.Wrap(err, "failed to validate logic deploy")
	}

	return nil
}

func (i *IxPool) validateLogicInvokePayload(ix *common.Interaction) error {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return err
	}

	// callsite cannot be empty
	if len(payload.Callsite) == 0 {
		return common.ErrEmptyCallSite
	}

	// logicID cannot be empty
	if len(payload.Logic) == 0 {
		return common.ErrMissingLogicID
	}

	receiverObject, err := i.sm.GetLatestStateObject(ix.Receiver())
	if err != nil {
		return err
	}

	if err := i.exec.ValidateLogicInvoke(receiverObject, ix); err != nil {
		return errors.Wrap(err, "failed to validate logic invoke")
	}

	// make sure logic is registered
	return i.sm.IsLogicRegistered(payload.Logic)
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

		time.Sleep(pruningCooldown)
	}
}

func (i *IxPool) Close() {
	i.logger.Info("Closing IxPool")
	i.ctxCancel()
}

func (i *IxPool) Start() {
	i.metrics.initMetrics()

	go i.handlePruning()
}

func (i *IxPool) post(ev interface{}) error {
	if i.mux != nil {
		return i.mux.Post(ev)
	}

	return nil
}

func (i *IxPool) postAddedInteractionEvent(ixns common.Interactions) error {
	return i.post(utils.AddedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postPromotedInteractionEvent(ixns common.Interactions) error {
	return i.post(utils.PromotedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postDroppedInteractionEvent(ixns common.Interactions) error {
	return i.post(utils.DroppedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postPrunedEnqueueInteractionEvent(ixns common.Interactions) error {
	return i.post(utils.PrunedEnqueuedInteractionEvent{Ixs: ixns})
}

func (i *IxPool) postPrunedPromotedInteractionEvent(ixns common.Interactions) error {
	return i.post(utils.PrunedPromotedInteractionEvent{Ixs: ixns})
}

// helper functions

func GetIxsSize(ixs common.Interactions) (uint64, error) {
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
