package api

import (
	"errors"
	"log"
	"math/big"
	"testing"

	"gitlab.com/sarvalabs/moichain/guna"

	"github.com/stretchr/testify/require"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/tests"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type AssetInfo struct {
	AssetID    ktypes.AssetID
	Asset      *ktypes.AssetInfo
	Hash       ktypes.Hash
	RandomHash ktypes.Hash
}

type Context struct {
	behaviourNodes []id.KramaID
	randomNodes    []id.KramaID
}

type MockChainManager struct {
	tesseracts          map[ktypes.Address]map[ktypes.Hash]*ktypes.Tesseract
	latestTesseractHash map[ktypes.Address]ktypes.Hash
	receipts            map[ktypes.Address]map[ktypes.Hash]*ktypes.Receipt
	storage             map[ktypes.Hash]*ktypes.Tesseract
	assets              map[ktypes.Hash]*ktypes.AssetData
}

type MockStateManager struct {
	balances          map[ktypes.Address]*ktypes.BalanceObject
	accounts          map[ktypes.Address]*ktypes.Account
	context           map[ktypes.Hash]*Context
	latestContextHash map[ktypes.Address]ktypes.Hash
}

func (ms *MockStateManager) GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error) {
	//TODO implement me
	panic("implement me")
}

var (
	ErrTesseractNotFound = errors.New("tesseract not found")
)

func NewMockChainManager(t *testing.T) *MockChainManager {
	t.Helper()

	var mockChain = new(MockChainManager)

	mockChain.tesseracts = make(map[ktypes.Address]map[ktypes.Hash]*ktypes.Tesseract, 0)
	mockChain.latestTesseractHash = make(map[ktypes.Address]ktypes.Hash)
	mockChain.receipts = make(map[ktypes.Address]map[ktypes.Hash]*ktypes.Receipt, 0)
	mockChain.assets = make(map[ktypes.Hash]*ktypes.AssetData, 0)
	mockChain.storage = make(map[ktypes.Hash]*ktypes.Tesseract, 0)

	return mockChain
}

func NewMockStateManager(t *testing.T) *MockStateManager {
	t.Helper()

	var mockState = new(MockStateManager)

	mockState.balances = make(map[ktypes.Address]*ktypes.BalanceObject)
	mockState.latestContextHash = make(map[ktypes.Address]ktypes.Hash)
	mockState.accounts = make(map[ktypes.Address]*ktypes.Account)
	mockState.context = make(map[ktypes.Hash]*Context)

	return mockState
}

// Chain manager mock functions
func (mc *MockChainManager) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	if _, ok := mc.tesseracts[addr]; ok {
		return mc.tesseracts[addr][mc.latestTesseractHash[addr]], nil
	}

	return nil, ktypes.ErrAccountNotFound
}

func (mc *MockChainManager) GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error) {
	for _, tesseracts := range mc.tesseracts {
		for tsHash, tesseract := range tesseracts {
			if tsHash == hash {
				return tesseract, nil
			}
		}
	}

	return nil, ErrTesseractNotFound
}

func (mc *MockChainManager) GetReceipt(addr ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error) {
	if receipt := mc.receipts[addr][ixHash]; receipt != nil {
		return receipt, nil
	}

	return nil, ktypes.ErrReceiptNotFound
}

func (mc *MockChainManager) GetAssetDataByAssetHash(assetHash []byte) (*ktypes.AssetData, error) {
	if result, ok := mc.assets[ktypes.BytesToHash(assetHash)]; ok {
		return result, nil
	}

	return nil, ktypes.ErrFetchingAssetDataInfo
}

func (mc *MockChainManager) GetTesseractByHeight(address string, height uint64) (*ktypes.Tesseract, error) {
	for _, tesseract := range mc.storage {
		if tesseract.Address() == ktypes.HexToAddress(address) && tesseract.Height() == height {
			return tesseract, nil
		}
	}

	return nil, errors.New("address height key not found")
}

func (mc *MockChainManager) setTesseracts(
	addr ktypes.Address,
	latestTesseractHash ktypes.Hash,
	tesseracts map[ktypes.Hash]*ktypes.Tesseract) {
	mc.latestTesseractHash[addr] = latestTesseractHash
	mc.tesseracts[addr] = tesseracts
}

func (mc *MockChainManager) setReceipt(addr ktypes.Address, receipts map[ktypes.Hash]*ktypes.Receipt) {
	mc.receipts[addr] = receipts
}

func (mc *MockChainManager) setStorage(hash ktypes.Hash, tesseract *ktypes.Tesseract) {
	mc.storage[hash] = tesseract
}

func (mc *MockChainManager) setAssets(addr ktypes.Address, assetInfo *AssetInfo) {
	assetInfo.Asset.Owner = addr.String()
	mc.assets[assetInfo.Hash] = &ktypes.AssetData{
		LogicID: assetInfo.RandomHash,
		Symbol:  assetInfo.Asset.Symbol,
		Owner:   addr,
		Extra:   big.NewInt(int64(assetInfo.Asset.TotalSupply)).Bytes(),
	}
}

// State Manager mock functions

func (ms *MockStateManager) GetContextByHash(address ktypes.Address,
	hash ktypes.Hash) (ktypes.Hash, []id.KramaID, []id.KramaID, error) {
	if hash == ktypes.NilHash {
		if hash, ok := ms.latestContextHash[address]; ok {
			return hash, ms.context[hash].behaviourNodes, ms.context[hash].randomNodes, nil
		} else {
			return ktypes.NilHash, nil, nil, ktypes.ErrAccountNotFound
		}
	}

	return hash, ms.context[hash].behaviourNodes, ms.context[hash].randomNodes, nil
}

func (ms *MockStateManager) GetBalances(addr ktypes.Address) (*ktypes.BalanceObject, error) {
	if _, ok := ms.balances[addr]; ok {
		return ms.balances[addr].Copy(), nil
	}

	return nil, ktypes.ErrAccountNotFound
}

func (ms *MockStateManager) GetBalance(addr ktypes.Address, assetID ktypes.AssetID) (*big.Int, error) {
	if _, ok := ms.balances[addr]; ok {
		if _, ok := ms.balances[addr].Bal[assetID]; ok {
			return ms.balances[addr].Bal[assetID], nil
		}

		return nil, ktypes.ErrAssetNotFound
	}

	return nil, ktypes.ErrAccountNotFound
}

func (ms *MockStateManager) GetLatestNonce(addr ktypes.Address) (uint64, error) {
	if _, ok := ms.accounts[addr]; ok {
		return ms.accounts[addr].Nonce, nil
	}

	return 0, ktypes.ErrAccountNotFound
}

func (ms *MockStateManager) setBalance(addr ktypes.Address, assetID ktypes.AssetID, balance *big.Int) {
	ms.balances[addr] = &ktypes.BalanceObject{
		Bal: make(ktypes.AssetMap),
	}
	ms.balances[addr].Bal[assetID] = balance
}

func (ms *MockStateManager) setContext(t *testing.T, hash ktypes.Hash) {
	t.Helper()

	ms.context[hash] = &Context{
		tests.GetTestKramaIDs(t, 10),
		tests.GetTestKramaIDs(t, 10),
	}
}

func (ms *MockStateManager) getContextNodes(hash ktypes.Hash) []string {
	return append(
		ktypes.KIPPeerIDToString(ms.context[hash].behaviourNodes),
		ktypes.KIPPeerIDToString(ms.context[hash].randomNodes)...,
	)
}

func (ms *MockStateManager) setAccounts(addr ktypes.Address, latestNonce uint64) {
	ms.accounts[addr] = &ktypes.Account{
		Nonce: latestNonce,
	}
}

func (ms *MockStateManager) getTDU(addr ktypes.Address) ktypes.AssetMap {
	data, _ := ms.balances[addr].TDU()

	return data
}

func (ms *MockStateManager) setLatestContextHash(addr ktypes.Address, hash ktypes.Hash) {
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
			expectedErr: ktypes.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: BalArgs{
				From:    tests.RandomAddress(t).String(),
				AssetID: string(assetInfo.AssetID),
			},
			expectedErr: ktypes.ErrAccountNotFound,
		},
		{
			name: "Invalid asset Id",
			args: BalArgs{
				From:    address.String(),
				AssetID: "01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			},
			expectedErr: ktypes.ErrInvalidAssetID,
		},
		{
			name: "Valid asset id without state",
			args: BalArgs{
				From:    address.String(),
				AssetID: tests.RandomAssetID(t, address),
			},
			expectedErr: ktypes.ErrAssetNotFound,
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
		expected    *ktypes.Tesseract
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: TesseractArgs{
				From: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: ktypes.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: TesseractArgs{
				From: tests.RandomAddress(t).String(),
			},
			expectedErr: ktypes.ErrAccountNotFound,
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
			expectedErr: ktypes.ErrInvalidAddress,
		},
		{
			name: "Invalid hash",
			args: ContextInfoByHashArgs{
				From: address.Hex(),
				Hash: "68510188Z8Bff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: ktypes.ErrInvalidHash,
		},
		{
			name: "Address without state",
			args: ContextInfoByHashArgs{
				From: tests.RandomAddress(t).Hex(),
				Hash: "",
			},
			expectedErr: ktypes.ErrAccountNotFound,
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
		expected    ktypes.AssetMap
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: TesseractArgs{
				From: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedErr: ktypes.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: TesseractArgs{
				From: tests.RandomAddress(t).String(),
			},
			expectedErr: ktypes.ErrAccountNotFound,
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
		expected    *ktypes.Receipt
		expectedErr error
	}{
		{
			name: "Invalid address",
			args: ReceiptArgs{
				Address: "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				Hash:    tests.RandomHash(t).String(),
			},
			expectedErr: ktypes.ErrInvalidAddress,
		},
		{
			name: "Invalid hash",
			args: ReceiptArgs{
				Address: address.String(),
				Hash:    tests.RandomHash(t).String(),
			},
			expectedErr: ktypes.ErrReceiptNotFound,
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
		expected        *ktypes.AssetInfo
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
			&ktypes.AssetInfo{},
			true,
		},
		{
			"Invalid asset id",
			&AssetInfoArgs{
				"01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			},
			&ktypes.AssetInfo{},
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
func newTesseract(t *testing.T, height int, address ktypes.Address) *ktypes.Tesseract {
	t.Helper()

	return &ktypes.Tesseract{
		Header: ktypes.TesseractHeader{
			Address:  address,
			PrevHash: tests.RandomHash(t),
			Height:   uint64(height),
		},
		Body: ktypes.TesseractBody{
			StateHash:       tests.RandomHash(t),
			ContextHash:     tests.RandomHash(t),
			InteractionHash: tests.RandomHash(t),
		},
	}
}

func newReceipt(interactionHash ktypes.Hash) *ktypes.Receipt {
	return &ktypes.Receipt{
		IxType: 1,
		IxHash: interactionHash,
	}
}

func getTesseracts(t *testing.T, address ktypes.Address) (ktypes.Hash, ktypes.Hash, map[ktypes.Hash]*ktypes.Tesseract) {
	t.Helper()

	tesseracts := make(map[ktypes.Hash]*ktypes.Tesseract)
	tesseract := newTesseract(t, 1, address)
	contextHash := tesseract.ContextHash()
	tesseractHash := tesseract.Hash()
	tesseracts[tesseractHash] = tesseract

	return contextHash, tesseractHash, tesseracts
}

func getReceipts(t *testing.T) (ktypes.Hash, map[ktypes.Hash]*ktypes.Receipt) {
	t.Helper()

	receipts := make(map[ktypes.Hash]*ktypes.Receipt)
	interactionHash := tests.RandomHash(t)
	receipts[interactionHash] = newReceipt(interactionHash)

	return interactionHash, receipts
}

func getAssetInfo(t *testing.T, address ktypes.Address) *AssetInfo {
	t.Helper()

	randomHash := tests.RandomHash(t)
	asset, err := tests.GetAsset(t)

	if err != nil {
		log.Panic("Failed to create asset")
	}

	assetID, hash, _ := ktypes.GetAssetID(
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
