package ixpool

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/types"
)

const (
	WaitMode = iota
	CostMode
)

const (
	ixMaxSize = 128 * 1024 // 128Kb
)

type stateManager interface {
	GetNonce(addr types.Address, stateHash types.Hash) (uint64, error)
	IsAccountRegistered(addr types.Address) (bool, error)
	IsLogicRegistered(logicID types.LogicID) error
}

type IxConfig struct {
	Mode       int
	PriceLimit uint64
}

type IxPool struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	logger       hclog.Logger
	cfg          *common.IxPoolConfig
	sm           stateManager
	allIxs       *lookupMap
	close        chan struct{}
	sealing      bool
	mux          *utils.TypeMux
	accounts     *accountsMap
	metrics      *Metrics
	enqueueReqCh chan enqueueRequest
	promoteReqCh chan promoteRequest
}

func NewIxPool(
	ctx context.Context,
	logger hclog.Logger,
	mux *utils.TypeMux,
	sm stateManager,
	cfg *common.IxPoolConfig,
	metrics *Metrics,
) *IxPool {
	ctx, ctxCancel := context.WithCancel(ctx)
	i := &IxPool{
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		cfg:          cfg,
		mux:          mux,
		sm:           sm,
		allIxs:       NewLookupMap(),
		close:        make(chan struct{}),
		sealing:      false,
		accounts:     new(accountsMap),
		metrics:      metrics,
		logger:       logger.Named("Ix-Pool"),
		enqueueReqCh: make(chan enqueueRequest),
		promoteReqCh: make(chan promoteRequest),
	}

	return i
}

func (i *IxPool) checkIx(ix *types.Interaction) error {
	// validate incoming ix
	if err := i.validateIx(ix); err != nil {
		return err
	}

	// TODO: check for overflow

	// check if already known
	if _, ok := i.allIxs.get(ix.Hash()); ok {
		return ErrAlreadyKnown
	}

	// initialize account for this address once
	if !i.accounts.exists(ix.Sender()) {
		i.createAccountOnce(ix.Sender(), ix.Nonce())
	}

	return nil
}

func (i *IxPool) AddInteractions(ixs types.Interactions) []error {
	newIxs := make(types.Interactions, 0, len(ixs))
	errs := make([]error, 0, len(ixs))

	for _, ix := range ixs {
		if err := i.checkIx(ix); err != nil {
			i.logger.Error("Error adding the interaction", "error", err)
			errs = append(errs, err)
		} else {
			newIxs = append(newIxs, ix)
		}
	}

	if len(newIxs) == 0 {
		return errs
	}

	i.enqueueReqCh <- enqueueRequest{ixs: newIxs}

	if err := i.mux.Post(utils.NewIxsEvent{Ixs: newIxs}); err != nil {
		i.logger.Error("Error posting event", "error", err)
	}

	return errs
}

func (i *IxPool) handleEnqueueRequest(req enqueueRequest) {
	dirtyAccounts := make(map[types.Address]interface{}, 0)

	for _, v := range req.ixs {
		senderAcc := i.accounts.get(v.Sender())

		if err := senderAcc.enqueue(v); err != nil {
			continue
		}

		i.allIxs.add(v)

		if ixSize, err := v.Size(); err == nil {
			i.metrics.captureIxPoolSize(float64(ixSize))
		}

		if v.Nonce() > senderAcc.getNonce() {
			continue
		}

		dirtyAccounts[v.Sender()] = nil
	}

	if len(dirtyAccounts) == 0 {
		return
	}

	i.promoteReqCh <- promoteRequest{account: dirtyAccounts}
}

func (i *IxPool) handlePromoteRequest(req promoteRequest) {
	for addr := range req.account {
		account := i.accounts.get(addr)

		// promote enqueued ixs
		promoted, _ := account.promote()
		i.metrics.capturePendingIxs(float64(promoted))
	}
}

// createAccountOnce creates an account and
// ensures it is only initialized once.
func (i *IxPool) createAccountOnce(newAddr types.Address, nonce uint64) *account {
	// fetch nonce from the latest state
	stateNonce, err := i.sm.GetNonce(newAddr, types.NilHash)
	if err != nil {
		stateNonce = nonce
	}
	// initialize the account
	account := i.accounts.initOnce(newAddr, stateNonce)

	return account
}

func (i *IxPool) ResetWithHeaders(ts *types.Tesseract) {
	if ts != nil && len(ts.Interactions()) > 0 {
		i.logger.Info("Reset interactions", "size", len(ts.Interactions()))
		i.ResetWithInteractions(ts.Interactions())
	}
}

func (i *IxPool) ResetWithInteractions(ixs types.Interactions) {
	updatedNonces := make(map[types.Address]uint64)
	// cleanup the lookup queue
	i.allIxs.remove(ixs)

	for _, ix := range ixs {
		from := ix.Sender()
		// skip already processed accounts
		if _, processed := updatedNonces[from]; processed {
			continue
		}

		// fetch the latest nonce from the state
		latestNonce, err := i.sm.GetNonce(from, types.NilHash)
		if err != nil {
			latestNonce = ix.Nonce() + 1
		}

		i.logger.Debug("Latest nonce in pool", latestNonce)
		// update the result map
		updatedNonces[from] = latestNonce
	}

	if len(updatedNonces) == 0 {
		return
	}

	i.resetAccounts(updatedNonces)
}

func (i *IxPool) resetAccounts(nonces map[types.Address]uint64) {
	for addr, nonce := range nonces {
		if !i.accounts.exists(addr) {
			continue
		}

		i.resetAccount(addr, nonce)
	}
}

func (i *IxPool) resetAccount(addr types.Address, nonce uint64) {
	account := i.accounts.get(addr)

	// lock promoted
	account.promoted.lock(true)
	defer account.promoted.unlock()

	// prune promoted
	pruned := account.promoted.prune(nonce)

	if len(pruned) > 0 {
		account.waitLock.Lock()
		i.metrics.captureAccountWaitTime(account.requestTime, account.waitTime)
		account.requestTime = time.Now()
		account.waitLock.Unlock()
		// update the account waitTime and counter
		account.resetWaitTimeAndCounter()
	}

	// update pool state
	i.allIxs.remove(pruned)

	// lock enqueued
	account.enqueued.lock(true)

	defer func() {
		// update accountsMap
		i.accounts.remove(addr)
		account.enqueued.unlock()
	}()

	i.metrics.capturePendingIxs(float64(-1 * len(pruned)))

	if ixSize, err := GetIxsSize(pruned); err == nil {
		i.metrics.captureIxPoolSize(-1 * float64(ixSize))
	}

	if nonce <= account.getNonce() {
		// only the promoted queue needed pruning
		return
	}

	// prune enqueued
	pruned = account.enqueued.prune(nonce)

	log.Println("Prunned ixs", pruned)

	// update pool state
	i.allIxs.remove(pruned)

	if ixSize, err := GetIxsSize(pruned); err == nil {
		i.metrics.captureIxPoolSize(-1 * float64(ixSize))
	}

	// update next nonce
	account.setNonce(nonce)

	if first := account.enqueued.peek(); first != nil &&
		first.Nonce() == nonce {
		// first enqueued ix is expected -> signal promotion
		req := promoteRequest{account: make(map[types.Address]interface{})}
		req.account[addr] = nil
		i.promoteReqCh <- req
	}
}

func (i *IxPool) Executables() InteractionQueue {
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
func (i *IxPool) Pop(ix *types.Interaction) {
	// fetch the associated account
	account := i.accounts.get(ix.Sender())

	account.promoted.lock(true)
	defer account.promoted.unlock()

	// pop the top most promoted ix
	/*
		TODO://Need to check whether to move ixs

			// update executables
			if ix := account.promoted.peek(); ix != nil {
				i.executableQueue = append(i.executableQueue, ix)
			}
	*/
	account.promoted.pop()
}

func (i *IxPool) Drop(ix *types.Interaction) {
	// fetch the associated account
	account := i.accounts.get(ix.Sender())

	if account != nil {
		// lock enqueued and promoted
		account.enqueued.lock(true)
		account.promoted.lock(true)

		defer func() {
			account.enqueued.unlock()
			account.promoted.unlock()
		}()

		noOfDroppedIxs := 0

		// remove the dropped ixs from the allIxs lookup map
		cleanAllIxs := func(ixs types.Interactions) {
			i.allIxs.remove(ixs)

			noOfDroppedIxs += len(ixs)
		}

		// drop promoted
		dropped := account.promoted.clear()
		cleanAllIxs(dropped)

		i.metrics.capturePendingIxs(float64(-1 * len(dropped)))

		// drop enqueued
		dropped = account.enqueued.clear()
		cleanAllIxs(dropped)

		// drop the account
		i.accounts.remove(ix.Sender())

		i.logger.Debug("Dropped ixs", "count", noOfDroppedIxs, "address", ix.Sender())
	}
}

// IncrementWaitTime updates the waitTime for the given account
func (i *IxPool) IncrementWaitTime(addr types.Address, baseTime time.Duration) error {
	acc := i.accounts.get(addr)
	if acc == nil {
		return types.ErrAccountNotFound
	}

	if acc.getDelayCounter()+1 <= MaxWaitCounter {
		acc.incrementCounter(baseTime)
	} else {
		acc.resetWaitTimeAndCounter()
	}

	return nil
}

func (i *IxPool) validateIx(ix *types.Interaction) error {
	// Check the interaction size to overcome DOS Attacks
	ixSize, err := ix.Size()
	if err != nil {
		return err
	}

	if ixSize > ixMaxSize {
		return ErrOversizedData
	}

	if ix.Sender().IsNil() {
		return types.ErrInvalidAddress
	}

	// TODO: Check the signature

	// Reject underpriced interactions
	if ix.IsUnderpriced(i.cfg.PriceLimit) {
		return types.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := i.sm.GetNonce(ix.Sender(), types.NilHash); n > ix.Nonce() {
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

	switch ix.Type() {
	case types.IxLogicDeploy:
		return i.validateLogicDeployPayload(ix)
	case types.IxLogicInvoke:
		return i.validateLogicInvokePayload(ix)
	}

	return nil
}

func (i *IxPool) validateLogicDeployPayload(ix *types.Interaction) error {
	if accountRegistered, err := i.sm.IsAccountRegistered(ix.Receiver()); err != nil || accountRegistered {
		return errors.Wrap(err, fmt.Sprintf("account registered %s", ix.Receiver()))
	}

	return nil
}

func (i *IxPool) validateLogicInvokePayload(ix *types.Interaction) error {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return err
	}

	return i.sm.IsLogicRegistered(payload.Logic)
}

func (i *IxPool) handleRequests() {
	for {
		select {
		case <-i.ctx.Done():
			return
		case req := <-i.enqueueReqCh:
			go i.handleEnqueueRequest(req)
		case req := <-i.promoteReqCh:
			go i.handlePromoteRequest(req)
		}
	}
}

func (i *IxPool) Close() {
	defer i.ctxCancel()
	log.Println("Closing IxPool")
}

func (i *IxPool) Start() {
	i.metrics.initMetrics()

	go i.handleRequests()
}

// helper functions

func GetIxsSize(ixs types.Interactions) (uint64, error) {
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
