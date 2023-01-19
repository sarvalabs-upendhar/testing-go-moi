package ixpool

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

type MockStateManager struct {
	acc map[types.Address]uint64
}

// CreateTestIxpool returns a new instance of IxPool
func CreateTestIxpool(t *testing.T, cfgCallback func(cfg *common.IxPoolConfig)) (*IxPool, *MockStateManager) {
	t.Helper()

	cfg := new(common.IxPoolConfig)
	sm := NewMockStateManager(t)

	if cfgCallback != nil {
		cfgCallback(cfg)
	}

	return NewIxPool(context.Background(), hclog.NewNullLogger(), new(utils.TypeMux), sm, cfg, NilMetrics()), sm
}

// GetLatestNonce returns the latest nonce from the mock account
func (ms *MockStateManager) GetNonce(addr types.Address, stateHash types.Hash) (uint64, error) {
	if account, ok := ms.acc[addr]; ok {
		return account, nil
	}

	return 0, errors.New("account doesn't exists")
}

func (ms *MockStateManager) IsAccountRegistered(addr types.Address) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) IsLogicRegistered(logicID types.LogicID) error {
	// TODO implement me
	panic("implement me")
}

// setLatestNonce updates the mock account with the latest nonce
func (ms *MockStateManager) setLatestNonce(addr types.Address, nonce uint64) {
	ms.acc[addr] = nonce
}

// NewMockStateManager returns a new instance of MockStateManager
func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	return &MockStateManager{
		acc: make(map[types.Address]uint64, 0),
	}
}

// newTestInteraction returns a new instance of types.Interaction with the input
func newTestInteraction(
	t *testing.T,
	nonce int,
	address types.Address,
	cb func(ixData *types.IxData),
) *types.Interaction {
	t.Helper()

	if address.IsNil() {
		address = tests.RandomAddress(t)
	}

	ixMsg := new(types.InteractionMessage)
	ixMsg.Data = types.IxData{
		Input: types.IxInput{
			Sender:    address,
			Nonce:     uint64(nonce),
			FuelPrice: big.NewInt(1000),
		},
	}

	if cb != nil {
		cb(&ixMsg.Data)
	}

	return ixMsg.ToInteraction()
}

// createTestIxs creates and returns multiple instances of types.Interactions based on the given range
func createTestIxs(t *testing.T, start int, end int, address types.Address) types.Interactions {
	t.Helper()

	ixs := make(types.Interactions, 0)

	for nonce := start; nonce < end; nonce++ {
		ixs = append(ixs, newTestInteraction(t, nonce, address, nil))
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

	ts := tests.GetTesseract(t, 0)
	ts.Ixns = types.Interactions{
		newTestInteraction(t, nonce, address, nil),
	}

	return ts
}

// newIxWithoutAddress returns a new instance of types.Interaction without sender address
func newIxWithoutAddress(t *testing.T, nonce int) *types.Interaction {
	t.Helper()

	return newTestInteraction(t, nonce, types.NilAddress, func(ixData *types.IxData) {
		ixData.Input.Sender = types.NilAddress
	})
}

// newIxWithFuelPrice returns a new instance of types.Interaction with the given fuelPrice
func newIxWithFuelPrice(t *testing.T, nonce int, address types.Address, fuelPrice int64) *types.Interaction {
	t.Helper()

	return newTestInteraction(t, nonce, address, func(ixData *types.IxData) {
		ixData.Input.FuelPrice = big.NewInt(fuelPrice)
	})
}

// newIxWithWaitCounter returns a new instance of WaitInteractions with the given waitCounter and new interaction
func newIxWithWaitCounter(t *testing.T, nonce int, address types.Address, waitCounter int32) *WaitInteractions {
	t.Helper()

	ix := newTestInteraction(t, nonce, address, nil)

	return &WaitInteractions{waitCounter, ix}
}

// newIxWithPayload returns a new instance of types.Interaction with the given payload
func newIxWithPayload(t *testing.T, nonce int, address types.Address, payload []byte) *types.Interaction {
	t.Helper()

	return newTestInteraction(t, nonce, address, func(ixData *types.IxData) {
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
func addAndProcessIxs(t *testing.T, ixPool *IxPool, ixs types.Interactions) {
	t.Helper()

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

// getIxNonce returns a map of ix sender address to nonce
func getIxNonce(t *testing.T, ixs types.Interactions) map[types.Address]uint64 {
	t.Helper()

	ixNonce := make(map[types.Address]uint64, 0)

	for _, ix := range ixs {
		ixNonce[ix.Sender()] = ix.Nonce()
	}

	return ixNonce
}
