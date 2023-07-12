package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/crypto"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
)

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type ixData struct {
	ix      *common.Interaction
	parts   *common.TesseractParts
	ixIndex int
}

type MockChainManager struct {
	receipts                   map[common.Hash]*common.Receipt
	tesseractsByHash           map[common.Hash]*common.Tesseract
	tesseractsByHeight         map[string]*common.Tesseract
	latestTesseracts           map[common.Address]*common.Tesseract
	ixByTesseract              map[common.Hash]ixData
	ixByHash                   map[common.Hash]ixData
	TSHashByHeight             map[string]common.Hash
	GetInteractionByIxHashHook func() error
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.receipts = make(map[common.Hash]*common.Receipt, 0)
	mockChain.tesseractsByHash = make(map[common.Hash]*common.Tesseract)
	mockChain.tesseractsByHeight = make(map[string]*common.Tesseract)
	mockChain.latestTesseracts = make(map[common.Address]*common.Tesseract)
	mockChain.ixByHash = make(map[common.Hash]ixData)
	mockChain.ixByTesseract = make(map[common.Hash]ixData)
	mockChain.TSHashByHeight = make(map[string]common.Hash)

	return mockChain
}

func (c *MockChainManager) SetTesseractHeightEntry(address common.Address, height uint64, hash common.Hash) {
	key := address.Hex() + strconv.FormatUint(height, 10)
	c.TSHashByHeight[key] = hash
}

func (c *MockChainManager) GetTesseractHeightEntry(address common.Address, height uint64) (common.Hash, error) {
	key := address.Hex() + strconv.FormatUint(height, 10)

	hash, ok := c.TSHashByHeight[key]
	if !ok {
		return common.NilHash, common.ErrKeyNotFound
	}

	return hash, nil
}

func (c *MockChainManager) SetInteractionDataByTSHash(
	tsHash common.Hash,
	ix *common.Interaction,
	parts *common.TesseractParts,
) {
	c.ixByTesseract[tsHash] = ixData{
		ix:    ix,
		parts: parts,
	}
}

func (c *MockChainManager) GetInteractionAndPartsByTSHash(tsHash common.Hash, ixIndex int) (
	*common.Interaction,
	*common.TesseractParts,
	error,
) {
	data, ok := c.ixByTesseract[tsHash]
	if !ok {
		return nil, nil, common.ErrFetchingInteraction
	}

	return data.ix, data.parts, nil
}

func (c *MockChainManager) SetInteractionDataByIxHash(
	ix *common.Interaction,
	parts *common.TesseractParts,
	ixIndex int,
) {
	c.ixByHash[ix.Hash()] = ixData{
		ix:      ix,
		parts:   parts,
		ixIndex: ixIndex,
	}
}

func (c *MockChainManager) GetInteractionAndPartsByIxHash(ixHash common.Hash) (
	*common.Interaction,
	*common.TesseractParts,
	int,
	error,
) {
	if c.GetInteractionByIxHashHook != nil {
		return nil, nil, 0, c.GetInteractionByIxHashHook()
	}

	data, ok := c.ixByHash[ixHash]
	if !ok {
		return nil, nil, 0, common.ErrFetchingInteraction
	}

	return data.ix, data.parts, data.ixIndex, nil
}

// Chain manager mock functions
func (c *MockChainManager) GetLatestTesseract(addr common.Address, withInteractions bool) (*common.Tesseract, error) {
	ts, ok := c.latestTesseracts[addr]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (c *MockChainManager) GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	ts, ok := c.tesseractsByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func (c *MockChainManager) setReceiptByIXHash(ixHash common.Hash, receipt *common.Receipt) {
	c.receipts[ixHash] = receipt
}

func (c *MockChainManager) GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error) {
	if receipt := c.receipts[ixHash]; receipt != nil {
		return receipt, nil
	}

	return nil, common.ErrReceiptNotFound
}

func (c *MockChainManager) setTesseractByHash(
	t *testing.T,
	ts *common.Tesseract,
) {
	t.Helper()

	c.tesseractsByHash[tests.GetTesseractHash(t, ts)] = ts
}

type MockStateManager struct {
	storage        map[common.Hash][]byte
	balances       map[common.Address]*state.BalanceObject
	accounts       map[common.Address]*common.Account
	context        map[common.Address]*Context
	assetRegistry  map[common.AssetID]*common.AssetDescriptor
	logicManifests map[string][]byte
	logicStorage   map[string]map[string]string // first key denotes logic id, second key denotes storage key
	accMetaInfo    map[common.Address]*common.AccountMetaInfo
	logicIDs       map[common.Address][]common.LogicID
}

func (s *MockStateManager) GetRegistry(addr common.Address, stateHash common.Hash) (map[string][]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetAssetInfo(
	assetID common.AssetID,
	stateHash common.Hash,
) (*common.AssetDescriptor, error) {
	v, ok := s.assetRegistry[assetID]
	if !ok {
		return nil, common.ErrAssetNotFound
	}

	return v, nil
}

func (s *MockStateManager) addAsset(assetID common.AssetID, descriptor *common.AssetDescriptor) {
	s.assetRegistry[assetID] = descriptor
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)
	mockState.assetRegistry = make(map[common.AssetID]*common.AssetDescriptor)
	mockState.balances = make(map[common.Address]*state.BalanceObject)
	mockState.storage = make(map[common.Hash][]byte)
	mockState.accounts = make(map[common.Address]*common.Account)
	mockState.context = make(map[common.Address]*Context)
	mockState.logicManifests = make(map[string][]byte)
	mockState.logicStorage = make(map[string]map[string]string, 0)
	mockState.accMetaInfo = make(map[common.Address]*common.AccountMetaInfo)
	mockState.logicIDs = make(map[common.Address][]common.LogicID)

	return mockState
}

func (s *MockStateManager) GetLogicManifest(logicID common.LogicID, stateHash common.Hash) ([]byte, error) {
	logicManifest, ok := s.logicManifests[logicID.String()]
	if !ok {
		return logicManifest, errors.New("logic manifest not found")
	}

	return logicManifest, nil
}

func (s *MockStateManager) setLogicIDs(
	t *testing.T,
	addr common.Address,
	logicIDs []common.LogicID,
) {
	t.Helper()

	s.logicIDs[addr] = logicIDs
}

func (s *MockStateManager) GetLogicIDs(addr common.Address, stateHash common.Hash) ([]common.LogicID, error) {
	logicIDs, ok := s.logicIDs[addr]
	if !ok {
		return logicIDs, errors.New("logic IDs not found")
	}

	return logicIDs, nil
}

func (s *MockStateManager) setAccountMetaInfo(
	t *testing.T,
	address common.Address,
	accMetaInfo *common.AccountMetaInfo,
) {
	t.Helper()

	s.accMetaInfo[address] = accMetaInfo
}

func (s *MockStateManager) GetAccountMetaInfo(addr common.Address) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := s.accMetaInfo[addr]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return accMetaInfo, nil
}

func (s *MockStateManager) SetStorageEntry(logicID common.LogicID, storage map[string]string) {
	s.logicStorage[logicID.String()] = storage
}

func (s *MockStateManager) GetStorageEntry(logicID common.LogicID, slot []byte, stateHash common.Hash) ([]byte, error) {
	storage, ok := s.logicStorage[logicID.String()]
	if !ok {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	value, ok := storage[string(slot)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return []byte(value), nil
}

func (s *MockStateManager) GetLatestStateObject(addr common.Address) (*state.Object, error) {
	// TODO implement me
	panic("implement me")
}

func (s *MockStateManager) GetAccountState(addr common.Address, stateHash common.Hash) (*common.Account, error) {
	account, ok := s.accounts[addr]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return account, nil
}

func (s *MockStateManager) GetContextByHash(address common.Address,
	hash common.Hash,
) (common.Hash, []id.KramaID, []id.KramaID, error) {
	context, ok := s.context[address]
	if !ok {
		return common.NilHash, nil, nil, common.ErrContextStateNotFound
	}

	return hash, context.behaviourNodes, context.randomNodes, nil
}

func (s *MockStateManager) GetBalances(addr common.Address, stateHash common.Hash) (*state.BalanceObject, error) {
	if _, ok := s.balances[addr]; ok {
		return s.balances[addr].Copy(), nil
	}

	return nil, common.ErrAccountNotFound
}

func (s *MockStateManager) GetBalance(
	addr common.Address,
	assetID common.AssetID,
	stateHash common.Hash,
) (*big.Int, error) {
	if _, ok := s.balances[addr]; ok {
		if _, ok := s.balances[addr].AssetMap[assetID]; ok {
			return s.balances[addr].AssetMap[assetID], nil
		}

		return nil, common.ErrAssetNotFound
	}

	return nil, common.ErrAccountNotFound
}

func (s *MockStateManager) GetNonce(addr common.Address, stateHash common.Hash) (uint64, error) {
	if _, ok := s.accounts[addr]; ok {
		return s.accounts[addr].Nonce, nil
	}

	return 0, common.ErrAccountNotFound
}

func (s *MockStateManager) IsGenesis(addr common.Address) (bool, error) {
	if _, ok := s.storage[common.GetHash(addr.Bytes())]; ok {
		return true, nil
	}

	return false, nil
}

func (s *MockStateManager) setBalance(addr common.Address, assetID common.AssetID, balance *big.Int) {
	s.balances[addr] = &state.BalanceObject{
		AssetMap: make(common.AssetMap),
	}
	s.balances[addr].AssetMap[assetID] = balance
}

func (s *MockStateManager) setContext(t *testing.T, address common.Address, context *Context) {
	t.Helper()

	s.context[address] = context
}

func (s *MockStateManager) setAccount(addr common.Address, acc common.Account) {
	s.accounts[addr] = &acc
}

func (s *MockStateManager) getTDU(addr common.Address, stateHash common.Hash) common.AssetMap {
	data, _ := s.balances[addr].TDU()

	return data
}

func (s *MockStateManager) setLogicManifest(logicID string, logicManifest []byte) {
	s.logicManifests[logicID] = logicManifest
}

type MockExecutionManager struct {
	logicCall map[common.Address]*rpcargs.LogicCallResult
}

func NewMockExecutionManager(t *testing.T) *MockExecutionManager {
	t.Helper()

	exec := new(MockExecutionManager)
	exec.logicCall = make(map[common.Address]*rpcargs.LogicCallResult)

	return exec
}

func (exec *MockExecutionManager) setLogicCall(addr common.Address, logicCallResult *rpcargs.LogicCallResult) {
	exec.logicCall[addr] = logicCallResult
}

func (exec *MockExecutionManager) LogicCall(
	logicID common.LogicID,
	addr common.Address,
	callsite string,
	calldata []byte,
) (engineio.Fuel, *common.LogicInvokeReceipt, error) {
	logicCall, ok := exec.logicCall[addr]
	if !ok {
		return nil, nil, common.ErrAccountNotFound
	}

	return logicCall.Consumed.ToInt(), &common.LogicInvokeReceipt{Outputs: logicCall.Outputs, Error: logicCall.Error}, nil
}

type MockIxPool struct {
	interactions       map[common.Hash]*common.Interaction
	nextNonce          map[common.Address]uint64
	waitTime           map[common.Address]*big.Int
	pending            map[common.Address][]*common.Interaction
	queued             map[common.Address][]*common.Interaction
	pendingIX          map[common.Hash]*common.Interaction
	addInteractionHook func() []error
}

func NewMockIxPool(t *testing.T) *MockIxPool {
	t.Helper()

	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[common.Hash]*common.Interaction)
	ixpool.nextNonce = make(map[common.Address]uint64)
	ixpool.waitTime = make(map[common.Address]*big.Int)
	ixpool.pending = make(map[common.Address][]*common.Interaction)
	ixpool.queued = make(map[common.Address][]*common.Interaction)
	ixpool.pendingIX = make(map[common.Hash]*common.Interaction)

	return ixpool
}

func (mc *MockIxPool) SetPendingIx(ix *common.Interaction) {
	mc.pendingIX[ix.Hash()] = ix
}

func (mc *MockIxPool) GetPendingIx(ixHash common.Hash) (*common.Interaction, bool) {
	ix, ok := mc.pendingIX[ixHash]
	if !ok {
		return nil, false
	}

	return ix, true
}

func (mc *MockIxPool) AddInteractions(ixs common.Interactions) []error {
	if mc.addInteractionHook != nil {
		return mc.addInteractionHook()
	}

	for _, ix := range ixs {
		mc.interactions[ix.Hash()] = ix
	}

	return nil
}

func (mc *MockIxPool) GetNonce(addr common.Address) (uint64, error) {
	if nextNonce, ok := mc.nextNonce[addr]; ok {
		return atomic.LoadUint64(&nextNonce), nil
	}

	return 0, common.ErrAccountNotFound
}

func (mc *MockIxPool) GetIxs(addr common.Address, inclQueued bool) (promoted, enqueued []*common.Interaction) {
	if inclQueued {
		return mc.pending[addr], mc.queued[addr]
	}

	return mc.pending[addr], common.Interactions{}
}

func (mc *MockIxPool) GetAllIxs(inclQueued bool) (allPromoted, allEnqueued map[common.Address][]*common.Interaction) {
	if inclQueued {
		return mc.pending, mc.queued
	}

	return mc.pending, map[common.Address][]*common.Interaction{}
}

func (mc *MockIxPool) GetAccountWaitTime(addr common.Address) (*big.Int, error) {
	if waitTime, ok := mc.waitTime[addr]; ok {
		return waitTime, nil
	}

	return nil, common.ErrAccountNotFound
}

func (mc *MockIxPool) GetAllAccountsWaitTime() map[common.Address]*big.Int {
	return mc.waitTime
}

func (mc *MockIxPool) setNonce(addr common.Address, nonce uint64) {
	mc.nextNonce[addr] = nonce
}

func (mc *MockIxPool) setWaitTime(addr common.Address, waitTime int64) {
	mc.waitTime[addr] = big.NewInt(waitTime)
}

func (mc *MockIxPool) setIxs(addr common.Address, pending, queued []*common.Interaction) {
	mc.pending[addr] = pending
	mc.queued[addr] = queued
}

type MockNetwork struct {
	peers   []id.KramaID
	version string
}

func (mn *MockNetwork) GetKramaID() id.KramaID {
	panic("implement me")
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
	addrList []common.Address
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

	return nil, common.ErrKeyNotFound
}

func (d *MockDatabase) setList(t *testing.T, addressList []common.Address) {
	t.Helper()

	d.addrList = addressList
}

func (d *MockDatabase) GetRegisteredAccounts() ([]common.Address, error) {
	return d.addrList, nil
}

func createHeaderCallbackWithTestData(t *testing.T) func(header *common.TesseractHeader) {
	t.Helper()

	return func(header *common.TesseractHeader) {
		header.Address = tests.RandomAddress(t)
		header.PrevHash = tests.RandomHash(t)
		header.Height = 4
		header.FuelUsed = big.NewInt(88)
		header.FuelLimit = big.NewInt(99)
		header.BodyHash = tests.RandomHash(t)
		header.GroupHash = tests.RandomHash(t)
		header.Operator = "operator"
		header.ClusterID = "cluster-id"
		header.Timestamp = 3
		header.ContextLock = make(map[common.Address]common.ContextLockInfo)
		header.ContextLock[tests.RandomAddress(t)] = common.ContextLockInfo{
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
	address common.Address,
	callback func(args *rpcargs.RPCLogicPayload),
) ([]byte, []byte) {
	t.Helper()

	logicArgs := &rpcargs.RPCLogicPayload{
		Manifest: hexutil.Bytes([]byte{0x00, 0x01}),
		Calldata: hexutil.Bytes(GenerateRandomIXPayload(t, 20)),
	}

	if callback != nil {
		callback(logicArgs)
	}

	rawJSON, err := json.Marshal(logicArgs)
	require.NoError(t, err)

	deployPayload := &common.LogicPayload{
		Calldata: logicArgs.Calldata.Bytes(),
		Manifest: logicArgs.Manifest.Bytes(),
	}

	rawPolo, err := polo.Polorize(deployPayload)
	require.NoError(t, err)

	return rawJSON, rawPolo
}

func GetTestIxCreationPayload(t *testing.T, callBack func(args *rpcargs.RPCAssetCreation)) ([]byte, []byte) {
	t.Helper()

	payloadArgs := &rpcargs.RPCAssetCreation{
		Symbol: "rahul",
		Supply: (*hexutil.Big)(big.NewInt(78)),
	}

	if callBack != nil {
		callBack(payloadArgs)
	}

	jsonRaw, err := json.Marshal(payloadArgs)
	assert.NoError(t, err)

	createPayload := &common.AssetCreatePayload{
		Symbol:     payloadArgs.Symbol,
		IsLogical:  payloadArgs.IsLogical,
		IsStateFul: payloadArgs.IsStateful,
	}

	if payloadArgs.Logic != nil {
		createPayload.LogicPayload = payloadArgs.Logic.LogicPayload()
	}

	if payloadArgs.Standard != nil {
		createPayload.Standard = common.AssetStandard(payloadArgs.Standard.ToInt())
	}

	if payloadArgs.Supply != nil {
		createPayload.Supply = payloadArgs.Supply.ToInt()
	}

	if payloadArgs.Dimension != nil {
		createPayload.Dimension = payloadArgs.Dimension.ToInt()
	}

	assetPayload := &common.AssetPayload{
		Create: createPayload,
	}

	poloRaw, err := polo.Polorize(assetPayload)
	assert.NoError(t, err)

	return jsonRaw, poloRaw
}

func getTesseractsHashes(t *testing.T, tesseracts []*common.Tesseract) []common.Hash {
	t.Helper()

	count := len(tesseracts)
	hashes := make([]common.Hash, count)

	for i, ts := range tesseracts {
		hashes[i] = getTesseractHash(t, ts)
	}

	return hashes
}

func getTesseractHash(t *testing.T, tesseract *common.Tesseract) common.Hash {
	t.Helper()

	return tesseract.Hash()
}

func getLogicID(t *testing.T, address common.Address) common.LogicID {
	t.Helper()

	return common.NewLogicIDv0(true, false, false, false, 0, address)
}

func getStorageMap(keys []string, values []string) map[string]string {
	storage := make(map[string]string)

	for i, key := range keys {
		storage[string(common.FromHex(key))] = values[i] // each hex character should be a byte
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
	ixType common.IxType,
	payload json.RawMessage,
	mtq uint,
	mode uint64,
) *tests.CreateIxParams {
	return &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
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

func getSignatureString(t *testing.T, sendIXArgs *common.SendIXArgs, mnemonic string) string {
	t.Helper()

	bz, err := sendIXArgs.Bytes()
	require.NoError(t, err)

	sign, err := crypto.GetSignature(bz, mnemonic)
	require.NoError(t, err)

	return sign
}

func getSignatureBytes(t *testing.T, sendIXArgs *common.SendIXArgs, mnemonic string) []byte {
	t.Helper()

	sign := getSignatureString(t, sendIXArgs, mnemonic)

	signBytes, err := hex.DecodeString(sign)
	require.NoError(t, err)

	return signBytes
}

func createInteractionWithTestData(t *testing.T, ixType common.IxType, payload []byte) *common.Interaction {
	t.Helper()

	ixData := common.IxData{
		Input:   tests.CreateIXInputWithTestData(t, ixType, payload, []byte{187, 1, 29, 103}),
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ix, err := common.NewInteraction(ixData, tests.RandomHash(t).Bytes())
	require.NoError(t, err)

	return ix
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
	ixType common.IxType,
	callback func(ixData *common.IxData),
) *common.Interaction {
	t.Helper()

	ixData := &common.IxData{
		Input: common.IxInput{
			Type:      ixType,
			FuelLimit: big.NewInt(10000),
		},
	}

	if callback != nil {
		callback(ixData)
	}

	ix, err := common.NewInteraction(*ixData, nil)
	require.NoError(t, err)

	return ix
}

// checkForRPCIxn validates a field from input, compute, trust, and verifies payload.
func checkForRPCIxn(
	t *testing.T,
	ix *common.Interaction,
	rpcIxn *rpcargs.RPCInteraction,
	grid map[common.Address]common.TesseractHeightAndHash,
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
	require.Equal(t, input.Nonce, rpcIxn.Nonce.ToUint64())

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

	require.Equal(t, compute.Mode, rpcIxn.Mode.ToUint64())
	require.Equal(t, compute.Hash, rpcIxn.ComputeHash)
	require.Equal(t, compute.ComputeNodes, rpcIxn.ComputeNodes)

	require.Equal(t, trust.MTQ, uint(rpcIxn.MTQ.ToUint64()))
	require.Equal(t, trust.TrustNodes, rpcIxn.TrustNodes)

	switch ix.Type() {
	case common.IxValueTransfer:
		require.Equal(t, json.RawMessage(nil), rpcIxn.Payload)

	case common.IxAssetCreate:
		assetCreationPayload := new(common.AssetCreatePayload)
		err := assetCreationPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcAssetCreationPayload := rpcargs.RPCAssetCreation{
			Symbol:     assetCreationPayload.Symbol,
			Supply:     (*hexutil.Big)(assetCreationPayload.Supply),
			Dimension:  (*hexutil.Uint8)(&assetCreationPayload.Dimension),
			Standard:   (*hexutil.Uint16)(&assetCreationPayload.Standard),
			IsLogical:  assetCreationPayload.IsLogical,
			IsStateful: assetCreationPayload.IsStateFul,

			Logic: rpcargs.RPClogicPayloadFromLogicPayload(assetCreationPayload.LogicPayload),
		}

		expectedPayload, err := json.Marshal(rpcAssetCreationPayload)
		require.NoError(t, err)

		require.Equal(t, expectedPayload, []byte(rpcIxn.Payload))

	case common.IxLogicDeploy:
		fallthrough

	case common.IxLogicInvoke:
		logicPayload := new(common.LogicPayload)

		err := logicPayload.FromBytes(ix.Payload())
		require.NoError(t, err)

		rpcLogicPayload := &rpcargs.RPCLogicPayload{
			Manifest: (hexutil.Bytes)(logicPayload.Manifest),
			LogicID:  logicPayload.Logic.String(),
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
	grid map[common.Address]common.TesseractHeightAndHash,
	rpcParts rpcargs.RPCTesseractParts,
) {
	t.Helper()

	require.Equal(t, len(grid), len(rpcParts))

	for _, rpcPart := range rpcParts {
		heightAndHash, ok := grid[rpcPart.Address]
		require.True(t, ok)

		require.Equal(t, heightAndHash.Hash, rpcPart.Hash)
		require.Equal(t, heightAndHash.Height, rpcPart.Height.ToUint64())
	}

	CheckIfPartsSorted(t, rpcParts)
}

func CheckIfPartsSorted(t *testing.T, parts rpcargs.RPCTesseractParts) {
	t.Helper()

	for i := 1; i < len(parts); i++ {
		require.True(t, parts[i-1].Address.Hex() < parts[i].Address.Hex())
	}
}

func checkForRPCTesseractGridID(
	t *testing.T,
	tesseractGridID *common.TesseractGridID,
	rpcTesseractGridID *rpcargs.RPCTesseractGridID,
) {
	t.Helper()

	if tesseractGridID == nil {
		require.Nil(t, rpcTesseractGridID)

		return
	}

	require.Equal(t, tesseractGridID.Hash, rpcTesseractGridID.Hash)

	if tesseractGridID.Parts != nil {
		require.Equal(t, uint64(tesseractGridID.Parts.Total), rpcTesseractGridID.Total.ToUint64())
		checkForRPCTesseractParts(t, tesseractGridID.Parts.Grid, rpcTesseractGridID.Parts)

		return
	}

	require.Equal(t, 0, int(rpcTesseractGridID.Total))
}

func checkForRPCCommitData(t *testing.T, commitData common.CommitData, rpcCommitData rpcargs.RPCCommitData) {
	t.Helper()

	require.Equal(t, uint64(commitData.Round), rpcCommitData.Round.ToUint64())
	require.Equal(t, commitData.CommitSignature, rpcCommitData.CommitSignature.Bytes())
	require.Equal(t, commitData.VoteSet.String(), rpcCommitData.VoteSet)
	require.Equal(t, commitData.EvidenceHash, rpcCommitData.EvidenceHash)

	if commitData.GridID != nil {
		checkForRPCTesseractGridID(t, commitData.GridID, rpcCommitData.GridID)
	}
}

func checkForRPCContextLockInfos(
	t *testing.T,
	expectedContextLockInfos map[common.Address]common.ContextLockInfo,
	rpcContextLockInfos rpcargs.RPCContextLockInfos,
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
		require.Equal(t, contextLockInfo.Height, rpcContextLockInfo.Height.ToUint64())
		require.Equal(t, contextLockInfo.TesseractHash, rpcContextLockInfo.TesseractHash)
	}

	for i := 1; i < len(rpcContextLockInfos); i++ {
		require.True(t, rpcContextLockInfos[i-1].Address.Hex() < rpcContextLockInfos[i].Address.Hex())
	}
}

func checkForRPCDeltaGroups(
	t *testing.T,
	expectedRPCDeltaGroups map[common.Address]*common.DeltaGroup,
	rpcDeltaGroups rpcargs.RPCDeltaGroups,
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

func checkForRPCHeader(t *testing.T, header common.TesseractHeader, rpcHeader rpcargs.RPCHeader) {
	t.Helper()

	require.Equal(t, header.Address, rpcHeader.Address)
	require.Equal(t, header.PrevHash, rpcHeader.PrevHash)
	require.Equal(t, header.Height, rpcHeader.Height.ToUint64())
	require.Equal(t, header.FuelUsed, rpcHeader.FuelUsed.ToInt())
	require.Equal(t, header.FuelLimit, rpcHeader.FuelLimit.ToInt())
	require.Equal(t, header.BodyHash, rpcHeader.BodyHash)
	require.Equal(t, header.GroupHash, rpcHeader.GridHash)
	require.Equal(t, header.Operator, rpcHeader.Operator)
	require.Equal(t, header.ClusterID, rpcHeader.ClusterID)
	require.Equal(t, uint64(header.Timestamp), rpcHeader.Timestamp.ToUint64())
	checkForRPCContextLockInfos(t, header.ContextLock, rpcHeader.ContextLock)

	checkForRPCCommitData(t, header.Extra, rpcHeader.Extra)
}

func checkForRPCBody(t *testing.T, body common.TesseractBody, rpcBody rpcargs.RPCBody) {
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
	ts *common.Tesseract,
	rpcTS *rpcargs.RPCTesseract,
) {
	t.Helper()

	var grid map[common.Address]common.TesseractHeightAndHash

	checkForRPCHeader(t, ts.Header(), rpcTS.Header)
	checkForRPCBody(t, ts.Body(), rpcTS.Body)
	require.Equal(t, ts.Seal(), rpcTS.Seal.Bytes())

	require.Equal(t, ts.Hash(), rpcTS.Hash)

	if ts.ClusterID() == common.GenesisIdentifier {
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

func checkForRPCHashes(
	t *testing.T,
	expectedRPCHashes common.ReceiptAccHashes,
	rpcHashes rpcargs.RPCHashes,
) {
	t.Helper()

	require.Equal(t, len(expectedRPCHashes), len(rpcHashes))

	for _, rpcHash := range rpcHashes {
		stateHash := expectedRPCHashes.StateHash(rpcHash.Address)
		contextHash := expectedRPCHashes.ContextHash(rpcHash.Address)

		require.Equal(t, stateHash, rpcHash.StateHash)
		require.Equal(t, contextHash, rpcHash.ContextHash)
	}

	for i := 1; i < len(rpcHashes); i++ {
		require.True(t, rpcHashes[i-1].Address.Hex() < rpcHashes[i].Address.Hex())
	}
}

func checkForRPCReceipt(
	t *testing.T,
	grid map[common.Address]common.TesseractHeightAndHash,
	ix *common.Interaction,
	receipt *common.Receipt,
	rpcReceipt *rpcargs.RPCReceipt,
	ixIndex int,
) {
	t.Helper()

	checkForRPCTesseractParts(t, grid, rpcReceipt.Parts)
	require.Equal(t, uint64(receipt.IxType), rpcReceipt.IxType.ToUint64())
	require.Equal(t, receipt.IxHash, rpcReceipt.IxHash)
	require.Equal(t, receipt.FuelUsed, rpcReceipt.FuelUsed.ToInt())
	checkForRPCHashes(t, receipt.Hashes, rpcReceipt.Hashes)
	require.Equal(t, receipt.ExtraData, rpcReceipt.ExtraData)
	require.Equal(t, ix.Sender(), rpcReceipt.From)
	require.Equal(t, ix.Receiver(), rpcReceipt.To)
	require.Equal(t, uint64(ixIndex), rpcReceipt.IXIndex.ToUint64())
}
