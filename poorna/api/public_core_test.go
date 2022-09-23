package api

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/guna"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/tests"
)

type asset struct {
	Dimension   uint8
	TotalSupply int64
	Symbol      string
	IsFungible  bool
	IsMintable  bool
}

type mockChainManager struct {
	mockStorage  map[ktypes.Hash]*ktypes.Tesseract
	assetStorage map[ktypes.Hash]*ktypes.AssetData
	nonceStorage map[ktypes.Address]uint64
}

func NewMockChainManager(t *testing.T) *mockChainManager {
	t.Helper()

	m := new(mockChainManager)
	m.mockStorage = make(map[ktypes.Hash]*ktypes.Tesseract, 0)
	m.assetStorage = make(map[ktypes.Hash]*ktypes.AssetData, 0)
	m.nonceStorage = make(map[ktypes.Address]uint64)

	return m
}

func NewPublicCoreAPITest(t *testing.T, m *mockChainManager) *PublicCoreAPI {
	t.Helper()

	p := &PublicCoreAPI{
		backend: &Backend{
			chain: m,
			sm:    m,
		},
	}

	return p
}

func (m *mockChainManager) GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error) {
	if val, ok := m.mockStorage[hash]; ok {
		return val, nil
	}

	return nil, errors.New("hash not found")
}

func (m *mockChainManager) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	return nil, nil
}

func (m *mockChainManager) GetReceipt(addr ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error) {
	return nil, nil
}

func (m *mockChainManager) GetTesseractByHeight(address string, height uint64) (*ktypes.Tesseract, error) {
	for k, v := range m.mockStorage {
		if v.Address() == ktypes.HexToAddress(address) && v.Height() == height {
			return m.GetTesseract(k)
		}
	}

	return nil, errors.New("addressheightkey not found")
}

func (m *mockChainManager) GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error) {
	return nil, nil
}

func (m *mockChainManager) GetLatestContext(address ktypes.Address) (ktypes.Hash, []id.KramaID, []id.KramaID, error) {
	return ktypes.HexToHash(""), nil, nil, nil
}

func (m *mockChainManager) GetBalances(addrs ktypes.Address) (*ktypes.BalanceObject, error) {
	return nil, nil
}

func (m *mockChainManager) GetLatestNonce(addr ktypes.Address) (uint64, error) {
	if val, ok := m.nonceStorage[addr]; ok {
		return val, nil
	}

	return 0, errors.New("nonce not found")
}

func TestGetTesseractByHash(t *testing.T) {
	m := NewMockChainManager(t)

	//generate dynamic tesseract and store it
	tesseract1 := tests.GetTestTesseract(t, 1)
	m.mockStorage[tesseract1.Hash()] = tesseract1

	p := NewPublicCoreAPITest(t, m)

	// allTests holds all types of hashes
	allTests := []struct {
		name            string
		testHash        string
		isErrorExpected bool
	}{
		{
			"valid hash",
			tesseract1.Hash().String(),
			false,
		},
		{
			"valid hash without state",
			tests.RandomHash(t).String(),
			true,
		},
		{
			"invalid hash",
			"68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			true,
		},
	}

	for _, test := range allTests {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := p.GetTesseractByHash(test.testHash)

			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedTesseract.Hash().String(), test.testHash)
			}
		})
	}
}

func TestGetTesseractByHeight(t *testing.T) {
	m := NewMockChainManager(t)

	//generate dynamic tesseract and store it
	tesseract1 := tests.GetTestTesseract(t, 1)
	m.mockStorage[tesseract1.Hash()] = tesseract1

	p := NewPublicCoreAPITest(t, m)

	// allTests holds all types of hashes
	allTests := []struct {
		name            string
		address         string
		height          uint64
		expectedHash    string
		isErrorExpected bool
	}{
		{
			"valid address",
			tesseract1.Address().String(),
			tesseract1.Height(),
			tesseract1.Hash().String(),
			false,
		},
		{
			"valid address without state",
			tests.RandomAddress(t).String(),
			22,
			"",
			true,
		},
		{
			"invalid address",
			"68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			8,
			"",
			true,
		},
	}

	for _, test := range allTests {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := p.GetTesseractByHeight(test.address, test.height)
			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedTesseract.Hash().String(), test.expectedHash)
			}
		})
	}
}

func (m *mockChainManager) GetAssetDataByAssetHash(assetHash []byte) (*ktypes.AssetData, error) {
	result, ok := m.assetStorage[ktypes.BytesToHash(assetHash)]
	if !ok {
		return nil, ktypes.ErrFetchingAssetDataInfo
	}

	return result, nil
}
func TestGetAssetInfoByAssetID(t *testing.T) {
	m := NewMockChainManager(t)

	assetInput := []asset{
		{1, 100000, "MOI", true, false},
	}

	randomHash := tests.RandomHash(t)
	owner := tests.RandomAddress(t)
	aID, hash, _ := ktypes.GetAssetID(owner, assetInput[0].Dimension, assetInput[0].IsFungible,
		assetInput[0].IsMintable, assetInput[0].Symbol, assetInput[0].TotalSupply, randomHash)

	m.assetStorage[hash] = &ktypes.AssetData{
		LogicID: randomHash,
		Symbol:  assetInput[0].Symbol,
		Owner:   owner,
		Extra:   big.NewInt(assetInput[0].TotalSupply).Bytes(),
	}

	p := NewPublicCoreAPITest(t, m)

	allTests := []struct {
		name              string
		assetID           string
		expectedAssetInfo *ktypes.AssetInfo
		isErrorExpected   bool
	}{
		{
			"valid assetID",
			string(aID),
			&ktypes.AssetInfo{
				Owner:       owner.String(),
				Dimension:   assetInput[0].Dimension,
				TotalSupply: uint64(assetInput[0].TotalSupply),
				Symbol:      assetInput[0].Symbol,
				IsFungible:  assetInput[0].IsFungible,
				IsMintable:  assetInput[0].IsMintable,
			},
			false,
		},
		{
			"valid asset id without state",
			"01801995a34ceda4db744a5b1363be9a0f2019e7481699c861ad7d1263c95473a2d9",
			&ktypes.AssetInfo{},
			true,
		},
		{
			"invalid asset id",
			"01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			&ktypes.AssetInfo{},
			true,
		},
	}

	for _, test := range allTests {
		t.Run(test.name, func(t *testing.T) {
			fetchedAssetInfo, err := p.GetAssetInfoByAssetID(test.assetID)

			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedAssetInfo, test.expectedAssetInfo)
			}
		})
	}
}

func TestGetTransactionCountByAddress(t *testing.T) {
	m := NewMockChainManager(t)

	address := tests.RandomAddress(t)
	m.nonceStorage[address] = 0

	p := NewPublicCoreAPITest(t, m)

	// allTests holds all types of hashes
	allTests := []struct {
		name            string
		address         string
		status          bool
		expectedNonce   uint64
		isErrorExpected bool
	}{
		{
			"valid address",
			address.Hex(),
			false,
			0,
			false,
		},
		{
			"valid address without state",
			tests.RandomAddress(t).String(),
			false,
			0,
			true,
		},
		{
			"invalid address",
			"68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			false,
			0,
			true,
		},
	}

	for _, test := range allTests {
		t.Run(test.name, func(t *testing.T) {
			fetchedNonce, err := p.GetTransactionCountByAddress(test.address, true)

			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, fetchedNonce, test.expectedNonce)
			}
		})
	}
}
