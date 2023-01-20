package api

import (
	"encoding/json"
	"errors"
	"math/big"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/utils"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/guna"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
)

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type MockChainManager struct {
	receipts           map[types.Hash]*types.Receipt
	assets             map[types.Hash]*gtypes.AssetObject
	tesseractsByHash   map[types.Hash]*types.Tesseract
	tesseractsByHeight map[string]*types.Tesseract
	latestTesseracts   map[types.Address]*types.Tesseract
}

type MockStateManager struct {
	storage        map[types.Hash][]byte
	balances       map[types.Address]*gtypes.BalanceObject
	accounts       map[types.Address]*types.Account
	context        map[types.Address]*Context
	logicManifests map[string][]byte
	logicStorage   map[string]map[string]string // first key denotes logic id, second key denotes storage key
}

func (s *MockStateManager) GetLogicManifest(logicID types.LogicID, stateHash types.Hash) ([]byte, error) {
	logicManifest, ok := s.logicManifests[logicID.Hex()]
	if !ok {
		return logicManifest, errors.New("logic manifest not found")
	}

	return logicManifest, nil
}

func (s *MockStateManager) SetStorageEntry(logicID types.LogicID, storage map[string]string) {
	s.logicStorage[logicID.Hex()] = storage
}

func (s *MockStateManager) GetStorageEntry(logicID types.LogicID, slot []byte, stateHash types.Hash) ([]byte, error) {
	storage, ok := s.logicStorage[logicID.Hex()]
	if !ok {
		return nil, types.ErrLogicStorageTreeNotFound
	}

	value, ok := storage[string(slot)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return []byte(value), nil
}

func (s *MockStateManager) GetLatestStateObject(addr types.Address) (*guna.StateObject, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetAccountState(addr types.Address, stateHash types.Hash) (*types.Account, error) {
	account, ok := s.accounts[addr]
	if !ok {
		return nil, types.ErrAccountNotFound
	}

	return account, nil
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.receipts = make(map[types.Hash]*types.Receipt, 0)
	mockChain.assets = make(map[types.Hash]*gtypes.AssetObject, 0)
	mockChain.tesseractsByHash = make(map[types.Hash]*types.Tesseract)
	mockChain.tesseractsByHeight = make(map[string]*types.Tesseract)
	mockChain.latestTesseracts = make(map[types.Address]*types.Tesseract)

	return mockChain
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)

	mockState.balances = make(map[types.Address]*gtypes.BalanceObject)
	mockState.storage = make(map[types.Hash][]byte)
	mockState.accounts = make(map[types.Address]*types.Account)
	mockState.context = make(map[types.Address]*Context)
	mockState.logicManifests = make(map[string][]byte)
	mockState.logicStorage = make(map[string]map[string]string, 0)

	return mockState
}

func (c *MockChainManager) setLatestTesseract(
	t *testing.T,
	ts *types.Tesseract,
) {
	t.Helper()

	c.latestTesseracts[ts.Address()] = ts
}

// Chain manager mock functions
func (c *MockChainManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	ts, ok := c.latestTesseracts[addr]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy.Ixns = nil
	}

	return &tsCopy, nil
}

func (c *MockChainManager) GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	ts, ok := c.tesseractsByHash[hash]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy.Ixns = nil
	}

	return &tsCopy, nil
}

func (c *MockChainManager) setTesseractByHeight(
	t *testing.T,
	ts *types.Tesseract,
) {
	t.Helper()

	key := ts.Address().Hex() + strconv.Itoa(int(ts.Height()))
	c.tesseractsByHeight[key] = ts
}

func (c *MockChainManager) GetTesseractByHeight(
	address types.Address,
	height uint64,
	withInteractions bool,
) (*types.Tesseract, error) {
	key := address.Hex() + strconv.Itoa(int(height))

	ts, ok := c.tesseractsByHeight[key]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy.Ixns = nil
	}

	return &tsCopy, nil
}

func (c *MockChainManager) GetReceiptByIxHash(ixHash types.Hash) (*types.Receipt, error) {
	if receipt := c.receipts[ixHash]; receipt != nil {
		return receipt, nil
	}

	return nil, types.ErrReceiptNotFound
}

func (c *MockChainManager) GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetObject, error) {
	if result, ok := c.assets[types.BytesToHash(assetHash)]; ok {
		return result, nil
	}

	return nil, types.ErrFetchingAssetDataInfo
}

func (c *MockChainManager) setTesseractByHash(
	t *testing.T,
	ts *types.Tesseract,
) {
	t.Helper()

	c.tesseractsByHash[tests.GetTesseractHash(t, ts)] = ts
}

func (c *MockChainManager) setReceipt(hash types.Hash, receipt *types.Receipt) {
	c.receipts[hash] = receipt
}

func (c *MockChainManager) setAssets(id types.AssetID, spec *types.AssetDescriptor) {
	c.assets[types.BytesToHash(id.GetCID())] = &gtypes.AssetObject{
		LogicID: spec.LogicID,
		Symbol:  spec.Symbol,
		Owner:   spec.Owner,
		Supply:  spec.Supply,
	}
}

// State Manager mock functions

func (s *MockStateManager) GetContextByHash(address types.Address,
	hash types.Hash,
) (types.Hash, []id.KramaID, []id.KramaID, error) {
	context, ok := s.context[address]
	if !ok {
		return types.NilHash, nil, nil, types.ErrContextStateNotFound
	}

	return hash, context.behaviourNodes, context.randomNodes, nil
}

func (s *MockStateManager) GetBalances(addr types.Address, stateHash types.Hash) (*gtypes.BalanceObject, error) {
	if _, ok := s.balances[addr]; ok {
		return s.balances[addr].Copy(), nil
	}

	return nil, types.ErrAccountNotFound
}

func (s *MockStateManager) GetBalance(
	addr types.Address,
	assetID types.AssetID,
	stateHash types.Hash,
) (*big.Int, error) {
	if _, ok := s.balances[addr]; ok {
		if _, ok := s.balances[addr].Balances[assetID]; ok {
			return s.balances[addr].Balances[assetID], nil
		}

		return nil, types.ErrAssetNotFound
	}

	return nil, types.ErrAccountNotFound
}

func (s *MockStateManager) GetNonce(addr types.Address, stateHash types.Hash) (uint64, error) {
	if _, ok := s.accounts[addr]; ok {
		return s.accounts[addr].Nonce, nil
	}

	return 0, types.ErrAccountNotFound
}

func (s *MockStateManager) IsGenesis(addr types.Address) (bool, error) {
	if _, ok := s.storage[types.GetHash(addr.Bytes())]; ok {
		return true, nil
	}

	return false, nil
}

func (s *MockStateManager) setBalance(addr types.Address, assetID types.AssetID, balance *big.Int) {
	s.balances[addr] = &gtypes.BalanceObject{
		Balances: make(types.AssetMap),
	}
	s.balances[addr].Balances[assetID] = balance
}

func getContext(t *testing.T, count int) *Context {
	t.Helper()

	return &Context{
		tests.GetTestKramaIDs(t, count),
		tests.GetTestKramaIDs(t, count),
	}
}

func (s *MockStateManager) setContext(t *testing.T, address types.Address, context *Context) {
	t.Helper()

	s.context[address] = context
}

func (s *MockStateManager) setAccount(addr types.Address, acc types.Account) {
	s.accounts[addr] = &acc
}

func (s *MockStateManager) getTDU(addr types.Address, stateHash types.Hash) types.AssetMap {
	data, _ := s.balances[addr].TDU()

	return data
}

func (s *MockStateManager) setLogicManifest(logicID string, logicManifest []byte) {
	s.logicManifests[logicID] = logicManifest
}

type MockIxPool struct {
	interactions map[types.Hash]*types.Interaction
	nextNonce    map[types.Address]uint64
	waitTime     map[types.Address]int64
	pending      map[types.Address][]*types.Interaction
	queued       map[types.Address][]*types.Interaction
}

func NewMockIxPool(t *testing.T) *MockIxPool {
	t.Helper()

	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[types.Hash]*types.Interaction)
	ixpool.nextNonce = make(map[types.Address]uint64)
	ixpool.waitTime = make(map[types.Address]int64)
	ixpool.pending = make(map[types.Address][]*types.Interaction)
	ixpool.queued = make(map[types.Address][]*types.Interaction)

	return ixpool
}

func (mc *MockIxPool) AddInteractions(ixs types.Interactions) []error {
	errs := make([]error, len(ixs))

	mc.interactions[ixs[0].Hash()] = ixs[0]
	mc.nextNonce[ixs[0].Sender()]++

	return errs
}

func (mc *MockIxPool) GetNonce(addr types.Address) (uint64, error) {
	if nextNonce, ok := mc.nextNonce[addr]; ok {
		return atomic.LoadUint64(&nextNonce), nil
	}

	return 0, types.ErrAccountNotFound
}

func (mc *MockIxPool) GetIxs(addr types.Address, inclQueued bool) (promoted, enqueued []*types.Interaction) {
	if inclQueued {
		return mc.pending[addr], mc.queued[addr]
	}

	return mc.pending[addr], types.Interactions{}
}

func (mc *MockIxPool) GetAllIxs(inclQueued bool) (allPromoted, allEnqueued map[types.Address][]*types.Interaction) {
	if inclQueued {
		return mc.pending, mc.queued
	}

	return mc.pending, map[types.Address][]*types.Interaction{}
}

func (mc *MockIxPool) GetAccountWaitTime(addr types.Address) (int64, error) {
	if waitTime, ok := mc.waitTime[addr]; ok {
		return waitTime, nil
	}

	return 0, types.ErrAccountNotFound
}

func (mc *MockIxPool) GetAllAccountsWaitTime() map[types.Address]int64 {
	return mc.waitTime
}

func (mc *MockIxPool) setNonce(addr types.Address, nonce uint64) {
	mc.nextNonce[addr] = nonce
}

func (mc *MockIxPool) setWaitTime(addr types.Address, waitTime int64) {
	mc.waitTime[addr] = waitTime
}

func (mc *MockIxPool) setIxs(addr types.Address, pending, queued []*types.Interaction) {
	mc.pending[addr] = pending
	mc.queued[addr] = queued
}

func GenerateRandomIXPayload(t *testing.T, size uint32) []byte {
	t.Helper()

	randomBytes := make([]byte, size)
	_, err := rand.Read(randomBytes)
	assert.NoError(t, err)

	return randomBytes
}

func GetTestLogicDeployPayload(
	t *testing.T,
	nonce uint64,
	address types.Address,
	callback func(args *LogicDeployArgs),
) ([]byte, []byte) {
	t.Helper()

	logicArgs := &LogicDeployArgs{
		Type:          0,
		IsStateFul:    true,
		IsInteractive: false,
		Manifest:      types.BytesToHex([]byte{0x00, 0x01}),
		CallData:      types.BytesToHex(GenerateRandomIXPayload(t, 20)),
	}

	if callback != nil {
		callback(logicArgs)
	}

	rawJSON, err := json.Marshal(logicArgs)
	require.NoError(t, err)

	logicID, _ := types.NewLogicIDv0(
		logicArgs.Type,
		logicArgs.IsStateFul,
		logicArgs.IsInteractive,
		0,
		utils.NewAccountAddress(nonce, address),
	)

	deployPayload := &types.LogicPayload{
		Logic:    logicID,
		Calldata: types.FromHex(logicArgs.CallData),
		Deploy: &types.LogicDeployPayload{
			Type:          logicArgs.Type,
			IsStateful:    logicArgs.IsStateFul,
			IsInteractive: logicArgs.IsInteractive,
			Manifest:      types.FromHex(logicArgs.Manifest),
		},
	}

	rawPolo, err := polo.Polorize(deployPayload)
	require.NoError(t, err)

	return rawJSON, rawPolo
}

func GetTestIxCreationPayload(t *testing.T, callBack func(args *AssetCreationArgs)) ([]byte, []byte) {
	t.Helper()

	payloadArgs := &AssetCreationArgs{
		Type:   1,
		Symbol: "rahul",
		Supply: "78",
	}

	if callBack != nil {
		callBack(payloadArgs)
	}

	jsonRaw, err := json.Marshal(payloadArgs)
	assert.NoError(t, err)

	createPayload := &types.AssetCreatePayload{
		Type:   payloadArgs.Type,
		Symbol: payloadArgs.Symbol,
		Supply: new(big.Int).SetInt64(120),

		Dimension: payloadArgs.Dimension,
		Decimals:  payloadArgs.Decimals,

		IsFungible:     payloadArgs.IsFungible,
		IsMintable:     payloadArgs.IsMintable,
		IsTransferable: payloadArgs.IsTransferable,

		LogicID: types.LogicID(payloadArgs.LogicID),
		// LogicCode: payloadArgs.LogicCode,
	}

	assetPayload := &types.AssetPayload{
		Create: createPayload,
	}

	poloRaw, err := polo.Polorize(assetPayload)
	assert.NoError(t, err)

	return jsonRaw, poloRaw
}

func getTesseractsHashes(t *testing.T, tesseracts []*types.Tesseract) []string {
	t.Helper()

	count := len(tesseracts)
	hashes := make([]string, count)

	for i, ts := range tesseracts {
		hashes[i] = getTesseractHash(t, ts).String()
	}

	return hashes
}

func getTesseractHash(t *testing.T, tesseract *types.Tesseract) types.Hash {
	t.Helper()

	tsHash, err := tesseract.Hash()
	require.NoError(t, err)

	return tsHash
}

func newReceipt(t *testing.T) *types.Receipt {
	t.Helper()

	return &types.Receipt{
		IxType: 1,
		IxHash: tests.RandomHash(t),
	}
}

func getReceipt(t *testing.T) (types.Hash, *types.Receipt) {
	t.Helper()

	receiptHash := tests.RandomHash(t)
	receipt := newReceipt(t)

	return receiptHash, receipt
}

func getLogicID(t *testing.T, address types.Address) types.LogicID {
	t.Helper()

	logicID, err := types.NewLogicIDv0(
		types.LogicKindSimple,
		true,
		true,
		1,
		address,
	)
	require.NoError(t, err)

	return logicID
}

func getStorageMap(keys []string, values []string) map[string]string {
	storage := make(map[string]string)

	for i, key := range keys {
		storage[string(types.FromHex(key))] = values[i] // each hex character should be a byte
	}

	return storage
}

func getHexEntries(t *testing.T, count int) []string {
	t.Helper()

	entries := make([]string, count)

	for i := 0; i < count; i++ {
		entries[i] = tests.RandomHash(t).Hex()
	}

	return entries
}

func checkForContext(
	t *testing.T,
	actualContext *Context,
	expectedBehaviouralNodes []string,
	expectedRandomNodes []string,
) {
	t.Helper()

	require.Equal(t, expectedBehaviouralNodes, utils.KramaIDToString(actualContext.behaviourNodes))
	require.Equal(t, expectedRandomNodes, utils.KramaIDToString(actualContext.randomNodes))
}

func newTestInteraction(
	t *testing.T,
	ixType types.IxType,
	callback func(ix *types.InteractionMessage),
) *types.Interaction {
	t.Helper()

	ixMsg := new(types.InteractionMessage)
	ixMsg.Data.Input.Type = ixType
	ixMsg.Data.Input.FuelLimit = big.NewInt(10000)

	if callback != nil {
		callback(ixMsg)
	}

	return ixMsg.ToInteraction()
}
