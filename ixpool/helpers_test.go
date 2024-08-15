package ixpool

import (
	"context"
	crand "crypto/rand"
	"errors"
	"math/big"
	"sync"
	"testing"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/sarvalabs/go-legacy-kramaid"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/state"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
)

type expectedResult struct {
	nonce            uint64
	enqueued         uint64
	promoted         uint64
	promotedAccounts uint64
}

type MockStateManager struct {
	nonce                    map[identifiers.Address]uint64
	balance                  map[identifiers.Address]map[identifiers.AssetID]*big.Int
	assetInfo                map[identifiers.AssetID]*common.AssetDescriptor
	accountRegistration      map[identifiers.Address]bool
	logicRegistration        map[identifiers.LogicID]bool
	removedCacheStateObjects map[identifiers.Address]struct{}
	latestStateObjects       map[identifiers.Address]*state.Object
}

// NewMockStateManager returns a new instance of MockStateManager
func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	return &MockStateManager{
		nonce:                    make(map[identifiers.Address]uint64),
		balance:                  map[identifiers.Address]map[identifiers.AssetID]*big.Int{},
		assetInfo:                map[identifiers.AssetID]*common.AssetDescriptor{},
		accountRegistration:      make(map[identifiers.Address]bool),
		logicRegistration:        make(map[identifiers.LogicID]bool),
		removedCacheStateObjects: make(map[identifiers.Address]struct{}),
		latestStateObjects:       make(map[identifiers.Address]*state.Object),
	}
}

func (ms *MockStateManager) setLatestStateObject(addr identifiers.Address, obj *state.Object) {
	ms.latestStateObjects[addr] = obj
}

func (ms *MockStateManager) GetLatestStateObject(addr identifiers.Address) (*state.Object, error) {
	s, ok := ms.latestStateObjects[addr]
	if !ok {
		return nil, errors.New("state object not found")
	}

	return s, nil
}

func (ms *MockStateManager) RemoveCachedObject(addr identifiers.Address) {
	ms.removedCacheStateObjects[addr] = struct{}{}
}

func (ms *MockStateManager) setTestMOIBalance(t *testing.T, addrs ...identifiers.Address) {
	t.Helper()

	for _, addr := range addrs {
		ms.setBalance(t, addr, common.KMOITokenAssetID, big.NewInt(1000))
	}
}

func (ms *MockStateManager) GetAssetInfo(
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*common.AssetDescriptor, error) {
	info, ok := ms.assetInfo[assetID]
	if !ok {
		return nil, common.ErrAssetNotFound
	}

	return info, nil
}

func (ms *MockStateManager) setAssetInfo(t *testing.T, assetID identifiers.AssetID, info *common.AssetDescriptor) {
	t.Helper()

	ms.assetInfo[assetID] = info
}

func (ms *MockStateManager) GetBalance(
	addrs identifiers.Address,
	assetID identifiers.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	assets, ok := ms.balance[addrs]
	if !ok {
		return big.NewInt(0), common.ErrFetchingBalance
	}

	va, ok := assets[assetID]
	if !ok {
		return big.NewInt(0), common.ErrFetchingBalance
	}

	return va, nil
}

func (ms *MockStateManager) setBalance(
	t *testing.T,
	addrs identifiers.Address,
	assetID identifiers.AssetID,
	amount *big.Int,
) {
	t.Helper()

	if _, ok := ms.balance[addrs]; !ok {
		ms.balance[addrs] = make(map[identifiers.AssetID]*big.Int)
	}

	ms.balance[addrs][assetID] = amount
}

// CreateTestIxpool returns a new instance of IxPool
func CreateTestIxpool(
	t *testing.T,
	cfgCallback func(cfg *config.IxPoolConfig),
	skipSignatureVerification bool,
	sm *MockStateManager,
	exec *MockExecutionManager,
	network *mockNetwork,
) *IxPool {
	t.Helper()

	verifier := crypto.Verify
	cfg := new(config.IxPoolConfig)

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

	return NewIxPool(
		hclog.NewNullLogger(),
		new(utils.TypeMux),
		network,
		sm,
		exec,
		cfg,
		NilMetrics(),
		verifier,
	)
}

// GetLatestNonce returns the latest nonce from the mock account
func (ms *MockStateManager) GetNonce(addr identifiers.Address, stateHash common.Hash) (uint64, error) {
	if account, ok := ms.nonce[addr]; ok {
		return account, nil
	}

	return 0, errors.New("account doesn't exists")
}

func (ms *MockStateManager) IsAccountRegistered(addr identifiers.Address) (bool, error) {
	_, ok := ms.accountRegistration[addr]

	return ok, nil
}

func (ms *MockStateManager) IsLogicRegistered(logicID identifiers.LogicID) error {
	if _, ok := ms.logicRegistration[logicID]; !ok {
		return errors.New("logic id is not registered")
	}

	return nil
}

func (ms *MockStateManager) registerLogicID(t *testing.T, logicID identifiers.LogicID) {
	t.Helper()

	ms.logicRegistration[logicID] = true
}

// setLatestNonce updates the mock account with the latest nonce
func (ms *MockStateManager) setLatestNonce(t *testing.T, addr identifiers.Address, nonce uint64) {
	t.Helper()

	ms.nonce[addr] = nonce
}

type MockExecutionManager struct {
	validateLogicDeployHook func() error
	validateLogicInvokeHook func() error
}

func (ms *MockExecutionManager) ValidateLogicInvoke(ix *common.Interaction, calleracc, logicacc *state.Object) error {
	if ms.validateLogicInvokeHook != nil {
		return ms.validateLogicInvokeHook()
	}

	return nil
}

func (ms *MockExecutionManager) ValidateLogicEnlist(ix *common.Interaction, calleracc, logicacc *state.Object) error {
	panic("implement me")
}

func NewMockExecutionManager(t *testing.T) *MockExecutionManager {
	t.Helper()

	exec := new(MockExecutionManager)

	return exec
}

func (ms *MockExecutionManager) ValidateLogicDeploy(ix *common.Interaction) error {
	if ms.validateLogicDeployHook != nil {
		return ms.validateLogicDeployHook()
	}

	return nil
}

type mockNetwork struct {
	broadcasted   map[string][]byte
	subscriptions map[string]struct{}
	kramaID       kramaid.KramaID
}

func newMockNetwork(kramaID kramaid.KramaID) *mockNetwork {
	return &mockNetwork{
		broadcasted:   make(map[string][]byte),
		subscriptions: make(map[string]struct{}),
		kramaID:       kramaID,
	}
}

func (m *mockNetwork) Subscribe(ctx context.Context, topicName string,
	validator utils.WrappedVal, defaultValidator bool, handler func(msg *pubsub.Message) error,
) error {
	m.subscriptions[topicName] = struct{}{}

	return nil
}

func (m *mockNetwork) Broadcast(topicName string, data []byte) error {
	m.broadcasted[topicName] = data

	return nil
}

func (m *mockNetwork) GetKramaID() kramaid.KramaID {
	return m.kramaID
}

// benchmark abstractions
type cachePusher interface {
	push()
}

type cacheMaker interface {
	make(size int) cachePusher
}

type (
	digestCacheMaker struct{}
	saltedCacheMaker struct{}
)

func (m digestCacheMaker) make(size int) cachePusher {
	return &digestCachePusher{c: newDigestCache(size)}
}

func (m saltedCacheMaker) make(size int) cachePusher {
	scp := &saltedCachePusher{c: newSaltedCache(size)}
	scp.c.Start(context.Background(), 0)

	return scp
}

type digestCachePusher struct {
	c *digestCache
}
type saltedCachePusher struct {
	c *ixSaltedCache
}

func (p *digestCachePusher) push() {
	var d [common.HashLength]byte

	if _, err := crand.Read(d[:]); err != nil {
		panic(err)
	}

	h := common.Hash(blake2b.Sum256(d[:])) // digestCache does not hashes so calculate hash here
	p.c.CheckAndInsert(&h)
}

func (p *saltedCachePusher) push() {
	var d [common.HashLength]byte
	if _, err := crand.Read(d[:]); err != nil {
		panic(err)
	}

	p.c.CheckAndPut(d[:]) // saltedCache hashes inside
}

func getIXParams(
	address identifiers.Address,
	ixType common.IxType,
	fuelPrice *big.Int,
	transferValues map[identifiers.AssetID]*big.Int,
	sign []byte,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Input.Type = ixType
			ix.Input.Sender = address
			ix.Input.FuelPrice = fuelPrice
			ix.Input.FuelLimit = 1
			ix.Input.TransferValues = transferValues
		},
		Sign: sign,
	}
}

// newTestInteraction returns a new instance of types.Interaction with the input
func newTestInteraction(
	t *testing.T,
	ixType common.IxType,
	nonce int,
	address identifiers.Address,
	cb func(ixData *common.IxData),
) *common.Interaction {
	t.Helper()

	if address.IsNil() {
		address = tests.RandomAddress(t)
	}

	ixData := &common.IxData{
		Input: common.IxInput{
			Type:      ixType,
			Sender:    address,
			Nonce:     uint64(nonce),
			FuelPrice: big.NewInt(1),
			FuelLimit: 1,
			TransferValues: map[identifiers.AssetID]*big.Int{
				common.KMOITokenAssetID: big.NewInt(1),
			},
		},
	}

	if cb != nil {
		cb(ixData)
	}

	ix, err := common.NewInteraction(*ixData, nil)
	require.NoError(t, err)

	return ix
}

// createTestIxs creates and returns multiple instances of types.Interactions based on the given range
func createTestIxs(
	t *testing.T,
	ixType common.IxType,
	startNonce int,
	endNonce int,
	address identifiers.Address,
) common.Interactions {
	t.Helper()

	ixs := make(common.Interactions, 0)

	for nonce := startNonce; nonce < endNonce; nonce++ {
		ixs = append(ixs, newTestInteraction(t, ixType, nonce, address, nil))
	}

	return ixs
}

// getTesseractWithIxs returns a new instance of types.Tesseract with interactions
func getTesseractWithIxs(t *testing.T, address identifiers.Address, nonce int) *common.Tesseract {
	t.Helper()

	ixns := common.Interactions{
		newTestInteraction(t, common.IxValueTransfer, nonce, address, nil),
	}

	tsParams := &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Heights:   []uint64{0},
		Ixns:      ixns,
	}

	ts := tests.CreateTesseract(t, tsParams)

	return ts
}

// newIxWithoutAddress returns a new instance of types.Interaction without sender address
func newIxWithoutAddress(t *testing.T, nonce int) *common.Interaction {
	t.Helper()

	return newTestInteraction(t, common.IxValueTransfer, nonce, identifiers.NilAddress, func(ixData *common.IxData) {
		ixData.Input.Sender = identifiers.NilAddress
	})
}

// newIxWithFuelPrice returns a new instance of types.Interaction with the given fuelPrice
func newIxWithFuelPrice(t *testing.T, nonce int, address identifiers.Address, fuelPrice int64) *common.Interaction {
	t.Helper()

	return newTestInteraction(t, common.IxValueTransfer, nonce, address, func(ixData *common.IxData) {
		ixData.Input.FuelPrice = big.NewInt(fuelPrice)
	})
}

// newIxWithWaitCounter returns a new instance of WaitInteractions with the given waitCounter and new interaction
func newIxWithWaitCounter(t *testing.T, nonce int, address identifiers.Address, waitCounter int32) *WaitInteractions {
	t.Helper()

	ix := newTestInteraction(t, common.IxValueTransfer, nonce, address, nil)

	return &WaitInteractions{waitCounter, ix}
}

// newIxWithPayload returns a new instance of types.Interaction with the given payload
func newIxWithPayload(
	t *testing.T,
	ixType common.IxType,
	nonce int,
	address identifiers.Address,
	payload []byte,
) *common.Interaction {
	t.Helper()

	return newTestInteraction(t, ixType, nonce, address, func(ixData *common.IxData) {
		ixData.Input.Payload = payload
	})
}

// addAndProcessIxs enqueues and promotes the ixs based on nonce
func addAndProcessIxs(t *testing.T, sm *MockStateManager, ixPool *IxPool, ixs common.Interactions) {
	t.Helper()

	for _, v := range ixs {
		sm.setTestMOIBalance(t, v.Sender())
	}

	errs := ixPool.AddLocalInteractions(ixs)
	require.Len(t, errs, 0)
}

// mintIxs mints and returns the interactions from the interactionQueue
func mintIxs(t *testing.T, ixPool *IxPool) common.Interactions {
	t.Helper()

	mintedIxs := make(common.Interactions, 0)

	interactionQueue := ixPool.Executables()

	for interactionQueue.Len() > 0 {
		ix, ok := interactionQueue.Pop().(*common.Interaction)
		require.True(t, ok)
		ixPool.Pop(ix)

		mintedIxs = append(mintedIxs, ix)
	}

	return mintedIxs
}

// getSuccessfulIxs mints and returns the expected number of interactions from the interactionQueue
func getSuccessfulIxs(t *testing.T, ixPool *IxPool, noOfExpectedIxs int) common.Interactions {
	t.Helper()

	successfulIxs := make(common.Interactions, 0)

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
func getIxNonce(t *testing.T, ixs common.Interactions) map[identifiers.Address]uint64 {
	t.Helper()

	ixNonce := make(map[identifiers.Address]uint64)

	for _, ix := range ixs {
		ixNonce[ix.Sender()] = ix.Nonce()
	}

	return ixNonce
}

func calcCacheSize(numIter int) int {
	size := numIter / 3 // in order to exercise map swaps
	if size == 0 {
		size++
	}

	return size
}

func benchmarkDigestCache(b *testing.B, m cacheMaker, numThreads int) {
	b.Helper()

	p := m.make(calcCacheSize(b.N))
	numHashes := b.N / numThreads // num hashes per goroutine
	// b.Logf("inserting %d (%d) values in %d threads into cache of size %d", b.N, numHashes,
	// numThreads, calcCacheSize(b.N))
	var wg sync.WaitGroup

	wg.Add(numThreads)

	for i := 0; i < numThreads; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numHashes; j++ {
				p.push()
			}
		}()
	}
	wg.Wait()
}
