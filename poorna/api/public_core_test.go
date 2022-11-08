package api

import (
	"errors"
	"log"
	"math/big"
	"testing"

	"gitlab.com/sarvalabs/moichain/guna"

	"github.com/stretchr/testify/require"

	"gitlab.com/sarvalabs/moichain/common/tests"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/types"
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
	balances          map[types.Address]*types.BalanceObject
	accounts          map[types.Address]*types.Account
	context           map[types.Hash]*Context
	latestContextHash map[types.Address]types.Hash
}

func (ms *MockStateManager) GetLatestStateObject(addr types.Address) (*guna.StateObject, error) {
	// TODO implement me
	panic("implement me")
}

var ErrTesseractNotFound = errors.New("tesseract not found")

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

	return nil, ErrTesseractNotFound
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

func (mc *MockChainManager) GetTesseractByHeight(address string, height uint64) (*types.Tesseract, error) {
	for _, tesseract := range mc.storage {
		if tesseract.Address() == types.HexToAddress(address) && tesseract.Height() == height {
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
		} else {
			return types.NilHash, nil, nil, types.ErrAccountNotFound
		}
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

// Core Api Testcases
func TestPublicCoreAPI_GetBalance(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	assetInfo := getAssetInfo(t, address)

	stateManager.setBalance(address, assetInfo.AssetID, big.NewInt(300))

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        BalArgs
		expected    *big.Int
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: BalArgs{
				From:    "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				AssetID: tests.RandomAssetID(t, address),
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: BalArgs{
				From:    tests.RandomAddress(t).String(),
				AssetID: string(assetInfo.AssetID),
			},
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name: "Invalid asset Id",
			args: BalArgs{
				From:    address.String(),
				AssetID: "01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			},
			expectedErr: types.ErrInvalidAssetID,
		},
		{
			name: "Valid asset id without state",
			args: BalArgs{
				From:    address.String(),
				AssetID: tests.RandomAssetID(t, address),
			},
			expectedErr: types.ErrAssetNotFound,
		},
		{
			name: "Valid address and asset id",
			args: BalArgs{
				From:    address.String(),
				AssetID: string(assetInfo.AssetID),
			},
			expected: big.NewInt(300),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(testing *testing.T) {
			balance, err := coreAPI.GetBalance(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected, balance)
			}
		})
	}
}

func TestPublicCoreAPI_GetLatestTesseract(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := new(MockStateManager)
	_, latestTesseractHash, tesseracts := getTesseracts(t, address)

	chainManager.setTesseracts(address, latestTesseractHash, tesseracts)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        TesseractArgs
		expected    *types.Tesseract
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: TesseractArgs{
				From: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: TesseractArgs{
				From: tests.RandomAddress(t).String(),
			},
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name: "Valid address",
			args: TesseractArgs{
				From: address.String(),
			},
			expected: tesseracts[latestTesseractHash],
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(testing *testing.T) {
			tesseract, err := coreAPI.GetLatestTesseract(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected, tesseract)
			}
		})
	}
}

func TestPublicCoreAPI_GetContextInfo(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	latestContextHash, _, _ := getTesseracts(t, address)

	stateManager.setLatestContextHash(address, latestContextHash)
	stateManager.setContext(t, latestContextHash)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        ContextInfoByHashArgs
		expected    []string
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: ContextInfoByHashArgs{
				From: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				Hash: "68510188a8Bff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Invalid hash",
			args: ContextInfoByHashArgs{
				From: address.Hex(),
				Hash: "68510188Z8Bff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: types.ErrInvalidHash,
		},
		{
			name: "Address without state",
			args: ContextInfoByHashArgs{
				From: tests.RandomAddress(t).Hex(),
				Hash: "",
			},
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name: "Valid Address and valid hash",
			args: ContextInfoByHashArgs{
				From: address.Hex(),
				Hash: latestContextHash.Hex(),
			},
			expected: stateManager.getContextNodes(latestContextHash),
		},
		{
			name: "Valid Address and empty hash",
			args: ContextInfoByHashArgs{
				From: address.Hex(),
				Hash: "",
			},
			expected: stateManager.getContextNodes(latestContextHash),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(testing *testing.T) {
			behaviour, observer, err := coreAPI.GetContextInfoByHash(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, append(behaviour, observer...), testcase.expected)
			}
		})
	}
}

func TestPublicCoreAPI_GetTDU(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	assetInfo := getAssetInfo(t, address)

	stateManager.setBalance(address, assetInfo.AssetID, big.NewInt(300))

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        TesseractArgs
		expected    types.AssetMap
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: TesseractArgs{
				From: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: TesseractArgs{
				From: tests.RandomAddress(t).String(),
			},
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name: "Valid address",
			args: TesseractArgs{
				From: address.String(),
			},
			expected: stateManager.getTDU(address),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(testing *testing.T) {
			data, err := coreAPI.GetTDU(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected, data)
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionReceipt(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := new(MockStateManager)
	validReceiptHash, receipts := getReceipts(t)

	chainManager.setReceipt(address, receipts)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        ReceiptArgs
		expected    *types.Receipt
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: ReceiptArgs{
				Address: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				Hash:    tests.RandomHash(t).String(),
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Invalid hash",
			args: ReceiptArgs{
				Address: address.String(),
				Hash:    tests.RandomHash(t).String(),
			},
			expectedErr: types.ErrReceiptNotFound,
		},
		{
			name: "Valid account and hash",
			args: ReceiptArgs{
				Address: address.String(),
				Hash:    validReceiptHash.String(),
			},
			expected: receipts[validReceiptHash],
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(testing *testing.T) {
			receipt, err := coreAPI.GetInteractionReceipt(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, receipt, testcase.expected)
			}
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHash(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := new(MockStateManager)
	_, latestTesseractHash, tesseracts := getTesseracts(t, address)

	chainManager.setTesseracts(address, latestTesseractHash, tesseracts)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name            string
		args            *TesseractByHashArgs
		isErrorExpected bool
	}{
		{
			"Valid hash",
			&TesseractByHashArgs{
				Hash: tesseracts[latestTesseractHash].Hash().String(),
			},
			false,
		},
		{
			"Valid hash without state",
			&TesseractByHashArgs{
				Hash: tests.RandomHash(t).String(),
			},
			true,
		},
		{
			"Invalid hash",
			&TesseractByHashArgs{
				Hash: "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.GetTesseractByHash(testcase.args)

			if testcase.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedTesseract.Hash().String(), testcase.args.Hash)
			}
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHeight(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	_, latestTesseractHash, tesseracts := getTesseracts(t, address)

	chainManager.setStorage(latestTesseractHash, tesseracts[latestTesseractHash])

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name            string
		args            *TesseractByHeightArgs
		expected        string
		isErrorExpected bool
	}{
		{
			"Valid address",
			&TesseractByHeightArgs{
				tesseracts[latestTesseractHash].Address().String(),
				tesseracts[latestTesseractHash].Height(),
			},
			tesseracts[latestTesseractHash].Hash().String(),
			false,
		},
		{
			"Valid address without state",
			&TesseractByHeightArgs{
				tests.RandomAddress(t).String(),
				22,
			},
			"",
			true,
		},
		{
			"Invalid address",
			&TesseractByHeightArgs{
				"68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				8,
			},
			"",
			true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.GetTesseractByHeight(testcase.args)
			if testcase.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedTesseract.Hash().String(), testcase.expected)
			}
		})
	}
}

func TestPublicCoreAPI_GetAssetInfoByAssetID(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := new(MockStateManager)
	assetInfo := getAssetInfo(t, address)

	chainManager.setAssets(address, assetInfo)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name            string
		args            *AssetInfoArgs
		expected        *types.AssetInfo
		isErrorExpected bool
	}{
		{
			"Valid asset id",
			&AssetInfoArgs{
				AssetID: string(assetInfo.AssetID),
			},
			assetInfo.Asset,
			false,
		},
		{
			"Valid asset id without state",
			&AssetInfoArgs{
				"01801995a34ceda4db744a5b1363be9a0f2019e7481699c861ad7d1263c95473a2d9",
			},
			&types.AssetInfo{},
			true,
		},
		{
			"Invalid asset id",
			&AssetInfoArgs{
				"01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			},
			&types.AssetInfo{},
			true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			fetchedAssetInfo, err := coreAPI.GetAssetInfoByAssetID(testcase.args.AssetID)
			if testcase.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedAssetInfo, testcase.expected)
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionCountByAddress(t *testing.T) {
	address := tests.RandomAddress(t)
	latestNonce := uint64(5)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)

	stateManager.setAccounts(address, latestNonce)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name            string
		args            *InteractionCountArgs
		expected        uint64
		isErrorExpected bool
	}{
		{
			"Valid address",
			&InteractionCountArgs{
				address.String(),
				false,
			},
			latestNonce,
			false,
		},
		{
			"Valid address without state",
			&InteractionCountArgs{
				tests.RandomAddress(t).String(),
				false,
			},
			0,
			true,
		},
		{
			"Invalid address",
			&InteractionCountArgs{
				"68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				false,
			},
			0,
			true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			fetchedNonce, err := coreAPI.GetInteractionCountByAddress(testcase.args)

			if testcase.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedNonce, testcase.expected)
			}
		})
	}
}

// helper function
func newTesseract(t *testing.T, height int, address types.Address) *types.Tesseract {
	t.Helper()

	return &types.Tesseract{
		Header: types.TesseractHeader{
			Address:  address,
			PrevHash: tests.RandomHash(t),
			Height:   uint64(height),
		},
		Body: types.TesseractBody{
			StateHash:       tests.RandomHash(t),
			ContextHash:     tests.RandomHash(t),
			InteractionHash: tests.RandomHash(t),
		},
	}
}

func newReceipt(interactionHash types.Hash) *types.Receipt {
	return &types.Receipt{
		IxType: 1,
		IxHash: interactionHash,
	}
}

func getTesseracts(t *testing.T, address types.Address) (types.Hash, types.Hash, map[types.Hash]*types.Tesseract) {
	t.Helper()

	tesseracts := make(map[types.Hash]*types.Tesseract)
	tesseract := newTesseract(t, 1, address)
	contextHash := tesseract.ContextHash()
	tesseractHash := tesseract.Hash()
	tesseracts[tesseractHash] = tesseract

	return contextHash, tesseractHash, tesseracts
}

func getReceipts(t *testing.T) (types.Hash, map[types.Hash]*types.Receipt) {
	t.Helper()

	receipts := make(map[types.Hash]*types.Receipt)
	interactionHash := tests.RandomHash(t)
	receipts[interactionHash] = newReceipt(interactionHash)

	return interactionHash, receipts
}

func getAssetInfo(t *testing.T, address types.Address) *AssetInfo {
	t.Helper()

	randomHash := tests.RandomHash(t)

	asset, err := tests.GetAsset(t)
	if err != nil {
		log.Panic("Failed to create asset")
	}

	assetID, hash, _ := types.GetAssetID(
		address,
		asset.Dimension,
		asset.IsFungible,
		asset.IsMintable,
		asset.Symbol,
		int64(asset.TotalSupply),
		randomHash)

	return &AssetInfo{
		assetID,
		asset,
		hash,
		randomHash,
	}
}
