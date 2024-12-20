package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

//nolint:dupl
func (te *TestEnvironment) transferAsset(
	sender tests.AccountWithMnemonic,
	assetActionPayload *common.AssetActionPayload,
) (common.Hash, error) {
	te.logger.Debug("transfer asset ", "sender", sender.Addr,
		"beneficiary", assetActionPayload.Beneficiary, "asset id", assetActionPayload.AssetID,
		"amount", assetActionPayload.Amount,
	)

	payload, err := assetActionPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, sender.Addr),
		Sender:    sender.Addr,
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Funds: []common.IxFund{
			{
				AssetID: assetActionPayload.AssetID,
				Amount:  big.NewInt(1),
			},
		},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetTransfer,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  sender.Addr,
				LockType: common.MutateLock,
			},
			{
				Address:  assetActionPayload.Beneficiary,
				LockType: common.MutateLock,
			},
			{
				Address:  common.SargaAddress,
				LockType: common.ReadLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, sender.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. obtain balances of sender and receiver before and after the transfer.
// 3. Ensure the sender's balance is decreased by the transfer amount.
// 4. Ensure the receiver's balance is increased by the transfer amount.
func validateAssetTransfer(
	te *TestEnvironment,
	sender identifiers.Address,
	payload *common.AssetActionPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, payload.AssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.AssetID, args.LatestTesseractHeight)

	receiverCurBal := getBalance(te, payload.Beneficiary, payload.AssetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64(), senderPrevBal-senderCurBal)
	require.Equal(te.T(), payload.Amount.Uint64(), receiverCurBal)
}

func (te *TestEnvironment) TestAssetTransfer() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	receiver := accs[1]
	initialAmount := big.NewInt(1000)

	MAS0AssetID := createAsset(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		nil,
	))

	MAS1AssetID := createAsset(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		big.NewInt(1),
		common.MAS1,
		nil,
	))

	testcases := []struct {
		name               string
		sender             tests.AccountWithMnemonic
		assetActionPayload *common.AssetActionPayload
		postTest           func(
			te *TestEnvironment,
			sender identifiers.Address,
			payload *common.AssetActionPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "transfer MAS0 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			postTest: validateAssetTransfer,
		},
		{
			name:   "transfer MAS1 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     MAS1AssetID,
				Amount:      big.NewInt(1),
			},
			postTest: validateAssetTransfer,
		},
		{
			name:   "amount is invalid",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(0),
			},
			expectedError: common.ErrInvalidValue,
		},
		{
			name:   "beneficiary is sarga account",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: common.SargaAddress,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(1),
			},
			expectedError: common.ErrGenesisAccount,
		},
		{
			name:   "asset ID doesn't exist",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     tests.GetRandomAssetID(te.T(), tests.RandomAddress(te.T())),
				Amount:      big.NewInt(100),
			},
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.transferAsset(test.sender, test.assetActionPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.Addr, test.assetActionPayload, ixHash)
		})
	}
}

func (te *TestEnvironment) TestAssetTransfer_checkFuelDeduction() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	receiver := accs[1]
	initialAmount := big.NewInt(1000)

	MAS0AssetID := createAsset(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		nil,
	))

	testcases := []struct {
		name               string
		assetActionPayload *common.AssetActionPayload
		expectedError      error
	}{
		{
			name: "transfer non fuel token",
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(88),
			},
		},
		{
			name: "transfer fuel token",
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     common.KMOITokenAssetID,
				Amount:      big.NewInt(66),
			},
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			preTransferFuelAmount := getBalance(te, sender.Addr, common.KMOITokenAssetID, -1)

			ixHash, err := te.transferAsset(sender, test.assetActionPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

			postTransferFuelAmount := getBalance(te, sender.Addr, common.KMOITokenAssetID, -1)

			if test.assetActionPayload.AssetID == common.KMOITokenAssetID {
				require.Equal(te.T(),
					preTransferFuelAmount-postTransferFuelAmount,
					receipt.FuelUsed.ToUint64()+test.assetActionPayload.Amount.Uint64(),
				)

				return
			}

			require.Equal(te.T(),
				preTransferFuelAmount-postTransferFuelAmount,
				receipt.FuelUsed.ToUint64(),
			)
		})
	}
}
