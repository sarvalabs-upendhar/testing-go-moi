package ixpool

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/moichain/mudra"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

type MockStateManager struct {
	nonce               map[types.Address]uint64
	balance             map[types.Address]map[types.AssetID]*big.Int
	assetInfo           map[types.AssetID]*types.AssetDescriptor
	accountRegistration map[types.Address]bool
	logicRegistration   map[types.LogicID]bool
}

// NewMockStateManager returns a new instance of MockStateManager
func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	return &MockStateManager{
		nonce:               make(map[types.Address]uint64, 0),
		balance:             map[types.Address]map[types.AssetID]*big.Int{},
		assetInfo:           map[types.AssetID]*types.AssetDescriptor{},
		accountRegistration: make(map[types.Address]bool),
		logicRegistration:   make(map[types.LogicID]bool),
	}
}

func (ms *MockStateManager) setTestMOIBalance(addrs ...types.Address) {
	for _, addr := range addrs {
		ms.setBalance(addr, types.KMOITokenAssetID, big.NewInt(1000))
	}
}

func (ms *MockStateManager) GetAssetInfo(assetID types.AssetID, stateHash types.Hash) (*types.AssetDescriptor, error) {
	info, ok := ms.assetInfo[assetID]
	if !ok {
		return nil, types.ErrAssetNotFound
	}

	return info, nil
}

func (ms *MockStateManager) setAssetInfo(assetID types.AssetID, info *types.AssetDescriptor) {
	ms.assetInfo[assetID] = info
}

func (ms *MockStateManager) GetBalance(
	addrs types.Address,
	assetID types.AssetID,
	stateHash types.Hash,
) (*big.Int, error) {
	assets, ok := ms.balance[addrs]
	if !ok {
		return nil, types.ErrFetchingBalance
	}

	va, ok := assets[assetID]
	if !ok {
		return nil, types.ErrFetchingBalance
	}

	return va, nil
}

func (ms *MockStateManager) setBalance(
	addrs types.Address,
	assetID types.AssetID,
	amount *big.Int,
) {
	if _, ok := ms.balance[addrs]; !ok {
		ms.balance[addrs] = make(map[types.AssetID]*big.Int)
	}

	ms.balance[addrs][assetID] = amount
}

// CreateTestIxpool returns a new instance of IxPool
func CreateTestIxpool(
	t *testing.T,
	cfgCallback func(cfg *common.IxPoolConfig),
	skipSignatureVerification bool,
	sm *MockStateManager,
) *IxPool {
	t.Helper()

	verifier := mudra.Verify
	cfg := new(common.IxPoolConfig)

	if sm == nil {
		sm = NewMockStateManager(t)
	}

	if cfgCallback != nil {
		cfgCallback(cfg)
	}

	if skipSignatureVerification {
		verifier = func(data, signature, pubBytes []byte) (bool, error) {
			return true, nil
		}
	}

	return NewIxPool(context.Background(), hclog.NewNullLogger(), new(utils.TypeMux), sm, cfg, NilMetrics(), verifier)
}

// GetLatestNonce returns the latest nonce from the mock account
func (ms *MockStateManager) GetNonce(addr types.Address, stateHash types.Hash) (uint64, error) {
	if account, ok := ms.nonce[addr]; ok {
		return account, nil
	}

	return 0, errors.New("account doesn't exists")
}

func (ms *MockStateManager) IsAccountRegistered(addr types.Address) (bool, error) {
	_, ok := ms.accountRegistration[addr]

	return ok, nil
}

func (ms *MockStateManager) registerAccount(addr types.Address) {
	ms.accountRegistration[addr] = true
}

func (ms *MockStateManager) IsLogicRegistered(logicID types.LogicID) error {
	if _, ok := ms.logicRegistration[logicID]; !ok {
		return errors.New("logic id is not registered")
	}

	return nil
}

func (ms *MockStateManager) registerLogicID(logicID types.LogicID) {
	ms.logicRegistration[logicID] = true
}

// setLatestNonce updates the mock account with the latest nonce
func (ms *MockStateManager) setLatestNonce(addr types.Address, nonce uint64) {
	ms.nonce[addr] = nonce
}

func getIXParams(
	address types.Address,
	ixType types.IxType,
	fuelPrice *big.Int,
	transferValues map[types.AssetID]*big.Int,
	sign []byte,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *types.IxData) {
			ix.Input.Type = ixType
			ix.Input.Sender = address
			ix.Input.FuelPrice = fuelPrice
			ix.Input.FuelLimit = big.NewInt(1)
			ix.Input.TransferValues = transferValues
		},
		Sign: sign,
	}
}

// newTestInteraction returns a new instance of types.Interaction with the input
func newTestInteraction(
	t *testing.T,
	ixType types.IxType,
	nonce int,
	address types.Address,
	cb func(ixData *types.IxData),
) *types.Interaction {
	t.Helper()

	if address.IsNil() {
		address = tests.RandomAddress(t)
	}

	ixData := &types.IxData{
		Input: types.IxInput{
			Type:      ixType,
			Sender:    address,
			Nonce:     uint64(nonce),
			FuelPrice: big.NewInt(1),
			FuelLimit: big.NewInt(1),
			TransferValues: map[types.AssetID]*big.Int{
				types.KMOITokenAssetID: big.NewInt(1),
			},
		},
	}

	if cb != nil {
		cb(ixData)
	}

	ix, err := types.NewInteraction(*ixData, nil)
	require.NoError(t, err)

	return ix
}

// createTestIxs creates and returns multiple instances of types.Interactions based on the given range
func createTestIxs(t *testing.T, ixType types.IxType, start int, end int, address types.Address) types.Interactions {
	t.Helper()

	ixs := make(types.Interactions, 0)

	for nonce := start; nonce < end; nonce++ {
		ixs = append(ixs, newTestInteraction(t, ixType, nonce, address, nil))
	}

	return ixs
}

// subscribeToNewIxsEvent creates a subscription for NewIxsEvent and returns it
func subscribeToNewIxsEvent(t *testing.T, eventMux *utils.TypeMux) *utils.Subscription {
	t.Helper()

	return eventMux.Subscribe(utils.NewIxsEvent{})
}

// getTesseractWithIxs returns a new instance of types.Tesseract with interactions
func getTesseractWithIxs(t *testing.T, address types.Address, nonce int) *types.Tesseract {
	t.Helper()

	ixns := types.Interactions{
		newTestInteraction(t, types.IxValueTransfer, nonce, address, nil),
	}

	ts := tests.GetTesseract(t, 0, ixns)

	return ts
}

// newIxWithoutAddress returns a new instance of types.Interaction without sender address
func newIxWithoutAddress(t *testing.T, nonce int) *types.Interaction {
	t.Helper()

	return newTestInteraction(t, types.IxValueTransfer, nonce, types.NilAddress, func(ixData *types.IxData) {
		ixData.Input.Sender = types.NilAddress
	})
}

// newIxWithFuelPrice returns a new instance of types.Interaction with the given fuelPrice
func newIxWithFuelPrice(t *testing.T, nonce int, address types.Address, fuelPrice int64) *types.Interaction {
	t.Helper()

	return newTestInteraction(t, types.IxValueTransfer, nonce, address, func(ixData *types.IxData) {
		ixData.Input.FuelPrice = big.NewInt(fuelPrice)
	})
}

// newIxWithWaitCounter returns a new instance of WaitInteractions with the given waitCounter and new interaction
func newIxWithWaitCounter(t *testing.T, nonce int, address types.Address, waitCounter int32) *WaitInteractions {
	t.Helper()

	ix := newTestInteraction(t, types.IxValueTransfer, nonce, address, nil)

	return &WaitInteractions{waitCounter, ix}
}

// newIxWithPayload returns a new instance of types.Interaction with the given payload
func newIxWithPayload(
	t *testing.T,
	ixType types.IxType,
	nonce int,
	address types.Address,
	payload []byte,
) *types.Interaction {
	t.Helper()

	return newTestInteraction(t, ixType, nonce, address, func(ixData *types.IxData) {
		ixData.Input.Payload = payload
	})
}

// waitForNewIxs listens for enqueue request and NewIxsEvent.
// returns the new interactions from enqueue request channel and NewIxsEvent
func waitForNewIxs(t *testing.T, ixPool *IxPool) (enqueuedIxs types.Interactions, newIxsEvent utils.NewIxsEvent) {
	t.Helper()

	var ok bool

	subscription := subscribeToNewIxsEvent(t, ixPool.mux)
	// listen for enqueue request
	enqueuedIxs = (<-ixPool.enqueueReqCh).ixs

	// listens for new ixs event
	event := <-subscription.Chan()
	newIxsEvent, ok = event.Data.(utils.NewIxsEvent)
	require.True(t, ok)

	return enqueuedIxs, newIxsEvent
}

// addAndEnqueueIxs adds and enqueues ixs
// returns the promoted ixs
func addAndEnqueueIxs(t *testing.T, ixPool *IxPool, ixs types.Interactions, senderAddr types.Address) promoteRequest {
	t.Helper()

	go func() {
		errs := ixPool.AddInteractions(ixs)
		require.Len(t, errs, 0)
	}()

	go ixPool.handleEnqueueRequest(<-ixPool.enqueueReqCh)

	time.Sleep(100 * time.Millisecond)

	ixPool.accounts.get(senderAddr).enqueued.lock(false)
	defer ixPool.accounts.get(senderAddr).enqueued.unlock()

	// checks whether the ixs are enqueued
	require.Equal(t, uint64(len(ixs)), ixPool.accounts.get(senderAddr).enqueued.length())
	require.Equal(t, uint64(0), ixPool.accounts.get(senderAddr).promoted.length())

	return <-ixPool.promoteReqCh
}

// addAndPromoteIxs adds, enqueues and promotes ixs
func addAndPromoteIxs(t *testing.T, ixPool *IxPool, ixs types.Interactions, senderAddr types.Address) {
	t.Helper()

	go func() {
		errs := ixPool.AddInteractions(ixs)
		require.Len(t, errs, 0)
	}()

	go ixPool.handleEnqueueRequest(<-ixPool.enqueueReqCh)

	promoteRequest := <-ixPool.promoteReqCh

	// checks whether the ixs are enqueued
	require.Equal(t, uint64(len(ixs)), ixPool.accounts.get(senderAddr).enqueued.length())
	require.Equal(t, uint64(0), ixPool.accounts.get(senderAddr).promoted.length())

	ixPool.handlePromoteRequest(promoteRequest)

	// checks whether the ixs are promoted
	require.Equal(t, uint64(0), ixPool.accounts.get(senderAddr).enqueued.length())
	require.Equal(t, uint64(len(ixs)), ixPool.accounts.get(senderAddr).promoted.length())
}

// addAndProcessIxs enqueues and promotes the ixs based on nonce
func addAndProcessIxs(t *testing.T, sm *MockStateManager, ixPool *IxPool, ixs types.Interactions) {
	t.Helper()

	for _, v := range ixs {
		sm.setTestMOIBalance(v.Sender())
	}

	go func() {
		errs := ixPool.AddInteractions(ixs)
		require.Len(t, errs, 0)
	}()

	go ixPool.handleEnqueueRequest(<-ixPool.enqueueReqCh)

	ixPool.handlePromoteRequest(<-ixPool.promoteReqCh)
}

// addAndEnqueueIxsWithoutPromoting adds and enqueues the ixs but won't promote it
func addAndEnqueueIxsWithoutPromoting(t *testing.T, ixPool *IxPool, ixs types.Interactions, senderAddr types.Address) {
	t.Helper()

	go func() {
		errs := ixPool.AddInteractions(ixs)
		require.Len(t, errs, 0)
	}()

	go ixPool.handleEnqueueRequest(<-ixPool.enqueueReqCh)

	<-ixPool.promoteReqCh

	require.Equal(t, uint64(len(ixs)), ixPool.accounts.get(senderAddr).enqueued.length())
}

// getPromotedAccounts adds the interactions and returns the promoted accounts after enqueuing
func getPromotedAccounts(
	t *testing.T,
	ixPool *IxPool,
	ixs types.Interactions,
	expectedAccounts int,
) map[types.Address]interface{} {
	t.Helper()

	promotedAccounts := make(map[types.Address]interface{}, 0)

	go func() {
		errs := ixPool.AddInteractions(ixs)
		require.Len(t, errs, 0)
	}()

	go ixPool.handleEnqueueRequest(<-ixPool.enqueueReqCh)

	if expectedAccounts > 0 {
		promotedAccounts = (<-ixPool.promoteReqCh).account
	}

	return promotedAccounts
}

// mintIxs mints and returns the interactions from the interactionQueue
func mintIxs(t *testing.T, ixPool *IxPool) types.Interactions {
	t.Helper()

	mintedIxs := make(types.Interactions, 0)

	interactionQueue := ixPool.Executables()

	for interactionQueue.Len() > 0 {
		ix, ok := interactionQueue.Pop().(*types.Interaction)
		require.True(t, ok)
		ixPool.Pop(ix)

		mintedIxs = append(mintedIxs, ix)
	}

	return mintedIxs
}

// getSuccessfulIxs mints and returns the expected number of interactions from the interactionQueue
func getSuccessfulIxs(t *testing.T, ixPool *IxPool, noOfExpectedIxs int) types.Interactions {
	t.Helper()

	successfulIxs := make(types.Interactions, 0)

	for len(successfulIxs) < noOfExpectedIxs {
		successfulIxs = append(successfulIxs, mintIxs(t, ixPool)...)
	}

	return successfulIxs
}

// setDelayCounter updates the given account's delay counter
func setDelayCounter(t *testing.T, acc *account, delayCount int32) {
	t.Helper()

	acc.waitLock.Lock()
	defer acc.waitLock.Unlock()

	acc.delayCounter = delayCount
}

// getIxNonce returns a map of ix sender address to nonce
func getIxNonce(t *testing.T, ixs types.Interactions) map[types.Address]uint64 {
	t.Helper()

	ixNonce := make(map[types.Address]uint64, 0)

	for _, ix := range ixs {
		ixNonce[ix.Sender()] = ix.Nonce()
	}

	return ixNonce
}
