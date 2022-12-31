package api

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
)

// Core Api Testcases
func TestPublicCoreAPI_GetBalance(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager()
	assetID, _ := tests.CreateTestAsset(t, address)

	stateManager.setBalance(address, assetID, big.NewInt(300))

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
				AssetID: "",
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: BalArgs{
				From:    tests.RandomAddress(t).String(),
				AssetID: string(assetID),
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
				AssetID: string(tests.GetRandomAssetID(t, address)),
			},
			expectedErr: types.ErrAssetNotFound,
		},
		{
			name: "Valid address and asset id",
			args: BalArgs{
				From:    address.String(),
				AssetID: string(assetID),
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
	interactions := getInteractions(t, tesseracts)
	latestTesseract := tesseracts[latestTesseractHash]
	latestTesseractWithIxns := &types.Tesseract{
		Header: latestTesseract.Header,
		Body:   latestTesseract.Body,
		Ixns:   interactions[latestTesseract.InteractionHash()],
	}

	chainManager.setTesseracts(address, latestTesseractHash, tesseracts)
	chainManager.setInteractions(address, interactions)

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
				From:             "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				WithInteractions: false,
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Account without state",
			args: TesseractArgs{
				From:             tests.RandomAddress(t).String(),
				WithInteractions: false,
			},
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name: "Valid address",
			args: TesseractArgs{
				From:             address.String(),
				WithInteractions: false,
			},
			expected: latestTesseract,
		},
		{
			name: "Tesseract with interactions",
			args: TesseractArgs{
				From:             address.String(),
				WithInteractions: true,
			},
			expected: latestTesseractWithIxns,
		},
		{
			name: "Tesseract without interactions",
			args: TesseractArgs{
				From:             address.String(),
				WithInteractions: false,
			},
			expected: latestTesseract,
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
	stateManager := NewMockStateManager()
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
			name: "Valid address and valid hash",
			args: ContextInfoByHashArgs{
				From: address.Hex(),
				Hash: latestContextHash.Hex(),
			},
			expected: stateManager.getContextNodes(latestContextHash),
		},
		{
			name: "Valid address and empty hash",
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
	stateManager := NewMockStateManager()
	assetID, _ := tests.CreateTestAsset(t, address)

	stateManager.setBalance(address, assetID, big.NewInt(300))

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
	interactions := getInteractions(t, tesseracts)
	latestTesseract := tesseracts[latestTesseractHash]
	latestTesseractWithIxns := &types.Tesseract{
		Header: latestTesseract.Header,
		Body:   latestTesseract.Body,
		Ixns:   interactions[latestTesseract.InteractionHash()],
	}

	chainManager.setTesseracts(address, latestTesseractHash, tesseracts)
	chainManager.setInteractions(address, interactions)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        *TesseractByHashArgs
		expected    *types.Tesseract
		expectedErr error
	}{
		{
			name: "Valid hash",
			args: &TesseractByHashArgs{
				Hash:             getTesseractHash(t, latestTesseract).String(),
				WithInteractions: false,
			},
			expected: latestTesseract,
		},
		{
			name: "Valid hash without state",
			args: &TesseractByHashArgs{
				Hash:             tests.RandomHash(t).String(),
				WithInteractions: false,
			},
			expectedErr: types.ErrKeyNotFound,
		},
		{
			name: "Invalid hash",
			args: &TesseractByHashArgs{
				Hash:             "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				WithInteractions: false,
			},
			expectedErr: types.ErrInvalidHash,
		},
		{
			name: "Tesseract with interactions",
			args: &TesseractByHashArgs{
				Hash:             getTesseractHash(t, latestTesseractWithIxns).String(),
				WithInteractions: true,
			},
			expected: latestTesseractWithIxns,
		},
		{
			name: "Tesseract without interactions",
			args: &TesseractByHashArgs{
				Hash:             getTesseractHash(t, latestTesseract).String(),
				WithInteractions: false,
			},
			expected: latestTesseract,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.GetTesseractByHash(testcase.args)

			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected, fetchedTesseract)
			}
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHeight(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager()
	_, latestTesseractHash, tesseracts := getTesseracts(t, address)
	interactions := getInteractions(t, tesseracts)
	latestTesseract := tesseracts[latestTesseractHash]
	latestTesseractWithIxns := &types.Tesseract{
		Header: latestTesseract.Header,
		Body:   latestTesseract.Body,
		Ixns:   interactions[latestTesseract.InteractionHash()],
	}

	chainManager.setStorage(latestTesseractHash, tesseracts[latestTesseractHash])
	chainManager.setInteractions(address, interactions)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name        string
		args        *TesseractByHeightArgs
		expected    *types.Tesseract
		expectedErr error
	}{
		{
			name: "Valid address",
			args: &TesseractByHeightArgs{
				From:             latestTesseract.Address().String(),
				Height:           latestTesseract.Height(),
				WithInteractions: false,
			},
			expected: latestTesseract,
		},
		{
			name: "Valid address without state",
			args: &TesseractByHeightArgs{
				From:             tests.RandomAddress(t).String(),
				Height:           22,
				WithInteractions: false,
			},
			expectedErr: types.ErrKeyNotFound,
		},
		{
			name: "Invalid address",
			args: &TesseractByHeightArgs{
				From:             "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				Height:           8,
				WithInteractions: false,
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Tesseract with interactions",
			args: &TesseractByHeightArgs{
				From:             latestTesseract.Address().String(),
				Height:           latestTesseract.Height(),
				WithInteractions: true,
			},
			expected: latestTesseractWithIxns,
		},
		{
			name: "Tesseract without interactions",
			args: &TesseractByHeightArgs{
				From:             latestTesseract.Address().String(),
				Height:           latestTesseract.Height(),
				WithInteractions: false,
			},
			expected: latestTesseract,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.GetTesseractByHeight(testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected, fetchedTesseract)
			}
		})
	}
}

func TestPublicCoreAPI_GetAssetInfoByAssetID(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	stateManager := new(MockStateManager)
	assetID, assetInfo := tests.CreateTestAsset(t, address)

	chainManager.setAssets(assetID, assetInfo)

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name            string
		args            *AssetDescriptorArgs
		expected        *types.AssetDescriptor
		isErrorExpected bool
	}{
		{
			"Valid asset id",
			&AssetDescriptorArgs{
				AssetID: string(assetID),
			},
			assetInfo,
			false,
		},
		{
			"Valid asset id without state",
			&AssetDescriptorArgs{
				"01801995a34ceda4db744a5b1363be9a0f2019e7481699c861ad7d1263c95473a2d9",
			},
			nil,
			true,
		},
		{
			"Invalid asset id",
			&AssetDescriptorArgs{
				"01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			},
			nil,
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
				require.Equal(t, testcase.expected, fetchedAssetInfo)
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionCountByAddress(t *testing.T) {
	address := tests.RandomAddress(t)
	latestNonce := uint64(5)
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager()

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

func TestPublicCoreAPI_GetLogicManifest(t *testing.T) {
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager()
	logicID := types.LogicID(tests.RandomHash(t).String())

	stateManager.setLogicManifest(logicID.Hex(), []byte{0x00, 0x01})

	coreAPI := NewPublicCoreAPI(chainManager, stateManager)

	testcases := []struct {
		name            string
		args            *GetLogicManifestArgs
		expected        []byte
		isErrorExpected bool
	}{
		{
			name: "Valid logic id without state",
			args: &GetLogicManifestArgs{
				LogicID: types.LogicID(tests.RandomHash(t).String()).Hex(),
			},
			isErrorExpected: true,
		},
		{
			name: "Valid logic id",
			args: &GetLogicManifestArgs{
				LogicID: logicID.Hex(),
			},
			expected:        []byte{0x00, 0x01},
			isErrorExpected: false,
		},
		// TODO: test case for invalid logic id
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			manifest, err := coreAPI.GetLogicManifest(testcase.args)

			if testcase.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, manifest, testcase.expected)
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

func newInteraction(t *testing.T) types.Interactions {
	t.Helper()

	return types.Interactions{
		types.NewRandomHashInteraction(),
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
	tesseractHash := getTesseractHash(t, tesseract)
	tesseracts[tesseractHash] = tesseract

	return contextHash, tesseractHash, tesseracts
}

func getInteractions(t *testing.T, tesseracts map[types.Hash]*types.Tesseract) map[types.Hash]types.Interactions {
	t.Helper()

	interactions := make(map[types.Hash]types.Interactions)

	for _, tesseract := range tesseracts {
		interactions[tesseract.InteractionHash()] = newInteraction(t)
	}

	return interactions
}

func getReceipts(t *testing.T) (types.Hash, map[types.Hash]*types.Receipt) {
	t.Helper()

	receipts := make(map[types.Hash]*types.Receipt)
	interactionHash := tests.RandomHash(t)
	receipts[interactionHash] = newReceipt(interactionHash)

	return interactionHash, receipts
}

func getTesseractHash(t *testing.T, tesseract *types.Tesseract) types.Hash {
	t.Helper()

	tsHash, err := tesseract.Hash()
	require.NoError(t, err)

	return tsHash
}
