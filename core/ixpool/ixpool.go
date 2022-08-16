package ixpool

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"log"

	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
)

const (
	WaitMode = iota
	CostMode
)

const (
	txMaxSize = 128 * 1024 //128Kb
)

type stateManager interface {
	GetLatestNonce(addr ktypes.Address) (uint64, error)
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
	mux          *kutils.TypeMux
	accounts     *accountsMap
	enqueueReqCh chan enqueueRequest
	promoteReqCh chan promoteRequest
}

func NewIxPool(
	ctx context.Context,
	logger hclog.Logger,
	mux *kutils.TypeMux,
	sm stateManager,
	cfg *common.IxPoolConfig,
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
		logger:       logger.Named("Ix-Pool"),
		enqueueReqCh: make(chan enqueueRequest),
		promoteReqCh: make(chan promoteRequest),
	}

	return i
}

func (i *IxPool) checkIx(ix *ktypes.Interaction) error {
	// validate incoming ix
	if err := i.validateIx(ix); err != nil {
		return err
	}

	//TODO: check for overflow

	// check if already known
	if _, ok := i.allIxs.get(ix.Hash); ok {
		return ErrAlreadyKnown
	}

	// initialize account for this address once
	if !i.accounts.exists(ix.FromAddress()) {
		i.createAccountOnce(ix.FromAddress(), ix.Data.Input.Nonce)
	}

	return nil
}
func (i *IxPool) GetNonce(addr ktypes.Address) (uint64, error) {
	if acc := i.accounts.get(addr); acc != nil {
		return acc.getNonce(), nil
	}

	return i.sm.GetLatestNonce(addr)
}

func (i *IxPool) AddInteractions(ixs ktypes.Interactions) []error {
	newIxs := make(ktypes.Interactions, 0, len(ixs))
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

	if err := i.mux.Post(kutils.NewIxsEvent{Ixs: ixs}); err != nil {
		i.logger.Error("Error posting event", "error", err)
	}

	return errs
}

func (i *IxPool) handleEnqueueRequest(req enqueueRequest) {
	dirtyAccounts := make(map[ktypes.Address]interface{}, 0)

	for _, v := range req.ix {
		senderAcc := i.accounts.get(v.FromAddress())

		if err := senderAcc.enqueue(v); err != nil {
			continue
		}

		i.allIxs.add(v)

		if v.Nonce() > senderAcc.getNonce() {
			return
		}

		dirtyAccounts[v.FromAddress()] = nil
	}
	i.promoteReqCh <- promoteRequest{account: dirtyAccounts}
}

func (i *IxPool) handlePromoteRequest(req promoteRequest) {
	for addr := range req.account {
		account := i.accounts.get(addr)

		// promote enqueued txs
		promoted, _ := account.promote()
		log.Println("promote request", "promoted", promoted, "addr", addr)
	}
}

// createAccountOnce creates an account and
// ensures it is only initialized once.
func (i *IxPool) createAccountOnce(newAddr ktypes.Address, nonce uint64) *account {
	// fetch nonce from the latest state
	stateNonce, err := i.sm.GetLatestNonce(newAddr)
	if err != nil {
		stateNonce = nonce
	}
	// initialize the account
	account := i.accounts.initOnce(newAddr, stateNonce)

	return account
}

func (i *IxPool) ResetWithHeaders(ts *ktypes.Tesseract) {
	i.ResetWithInteractions(ts.Interactions())
}
func (i *IxPool) ResetWithInteractions(ixs ktypes.Interactions) {
	updatedNonces := make(map[ktypes.Address]uint64)
	// cleanup the lookup queue
	i.allIxs.remove(ixs)

	for _, ix := range ixs {
		from := ix.FromAddress()
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
func (i *IxPool) resetAccounts(nonces map[ktypes.Address]uint64) {
	for addr, nonce := range nonces {
		if !i.accounts.exists(addr) {
			continue
		}

		i.resetAccount(addr, nonce)
	}
}
func (i *IxPool) resetAccount(addr ktypes.Address, nonce uint64) {
	account := i.accounts.get(addr)

	// lock promoted
	account.promoted.lock(true)
	defer account.promoted.unlock()

	// update the account waitTime and counter
	account.resetWaitTimeAndCounter()

	// prune promoted
	pruned := account.promoted.prune(nonce)

	// update pool state
	i.allIxs.remove(pruned)

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
	//p.gauge.decrease(slotsRequired(pruned))

	// update next nonce
	account.setNonce(nonce)

	if first := account.enqueued.peek(); first != nil &&
		first.Nonce() == nonce {
		// first enqueued tx is expected -> signal promotion
		req := promoteRequest{account: make(map[ktypes.Address]interface{})}
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
func (i *IxPool) Pop(ix *ktypes.Interaction) {
	// fetch the associated account
	account := i.accounts.get(ix.FromAddress())

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

//IncrementWaitTime updates the waitTime for the given account
func (i *IxPool) IncrementWaitTime(addr ktypes.Address) error {
	acc := i.accounts.get(addr)
	if acc == nil {
		return ktypes.ErrAccountNotFound
	}

	if acc.getDelayCounter()+1 <= MaxWaitCounter {
		acc.incrementCounter()
	} else {
		acc.resetWaitTimeAndCounter()
	}

	return nil
}

func (i *IxPool) validateIx(ix *ktypes.Interaction) error {
	// Check the interaction size to overcome DOS Attacks
	if uint64(ix.GetSize()) > txMaxSize {
		return ErrOversizedData
	}

	if ix.FromAddress() == ktypes.NilAddress {
		return ktypes.ErrInvalidAddress
	}

	// TODO: Check the signature

	// Reject underpriced transactions
	if ix.IsUnderpriced(i.cfg.PriceLimit) {
		return ktypes.ErrUnderpriced
	}

	// Check nonce ordering
	if n, _ := i.sm.GetLatestNonce(ix.FromAddress()); n > ix.Nonce() {
		return ErrNonceTooLow
	}
	/*
		accountBalance, balanceErr := i.chain.GetBalance(stateRoot, tx.From)
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
	go i.handleRequests()
}
