package ixpool

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
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
	GetBalance(addrs types.Address, assetID types.AssetID, stateHash types.Hash) (*big.Int, error)
	GetAssetInfo(assetID types.AssetID, hash types.Hash) (*types.AssetDescriptor, error)
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
	verifier     func(data, signature, pubBytes []byte) (bool, error)
}

func NewIxPool(
	ctx context.Context,
	logger hclog.Logger,
	mux *utils.TypeMux,
	sm stateManager,
	cfg *common.IxPoolConfig,
	metrics *Metrics,
	verifier func(data, signature, pubBytes []byte) (bool, error),
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
		verifier:     verifier,
	}

	return i
}

// GetPendingIx returns the interaction in ixpool for the given interaction hash
func (i *IxPool) GetPendingIx(ixHash types.Hash) (*types.Interaction, bool) {
	return i.allIxs.get(ixHash)
}

func (i *IxPool) checkIx(ix *types.Interaction) error {
	// validate incoming ix
	if err := i.validateIx(ix); err != nil {
		return err
	}

	// TODO: check for overflow

	// check if already known
	if _, ok := i.allIxs.get(ix.Hash()); ok {
		return types.ErrAlreadyKnown
	}

	i.allIxs.add(ix)

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

	for _, ixn := range req.ixs {
		senderAcc := i.accounts.get(ixn.Sender())
		if senderAcc == nil {
			i.logger.Error("Queue for account is nil", ixn.Sender(), ixn)
		}

		if err := senderAcc.enqueue(ixn); err != nil {
			i.allIxs.remove([]*types.Interaction{ixn})

			continue
		}

		if ixSize, err := ixn.Size(); err == nil {
			i.metrics.captureIxPoolSize(float64(ixSize))
		}

		if ixn.Nonce() > senderAcc.getNonce() {
			continue
		}

		dirtyAccounts[ixn.Sender()] = nil
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
		// i.accounts.remove(addr) FIXME: Issue(https://github.com/sarvalabs/moichain/issues/256)
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

	i.logger.Info("Pruned Interactions", pruned)

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

		nonce := ix.Nonce()
		account.setNonce(nonce)

		// drop promoted
		dropped := account.promoted.clear()
		cleanAllIxs(dropped)

		i.metrics.capturePendingIxs(float64(-1 * len(dropped)))

		// drop enqueued
		dropped = account.enqueued.clear()
		cleanAllIxs(dropped)

		// drop the account
		// i.accounts.remove(ix.Sender()) FIXME: Issue(https://github.com/sarvalabs/moichain/issues/256)

		i.logger.Debug("Dropped ixs", "count", noOfDroppedIxs, "next-nonce", nonce, "address", ix.Sender())
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

	moiBal, err := i.sm.GetBalance(ix.Sender(), types.KMOITokenAssetID, types.NilHash)
	if err != nil {
		i.logger.Error("error fetching balance", "sender", ix.Sender(), "error", err)

		return types.ErrInsufficientFunds
	}

	if moiBal.Cmp(new(big.Int).Add(ix.MOITokenValue(), ix.FuelLimit())) < 0 {
		return types.ErrInsufficientFunds
	}

	rawPayload, err := ix.PayloadForSignature()
	if err != nil {
		return err
	}

	isVerified, err := i.verifier(rawPayload, ix.Signature(), ix.Sender().Bytes())
	if !isVerified || err != nil {
		return types.ErrInvalidIXSignature
	}

	switch ix.Type() {
	case types.IxAssetCreate:
		return i.validateAssetCreate(ix)
	case types.IxValueTransfer:
		return i.validateValueTransfer(ix)
	case types.IxAssetMint:
		return i.validateAssetMint(ix)
	case types.IxAssetBurn:
		return i.validateAssetBurn(ix)
	case types.IxLogicDeploy:
		return i.validateLogicDeployPayload(ix)
	case types.IxLogicInvoke:
		return i.validateLogicInvokePayload(ix)
	default:
		return types.ErrInvalidInteractionType
	}
}

func (i *IxPool) validateAssetCreate(ix *types.Interaction) error {
	payload, err := ix.GetAssetPayload()
	if err != nil {
		return err
	}

	// asset standard should be mas1 or mas2
	if payload.Create.Standard != types.MAS1 && payload.Create.Standard != types.MAS0 {
		return types.ErrInvalidAssetStandard
	}

	// supply should be one if asset standard is mas1
	if payload.Create.Standard == types.MAS1 {
		if payload.Create.Supply == nil || payload.Create.Supply.Uint64() != 1 {
			return types.ErrInvalidAssetSupply
		}
	}

	return nil
}

func (i *IxPool) validateValueTransfer(ix *types.Interaction) error {
	if len(ix.TransferValues()) == 0 {
		return errors.New("empty transfer values")
	}

	for assetID, v := range ix.TransferValues() {
		if v.Sign() < 0 {
			return types.ErrInvalidValue
		}

		currentBalance, err := i.sm.GetBalance(ix.Sender(), assetID, types.NilHash)
		if err != nil {
			return err
		}

		if currentBalance.Cmp(v) < 0 {
			return types.ErrInsufficientFunds
		}
	}

	return nil
}

func (i *IxPool) validateAssetMint(ix *types.Interaction) error {
	assetPayload, err := ix.GetAssetPayload()
	if err != nil {
		return err
	}

	assetID, err := assetPayload.Mint.Asset.Identifier()
	if err != nil {
		return err
	}

	// can not mint asset standard mas1
	if assetID.Standard() == types.MAS1 {
		return types.ErrMintNonFungibleToken
	}

	assetInfo, err := i.sm.GetAssetInfo(assetPayload.Mint.Asset, types.NilHash)
	if err != nil {
		return types.ErrAssetNotFound
	}

	// only operator can mint asset
	if assetInfo.Operator != ix.Sender() {
		return errors.New("Operator address mismatch")
	}

	return nil
}

func (i *IxPool) validateAssetBurn(ix *types.Interaction) error {
	assetPayload, err := ix.GetAssetPayload()
	if err != nil {
		return err
	}

	// make sure asset exists
	assetInfo, err := i.sm.GetAssetInfo(assetPayload.Mint.Asset, types.NilHash)
	if err != nil {
		return types.ErrAssetNotFound
	}

	currentBal, err := i.sm.GetBalance(ix.Sender(), assetPayload.Mint.Asset, types.NilHash)
	if err != nil {
		return err
	}

	// cannot burn amount greater than current balance
	if currentBal.Cmp(assetPayload.Mint.Amount) < 0 {
		return types.ErrInsufficientFunds
	}

	// only operator can burn asset
	if assetInfo.Operator != ix.Sender() {
		return errors.New("Operator address mismatch")
	}

	return nil
}

func (i *IxPool) validateLogicDeployPayload(ix *types.Interaction) error {
	// make sure logic isn't created previously
	accountRegistered, err := i.sm.IsAccountRegistered(ix.Receiver())
	if err != nil {
		return err
	}

	if accountRegistered {
		return errors.New(fmt.Sprintf("account registered %s", ix.Receiver()))
	}

	payload, err := ix.GetLogicPayload()
	if err != nil {
		return err
	}

	// manifest cannot be empty
	if len(payload.Manifest) == 0 {
		return types.ErrEmptyManifest
	}

	return nil
}

func (i *IxPool) validateLogicInvokePayload(ix *types.Interaction) error {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return err
	}

	// callsite cannot be empty
	if len(payload.Callsite) == 0 {
		return types.ErrEmptyCallSite
	}

	// make sure logic is registered
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
