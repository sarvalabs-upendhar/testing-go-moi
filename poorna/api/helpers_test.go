package api

import (
	"math/big"
	"sync/atomic"
	"testing"

	gtypes "gitlab.com/sarvalabs/moichain/guna/types"

	"gitlab.com/sarvalabs/moichain/types"

	"gitlab.com/sarvalabs/moichain/common/tests"
	"gitlab.com/sarvalabs/moichain/guna"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
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
	assets              map[types.Hash]*gtypes.AssetData
}

type MockStateManager struct {
	storage           map[types.Hash][]byte
	balances          map[types.Address]*gtypes.BalanceObject
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
	mockChain.interactions = make(map[types.Address]map[types.Hash]types.Interactions, 0)
	mockChain.receipts = make(map[types.Address]map[types.Hash]*types.Receipt, 0)
	mockChain.assets = make(map[types.Hash]*gtypes.AssetData, 0)
	mockChain.storage = make(map[types.Hash]*types.Tesseract, 0)

	return mockChain
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

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

func (mc *MockChainManager) GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetData, error) {
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

func (mc *MockChainManager) setAssets(id types.AssetID, info *types.AssetInfo) {
	mc.assets[types.BytesToHash(id.GetCID())] = &gtypes.AssetData{
		LogicID: info.LogicID,
		Symbol:  info.Symbol,
		Owner:   types.HexToAddress(info.Owner),
		Extra:   big.NewInt(int64(info.TotalSupply)).Bytes(),
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
	ms.balances[addr] = &gtypes.BalanceObject{
		Bal: make(gtypes.AssetMap),
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

func (ms *MockStateManager) getTDU(addr types.Address) gtypes.AssetMap {
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
