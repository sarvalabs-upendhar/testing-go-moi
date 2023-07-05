//nolint:thelper
package moiclient

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

type StrMap map[string]types.Address

// url of the node to be used by moiclient.
const url = "http://0.0.0.0:1601"

// Need to run minimum 20 fresh nodes for this test to run successfully
// Ensure chain is set up to run individual tests
// TestMoiClient tests moi client functions unless short flag is provided
func TestMoiClient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	addrsMap := make(StrMap)

	client, err := NewClient(url)
	require.NoError(t, err)

	accs, err := client.Accounts()
	require.NoError(t, err)

	addrs, err := SetupAddrs(accs)
	require.NoError(t, err)

	setupChain(t, client, addrs, addrsMap)

	testcases := map[string]struct {
		test func(t *testing.T)
	}{
		"Tesseract": {
			test: func(t *testing.T) { testTesseract(t, client, addrsMap["deployAddr"]) },
		},
		"DBGet": {
			test: func(t *testing.T) { testDBGet(t, client) },
		},
		"GetAssetInfoByAssetID": {
			test: func(t *testing.T) { testGetAssetInfoByAssetID(t, client, addrsMap["assetAddr"]) },
		},
		"GetBalance": {
			test: func(t *testing.T) { testGetBalance(t, client, addrsMap) },
		},
		"TDU": {
			test: func(t *testing.T) { testTDU(t, client, addrsMap["assetAddr"]) },
		},
		"GetContextInfo": {
			test: func(t *testing.T) { testGetContextInfo(t, client, addrsMap["deployAddr"]) },
		},
		"InteractionReceipt": {
			test: func(t *testing.T) { testInteractionReceipt(t, client, addrsMap["assetAddr"]) },
		},
		"InteractionCount": {
			test: func(t *testing.T) { testInteractionCount(t, client, addrsMap["assetAddr"]) },
		},
		"PendingInteractionCount": {
			test: func(t *testing.T) { testPendingInteractionCount(t, client, addrsMap["assetAddr"]) },
		},
		"Storage": {
			test: func(t *testing.T) { testStorage(t, client, addrsMap["deployAddr"]) },
		},
		"AccountState": {
			test: func(t *testing.T) { testAccountState(t, client, addrsMap["deployAddr"]) },
		},
		"LogicIDs": {
			test: func(t *testing.T) { testLogics(t, client, addrsMap["deployAddr"]) },
		},
		"LogicManifest": {
			test: func(t *testing.T) { testLogicManifest(t, client, addrsMap["deployAddr"]) },
		},
		"LogicCall": {
			test: func(t *testing.T) { testLogicCall(t, client, addrsMap["deployAddr"]) },
		},
		"Content": {
			test: func(t *testing.T) { testContent(t, client) },
		},
		"ContentFrom": {
			test: func(t *testing.T) { testContentFrom(t, client, addrsMap["dumpAddr"]) },
		},
		"Status": {
			test: func(t *testing.T) { testStatus(t, client) },
		},
		"Inspect": {
			test: func(t *testing.T) { testInspect(t, client) },
		},
		"WaitTime": {
			test: func(t *testing.T) { testWaitTime(t, client, addrsMap["dumpAddr"]) },
		},
		"Peers": {
			test: func(t *testing.T) { testPeers(t, client) },
		},
		"Version": {
			test: func(t *testing.T) { testVersion(t, client) },
		},
		"Info": {
			test: func(t *testing.T) { testInfo(t, client) },
		},
		"SendInteraction": {
			test: func(t *testing.T) { testSendInteraction(t, client) },
		},
		"ixByHash": {
			test: func(t *testing.T) { testInteractionByHash(t, client, addrsMap["deployAddr"]) },
		},
		"ixByTesseract": {
			test: func(t *testing.T) { testInteractionByTesseract(t, client, addrsMap["deployAddr"]) },
		},
		"testAccounts": {
			test: func(t *testing.T) { testAccounts(t, client) },
		},
		"testAccountMetaInfo": {
			test: func(t *testing.T) { testAccountMetaInfo(t, client) },
		},
		"testFuelDeduction": {
			test: func(t *testing.T) { testFuelDeduction(t, client, addrsMap) },
		},
	}

	t.Parallel()

	for name, testcase := range testcases {
		t.Run(name, testcase.test)
	}
}

// chooseAcc chooses an account which is neither logic nor asset, as we don't have mnemonic for those accounts
func chooseAcc(
	t *testing.T,
	client *Client,
	i int,
	addrs []types.Address,
	accs []tests.AccountWithMnemonic,
) (int, tests.AccountWithMnemonic) {
	for j := i; j < len(addrs); j++ {
		acc, exists := GetMnemonicFromAccounts(addrs[j], accs)
		if exists {
			return j, acc
		}

		accMetaInfo, err := client.AccountMetaInfo(&ptypes.GetAccountArgs{
			Address: addrs[j],
		})
		require.NoError(t, err)

		require.True(t, (accMetaInfo.Type == types.LogicAccount) || (accMetaInfo.Type == types.AssetAccount))
	}

	require.Error(t, errors.New("insufficient accounts on chain"))

	return 0, tests.AccountWithMnemonic{}
}

// setupChain runs these functions in order as tests use heights for identifying the event happened
// Every function is an interaction, and we only proceed to the next one after receiving the receipt for the current one
// We are testing interaction execution and receipt generation for every function
// More verification of interaction execution (like fields) will be done in the following table tests
// senderAddr account :
// At height 1, logic is deployed
// At height 2, asset is created
// At height 3, tokens are transferred
// Logic Address account :
// At height 1, transfer call is made to contract by senderAddr
func setupChain(t *testing.T, client *Client, addrs []types.Address, addrsMap StrMap) {
	var (
		i   = 0
		acc tests.AccountWithMnemonic
	)

	accs, err := tests.GetAccountMnemonicsFromFile("../accounts.json")
	require.NoError(t, err)

	t.Run("DeployLogic", func(t *testing.T) {
		i, acc = chooseAcc(t, client, i, addrs, accs)

		t.Log(addrs[i])

		deployLogic(t, client, addrs[i], acc.Mnemonic)
		addrsMap["deployAddr"] = addrs[i]
	})
	i++

	t.Run("ExecuteLogic", func(t *testing.T) {
		i, acc = chooseAcc(t, client, i, addrs, accs)

		t.Log(addrs[i])

		executeLogic(t, client, addrsMap["deployAddr"], addrs[i], acc.Mnemonic)
		addrsMap["executeAddr"] = addrs[i]
	})
	i++

	t.Run("CreateAsset", func(t *testing.T) {
		i, acc = chooseAcc(t, client, i, addrs, accs)

		t.Log(addrs[i])

		createAsset(t, client, addrs[i], acc.Mnemonic)
		addrsMap["assetAddr"] = addrs[i]
	})
	i++

	t.Run("TransferTokens", func(t *testing.T) {
		acc, exists := GetMnemonicFromAccounts(addrsMap["assetAddr"], accs)
		require.True(t, exists)

		t.Log(addrs[i])

		transferTokens(t, client, addrsMap["assetAddr"], addrs[i], acc.Mnemonic)
		addrsMap["receiverAddr"] = addrs[i]
	})
	i++

	t.Run("Send IX invalid signature", func(t *testing.T) {
		i, acc = chooseAcc(t, client, i, addrs, accs)

		t.Log(addrsMap["deployAddr"])

		SendIxWithInvalidSign(t, client, addrsMap["deployAddr"], acc.Mnemonic)
	})

	i += 2

	t.Run("IXPoolAPI", func(t *testing.T) {
		i, acc = chooseAcc(t, client, i, addrs, accs)

		t.Log(addrs[i])

		fillIXPool(t, client, addrs[i], acc.Mnemonic)
		addrsMap["dumpAddr"] = addrs[i]
	})
}

// deployLogic deploys logic manifest
func deployLogic(t *testing.T, client *Client, addr types.Address, mnemonic string) {
	sendIXArgs := getIXArgsForLogicDeployment(t, addr)
	sendIX := createSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(sendIX)
	require.NoError(t, err)

	log.Println("Logic Deploy Interaction hash", ixHash)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	retryFetchReceipt(t, ctx, client, ixHash)
}

// executeLogic executes the transfer method on deployed logic
func executeLogic(t *testing.T, client *Client, deployAddr, addr types.Address, mnemonic string) {
	calldata := "0x0daf010665a601e501f6059506616d6f756e74030f424066726f6d06ffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58" +
		"052237d521654740857716b34746f060fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"

	logicPayload := &types.LogicPayload{
		Logic:    getLogicID(t, client, deployAddr, &deployLogicHeight),
		Callsite: "Transfer!",
		Calldata: types.Hex2Bytes(calldata),
	}

	payload, err := polo.Polorize(logicPayload)
	require.NoError(t, err)

	sendIXArgs := &types.SendIXArgs{
		Type:      types.IxLogicInvoke,
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(200),
		Sender:    addr,
		Payload:   payload,
	}

	sendIX := createSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(sendIX)
	require.NoError(t, err)

	log.Println("Hello", ixHash)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	retryFetchReceipt(t, ctx, client, ixHash)
}

// createAsset creates asset named "MOI"
func createAsset(t *testing.T, client *Client, addr types.Address, mnemonic string) {
	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &types.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	sendIXArgs := &types.SendIXArgs{
		Type:      types.IxAssetCreate,
		Sender:    addr,
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(200),
		Payload:   payload,
	}

	sendIX := createSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(sendIX)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	retryFetchReceipt(t, ctx, client, ixHash)
}

// transferTokens transfers tokens from senderAddr to receiver
func transferTokens(t *testing.T, client *Client, sender, receiver types.Address, mnemonic string) {
	assetID := getAssetID(t, client, sender, &createAssetHeight)

	sendIXArgs := &types.SendIXArgs{
		Type:     types.IxValueTransfer,
		Nonce:    1,
		Sender:   sender,
		Receiver: receiver,
		TransferValues: map[types.AssetID]*big.Int{
			assetID: big.NewInt(16),
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(200),
	}

	sendIX := createSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(sendIX)
	require.NoError(t, err)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	retryFetchReceipt(t, ctx, client, ixHash)
}

// SendIxWithInvalidSign sends ix with invalid sign
func SendIxWithInvalidSign(t *testing.T, client *Client, addr types.Address, mnemonic string) {
	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &types.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	sendIXArgs := &types.SendIXArgs{
		Type:      types.IxAssetCreate,
		Nonce:     1,
		Sender:    addr,
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(100),
		Payload:   payload,
	}

	sendIX := createSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	_, err = client.SendInteractions(sendIX)
	require.ErrorContains(t, err, types.ErrInvalidIXSignature.Error())
}

// fillIXPool sends ixnPendingCount number of deploy interactions
func fillIXPool(t *testing.T, client *Client, addr types.Address, mnemonic string) {
	sendIXArgs := getIXArgsForLogicDeployment(t, addr)
	sendIXArgs.Nonce = uint64(0)
	increment := uint64(1)

	for i := 0; i < ixnPendingCount; i++ { // send ixns just to fill ixpool with some data
		sendIX := createSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

		_, err := client.SendInteractions(sendIX)
		require.NoError(t, err, "sending interaction failed")

		sendIXArgs.Nonce += increment // increment nonce to avoid ix already known error
	}
}

func testTesseract(t *testing.T, client *Client, addr types.Address) {
	invalidHeight := int64(100000)

	testcases := []struct {
		name          string
		tesseractArgs *ptypes.TesseractArgs
		expectedError error
	}{
		{
			name: "fetch tesseract with interactions",
			tesseractArgs: &ptypes.TesseractArgs{
				Address:          addr,
				WithInteractions: true,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &deployLogicHeight,
				},
			},
		},
		{
			name: "fetch tesseract without interactions",
			tesseractArgs: &ptypes.TesseractArgs{
				Address:          addr,
				WithInteractions: false,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &deployLogicHeight,
				},
			},
		},
		{
			name: "fetch genesis tesseract",
			tesseractArgs: &ptypes.TesseractArgs{
				Address:          addr,
				WithInteractions: false,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &genesisHeight,
				},
			},
		},
		{
			name: "invalid tesseract number",
			tesseractArgs: &ptypes.TesseractArgs{
				Address:          addr,
				WithInteractions: false,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("failed to fetch tesseract height entry"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := client.Tesseract(test.tesseractArgs)

			t.Log(addr)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpTS := httpTesseract(t, test.tesseractArgs)

			require.Equal(t, httpTS, ts)
			require.Equal(t, addr, ts.Address())

			if test.tesseractArgs.WithInteractions {
				require.Greater(t, len(ts.Ixns), 0)
				// require.Equal(t, types.IxLogicDeploy, ts.Ixns[0].Type)

				return
			}

			require.Equal(t, 0, len(ts.Ixns))
		})
	}
}

func testDBGet(t *testing.T, client *Client) {
	// key and value belongs to genesis tesseract account meta info
	key, _ := dhruva.BucketKeyAndID(types.SargaAddress)

	testcases := []struct {
		name          string
		debugArgs     *ptypes.DebugArgs
		expectedError error
	}{
		{
			name: "fetch value for existing key in db",
			debugArgs: &ptypes.DebugArgs{
				Key: types.BytesToHex(key),
			},
		},
		{
			name: "fetch value for non-existing key in db",
			debugArgs: &ptypes.DebugArgs{
				Key: "822c978f24933d17d4a6d8e40459c30ba9ba12d4d958ab2dc80d1720e39fa73ae5",
			},
			expectedError: types.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := client.DBGet(test.debugArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpValue := httpDBGet(t, test.debugArgs)
			require.Equal(t, httpValue, value)

			accMetaInfo := new(types.AccountMetaInfo)
			require.NoError(t, accMetaInfo.FromBytes(types.Hex2Bytes(httpValue)))
			require.Equal(t, types.SargaAddress, accMetaInfo.Address)
			require.Equal(t, types.SargaAccount, accMetaInfo.Type)
		})
	}
}

func testGetAssetInfoByAssetID(t *testing.T, client *Client, addr types.Address) {
	tsHeight := int64(-1)
	testcases := []struct {
		name          string
		assetID       types.AssetID
		expectedError error
	}{
		{
			name:    "fetch asset info for existing assetID",
			assetID: getAssetID(t, client, addr, &createAssetHeight),
		},
		{
			name:          "fetch asset info for non-existing assetID",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: types.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			args := &ptypes.GetAssetInfoArgs{
				AssetID: test.assetID,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &tsHeight,
				},
			}
			assetInfo, err := client.AssetInfoByAssetID(args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpAssetInfo := httpGetAssetInfoByAssetID(t, args)
			require.Equal(t, *httpAssetInfo, *assetInfo)
		})
	}
}

func testGetBalance(t *testing.T, client *Client, addrsMap StrMap) {
	receiverTokenTransferHeight := int64(1)
	sender := addrsMap["assetAddr"]
	receiver := addrsMap["receiverAddr"]
	assetID := getAssetID(t, client, sender, &createAssetHeight)

	createAssetBalance := new(big.Int).SetUint64(1248577)
	senderTransferTokenBalance := new(big.Int).SetUint64(1248561)
	receiverTransferTokenBalance := new(big.Int).SetUint64(16)

	testcases := []struct {
		name            string
		balanceArgs     *ptypes.BalArgs
		expectedBalance *hexutil.Big
		expectedError   error
	}{
		{
			name: "fetch senderAddr balance at create asset height",
			balanceArgs: &ptypes.BalArgs{
				Address: sender,
				AssetID: assetID,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
			expectedBalance: (*hexutil.Big)(createAssetBalance),
		},
		{
			name: "fetch sender balance at transfer token height",
			balanceArgs: &ptypes.BalArgs{
				Address: sender,
				AssetID: assetID,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &transferTokensHeight,
				},
			},
			expectedBalance: (*hexutil.Big)(senderTransferTokenBalance),
		},
		{
			name: "fetch receiver balance at receiver transfer token height",
			balanceArgs: &ptypes.BalArgs{
				Address: receiver,
				AssetID: assetID,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &receiverTokenTransferHeight,
				},
			},
			expectedBalance: (*hexutil.Big)(receiverTransferTokenBalance),
		},
		{
			name: "get balance returns error for unknown asset ID",
			balanceArgs: &ptypes.BalArgs{
				Address: sender,
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("asset not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := client.Balance(test.balanceArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, balance, test.expectedBalance)

			httpBalance := httpGetBalance(t, test.balanceArgs)
			require.Equal(t, httpBalance, balance)
		})
	}
}

func testTDU(t *testing.T, client *Client, addr types.Address) {
	assetID := getAssetID(t, client, addr, &createAssetHeight)
	sortTDU := func(tdu []ptypes.TDU) {
		sort.Slice(tdu, func(i, j int) bool {
			return tdu[i].AssetID < tdu[j].AssetID
		})
	}

	testcases := []struct {
		name          string
		queryArgs     *ptypes.QueryArgs
		expectedError error
	}{
		{
			name: "fetch TDU for existing address",
			queryArgs: &ptypes.QueryArgs{
				Address: addr,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
		},
		{
			name: "fetch TDU for non-existing address",
			queryArgs: &ptypes.QueryArgs{
				Address: tests.RandomAddress(t),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tdu, err := client.TDU(test.queryArgs)

			t.Log(addr)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			isAssetFound := false

			for i := 0; i < len(tdu); i++ {
				if assetID == tdu[i].AssetID {
					isAssetFound = true
				}
			}

			require.True(t, isAssetFound)
			require.Equal(t, 2, len(tdu))

			httpTDU := httpTDU(t, test.queryArgs)

			sortTDU(httpTDU)
			sortTDU(tdu)

			require.Equal(t, httpTDU, tdu)
		})
	}
}

func testGetContextInfo(t *testing.T, client *Client, addr types.Address) {
	testcases := []struct {
		name            string
		contextInfoArgs *ptypes.ContextInfoArgs
		expectedError   error
	}{
		{
			name: "fetch context info for existing address",
			contextInfoArgs: &ptypes.ContextInfoArgs{
				Address: addr,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch context info for non-existing address",
			contextInfoArgs: &ptypes.ContextInfoArgs{
				Address: tests.RandomAddress(t),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contextInfo, err := client.ContextInfo(test.contextInfoArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.GreaterOrEqual(t, len(contextInfo.BehaviourNodes), 1)

			httpContextInfo := httpGetContextInfo(t, test.contextInfoArgs)
			require.Equal(t, *httpContextInfo, *contextInfo)
		})
	}
}

func testInteractionReceipt(t *testing.T, client *Client, addr types.Address) {
	ts := getTesseract(t, client, addr, &executeLogicHeight)

	testcases := []struct {
		name          string
		receiptArgs   *ptypes.ReceiptArgs
		expectedError error
	}{
		{
			name: "fetch receipt for existing hash",
			receiptArgs: &ptypes.ReceiptArgs{
				Hash: ts.Ixns[0].Hash,
			},
		},
		{
			name: "fetch receipt for non-existing hash",
			receiptArgs: &ptypes.ReceiptArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: types.ErrGridHashNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := client.InteractionReceipt(test.receiptArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, ts.Ixns[0].Hash, receipt.IxHash)

			httpReceipt := httpInteractionReceipt(t, test.receiptArgs)
			checkForRPCReceipt(t, httpReceipt, receipt)
		})
	}
}

func testInteractionCount(t *testing.T, client *Client, addr types.Address) {
	testcases := []struct {
		name                 string
		interactionCountArgs *ptypes.InteractionCountArgs
		expectedError        error
	}{
		{
			name: "fetch interaction count for existing address",
			interactionCountArgs: &ptypes.InteractionCountArgs{
				Address: addr,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &transferTokensHeight,
				},
			},
		},
		{
			name: "fetch interaction count for non-existing address",
			interactionCountArgs: &ptypes.InteractionCountArgs{
				Address: tests.RandomAddress(t),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			interactionCount, err := client.InteractionCount(test.interactionCountArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.GreaterOrEqual(t, interactionCount.ToUint64(), uint64(1))

			httpInteractionCount := httpInteractionCount(t, test.interactionCountArgs)
			require.Equal(t, httpInteractionCount, interactionCount)
		})
	}
}

func testPendingInteractionCount(t *testing.T, client *Client, addr types.Address) {
	testcases := []struct {
		name                 string
		interactionCountArgs *ptypes.InteractionCountArgs
		expectedError        error
	}{
		{
			name: "fetch pending interaction count for non-existing address",
			interactionCountArgs: &ptypes.InteractionCountArgs{
				Address: tests.RandomAddress(t),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
		{
			name: "fetch pending interaction count for existing address",
			interactionCountArgs: &ptypes.InteractionCountArgs{
				Address: addr,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			pendingInteractionCount, err := client.PendingInteractionCount(test.interactionCountArgs)

			t.Log(addr)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, pendingInteractionCount.ToUint64(), uint64(2))

			httpPendingInteractionCount := httpPendingInteractionCount(t, test.interactionCountArgs)
			require.Equal(t, httpPendingInteractionCount, pendingInteractionCount)
		})
	}
}

func testStorage(t *testing.T, client *Client, addr types.Address) {
	logicID := getLogicID(t, client, addr, &deployLogicHeight)

	testcases := []struct {
		name                 string
		interactionCountArgs *ptypes.GetStorageArgs
		expectedError        error
	}{
		{
			name: "fetch storage value for existing logic ID",
			interactionCountArgs: &ptypes.GetStorageArgs{
				LogicID:    logicID,
				StorageKey: types.Hex2Bytes("e88bd757ad5b9bedf372d8d3f0cf6c962a469db61a265f6418e1ffed86da29ec"),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch storage value for non-existing logic ID",
			interactionCountArgs: &ptypes.GetStorageArgs{
				LogicID:    "",
				StorageKey: types.Hex2Bytes("e88bd757ad5b9bedf372d8d3f0cf6c962a469db61a265f6418e1ffed86da29ec"),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("invalid logic ID"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			storageValue, err := client.Storage(test.interactionCountArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpStorageValue := httpStorage(t, test.interactionCountArgs)
			require.Equal(t, httpStorageValue, storageValue)
		})
	}
}

func testAccountState(t *testing.T, client *Client, addr types.Address) {
	testcases := []struct {
		name          string
		accountArgs   *ptypes.GetAccountArgs
		expectedError error
	}{
		{
			name: "fetch account state for existing address",
			accountArgs: &ptypes.GetAccountArgs{
				Address: addr,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
		},
		{
			name: "fetch account state for non-existing address",
			accountArgs: &ptypes.GetAccountArgs{
				Address: tests.RandomAddress(t),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountState, err := client.AccountState(test.accountArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			t.Log(addr)
			require.NoError(t, err)
			require.GreaterOrEqual(t, accountState.Nonce.ToUint64(), uint64(1))

			httpAccountState := httpAccountState(t, test.accountArgs)
			require.Equal(t, *httpAccountState, *accountState)
		})
	}
}

func testLogics(t *testing.T, client *Client, addr types.Address) {
	logicID := getLogicID(t, client, addr, &deployLogicHeight)

	testcases := []struct {
		name             string
		LogicIDArgs      *ptypes.GetLogicIDArgs
		expectedLogicIDs []types.LogicID
		expectedError    error
	}{
		{
			name: "fetch logicIDs for existing address",
			LogicIDArgs: &ptypes.GetLogicIDArgs{
				Address: logicID.Address(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
			expectedLogicIDs: []types.LogicID{logicID},
		},
		{
			name: "fetch logicIDs for non-existing address",
			LogicIDArgs: &ptypes.GetLogicIDArgs{
				Address: tests.RandomAddress(t),
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicIDs, err := client.LogicIDs(test.LogicIDArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpLogicIDs := httpLogicIDs(t, test.LogicIDArgs)
			require.Equal(t, httpLogicIDs, logicIDs)

			require.Equal(t, test.expectedLogicIDs, logicIDs)
		})
	}
}

func testLogicManifest(t *testing.T, client *Client, addr types.Address) {
	logicID := getLogicID(t, client, addr, &deployLogicHeight)
	ts := getTesseract(t, client, addr, &deployLogicHeight)

	var logic *ptypes.RPCLogicPayload

	err := json.Unmarshal(ts.Ixns[0].Payload, &logic)
	require.NoError(t, err)

	testcases := []struct {
		name              string
		logicManifestArgs *ptypes.LogicManifestArgs
		expectedError     error
	}{
		{
			name: "fetch json logic manifest for existing logicID",
			logicManifestArgs: &ptypes.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "JSON",
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch polo logic manifest for existing logicID",
			logicManifestArgs: &ptypes.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "POLO",
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch yaml logic manifest for existing logicID",
			logicManifestArgs: &ptypes.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "YAML",
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch logic manifest for non-existing logicID",
			logicManifestArgs: &ptypes.LogicManifestArgs{
				LogicID:  "0200000070c34ed6ec4384c75d469894052647a078b33ac0f08db0d3751c1fce29a49f",
				Encoding: "JSON",
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicManifest, err := client.LogicManifest(test.logicManifestArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			manifest, err := getLogicManifestByEncodingType(t, logic.Manifest, test.logicManifestArgs)
			require.NoError(t, err)

			require.Equal(t, manifest, logicManifest)

			httpLogicManifest := httpLogicManifest(t, test.logicManifestArgs)
			require.Equal(t, httpLogicManifest, logicManifest)
		})
	}
}

func testLogicCall(t *testing.T, client *Client, addr types.Address) {
	logicID := getLogicID(t, client, addr, &deployLogicHeight)
	calldata := "0x0daf010665a601e501f6059506616d6f756e74030f424066726f6d06ffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58" +
		"052237d521654740857716b34746f060fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"

	testcases := []struct {
		name              string
		logicCallArgs     *ptypes.LogicCallArgs
		expectedLogicCall *ptypes.LogicCallResult
		expectedError     error
	}{
		{
			name: "fetched logic call result successfully",
			logicCallArgs: &ptypes.LogicCallArgs{
				Invoker:  logicID.Address(),
				LogicID:  logicID,
				Callsite: "Transfer!",
				Calldata: types.Hex2Bytes(calldata),
			},
		},
		{
			name: "fetch logic call result for non-existing address and logicID",
			logicCallArgs: &ptypes.LogicCallArgs{
				Invoker:  tests.RandomAddress(t),
				LogicID:  "0200000070c34ed6ec4384c75d469894052647a078b33ac0f08db0d3751c1fce29a49f",
				Callsite: "Transfer!",
				Calldata: types.Hex2Bytes(calldata),
			},
			expectedError: types.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicCall, err := client.LogicCall(test.logicCallArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpLogicCall := httpLogicCall(t, test.logicCallArgs)
			require.Equal(t, httpLogicCall, logicCall)
		})
	}
}

func testContent(t *testing.T, client *Client) {
	testcases := []struct {
		name          string
		contentArgs   *ptypes.ContentArgs
		expectedError error
	}{
		{
			name:        "fetch content from ixpool",
			contentArgs: &ptypes.ContentArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contentResponse, err := client.Content(test.contentArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Greater(t, len(contentResponse.Pending), 0)

			httpContent := httpContent(t, test.contentArgs)
			require.Equal(t, *httpContent, *contentResponse)
		})
	}
}

func testContentFrom(t *testing.T, client *Client, addr types.Address) {
	testcases := []struct {
		name          string
		ixPoolArgs    *ptypes.IxPoolArgs
		expectedCount int
	}{
		{
			name: "fetch content from for existing address",
			ixPoolArgs: &ptypes.IxPoolArgs{
				Address: addr,
			},
			expectedCount: 1,
		},
		{
			name: "fetch  content from for non-existing address",
			ixPoolArgs: &ptypes.IxPoolArgs{
				Address: tests.RandomAddress(t),
			},
			expectedCount: 0,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contentFromResponse, err := client.ContentFrom(test.ixPoolArgs)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(contentFromResponse.Pending), test.expectedCount)

			httpContentFrom := httpContentFrom(t, test.ixPoolArgs)
			require.Equal(t, *httpContentFrom, *contentFromResponse)
		})
	}
}

func testStatus(t *testing.T, client *Client) {
	testcases := []struct {
		name       string
		ixPoolArgs *ptypes.StatusArgs
	}{
		{
			name:       "fetch status of ixpool",
			ixPoolArgs: &ptypes.StatusArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			statusResponse, err := client.Status(test.ixPoolArgs)
			require.NoError(t, err)
			require.GreaterOrEqual(t, statusResponse.Pending.ToUint64(), uint64(0))

			httpStatus := httpStatus(t, test.ixPoolArgs)
			require.Equal(t, *httpStatus, *statusResponse)
		})
	}
}

func testInspect(t *testing.T, client *Client) {
	testcases := []struct {
		name          string
		inspectArgs   *ptypes.InspectArgs
		expectedError error
	}{
		{
			name:        "inspect ixpool",
			inspectArgs: &ptypes.InspectArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			inspectResponse, err := client.Inspect(test.inspectArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.GreaterOrEqual(t, len(inspectResponse.Pending), 0)

			httpInspectResponse := httpInspect(t, test.inspectArgs)
			require.Equal(t, httpInspectResponse.Pending, inspectResponse.Pending)
		})
	}
}

func testInteractionByHash(t *testing.T, client *Client, addr types.Address) {
	ts := getTesseract(t, client, addr, &deployLogicHeight)

	testcases := []struct {
		name          string
		ixArgs        *ptypes.InteractionByHashArgs
		expectedError error
	}{
		{
			name: "fetch interaction for existing ix hash",
			ixArgs: &ptypes.InteractionByHashArgs{
				Hash: ts.Ixns[0].Hash,
			},
		},
		{
			name: "fetch interaction for non-existing ix hash",
			ixArgs: &ptypes.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: types.ErrFetchingInteraction,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := client.InteractionByHash(test.ixArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, addr, rpcIxn.Sender)

			httpIXResponse := httpInteractionByHash(t, test.ixArgs)
			require.Equal(t, httpIXResponse, *rpcIxn)
		})
	}
}

func testInteractionByTesseract(t *testing.T, client *Client, addr types.Address) {
	ts := getTesseract(t, client, addr, &deployLogicHeight)
	randomHash := tests.RandomHash(t)
	ixIndex := uint64(0)

	testcases := []struct {
		name          string
		ixArgs        *ptypes.InteractionByTesseract
		expectedError error
	}{
		{
			name: "fetch interaction for existing tesseract hash",
			ixArgs: &ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &ts.Ixns[0].Parts[0].Hash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
		},
		{
			name: "fetch interaction for non-existing tesseract hash",
			ixArgs: &ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: errors.New("interaction not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := client.InteractionByTesseract(test.ixArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, addr, rpcIxn.Sender)

			httpIXResponse := httpInteractionByTesseract(t, test.ixArgs)
			require.Equal(t, httpIXResponse, *rpcIxn)
		})
	}
}

func testWaitTime(t *testing.T, client *Client, addr types.Address) {
	testcases := []struct {
		name          string
		ixPoolArgs    *ptypes.IxPoolArgs
		expectedError error
	}{
		{
			name: "fetch wait time for existing address",
			ixPoolArgs: &ptypes.IxPoolArgs{
				Address: addr,
			},
		},
		{
			name: "fetch wait time for non-existing address",
			ixPoolArgs: &ptypes.IxPoolArgs{
				Address: tests.RandomAddress(t),
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.WaitTime(test.ixPoolArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// Avoid comparison between client wait time and http wait time as it changes rapidly
			httpWaitTime(t, test.ixPoolArgs)
		})
	}
}

func testPeers(t *testing.T, client *Client) {
	testcases := []struct {
		name    string
		netArgs *ptypes.NetArgs
	}{
		{
			name:    "fetch peers",
			netArgs: &ptypes.NetArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			clientPeers, err := client.Peers(test.netArgs)
			require.NoError(t, err)

			// check if client peers and http peers are same
			httpPeers := httpPeers(t, test.netArgs)
			for _, id := range *httpPeers {
				require.True(t, utils.ContainsKramaID(clientPeers, id))
			}
		})
	}
}

func testVersion(t *testing.T, client *Client) {
	testcases := []struct {
		name          string
		netArgs       *ptypes.NetArgs
		expectedValue string
	}{
		{
			name:          "fetch version",
			netArgs:       &ptypes.NetArgs{},
			expectedValue: common.ProtocolVersion,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			version, err := client.Version(test.netArgs)
			require.NoError(t, err)

			// check if client peers and http peers are same
			httpVersion := httpVersion(t, test.netArgs)
			require.Equal(t, httpVersion, version)
		})
	}
}

func testInfo(t *testing.T, client *Client) {
	testcases := []struct {
		name    string
		netArgs *ptypes.NetArgs
	}{
		{
			name:    "fetch krama id",
			netArgs: &ptypes.NetArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nodeInfo, err := client.Info(test.netArgs)
			require.NoError(t, err)

			httpNodeInfo := httpInfo(t, test.netArgs)
			require.Equal(t, httpNodeInfo.KramaID, nodeInfo.KramaID)
		})
	}
}

func testSendInteraction(t *testing.T, client *Client) {
	testcases := []struct {
		name          string
		ixPoolArgs    *types.SendIXArgs
		expectedError error
	}{
		{
			name: "invalid account",
			ixPoolArgs: &types.SendIXArgs{
				Type:      types.IxValueTransfer,
				Sender:    tests.RandomAddress(t),
				FuelPrice: big.NewInt(1),
				FuelLimit: big.NewInt(200),
			},
			expectedError: types.ErrInsufficientFunds,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			bz, err := polo.Polorize(test.ixPoolArgs)
			require.NoError(t, err)

			sendIx := &ptypes.SendIX{
				IXArgs:    hex.EncodeToString(bz),
				Signature: "",
			}

			_, err = client.SendInteractions(sendIx)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func testAccounts(t *testing.T, client *Client) {
	testcases := []struct {
		name    string
		accArgs *ptypes.AccountArgs
	}{
		{
			name:    "fetch accounts",
			accArgs: &ptypes.AccountArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountsResponse, err := client.Accounts()
			require.NoError(t, err)

			httpAccounts := httpAccounts(t, test.accArgs)
			require.Equal(t, httpAccounts, accountsResponse)

			for _, account := range accountsResponse {
				if account == types.SargaAddress {
					return
				}
			}

			require.FailNow(t, "sarga address not found in list of accounts")
		})
	}
}

func testAccountMetaInfo(t *testing.T, client *Client) {
	testcases := []struct {
		name          string
		accArgs       *ptypes.GetAccountArgs
		expectedError error
	}{
		{
			name: "fetch account meta info for sarga address",
			accArgs: &ptypes.GetAccountArgs{
				Address: types.SargaAddress,
			},
		},
		{
			name: "fetch account meta info for random address",
			accArgs: &ptypes.GetAccountArgs{
				Address: tests.RandomAddress(t),
			},
			expectedError: types.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountMetaInfoResponse, err := client.AccountMetaInfo(test.accArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpAccountMetaInfo := httpAccountMetaInfo(t, test.accArgs)
			require.Equal(t, httpAccountMetaInfo, accountMetaInfoResponse)
			require.Equal(t, types.SargaAddress, accountMetaInfoResponse.Address)
			require.Equal(t, types.SargaAccount, accountMetaInfoResponse.Type)
		})
	}
}

func testFuelDeduction(t *testing.T, client *Client, addrs map[string]types.Address) {
	testcases := []struct {
		name    string
		address types.Address
		height  int64
	}{
		{"deploy logic", addrs["deployAddr"], deployLogicHeight},
		{"value transfer", addrs["assetAddr"], transferTokensHeight},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// Get tesseract for the height
			ts := getTesseract(t, client, test.address, &test.height)
			// Get the interaction receipt
			receipt, err := client.InteractionReceipt(&ptypes.ReceiptArgs{
				Hash: ts.Ixns[0].Hash,
			})
			require.NoError(t, err)

			// Determine the expected deduction of MOI Token Balance
			var deducted *big.Int
			// If there are MOI Tokens in the transferred values, add that to the total deductions
			// else, only fuel consumed on the receipt is the expected deduction
			if transferValue, ok := ts.Ixns[0].TransferValues[types.KMOITokenAssetID]; !ok {
				deducted = receipt.FuelUsed.ToInt()
			} else {
				deducted = new(big.Int).Add(receipt.FuelUsed.ToInt(), transferValue.ToInt())
			}

			// Determine balance of MOI Tokens BEFORE the interaction
			preHeight := test.height - 1
			preBalance, err := client.Balance(&ptypes.BalArgs{
				Address: test.address,
				AssetID: types.KMOITokenAssetID,
				Options: ptypes.TesseractNumberOrHash{TesseractNumber: &preHeight},
			})
			require.NoError(t, err)

			// Determine balance of MOI Tokens AFTER the interaction
			postBalance, err := client.Balance(&ptypes.BalArgs{
				Address: test.address,
				AssetID: types.KMOITokenAssetID,
				Options: ptypes.TesseractNumberOrHash{TesseractNumber: &test.height},
			})
			require.NoError(t, err)

			// Verify that post balance == (pre balance - deducted fuel - transfer values of MOI)
			require.Zero(t, postBalance.ToInt().Cmp(new(big.Int).Sub(preBalance.ToInt(), deducted)))
		})
	}
}
