package api

import (
	"encoding/json"
	"math/big"
	"math/rand"
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
	tesseracts          map[types.Address]map[types.Hash]*types.Tesseract
	latestTesseractHash map[types.Address]types.Hash
	interactions        map[types.Address]map[types.Hash]types.Interactions
	receipts            map[types.Address]map[types.Hash]*types.Receipt
	storage             map[types.Hash]*types.Tesseract
	assets              map[types.Hash]*gtypes.AssetObject
}

type MockStateManager struct {
	storage           map[types.Hash][]byte
	balances          map[types.Address]*gtypes.BalanceObject
	accounts          map[types.Address]*types.Account
	context           map[types.Hash]*Context
	latestContextHash map[types.Address]types.Hash
}

func (ms *MockStateManager) GetStorageEntry(logicID types.LogicID, slot []byte) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (ms *MockStateManager) GetLatestStateObject(addr types.Address) (*guna.StateObject, error) {
	// TODO implement me
	panic("implement me")
}

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	mockChain := new(MockChainManager)

	mockChain.tesseracts = make(map[types.Address]map[types.Hash]*types.Tesseract, 0)
	mockChain.latestTesseractHash = make(map[types.Address]types.Hash)
	mockChain.interactions = make(map[types.Address]map[types.Hash]types.Interactions, 0)
	mockChain.receipts = make(map[types.Address]map[types.Hash]*types.Receipt, 0)
	mockChain.assets = make(map[types.Hash]*gtypes.AssetObject, 0)
	mockChain.storage = make(map[types.Hash]*types.Tesseract, 0)

	return mockChain
}

func NewMockStateManager() *MockStateManager {
	mockState := new(MockStateManager)

	mockState.balances = make(map[types.Address]*gtypes.BalanceObject)
	mockState.latestContextHash = make(map[types.Address]types.Hash)
	mockState.storage = make(map[types.Hash][]byte)
	mockState.accounts = make(map[types.Address]*types.Account)
	mockState.context = make(map[types.Hash]*Context)

	return mockState
}

// Chain manager mock functions
func (mc *MockChainManager) GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error) {
	if _, ok := mc.tesseracts[addr]; ok {
		tesseract := mc.tesseracts[addr][mc.latestTesseractHash[addr]]

		if withInteractions {
			tesseract.Ixns = mc.interactions[addr][tesseract.InteractionHash()]
		}

		return tesseract, nil
	}

	return nil, types.ErrAccountNotFound
}

func (mc *MockChainManager) GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error) {
	for _, tesseracts := range mc.tesseracts {
		for tsHash, tesseract := range tesseracts {
			if tsHash == hash {
				if withInteractions {
					tesseract.Ixns = mc.interactions[tesseract.Address()][tesseract.InteractionHash()]
				}

				return tesseract, nil
			}
		}
	}

	return nil, types.ErrKeyNotFound
}

func (mc *MockChainManager) GetReceipt(addr types.Address, ixHash types.Hash) (*types.Receipt, error) {
	if receipt := mc.receipts[addr][ixHash]; receipt != nil {
		return receipt, nil
	}

	return nil, types.ErrReceiptNotFound
}

func (mc *MockChainManager) GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetObject, error) {
	if result, ok := mc.assets[types.BytesToHash(assetHash)]; ok {
		return result, nil
	}

	return nil, types.ErrFetchingAssetDataInfo
}

func (mc *MockChainManager) GetTesseractByHeight(
	address types.Address,
	height uint64,
	withInteractions bool,
) (*types.Tesseract, error) {
	for _, tesseract := range mc.storage {
		if tesseract.Address() == address && tesseract.Height() == height {
			if withInteractions {
				tesseract.Ixns = mc.interactions[address][tesseract.InteractionHash()]
			}

			return tesseract, nil
		}
	}

	return nil, types.ErrKeyNotFound
}

func (mc *MockChainManager) setTesseracts(
	addr types.Address,
	latestTesseractHash types.Hash,
	tesseracts map[types.Hash]*types.Tesseract,
) {
	mc.latestTesseractHash[addr] = latestTesseractHash
	mc.tesseracts[addr] = tesseracts
}

func (mc *MockChainManager) setInteractions(addr types.Address, interactions map[types.Hash]types.Interactions) {
	mc.interactions[addr] = interactions
}

func (mc *MockChainManager) setReceipt(addr types.Address, receipts map[types.Hash]*types.Receipt) {
	mc.receipts[addr] = receipts
}

func (mc *MockChainManager) setStorage(hash types.Hash, tesseract *types.Tesseract) {
	mc.storage[hash] = tesseract
}

func (mc *MockChainManager) setAssets(id types.AssetID, spec *types.AssetDescriptor) {
	mc.assets[types.BytesToHash(id.GetCID())] = &gtypes.AssetObject{
		LogicID: spec.LogicID,
		Symbol:  spec.Symbol,
		Owner:   spec.Owner,
		Supply:  spec.Supply,
	}
}

// State Manager mock functions

func (ms *MockStateManager) GetContextByHash(address types.Address,
	hash types.Hash,
) (types.Hash, []id.KramaID, []id.KramaID, error) {
	if hash == types.NilHash {
		if hash, ok := ms.latestContextHash[address]; ok {
			return hash, ms.context[hash].behaviourNodes, ms.context[hash].randomNodes, nil
		}

		return types.NilHash, nil, nil, types.ErrAccountNotFound
	}

	return hash, ms.context[hash].behaviourNodes, ms.context[hash].randomNodes, nil
}

func (ms *MockStateManager) GetBalances(addr types.Address) (*gtypes.BalanceObject, error) {
	if _, ok := ms.balances[addr]; ok {
		return ms.balances[addr].Copy(), nil
	}

	return nil, types.ErrAccountNotFound
}

func (ms *MockStateManager) GetBalance(addr types.Address, assetID types.AssetID) (*big.Int, error) {
	if _, ok := ms.balances[addr]; ok {
		if _, ok := ms.balances[addr].Balances[assetID]; ok {
			return ms.balances[addr].Balances[assetID], nil
		}

		return nil, types.ErrAssetNotFound
	}

	return nil, types.ErrAccountNotFound
}

func (ms *MockStateManager) GetLatestNonce(addr types.Address) (uint64, error) {
	if _, ok := ms.accounts[addr]; ok {
		return ms.accounts[addr].Nonce, nil
	}

	return 0, types.ErrAccountNotFound
}

func (ms *MockStateManager) IsGenesis(addr types.Address) (bool, error) {
	if _, ok := ms.storage[types.GetHash(addr.Bytes())]; ok {
		return true, nil
	}

	return false, nil
}

func (ms *MockStateManager) setBalance(addr types.Address, assetID types.AssetID, balance *big.Int) {
	ms.balances[addr] = &gtypes.BalanceObject{
		Balances: make(types.AssetMap),
	}
	ms.balances[addr].Balances[assetID] = balance
}

func (ms *MockStateManager) setContext(t *testing.T, hash types.Hash) {
	t.Helper()

	ms.context[hash] = &Context{
		tests.GetTestKramaIDs(t, 10),
		tests.GetTestKramaIDs(t, 10),
	}
}

func (ms *MockStateManager) getContextNodes(hash types.Hash) []string {
	return append(
		utils.KramaIDToString(ms.context[hash].behaviourNodes),
		utils.KramaIDToString(ms.context[hash].randomNodes)...,
	)
}

func (ms *MockStateManager) setAccounts(addr types.Address, latestNonce uint64) {
	ms.accounts[addr] = &types.Account{
		Nonce: latestNonce,
	}
}

func (ms *MockStateManager) getTDU(addr types.Address) types.AssetMap {
	data, _ := ms.balances[addr].TDU()

	return data
}

func (ms *MockStateManager) setLatestContextHash(addr types.Address, hash types.Hash) {
	ms.latestContextHash[addr] = hash
}

type MockIxPool struct {
	interactions map[types.Hash]*types.Interaction
	nextNonce    map[types.Address]uint64
}

func NewMockIxPool() *MockIxPool {
	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[types.Hash]*types.Interaction)
	ixpool.nextNonce = make(map[types.Address]uint64)

	return ixpool
}

func (mc *MockIxPool) AddInteractions(ixs types.Interactions) []error {
	errs := make([]error, len(ixs))

	mc.interactions[ixs[0].Hash()] = ixs[0]
	mc.nextNonce[ixs[0].Sender()]++

	return errs
}

func (mc *MockIxPool) GetNonce(addr types.Address) (uint64, error) {
	nextNonce := mc.nextNonce[addr]

	return atomic.LoadUint64(&nextNonce), nil
}

func (mc *MockIxPool) setNonce(addr types.Address, nonce uint64) {
	mc.nextNonce[addr] = nonce
}

func NewTestInteraction(ixType types.IxType, callback func(ix *types.InteractionMessage)) *types.Interaction {
	ixMsg := new(types.InteractionMessage)
	ixMsg.Data.Input.Type = ixType

	if callback != nil {
		callback(ixMsg)
	}

	return ixMsg.ToInteraction()
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
