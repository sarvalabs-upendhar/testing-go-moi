package api

import (
	"errors"
	"math/big"
	"sync/atomic"
	"testing"

	"gitlab.com/sarvalabs/moichain/types"

	"gitlab.com/sarvalabs/moichain/common/tests"
	"gitlab.com/sarvalabs/moichain/guna"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type AssetInfo struct {
	AssetID    types.AssetID
	Asset      *types.AssetInfo
	Hash       types.Hash
	RandomHash types.Hash
}

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type MockChainManager struct {
	tesseracts          map[types.Address]map[types.Hash]*types.Tesseract
	latestTesseractHash map[types.Address]types.Hash
	receipts            map[types.Address]map[types.Hash]*types.Receipt
	storage             map[types.Hash]*types.Tesseract
	assets              map[types.Hash]*types.AssetData
}

type MockStateManager struct {
	storage           map[types.Hash][]byte
	balances          map[types.Address]*types.BalanceObject
	accounts          map[types.Address]*types.Account
	context           map[types.Hash]*Context
	latestContextHash map[types.Address]types.Hash
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
	mockChain.receipts = make(map[types.Address]map[types.Hash]*types.Receipt, 0)
	mockChain.assets = make(map[types.Hash]*types.AssetData, 0)
	mockChain.storage = make(map[types.Hash]*types.Tesseract, 0)

	return mockChain
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	mockState := new(MockStateManager)

	mockState.balances = make(map[types.Address]*types.BalanceObject)
	mockState.latestContextHash = make(map[types.Address]types.Hash)
	mockState.storage = make(map[types.Hash][]byte)
	mockState.accounts = make(map[types.Address]*types.Account)
	mockState.context = make(map[types.Hash]*Context)

	return mockState
}

// Chain manager mock functions
func (mc *MockChainManager) GetLatestTesseract(addr types.Address) (*types.Tesseract, error) {
	if _, ok := mc.tesseracts[addr]; ok {
		return mc.tesseracts[addr][mc.latestTesseractHash[addr]], nil
	}

	return nil, types.ErrAccountNotFound
}

func (mc *MockChainManager) GetTesseract(hash types.Hash) (*types.Tesseract, error) {
	for _, tesseracts := range mc.tesseracts {
		for tsHash, tesseract := range tesseracts {
			if tsHash == hash {
				return tesseract, nil
			}
		}
	}

	return nil, errors.New("tesseract not found")
}

func (mc *MockChainManager) GetReceipt(addr types.Address, ixHash types.Hash) (*types.Receipt, error) {
	if receipt := mc.receipts[addr][ixHash]; receipt != nil {
		return receipt, nil
	}

	return nil, types.ErrReceiptNotFound
}

func (mc *MockChainManager) GetAssetDataByAssetHash(assetHash []byte) (*types.AssetData, error) {
	if result, ok := mc.assets[types.BytesToHash(assetHash)]; ok {
		return result, nil
	}

	return nil, types.ErrFetchingAssetDataInfo
}

func (mc *MockChainManager) GetTesseractByHeight(address types.Address, height uint64) (*types.Tesseract, error) {
	for _, tesseract := range mc.storage {
		if tesseract.Address() == address && tesseract.Height() == height {
			return tesseract, nil
		}
	}

	return nil, errors.New("address height key not found")
}

func (mc *MockChainManager) setTesseracts(
	addr types.Address,
	latestTesseractHash types.Hash,
	tesseracts map[types.Hash]*types.Tesseract,
) {
	mc.latestTesseractHash[addr] = latestTesseractHash
	mc.tesseracts[addr] = tesseracts
}

func (mc *MockChainManager) setReceipt(addr types.Address, receipts map[types.Hash]*types.Receipt) {
	mc.receipts[addr] = receipts
}

func (mc *MockChainManager) setStorage(hash types.Hash, tesseract *types.Tesseract) {
	mc.storage[hash] = tesseract
}

func (mc *MockChainManager) setAssets(addr types.Address, assetInfo *AssetInfo) {
	assetInfo.Asset.Owner = addr.String()
	mc.assets[assetInfo.Hash] = &types.AssetData{
		LogicID: assetInfo.RandomHash,
		Symbol:  assetInfo.Asset.Symbol,
		Owner:   addr,
		Extra:   big.NewInt(int64(assetInfo.Asset.TotalSupply)).Bytes(),
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

func (ms *MockStateManager) GetBalances(addr types.Address) (*types.BalanceObject, error) {
	if _, ok := ms.balances[addr]; ok {
		return ms.balances[addr].Copy(), nil
	}

	return nil, types.ErrAccountNotFound
}

func (ms *MockStateManager) GetBalance(addr types.Address, assetID types.AssetID) (*big.Int, error) {
	if _, ok := ms.balances[addr]; ok {
		if _, ok := ms.balances[addr].Bal[assetID]; ok {
			return ms.balances[addr].Bal[assetID], nil
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
	ms.balances[addr] = &types.BalanceObject{
		Bal: make(types.AssetMap),
	}
	ms.balances[addr].Bal[assetID] = balance
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
		types.KIPPeerIDToString(ms.context[hash].behaviourNodes),
		types.KIPPeerIDToString(ms.context[hash].randomNodes)...,
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

func NewIxPool() *MockIxPool {
	ixpool := new(MockIxPool)
	ixpool.interactions = make(map[types.Hash]*types.Interaction)
	ixpool.nextNonce = make(map[types.Address]uint64)

	return ixpool
}

func (mc *MockIxPool) AddInteractions(ixs types.Interactions) []error {
	errs := make([]error, len(ixs))

	mc.interactions[ixs[0].Hash] = ixs[0]
	mc.nextNonce[ixs[0].FromAddress()]++

	return errs
}

func (mc *MockIxPool) GetNonce(addr types.Address) (uint64, error) {
	nextNonce := mc.nextNonce[addr]

	return atomic.LoadUint64(&nextNonce), nil
}

func (mc *MockIxPool) setNonce(addr types.Address, nonce uint64) {
	mc.nextNonce[addr] = nonce
}
