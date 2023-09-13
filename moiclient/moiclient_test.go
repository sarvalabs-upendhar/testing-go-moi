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

	"github.com/sarvalabs/go-polo"

	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/storage"
)

type StrMap map[string]common.Address

// url of the node to be used by moiclient.
const localURL = "http://0.0.0.0:1601"

// Need to run minimum 20 fresh nodes for this test to run successfully
// Ensure chain is set up to run individual tests
// TestMoiClient tests moi client functions unless short flag is provided
func TestMoiClient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	addrsMap := make(StrMap)

	client, err := NewClient(localURL)
	require.NoError(t, err)

	accs, err := client.Accounts(ctx)
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
		"FuelEstimate": {
			test: func(t *testing.T) { testFuelEstimate(t, client, addrsMap["assetAddr"]) },
		},
		"Syncing": {
			test: func(t *testing.T) { testSyncing(t, client, addrsMap["assetAddr"]) },
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
		"LogicStorage": {
			test: func(t *testing.T) { testLogicStorage(t, client, addrsMap["deployAddr"]) },
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
		"Call": {
			test: func(t *testing.T) { testCall(t, client, addrsMap["assetAddr"]) },
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
		"testConnections": {
			test: func(t *testing.T) { testConnections(t, client) },
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
	addrs []common.Address,
	accs []tests.AccountWithMnemonic,
) (int, tests.AccountWithMnemonic) {
	ctx := context.Background()

	for j := i; j < len(addrs); j++ {
		acc, exists := GetMnemonicFromAccounts(addrs[j], accs)
		if exists {
			return j, acc
		}

		accMetaInfo, err := client.AccountMetaInfo(ctx, &rpcargs.GetAccountArgs{
			Address: addrs[j],
		})
		require.NoError(t, err)

		require.True(t, (accMetaInfo.Type == common.LogicAccount) || (accMetaInfo.Type == common.AssetAccount))
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
func setupChain(t *testing.T, client *Client, addrs []common.Address, addrsMap StrMap) {
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
func deployLogic(t *testing.T, client *Client, addr common.Address, mnemonic string) {
	sendIXArgs := getIXArgsForLogicDeployment(t, client, addr)
	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ctx := context.Background()

	ixHash, err := client.SendInteractions(ctx, sendIX)
	require.NoError(t, err)

	log.Println("Logic Deploy Interaction hash", ixHash)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	moiclientRetryFetchReceipt(t, ctx, client, ixHash)
}

// executeLogic executes the transfer method on deployed logic
func executeLogic(t *testing.T, client *Client, deployAddr, addr common.Address, mnemonic string) {
	calldata := "0x0daf010665a601e501f6059506616d6f756e74030f424066726f6d06ffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58" +
		"052237d521654740857716b34746f060fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"

	logicPayload := &common.LogicPayload{
		Logic:    GetLogicID(t, client, deployAddr, deployLogicHeight),
		Callsite: "Transfer!",
		Calldata: common.Hex2Bytes(calldata),
	}

	payload, err := polo.Polorize(logicPayload)
	require.NoError(t, err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxLogicInvoke,
		Nonce:     GetLatestNonce(t, client, addr),
		FuelPrice: big.NewInt(1),
		FuelLimit: 500,
		Sender:    addr,
		Payload:   payload,
	}

	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)

	log.Println("Hello", ixHash)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	moiclientRetryFetchReceipt(t, ctx, client, ixHash)
}

// createAsset creates asset named "MOI"
func createAsset(t *testing.T, client *Client, addr common.Address, mnemonic string) {
	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Nonce:     GetLatestNonce(t, client, addr),
		Sender:    addr,
		FuelPrice: big.NewInt(1),
		FuelLimit: 200,
		Payload:   payload,
	}

	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	moiclientRetryFetchReceipt(t, ctx, client, ixHash)
}

// transferTokens transfers tokens from senderAddr to receiver
func transferTokens(t *testing.T, client *Client, sender, receiver common.Address, mnemonic string) {
	assetID := getAssetID(t, client, sender, createAssetHeight)

	sendIXArgs := &common.SendIXArgs{
		Type:     common.IxValueTransfer,
		Nonce:    GetLatestNonce(t, client, sender),
		Sender:   sender,
		Receiver: receiver,
		TransferValues: map[common.AssetID]*big.Int{
			assetID: big.NewInt(16),
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 200,
	}

	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	moiclientRetryFetchReceipt(t, ctx, client, ixHash)
}

// SendIxWithInvalidSign sends ix with invalid sign
func SendIxWithInvalidSign(t *testing.T, client *Client, addr common.Address, mnemonic string) {
	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Nonce:     1,
		Sender:    addr,
		FuelPrice: big.NewInt(1),
		FuelLimit: 100,
		Payload:   payload,
	}

	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	_, err = client.SendInteractions(context.Background(), sendIX)
	require.ErrorContains(t, err, common.ErrInvalidIXSignature.Error())
}

// fillIXPool sends ixnPendingCount number of deploy interactions
func fillIXPool(t *testing.T, client *Client, addr common.Address, mnemonic string) {
	sendIXArgs := getIXArgsForLogicDeployment(t, client, addr)
	sendIXArgs.Nonce = uint64(0)
	increment := uint64(1)

	for i := 0; i < ixnPendingCount; i++ { // send ixns just to fill ixpool with some data
		sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

		_, err := client.SendInteractions(context.Background(), sendIX)
		require.NoError(t, err, "sending interaction failed")

		sendIXArgs.Nonce += increment // increment nonce to avoid ix already known error
	}
}

func testTesseract(t *testing.T, client *Client, addr common.Address) {
	invalidHeight := int64(100000)
	ctx := context.Background()
	testcases := []struct {
		name          string
		tesseractArgs *rpcargs.TesseractArgs
		expectedError error
	}{
		{
			name: "fetch tesseract with interactions",
			tesseractArgs: &rpcargs.TesseractArgs{
				Address:          addr,
				WithInteractions: true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &deployLogicHeight,
				},
			},
		},
		{
			name: "fetch tesseract without interactions",
			tesseractArgs: &rpcargs.TesseractArgs{
				Address:          addr,
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &deployLogicHeight,
				},
			},
		},
		{
			name: "fetch genesis tesseract",
			tesseractArgs: &rpcargs.TesseractArgs{
				Address:          addr,
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &genesisHeight,
				},
			},
		},
		{
			name: "invalid tesseract number",
			tesseractArgs: &rpcargs.TesseractArgs{
				Address:          addr,
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("failed to fetch tesseract height entry"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := client.Tesseract(ctx, test.tesseractArgs)

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
	key, _ := storage.BucketKeyAndID(common.SargaAddress)
	ctx := context.Background()
	testcases := []struct {
		name          string
		debugArgs     *rpcargs.DebugArgs
		expectedError error
	}{
		{
			name: "fetch value for existing key in db",
			debugArgs: &rpcargs.DebugArgs{
				Key: common.BytesToHex(key),
			},
		},
		{
			name: "fetch value for non-existing key in db",
			debugArgs: &rpcargs.DebugArgs{
				Key: "822c978f24933d17d4a6d8e40459c30ba9ba12d4d958ab2dc80d1720e39fa73ae5",
			},
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := client.DBGet(ctx, test.debugArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpValue := httpDBGet(t, test.debugArgs)
			require.Equal(t, httpValue, value)

			accMetaInfo := new(common.AccountMetaInfo)
			require.NoError(t, accMetaInfo.FromBytes(common.Hex2Bytes(httpValue)))
			require.Equal(t, common.SargaAddress, accMetaInfo.Address)
			require.Equal(t, common.SargaAccount, accMetaInfo.Type)
		})
	}
}

func testGetAssetInfoByAssetID(t *testing.T, client *Client, addr common.Address) {
	tsHeight := int64(-1)
	ctx := context.Background()
	testcases := []struct {
		name          string
		assetID       common.AssetID
		expectedError error
	}{
		{
			name:    "fetch asset info for existing assetID",
			assetID: getAssetID(t, client, addr, createAssetHeight),
		},
		{
			name:          "fetch asset info for non-existing assetID",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			args := &rpcargs.GetAssetInfoArgs{
				AssetID: test.assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &tsHeight,
				},
			}
			assetInfo, err := client.AssetInfoByAssetID(ctx, args)

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
	assetID := getAssetID(t, client, sender, createAssetHeight)
	ctx := context.Background()

	createAssetBalance := new(big.Int).SetUint64(1248577)
	senderTransferTokenBalance := new(big.Int).SetUint64(1248561)
	receiverTransferTokenBalance := new(big.Int).SetUint64(16)

	testcases := []struct {
		name            string
		balanceArgs     *rpcargs.BalArgs
		expectedBalance *hexutil.Big
		expectedError   error
	}{
		{
			name: "fetch senderAddr balance at create asset height",
			balanceArgs: &rpcargs.BalArgs{
				Address: sender,
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
			expectedBalance: (*hexutil.Big)(createAssetBalance),
		},
		{
			name: "fetch sender balance at transfer token height",
			balanceArgs: &rpcargs.BalArgs{
				Address: sender,
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &transferTokensHeight,
				},
			},
			expectedBalance: (*hexutil.Big)(senderTransferTokenBalance),
		},
		{
			name: "fetch receiver balance at receiver transfer token height",
			balanceArgs: &rpcargs.BalArgs{
				Address: receiver,
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &receiverTokenTransferHeight,
				},
			},
			expectedBalance: (*hexutil.Big)(receiverTransferTokenBalance),
		},
		{
			name: "get balance returns error for unknown asset ID",
			balanceArgs: &rpcargs.BalArgs{
				Address: sender,
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("asset not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := client.Balance(ctx, test.balanceArgs)

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

func testFuelEstimate(t *testing.T, client *Client, addr common.Address) {
	ts := GetTesseract(t, client, addr, createAssetHeight)
	supply, _ := new(big.Int).SetString("130D52", 16)

	ctx := context.Background()

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "ASSETCREATE",
		Supply: supply,
	}

	assetCreatePayload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		callArgs             *rpcargs.CallArgs
		expectedFuelConsumed *hexutil.Big
		expectedError        error
	}{
		{
			name: "retrieved fuel used in asset create interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    ts.Address(),
					Nonce:     hexutil.Uint64(5),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(200),
					Payload:   (hexutil.Bytes)(assetCreatePayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts.Address(): {
						TesseractNumber: &createAssetHeight,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(100)),
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    addr,
					Nonce:     hexutil.Uint64(3),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(assetCreatePayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts.Address(): {
						TesseractNumber: nil,
					},
				},
			},
			expectedError: common.ErrEmptyOptions,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fuelConsumed, err := client.FuelEstimate(ctx, test.callArgs)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, fuelConsumed, test.expectedFuelConsumed)
		})
	}
}

func testSyncing(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		StatusArgs    *rpcargs.SyncStatusRequest
		expectedError error
	}{
		{
			name: "account sync status fetched successfully",
			StatusArgs: &rpcargs.SyncStatusRequest{
				Address: addr,
			},
		},
		{
			name: "should return error if failed to fetch account sync status",
			StatusArgs: &rpcargs.SyncStatusRequest{
				Address: tests.RandomAddress(t),
			},
			expectedError: common.ErrAccSyncStatusNotFound,
		},
		{
			name:       "node sync status fetched successfully",
			StatusArgs: &rpcargs.SyncStatusRequest{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.Syncing(ctx, test.StatusArgs)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func testTDU(t *testing.T, client *Client, addr common.Address) {
	assetID := getAssetID(t, client, addr, createAssetHeight)
	ctx := context.Background()
	sortTDU := func(tdu []rpcargs.TDU) {
		sort.Slice(tdu, func(i, j int) bool {
			return tdu[i].AssetID < tdu[j].AssetID
		})
	}

	testcases := []struct {
		name          string
		queryArgs     *rpcargs.QueryArgs
		expectedError error
	}{
		{
			name: "fetch TDU for existing address",
			queryArgs: &rpcargs.QueryArgs{
				Address: addr,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
		},
		{
			name: "fetch TDU for non-existing address",
			queryArgs: &rpcargs.QueryArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tdu, err := client.TDU(ctx, test.queryArgs)

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

func testGetContextInfo(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name            string
		contextInfoArgs *rpcargs.ContextInfoArgs
		expectedError   error
	}{
		{
			name: "fetch context info for existing address",
			contextInfoArgs: &rpcargs.ContextInfoArgs{
				Address: addr,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch context info for non-existing address",
			contextInfoArgs: &rpcargs.ContextInfoArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contextInfo, err := client.ContextInfo(ctx, test.contextInfoArgs)

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

func testInteractionReceipt(t *testing.T, client *Client, addr common.Address) {
	ts := GetTesseract(t, client, addr, executeLogicHeight)
	ctx := context.Background()
	testcases := []struct {
		name          string
		receiptArgs   *rpcargs.ReceiptArgs
		expectedError error
	}{
		{
			name: "fetch receipt for existing hash",
			receiptArgs: &rpcargs.ReceiptArgs{
				Hash: ts.Ixns[0].Hash,
			},
		},
		{
			name: "fetch receipt for non-existing hash",
			receiptArgs: &rpcargs.ReceiptArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: common.ErrGridHashNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := client.InteractionReceipt(ctx, test.receiptArgs)

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

func testInteractionCount(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name                 string
		interactionCountArgs *rpcargs.InteractionCountArgs
		expectedError        error
	}{
		{
			name: "fetch interaction count for existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: addr,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &transferTokensHeight,
				},
			},
		},
		{
			name: "fetch interaction count for non-existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			interactionCount, err := client.InteractionCount(ctx, test.interactionCountArgs)

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

func testPendingInteractionCount(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name                 string
		interactionCountArgs *rpcargs.InteractionCountArgs
		expectedError        error
	}{
		{
			name: "fetch pending interaction count for non-existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
		{
			name: "fetch pending interaction count for existing address",
			interactionCountArgs: &rpcargs.InteractionCountArgs{
				Address: addr,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			pendingInteractionCount, err := client.PendingInteractionCount(ctx, test.interactionCountArgs)

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

func testLogicStorage(t *testing.T, client *Client, addr common.Address) {
	logicID := getLogicID(t, client, addr, deployLogicHeight)
	ctx := context.Background()
	testcases := []struct {
		name                 string
		interactionCountArgs *rpcargs.GetLogicStorageArgs
		expectedError        error
	}{
		{
			name: "fetch storage value for existing logic ID",
			interactionCountArgs: &rpcargs.GetLogicStorageArgs{
				LogicID:    logicID,
				StorageKey: common.Hex2Bytes("e88bd757ad5b9bedf372d8d3f0cf6c962a469db61a265f6418e1ffed86da29ec"),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch storage value for non-existing logic ID",
			interactionCountArgs: &rpcargs.GetLogicStorageArgs{
				LogicID:    "",
				StorageKey: common.Hex2Bytes("e88bd757ad5b9bedf372d8d3f0cf6c962a469db61a265f6418e1ffed86da29ec"),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("invalid logic ID"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicStorageValue, err := client.LogicStorage(ctx, test.interactionCountArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpLogicStorageValue := httpLogicStorage(t, test.interactionCountArgs)
			require.Equal(t, httpLogicStorageValue, logicStorageValue)
		})
	}
}

func testAccountState(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		accountArgs   *rpcargs.GetAccountArgs
		expectedError error
	}{
		{
			name: "fetch account state for existing address",
			accountArgs: &rpcargs.GetAccountArgs{
				Address: addr,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
		},
		{
			name: "fetch account state for non-existing address",
			accountArgs: &rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountState, err := client.AccountState(ctx, test.accountArgs)

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

func testLogics(t *testing.T, client *Client, addr common.Address) {
	logicID := getLogicID(t, client, addr, deployLogicHeight)
	ctx := context.Background()
	testcases := []struct {
		name             string
		LogicIDArgs      *rpcargs.GetLogicIDArgs
		expectedLogicIDs []common.LogicID
		expectedError    error
	}{
		{
			name: "fetch logicIDs for existing address",
			LogicIDArgs: &rpcargs.GetLogicIDArgs{
				Address: logicID.Address(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &createAssetHeight,
				},
			},
			expectedLogicIDs: []common.LogicID{logicID},
		},
		{
			name: "fetch logicIDs for non-existing address",
			LogicIDArgs: &rpcargs.GetLogicIDArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicIDs, err := client.LogicIDs(ctx, test.LogicIDArgs)

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

func testLogicManifest(t *testing.T, client *Client, addr common.Address) {
	logicID := getLogicID(t, client, addr, deployLogicHeight)
	ts := GetTesseract(t, client, addr, deployLogicHeight)
	ctx := context.Background()

	var logic *rpcargs.RPCLogicPayload

	err := json.Unmarshal(ts.Ixns[0].Payload, &logic)
	require.NoError(t, err)

	testcases := []struct {
		name              string
		logicManifestArgs *rpcargs.LogicManifestArgs
		expectedError     error
	}{
		{
			name: "fetch json logic manifest for existing logicID",
			logicManifestArgs: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch polo logic manifest for existing logicID",
			logicManifestArgs: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "POLO",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch yaml logic manifest for existing logicID",
			logicManifestArgs: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "YAML",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
		},
		{
			name: "fetch logic manifest for non-existing logicID",
			logicManifestArgs: &rpcargs.LogicManifestArgs{
				LogicID:  "0200000070c34ed6ec4384c75d469894052647a078b33ac0f08db0d3751c1fce29a49f",
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &LatestTesseractNumber,
				},
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicManifest, err := client.LogicManifest(ctx, test.logicManifestArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			manifest, err := GetLogicManifestByEncodingType(t, logic.Manifest, test.logicManifestArgs.Encoding)
			require.NoError(t, err)

			require.Equal(t, manifest, logicManifest)

			httpLogicManifest := httpLogicManifest(t, test.logicManifestArgs)
			require.Equal(t, httpLogicManifest, logicManifest)
		})
	}
}

func testContent(t *testing.T, client *Client) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		contentArgs   *rpcargs.ContentArgs
		expectedError error
	}{
		{
			name:        "fetch content from ixpool",
			contentArgs: &rpcargs.ContentArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contentResponse, err := client.Content(ctx, test.contentArgs)

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

func testContentFrom(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		ixPoolArgs    *rpcargs.IxPoolArgs
		expectedCount int
	}{
		{
			name: "fetch content from for existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: addr,
			},
			expectedCount: 1,
		},
		{
			name: "fetch  content from for non-existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: tests.RandomAddress(t),
			},
			expectedCount: 0,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contentFromResponse, err := client.ContentFrom(ctx, test.ixPoolArgs)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(contentFromResponse.Pending), test.expectedCount)

			httpContentFrom := httpContentFrom(t, test.ixPoolArgs)
			require.Equal(t, *httpContentFrom, *contentFromResponse)
		})
	}
}

func testStatus(t *testing.T, client *Client) {
	ctx := context.Background()
	testcases := []struct {
		name       string
		ixPoolArgs *rpcargs.StatusArgs
	}{
		{
			name:       "fetch status of ixpool",
			ixPoolArgs: &rpcargs.StatusArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			statusResponse, err := client.Status(ctx, test.ixPoolArgs)
			require.NoError(t, err)
			require.GreaterOrEqual(t, statusResponse.Pending.ToUint64(), uint64(0))

			httpStatus := httpStatus(t, test.ixPoolArgs)
			require.Equal(t, *httpStatus, *statusResponse)
		})
	}
}

func testInspect(t *testing.T, client *Client) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		inspectArgs   *rpcargs.InspectArgs
		expectedError error
	}{
		{
			name:        "inspect ixpool",
			inspectArgs: &rpcargs.InspectArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			inspectResponse, err := client.Inspect(ctx, test.inspectArgs)

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

func testInteractionByHash(t *testing.T, client *Client, addr common.Address) {
	ts := GetTesseract(t, client, addr, deployLogicHeight)
	ctx := context.Background()
	testcases := []struct {
		name          string
		ixArgs        *rpcargs.InteractionByHashArgs
		expectedError error
	}{
		{
			name: "fetch interaction for existing ix hash",
			ixArgs: &rpcargs.InteractionByHashArgs{
				Hash: ts.Ixns[0].Hash,
			},
		},
		{
			name: "fetch interaction for non-existing ix hash",
			ixArgs: &rpcargs.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: common.ErrFetchingInteraction,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := client.InteractionByHash(ctx, test.ixArgs)

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

func testInteractionByTesseract(t *testing.T, client *Client, addr common.Address) {
	ts := GetTesseract(t, client, addr, deployLogicHeight)
	randomHash := tests.RandomHash(t)
	ixIndex := uint64(0)
	ctx := context.Background()

	testcases := []struct {
		name          string
		ixArgs        *rpcargs.InteractionByTesseract
		expectedError error
	}{
		{
			name: "fetch interaction for existing tesseract hash",
			ixArgs: &rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &ts.Ixns[0].Parts[0].Hash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
		},
		{
			name: "fetch interaction for non-existing tesseract hash",
			ixArgs: &rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: errors.New("interaction not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := client.InteractionByTesseract(ctx, test.ixArgs)

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

func testWaitTime(t *testing.T, client *Client, addr common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		ixPoolArgs    *rpcargs.IxPoolArgs
		expectedError error
	}{
		{
			name: "fetch wait time for existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: addr,
			},
		},
		{
			name: "fetch wait time for non-existing address",
			ixPoolArgs: &rpcargs.IxPoolArgs{
				Address: tests.RandomAddress(t),
			},
			expectedError: errors.New("account not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.WaitTime(ctx, test.ixPoolArgs)

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
		netArgs *rpcargs.NetArgs
	}{
		{
			name:    "fetch peers",
			netArgs: &rpcargs.NetArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			clientPeers, err := client.Peers(context.Background(), test.netArgs)
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
		netArgs       *rpcargs.NetArgs
		expectedValue string
	}{
		{
			name:          "fetch version",
			netArgs:       &rpcargs.NetArgs{},
			expectedValue: config.ProtocolVersion,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			version, err := client.Version(context.Background(), test.netArgs)
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
		netArgs *rpcargs.NetArgs
	}{
		{
			name:    "fetch krama id",
			netArgs: &rpcargs.NetArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nodeInfo, err := client.Info(context.Background(), test.netArgs)
			require.NoError(t, err)

			httpNodeInfo := httpInfo(t, test.netArgs)
			require.Equal(t, httpNodeInfo.KramaID, nodeInfo.KramaID)
		})
	}
}

func testSendInteraction(t *testing.T, client *Client) {
	testcases := []struct {
		name          string
		ixPoolArgs    *common.SendIXArgs
		expectedError error
	}{
		{
			name: "invalid account",
			ixPoolArgs: &common.SendIXArgs{
				Type:      common.IxValueTransfer,
				Sender:    tests.RandomAddress(t),
				FuelPrice: big.NewInt(1),
				FuelLimit: 200,
			},
			expectedError: common.ErrInsufficientFunds,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			bz, err := polo.Polorize(test.ixPoolArgs)
			require.NoError(t, err)

			sendIx := &rpcargs.SendIX{
				IXArgs:    hex.EncodeToString(bz),
				Signature: "",
			}

			_, err = client.SendInteractions(context.Background(), sendIx)

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
		accArgs *rpcargs.AccountArgs
	}{
		{
			name:    "fetch accounts",
			accArgs: &rpcargs.AccountArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountsResponse, err := client.Accounts(context.Background())
			require.NoError(t, err)

			httpAccounts := httpAccounts(t, test.accArgs)
			require.Equal(t, httpAccounts, accountsResponse)

			for _, account := range accountsResponse {
				if account == common.SargaAddress {
					return
				}
			}

			require.FailNow(t, "sarga address not found in list of accounts")
		})
	}
}

func testConnections(t *testing.T, client *Client) {
	ctx := context.Background()

	testcases := []struct {
		name    string
		accArgs *rpcargs.ConnArgs
	}{
		{
			name:    "fetch connections",
			accArgs: &rpcargs.ConnArgs{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connResp, err := client.Connections(ctx)
			require.NoError(t, err)

			httpConnResp := httpConnections(t, test.accArgs)
			require.Equal(t, len(httpConnResp.Conns), len(connResp.Conns))
			require.Equal(t, httpConnResp.InboundConnCount, connResp.InboundConnCount)
			require.Equal(t, httpConnResp.OutboundConnCount, connResp.OutboundConnCount)
			require.Equal(t, httpConnResp.ActivePubSubTopics, connResp.ActivePubSubTopics)

			require.Greater(t, len(connResp.Conns), 0)
		})
	}
}

func testAccountMetaInfo(t *testing.T, client *Client) {
	ctx := context.Background()
	testcases := []struct {
		name          string
		accArgs       *rpcargs.GetAccountArgs
		expectedError error
	}{
		{
			name: "fetch account meta info for sarga address",
			accArgs: &rpcargs.GetAccountArgs{
				Address: common.SargaAddress,
			},
		},
		{
			name: "fetch account meta info for random address",
			accArgs: &rpcargs.GetAccountArgs{
				Address: tests.RandomAddress(t),
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accountMetaInfoResponse, err := client.AccountMetaInfo(ctx, test.accArgs)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			httpAccountMetaInfo := httpAccountMetaInfo(t, test.accArgs)
			require.Equal(t, httpAccountMetaInfo, accountMetaInfoResponse)
			require.Equal(t, common.SargaAddress, accountMetaInfoResponse.Address)
			require.Equal(t, common.SargaAccount, accountMetaInfoResponse.Type)
		})
	}
}

func testCall(t *testing.T, client *Client, addr common.Address) {
	invalidHeight := int64(-2)
	ts := GetTesseract(t, client, addr, createAssetHeight)
	supply, _ := new(big.Int).SetString("130D46", 16)

	ctx := context.Background()

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOITOKEN",
		Supply: supply,
	}

	assetCreatePayload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	expectedAssetAddr := common.NewAccountAddress(4, ts.Address())
	expectedAssetID := common.NewAssetIDv0(false, false, 0, 0, expectedAssetAddr)

	extraData := &common.AssetCreationReceipt{
		AssetID:      expectedAssetID,
		AssetAccount: expectedAssetAddr,
	}

	expectedReceipt := common.Receipt{
		IxType:   common.IxAssetCreate,
		FuelUsed: 100,
	}

	err = expectedReceipt.SetExtraData(extraData)
	if err != nil {
		return
	}

	testcases := []struct {
		name            string
		callArgs        *rpcargs.CallArgs
		expectedReceipt *rpcargs.RPCReceipt
		expectedError   error
	}{
		{
			name: "fetched rpc receipt successfully",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    ts.Address(),
					Nonce:     hexutil.Uint64(4),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(200),
					Payload:   (hexutil.Bytes)(assetCreatePayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts.Address(): {
						TesseractNumber: &createAssetHeight,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxType:    hexutil.Uint64(common.IxAssetCreate),
				FuelUsed:  hexutil.Uint64(expectedReceipt.FuelUsed),
				ExtraData: expectedReceipt.ExtraData,
				From:      addr,
				To:        expectedAssetAddr,
			},
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    addr,
					Nonce:     hexutil.Uint64(3),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(assetCreatePayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts.Address(): {
						TesseractNumber: &invalidHeight,
					},
				},
			},
			expectedError: errors.New("invalid options"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := client.InteractionCall(ctx, test.callArgs)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForCallReceipt(t, test.expectedReceipt, receipt)
		})
	}
}

func testFuelDeduction(t *testing.T, client *Client, addrs map[string]common.Address) {
	ctx := context.Background()
	testcases := []struct {
		name    string
		address common.Address
		height  int64
	}{
		{"deploy logic", addrs["deployAddr"], deployLogicHeight},
		{"value transfer", addrs["assetAddr"], transferTokensHeight},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// Get tesseract for the height
			ts := GetTesseract(t, client, test.address, test.height)
			// Get the interaction receipt
			receipt, err := client.InteractionReceipt(ctx, &rpcargs.ReceiptArgs{
				Hash: ts.Ixns[0].Hash,
			})
			require.NoError(t, err)

			// Determine the expected deduction of MOI Token Balance
			var deducted *big.Int
			// If there are MOI Tokens in the transferred values, add that to the total deductions
			// else, only fuel consumed on the receipt is the expected deduction
			if transferValue, ok := ts.Ixns[0].TransferValues[common.KMOITokenAssetID]; !ok {
				deducted = new(big.Int).SetUint64(uint64(receipt.FuelUsed))
			} else {
				deducted = new(big.Int).Add(new(big.Int).SetUint64(uint64(receipt.FuelUsed)), transferValue.ToInt())
			}

			// Determine balance of MOI Tokens BEFORE the interaction
			preHeight := test.height - 1
			preBalance, err := client.Balance(ctx, &rpcargs.BalArgs{
				Address: test.address,
				AssetID: common.KMOITokenAssetID,
				Options: rpcargs.TesseractNumberOrHash{TesseractNumber: &preHeight},
			})
			require.NoError(t, err)

			// Determine balance of MOI Tokens AFTER the interaction
			postBalance, err := client.Balance(ctx, &rpcargs.BalArgs{
				Address: test.address,
				AssetID: common.KMOITokenAssetID,
				Options: rpcargs.TesseractNumberOrHash{TesseractNumber: &test.height},
			})
			require.NoError(t, err)

			// Verify that post balance == (pre balance - deducted fuel - transfer values of MOI)
			require.Zero(t, postBalance.ToInt().Cmp(new(big.Int).Sub(preBalance.ToInt(), deducted)))
		})
	}
}
