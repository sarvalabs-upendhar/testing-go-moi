package ixpool

import (
	"context"
	"log"
	"time"

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
	txMaxSize = 128 * 1024 // 128Kb
)

type stateManager interface {
	GetLatestNonce(addr types.Address) (uint64, error)
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

func (i *IxPool) GetNonce(addr types.Address) (uint64, error) {
	if acc := i.accounts.get(addr); acc != nil {
		return acc.getNonce(), nil
	}

	return i.sm.GetLatestNonce(addr)
}

func (i *IxPool) AddInteractions(ixs types.Interactions) []error {
	newIxs := make(types.Interactions, 0, len(ixs))
	errs := make([]error, len(ixs))

	for index, ix := range ixs {
		if err := i.checkIx(ix); err != nil {
			log.Println("Error adding the interaction", err)
			errs[index] = err
		} else {
			newIxs = append(newIxs, ix)
		}
	}

	if len(newIxs) == 0 {
		return errs
	}

	i.enqueueReqCh <- enqueueRequest{ix: newIxs}

	if err := i.mux.Post(utils.NewIxsEvent{Ixs: ixs}); err != nil {
		i.logger.Error("Error posting event", "error", err)
	}

	return errs
}

func (i *IxPool) handleEnqueueRequest(req enqueueRequest) {
	dirtyAccounts := make(map[types.Address]interface{}, 0)

	for _, v := range req.ix {
		senderAcc := i.accounts.get(v.Sender())
		if senderAcc == nil {
			log.Panicln("Account not found") // FIXME: Added this to identify runtime panic
		}

		if err := senderAcc.enqueue(v); err != nil {
			continue
		}

		i.allIxs.add(v)

		if ixSize, err := v.Size(); err == nil {
			i.metrics.captureIxPoolSize(float64(ixSize))
		}

		if v.Nonce() > senderAcc.getNonce() {
			return
		}

		dirtyAccounts[v.Sender()] = nil
	}
	i.promoteReqCh <- promoteRequest{account: dirtyAccounts}
}

func (i *IxPool) handlePromoteRequest(req promoteRequest) {
	for addr := range req.account {
		account := i.accounts.get(addr)

		// promote enqueued txs
		promoted, _ := account.promote()
		i.metrics.capturePendingTxs(float64(promoted))
		log.Println("promote request", "promoted", promoted, "addr", addr)
	}
}

// createAccountOnce creates an account and
// ensures it is only initialized once.
func (i *IxPool) createAccountOnce(newAddr types.Address, nonce uint64) *account {
	// fetch nonce from the latest state
	stateNonce, err := i.sm.GetLatestNonce(newAddr)
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
		latestNonce, err := i.sm.GetLatestNonce(from)
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

	i.metrics.capturePendingTxs(float64(-1 * len(pruned)))

	if ixSize, err := GetIxsSize(pruned); err == nil {
		i.metrics.captureIxPoolSize(-1 * float64(ixSize))
	}

	if nonce <= account.getNonce() {
		// only the promoted queue needed pruning
		return
	}

	// lock enqueued
	account.enqueued.lock(true)
	defer account.enqueued.unlock()

	// prune enqueued
	pruned = account.enqueued.prune(nonce)

	log.Println("Prunned tx", pruned)
	// update pool state
	i.allIxs.remove(pruned)
	// p.gauge.decrease(slotsRequired(pruned))

	if ixSize, err := GetIxsSize(pruned); err == nil {
		i.metrics.captureIxPoolSize(-1 * float64(ixSize))
	}

	// update next nonce
	account.setNonce(nonce)

	if first := account.enqueued.peek(); first != nil &&
		first.Nonce() == nonce {
		// first enqueued tx is expected -> signal promotion
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

// Pop removes the given transaction from the
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

	if ixSize > txMaxSize {
		return ErrOversizedData
	}

	if ix.Sender().IsNil() {
		return types.ErrInvalidAddress
	}

	// TODO: Check the signature

	// Reject underpriced transactions
	if ix.IsUnderpriced(i.cfg.PriceLimit) {
		return types.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := i.sm.GetLatestNonce(ix.Sender()); n > ix.Nonce() {
		return ErrNonceTooLow
	}
	/*
		accountBalance, balanceErr := i.lattice.GetBalance(stateRoot, tx.From)
		if balanceErr != nil {
			return ErrInvalidAccountState
		}

		// Check if the sender has enough funds to execute the transaction
		if accountBalance.Cmp(ix.Cost()) < 0 {
			return ErrInsufficientFunds
		}
	*/

	return nil
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
	var sumSize uint64

	for _, ix := range ixs {
		size, err := ix.Size()
		if err != nil {
			return 0, err
		}

		sumSize += size
	}

	return sumSize, nil
}
