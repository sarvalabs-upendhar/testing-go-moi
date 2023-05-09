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

	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/guna"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/lattice"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type ixData struct {
	ix      *types.Interaction
	parts   *types.TesseractParts
	ixIndex int
}

type MockChainManager struct {
	receipts                   map[types.Hash]*types.Receipt
	assets                     map[types.Hash]*gtypes.AssetObject
	tesseractsByHash           map[types.Hash]*types.Tesseract
	tesseractsByHeight         map[string]*types.Tesseract
	latestTesseracts           map[types.Address]*types.Tesseract
	ixByTesseract              map[types.Hash]ixData
	ixByHash                   map[types.Hash]ixData
	TSHashByHeight             map[string]types.Hash
	GetInteractionByIxHashHook func() error
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.receipts = make(map[types.Hash]*types.Receipt, 0)
	mockChain.assets = make(map[types.Hash]*gtypes.AssetObject, 0)
	mockChain.tesseractsByHash = make(map[types.Hash]*types.Tesseract)
	mockChain.tesseractsByHeight = make(map[string]*types.Tesseract)
	mockChain.latestTesseracts = make(map[types.Address]*types.Tesseract)
	mockChain.ixByHash = make(map[types.Hash]ixData)
	mockChain.ixByTesseract = make(map[types.Hash]ixData)
	mockChain.TSHashByHeight = make(map[string]types.Hash)

	return mockChain
}

func (c *MockChainManager) SetTesseractHeightEntry(address types.Address, height uint64, hash types.Hash) {
	key := address.Hex() + strconv.FormatUint(height, 10)
	c.TSHashByHeight[key] = hash
}

func (c *MockChainManager) GetTesseractHeightEntry(address types.Address, height uint64) (types.Hash, error) {
	key := address.Hex() + strconv.FormatUint(height, 10)

	hash, ok := c.TSHashByHeight[key]
	if !ok {
		return types.NilHash, types.ErrKeyNotFound
	}

	return hash, nil
}

func (c *MockChainManager) SetInteractionDataByTSHash(
	tsHash types.Hash,
	ix *types.Interaction,
	parts *types.TesseractParts,
) {
	c.ixByTesseract[tsHash] = ixData{
		ix:    ix,
		parts: parts,
	}
}

func (c *MockChainManager) GetInteractionAndPartsByTSHash(tsHash types.Hash, ixIndex int) (
	*types.Interaction,
	*types.TesseractParts,
	error,
) {
	data, ok := c.ixByTesseract[tsHash]
	if !ok {
		return nil, nil, types.ErrFetchingInteraction
	}

	return data.ix, data.parts, nil
}

func (c *MockChainManager) SetInteractionDataByIxHash(ix *types.Interaction, parts *types.TesseractParts, ixIndex int) {
	c.ixByHash[ix.Hash()] = ixData{
		ix:      ix,
		parts:   parts,
		ixIndex: ixIndex,
	}
}

func (c *MockChainManager) GetInteractionAndPartsByIxHash(ixHash types.Hash) (
	*types.Interaction,
	*types.TesseractParts,
	int,
	error,
) {
	if c.GetInteractionByIxHashHook != nil {
		return nil, nil, 0, c.GetInteractionByIxHashHook()
	}

	data, ok := c.ixByHash[ixHash]
	if !ok {
		return nil, nil, 0, types.ErrFetchingInteraction
	}

	return data.ix, data.parts, data.ixIndex, nil
}

// Chain manager mock functions
func (c *MockChainManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	ts, ok := c.latestTesseracts[addr]
	if !ok {
		return nil, types.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
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
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (c *MockChainManager) setReceiptByIXHash(ixHash types.Hash, receipt *types.Receipt) {
	c.receipts[ixHash] = receipt
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

func (c *MockChainManager) setAssets(id types.AssetID, spec *types.AssetDescriptor) {
	c.assets[types.BytesToHash(id.GetCID())] = &gtypes.AssetObject{
		LogicID: spec.LogicID,
		Symbol:  spec.Symbol,
		Owner:   spec.Owner,
		Supply:  spec.Supply,
	}
}

type MockStateManager struct {
	storage        map[types.Hash][]byte
	balances       map[types.Address]*gtypes.BalanceObject
	accounts       map[types.Address]*types.Account
	context        map[types.Address]*Context
	logicManifests map[string][]byte
	logicStorage   map[string]map[string]string // first key denotes logic id, second key denotes storage key
	accMetaInfo    map[types.Address]*types.AccountMetaInfo
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
	mockState.accMetaInfo = make(map[types.Address]*types.AccountMetaInfo)

	return mockState
}

func (s *MockStateManager) GetLogicManifest(logicID types.LogicID, stateHash types.Hash) ([]byte, error) {
	logicManifest, ok := s.logicManifests[logicID.Hex()]
	if !ok {
		return logicManifest, errors.New("logic manifest not found")
	}

	return logicManifest, nil
}

func (s *MockStateManager) setAccountMetaInfo(
	t *testing.T,
	address types.Address,
	accMetaInfo *types.AccountMetaInfo,
) {
	t.Helper()

	s.accMetaInfo[address] = accMetaInfo
}

func (s *MockStateManager) GetAccountMetaInfo(addr types.Address) (*types.AccountMetaInfo, error) {
	accMetaInfo, ok := s.accMetaInfo[addr]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return accMetaInfo, nil
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
		if _, ok := s.balances[addr].AssetMap[assetID]; ok {
			return s.balances[addr].AssetMap[assetID], nil
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
		AssetMap: make(types.AssetMap),
	}
	s.balances[addr].AssetMap[assetID] = balance
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
	waitTime     map[types.Address]*big.Int
	pending      map[types.Address][]*types.Interaction
	queued       map[types.Address][]*types.Interaction
	pendingIX    map[types.Hash]*types.Interaction
}

func NewMockIxPool(t *testing.T) *MockIxPool {
	t.Helper()

	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[types.Hash]*types.Interaction)
	ixpool.nextNonce = make(map[types.Address]uint64)
	ixpool.waitTime = make(map[types.Address]*big.Int)
	ixpool.pending = make(map[types.Address][]*types.Interaction)
	ixpool.queued = make(map[types.Address][]*types.Interaction)
	ixpool.pendingIX = make(map[types.Hash]*types.Interaction)

	return ixpool
}

func (mc *MockIxPool) SetPendingIx(ix *types.Interaction) {
	mc.pendingIX[ix.Hash()] = ix
}

func (mc *MockIxPool) GetPendingIx(ixHash types.Hash) (*types.Interaction, bool) {
	ix, ok := mc.pendingIX[ixHash]
	if !ok {
		return nil, false
	}

	return ix, true
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

func (mc *MockIxPool) GetAccountWaitTime(addr types.Address) (*big.Int, error) {
	if waitTime, ok := mc.waitTime[addr]; ok {
		return waitTime, nil
	}

	return nil, types.ErrAccountNotFound
}

func (mc *MockIxPool) GetAllAccountsWaitTime() map[types.Address]*big.Int {
	return mc.waitTime
}

func (mc *MockIxPool) setNonce(addr types.Address, nonce uint64) {
	mc.nextNonce[addr] = nonce
}

func (mc *MockIxPool) setWaitTime(addr types.Address, waitTime int64) {
	mc.waitTime[addr] = big.NewInt(waitTime)
}

func (mc *MockIxPool) setIxs(addr types.Address, pending, queued []*types.Interaction) {
	mc.pending[addr] = pending
	mc.queued[addr] = queued
}

type MockNetwork struct {
	peers   []id.KramaID
	version string
}

func NewMockNetwork(t *testing.T) *MockNetwork {
	t.Helper()

	network := new(MockNetwork)
	network.peers = make([]id.KramaID, 0)
	network.version = ""

	return network
}

func (mn *MockNetwork) setPeers(peersList []id.KramaID) {
	mn.peers = peersList
}

func (mn *MockNetwork) GetPeers() []id.KramaID {
	return mn.peers
}

func (mn *MockNetwork) setVersion(version string) {
	mn.version = version
}

func (mn *MockNetwork) GetVersion() string {
	return mn.version
}

type MockDatabase struct {
	database map[string][]byte
	addrList []types.Address
}

func NewMockDatabase(t *testing.T) *MockDatabase {
	t.Helper()

	db := new(MockDatabase)
	db.database = make(map[string][]byte)

	return db
}

func (d *MockDatabase) setDBEntry(key []byte) {
	d.database[string(key)] = key
}

func (d *MockDatabase) ReadEntry(key []byte) ([]byte, error) {
	if _, ok := d.database[string(key)]; ok {
		return d.database[string(key)], nil
	}

	return nil, types.ErrKeyNotFound
}

func (d *MockDatabase) setList(t *testing.T, addressList []types.Address) {
	t.Helper()

	d.addrList = addressList
}

func (d *MockDatabase) GetRegisteredAccounts() ([]types.Address, error) {
	return d.addrList, nil
}

func createHeaderCallbackWithTestData(t *testing.T) func(header *types.TesseractHeader) {
	t.Helper()

	return func(header *types.TesseractHeader) {
		header.Address = tests.RandomAddress(t)
		header.PrevHash = tests.RandomHash(t)
		header.Height = 4
		header.FuelUsed = 88
		header.FuelLimit = 99
		header.BodyHash = tests.RandomHash(t)
		header.GroupHash = tests.RandomHash(t)
		header.Operator = "operator"
		header.ClusterID = "cluster-id"
		header.Timestamp = 3
		header.ContextLock = make(map[types.Address]types.ContextLockInfo)
		header.ContextLock[tests.RandomAddress(t)] = types.ContextLockInfo{
			Height: 4,
		}
		header.Extra = tests.CreateCommitDataWithTestData(t)
	}
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
	callback func(args *ptypes.RPCLogicPayload),
) ([]byte, []byte) {
	t.Helper()

	logicArgs := &ptypes.RPCLogicPayload{
		Manifest: hexutil.Bytes([]byte{0x00, 0x01}),
		Calldata: hexutil.Bytes(GenerateRandomIXPayload(t, 20)),
	}

	if callback != nil {
		callback(logicArgs)
	}

	rawJSON, err := json.Marshal(logicArgs)
	require.NoError(t, err)

	deployPayload := &types.LogicPayload{
		Calldata: logicArgs.Calldata.Bytes(),
		Manifest: logicArgs.Manifest.Bytes(),
	}

	rawPolo, err := polo.Polorize(deployPayload)
	require.NoError(t, err)

	return rawJSON, rawPolo
}

func GetTestIxCreationPayload(t *testing.T, callBack func(args *ptypes.RPCAssetCreation)) ([]byte, []byte) {
	t.Helper()

	payloadArgs := &ptypes.RPCAssetCreation{
		Type:   1,
		Symbol: "rahul",
		Supply: (*hexutil.Big)(big.NewInt(78)),
	}

	if callBack != nil {
		callBack(payloadArgs)
	}

	jsonRaw, err := json.Marshal(payloadArgs)
	assert.NoError(t, err)

	createPayload := &types.AssetCreatePayload{
		Type:   payloadArgs.Type,
		Symbol: payloadArgs.Symbol,

		IsFungible:     payloadArgs.IsFungible,
		IsMintable:     payloadArgs.IsMintable,
		IsTransferable: payloadArgs.IsTransferable,

		LogicID: types.LogicID(payloadArgs.LogicID),
		// LogicCode: payloadArgs.LogicCode,
	}

	if payloadArgs.Supply != nil {
		createPayload.Supply = payloadArgs.Supply.ToInt()
	}

	if payloadArgs.Dimension != nil {
		createPayload.Dimension = payloadArgs.Dimension.ToInt()
	}

	if payloadArgs.Decimals != nil {
		createPayload.Decimals = payloadArgs.Decimals.ToInt()
	}

	assetPayload := &types.AssetPayload{
		Create: createPayload,
	}

	poloRaw, err := polo.Polorize(assetPayload)
	assert.NoError(t, err)

	return jsonRaw, poloRaw
}

func getTesseractsHashes(t *testing.T, tesseracts []*types.Tesseract) []types.Hash {
	t.Helper()

	count := len(tesseracts)
	hashes := make([]types.Hash, count)

	for i, ts := range tesseracts {
		hashes[i] = getTesseractHash(t, ts)
	}

	return hashes
}

func getTesseractHash(t *testing.T, tesseract *types.Tesseract) types.Hash {
	t.Helper()

	return tesseract.Hash()
}

func getLogicID(t *testing.T, address types.Address) types.LogicID {
	t.Helper()

	logicID, err := types.NewLogicIDv0(true, false, false, false, 0, address)
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

// getIxParamsWithInputComputeTrust returns ixparams initialized with ixType, payload, mtq, and mode
// We initialize at least one field in input, compute, and trust.
func getIxParamsWithInputComputeTrust(
	ixType types.IxType,
	payload json.RawMessage,
	mtq uint,
	mode uint64,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *types.IxData) {
			ix.Input.Type = ixType
			ix.Input.Payload = payload
			ix.Compute.Mode = mode
			ix.Trust.MTQ = mtq
		},
	}
}

func getContext(t *testing.T, count int) *Context {
	t.Helper()

	return &Context{
		tests.GetTestKramaIDs(t, count),
		tests.GetTestKramaIDs(t, count),
	}
}

func createInteractionWithTestData(t *testing.T, ixType types.IxType, payload []byte) *types.Interaction {
	t.Helper()

	ixData := types.IxData{
		Input:   tests.CreateIXInputWithTestData(t, ixType, payload, []byte{187, 1, 29, 103}),
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	return types.NewInteraction(ixData, tests.RandomHash(t).Bytes())
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
	callback func(ixData *types.IxData),
) *types.Interaction {
	t.Helper()

	ixData := &types.IxData{
		Input: types.IxInput{
			Type:      ixType,
			FuelLimit: big.NewInt(10000),
		},
	}

	if callback != nil {
		callback(ixData)
	}

	return types.NewInteraction(*ixData, nil)
}

// checkForRPCIxn validates a field from input, compute, trust, and verifies payload.
func checkForRPCIxn(
	t *testing.T,
	ix *types.Interaction,
	rpcIxn *ptypes.RPCInteraction,
	grid map[types.Address]types.TesseractHeightAndHash,
) {
	t.Helper()

	if len(grid) != 0 {
		checkForRPCTesseractParts(t, grid, rpcIxn.Parts)
	}

	input := ix.Input()
	compute := ix.Compute()
	trust := ix.Trust()

	require.Equal(t, ix.Hash(), rpcIxn.Hash)
	require.Equal(t, ix.Signature(), rpcIxn.Signature.Bytes())

	require.Equal(t, input.Type, rpcIxn.Type)
	require.Equal(t, input.Nonce, rpcIxn.Nonce.ToInt())

	require.Equal(t, input.Sender, rpcIxn.Sender)
	require.Equal(t, input.Receiver, rpcIxn.Receiver)
	require.Equal(t, input.Payer, rpcIxn.Payer)

	require.Equal(t, len(input.TransferValues), len(rpcIxn.TransferValues))
	require.Equal(t, len(input.PerceivedValues), len(rpcIxn.PerceivedValues))

	for assetID, amount := range input.TransferValues {
		flag := false

		for rpcAssetID, rpcAmount := range rpcIxn.TransferValues {
			if assetID == rpcAssetID {
				flag = true

				require.Equal(t, amount, rpcAmount.ToInt())
			}
		}

		require.True(t, flag)
	}

	for assetID, amount := range input.PerceivedValues {
		flag := false

		for rpcAssetID, rpcAmount := range rpcIxn.PerceivedValues {
			if assetID == rpcAssetID {
				flag = true

				require.Equal(t, amount, rpcAmount.ToInt())
			}
		}

		require.True(t, flag)
	}

	require.Equal(t, input.PerceivedProofs, rpcIxn.PerceivedProofs.Bytes())

	require.Equal(t, input.FuelLimit, rpcIxn.FuelLimit.ToInt())
	require.Equal(t, input.FuelPrice, rpcIxn.FuelPrice.ToInt())

	require.Equal(t, compute.Mode, rpcIxn.Mode.ToInt())
	require.Equal(t, compute.Hash, rpcIxn.ComputeHash)
	require.Equal(t, compute.ComputeNodes, rpcIxn.ComputeNodes)

	require.Equal(t, trust.MTQ, uint(rpcIxn.MTQ.ToInt()))
	require.Equal(t, trust.TrustNodes, rpcIxn.TrustNodes)

	switch ix.Type() {
	case types.IxValueTransfer:
		require.Equal(t, json.RawMessage(nil), rpcIxn.Payload)

	case types.IxAssetCreate:
		assetCreationPayload := new(types.AssetPayload)
		err := assetCreationPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcAssetCreationPayload := ptypes.RPCAssetCreation{
			Type:   assetCreationPayload.Create.Type,
			Symbol: assetCreationPayload.Create.Symbol,
			Supply: (*hexutil.Big)(assetCreationPayload.Create.Supply),

			Dimension: (*hexutil.Uint8)(&assetCreationPayload.Create.Dimension),
			Decimals:  (*hexutil.Uint8)(&assetCreationPayload.Create.Decimals),

			IsFungible:     assetCreationPayload.Create.IsFungible,
			IsMintable:     assetCreationPayload.Create.IsMintable,
			IsTransferable: assetCreationPayload.Create.IsTransferable,

			LogicID: types.BytesToHex(assetCreationPayload.Create.LogicID),
		}

		expectedPayload, err := json.Marshal(rpcAssetCreationPayload)
		require.NoError(t, err)

		require.Equal(t, expectedPayload, []byte(rpcIxn.Payload))

	case types.IxLogicDeploy:
		fallthrough

	case types.IxLogicInvoke:
		logicPayload := new(types.LogicPayload)

		err := logicPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcLogicPayload := &ptypes.RPCLogicPayload{
			Manifest: (hexutil.Bytes)(logicPayload.Manifest),
			LogicID:  types.BytesToHex(logicPayload.Logic),
			Callsite: logicPayload.Callsite,
			Calldata: (hexutil.Bytes)(logicPayload.Calldata),
		}

		expectedPayload, err := json.Marshal(rpcLogicPayload)
		require.NoError(t, err)

		require.Equal(t, expectedPayload, []byte(rpcIxn.Payload))
	default:
		require.FailNow(t, "invalid ix type")
	}
}

func checkForRPCTesseractParts(
	t *testing.T,
	grid map[types.Address]types.TesseractHeightAndHash,
	rpcParts ptypes.RPCTesseractParts,
) {
	t.Helper()

	require.Equal(t, len(grid), len(rpcParts))

	for _, rpcPart := range rpcParts {
		heightAndHash, ok := grid[rpcPart.Address]
		require.True(t, ok)

		require.Equal(t, heightAndHash.Hash, rpcPart.Hash)
		require.Equal(t, heightAndHash.Height, rpcPart.Height.ToInt())
	}

	tests.CheckIfPartsSorted(t, rpcParts)
}

func checkForRPCTesseractGridID(
	t *testing.T,
	tesseractGridID *types.TesseractGridID,
	rpcTesseractGridID *ptypes.RPCTesseractGridID,
) {
	t.Helper()

	if tesseractGridID == nil {
		require.Nil(t, rpcTesseractGridID)

		return
	}

	require.Equal(t, tesseractGridID.Hash, rpcTesseractGridID.Hash)

	if tesseractGridID.Parts != nil {
		require.Equal(t, uint64(tesseractGridID.Parts.Total), rpcTesseractGridID.Total.ToInt())
		checkForRPCTesseractParts(t, tesseractGridID.Parts.Grid, rpcTesseractGridID.Parts)

		return
	}

	require.Equal(t, 0, int(rpcTesseractGridID.Total))
}

func checkForRPCCommitData(t *testing.T, commitData types.CommitData, rpcCommitData ptypes.RPCCommitData) {
	t.Helper()

	require.Equal(t, uint64(commitData.Round), rpcCommitData.Round.ToInt())
	require.Equal(t, commitData.CommitSignature, rpcCommitData.CommitSignature.Bytes())
	require.Equal(t, commitData.VoteSet.String(), rpcCommitData.VoteSet)
	require.Equal(t, commitData.EvidenceHash, rpcCommitData.EvidenceHash)

	if commitData.GridID != nil {
		checkForRPCTesseractGridID(t, commitData.GridID, rpcCommitData.GridID)
	}
}

func checkForRPCContextLockInfos(
	t *testing.T,
	expectedContextLockInfos map[types.Address]types.ContextLockInfo,
	rpcContextLockInfos ptypes.RPCContextLockInfos,
) {
	t.Helper()

	if len(expectedContextLockInfos) == 0 {
		require.Nil(t, rpcContextLockInfos)

		return
	}

	require.Equal(t, len(expectedContextLockInfos), len(rpcContextLockInfos))

	for _, rpcContextLockInfo := range rpcContextLockInfos {
		contextLockInfo, ok := expectedContextLockInfos[rpcContextLockInfo.Address]
		require.True(t, ok)

		require.Equal(t, contextLockInfo.ContextHash, rpcContextLockInfo.ContextHash)
		require.Equal(t, contextLockInfo.Height, rpcContextLockInfo.Height.ToInt())
		require.Equal(t, contextLockInfo.TesseractHash, rpcContextLockInfo.TesseractHash)
	}

	for i := 1; i < len(rpcContextLockInfos); i++ {
		require.True(t, rpcContextLockInfos[i-1].Address.Hex() < rpcContextLockInfos[i].Address.Hex())
	}
}

func checkForRPCDeltaGroups(
	t *testing.T,
	expectedRPCDeltaGroups map[types.Address]*types.DeltaGroup,
	rpcDeltaGroups ptypes.RPCDeltaGroups,
) {
	t.Helper()

	if len(expectedRPCDeltaGroups) == 0 {
		require.Nil(t, rpcDeltaGroups)

		return
	}

	require.Equal(t, len(expectedRPCDeltaGroups), len(rpcDeltaGroups))

	for _, rpcDeltaGroup := range rpcDeltaGroups {
		deltaGroup, ok := expectedRPCDeltaGroups[rpcDeltaGroup.Address]
		require.True(t, ok)

		require.Equal(t, deltaGroup.Role, rpcDeltaGroup.Role)
		require.Equal(t, deltaGroup.BehaviouralNodes, rpcDeltaGroup.BehaviouralNodes)
		require.Equal(t, deltaGroup.RandomNodes, rpcDeltaGroup.RandomNodes)
		require.Equal(t, deltaGroup.ReplacedNodes, rpcDeltaGroup.ReplacedNodes)
	}

	for i := 1; i < len(rpcDeltaGroups); i++ {
		require.True(t, rpcDeltaGroups[i-1].Address.Hex() < rpcDeltaGroups[i].Address.Hex())
	}
}

func checkForRPCHeader(t *testing.T, header types.TesseractHeader, rpcHeader ptypes.RPCHeader) {
	t.Helper()

	require.Equal(t, header.Address, rpcHeader.Address)
	require.Equal(t, header.PrevHash, rpcHeader.PrevHash)
	require.Equal(t, header.Height, rpcHeader.Height.ToInt())
	require.Equal(t, header.FuelUsed, rpcHeader.FuelUsed.ToInt())
	require.Equal(t, header.FuelLimit, rpcHeader.FuelLimit.ToInt())
	require.Equal(t, header.BodyHash, rpcHeader.BodyHash)
	require.Equal(t, header.GroupHash, rpcHeader.GridHash)
	require.Equal(t, header.Operator, rpcHeader.Operator)
	require.Equal(t, header.ClusterID, rpcHeader.ClusterID)
	require.Equal(t, uint64(header.Timestamp), rpcHeader.Timestamp.ToInt())
	checkForRPCContextLockInfos(t, header.ContextLock, rpcHeader.ContextLock)

	checkForRPCCommitData(t, header.Extra, rpcHeader.Extra)
}

func checkForRPCBody(t *testing.T, body types.TesseractBody, rpcBody ptypes.RPCBody) {
	t.Helper()

	require.Equal(t, body.StateHash, rpcBody.StateHash)
	require.Equal(t, body.ContextHash, rpcBody.ContextHash)
	require.Equal(t, body.InteractionHash, rpcBody.InteractionHash)
	require.Equal(t, body.ReceiptHash, rpcBody.ReceiptHash)
	checkForRPCDeltaGroups(t, body.ContextDelta, rpcBody.ContextDelta)
	require.Equal(t, body.ConsensusProof, rpcBody.ConsensusProof)
}

// checkForRPCTesseract validates fields of rpc tesseract
func checkForRPCTesseract(
	t *testing.T,
	ts *types.Tesseract,
	rpcTS *ptypes.RPCTesseract,
) {
	t.Helper()

	var grid map[types.Address]types.TesseractHeightAndHash

	checkForRPCHeader(t, ts.Header(), rpcTS.Header)
	checkForRPCBody(t, ts.Body(), rpcTS.Body)
	require.Equal(t, ts.Seal(), rpcTS.Seal.Bytes())

	require.Equal(t, ts.Hash(), rpcTS.Hash)

	if ts.ClusterID() == lattice.GenesisIdentifier {
		for _, ix := range rpcTS.Ixns {
			require.Nil(t, ix)
		}

		return
	}

	parts, err := ts.Parts()
	if err == nil {
		grid = parts.Grid
	}

	for i, ixn := range ts.Interactions() {
		checkForRPCIxn(t, ixn, rpcTS.Ixns[i], grid)
	}
}

func checkForRPCStateHashes(
	t *testing.T,
	expectedRPCStateHashes map[types.Address]types.Hash,
	rpcStateHashes ptypes.RPCStateHashes,
) {
	t.Helper()

	require.Equal(t, len(expectedRPCStateHashes), len(rpcStateHashes))

	for _, rpcStateHash := range rpcStateHashes {
		hash, ok := expectedRPCStateHashes[rpcStateHash.Address]
		require.True(t, ok)

		require.Equal(t, hash, rpcStateHash.Hash)
	}

	for i := 1; i < len(rpcStateHashes); i++ {
		require.True(t, rpcStateHashes[i-1].Address.Hex() < rpcStateHashes[i].Address.Hex())
	}
}

func checkForRPCContextHashes(
	t *testing.T,
	expectedRPCStateHashes map[types.Address]types.Hash,
	rpcContextHashes ptypes.RPCContextHashes,
) {
	t.Helper()

	require.Equal(t, len(expectedRPCStateHashes), len(rpcContextHashes))

	for _, rpcStateHash := range rpcContextHashes {
		hash, ok := expectedRPCStateHashes[rpcStateHash.Address]
		require.True(t, ok)

		require.Equal(t, hash, rpcStateHash.Hash)
	}

	for i := 1; i < len(rpcContextHashes); i++ {
		require.True(t, rpcContextHashes[i-1].Address.Hex() < rpcContextHashes[i].Address.Hex())
	}
}

func checkForRPCReceipt(
	t *testing.T,
	grid map[types.Address]types.TesseractHeightAndHash,
	ix *types.Interaction,
	receipt *types.Receipt,
	rpcReceipt *ptypes.RPCReceipt,
	ixIndex int,
) {
	t.Helper()

	checkForRPCTesseractParts(t, grid, rpcReceipt.Parts)
	require.Equal(t, uint64(receipt.IxType), rpcReceipt.IxType.ToInt())
	require.Equal(t, receipt.IxHash, rpcReceipt.IxHash)
	require.Equal(t, receipt.FuelUsed, rpcReceipt.FuelUsed.ToInt())
	checkForRPCStateHashes(t, receipt.StateHashes, rpcReceipt.StateHashes)
	checkForRPCContextHashes(t, receipt.ContextHashes, rpcReceipt.ContextHashes)
	require.Equal(t, receipt.ExtraData, rpcReceipt.ExtraData)
	require.Equal(t, ix.Sender(), rpcReceipt.From)
	require.Equal(t, ix.Receiver(), rpcReceipt.To)
	require.Equal(t, uint64(ixIndex), rpcReceipt.IXIndex.ToInt())
}
