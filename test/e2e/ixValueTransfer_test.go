package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

func (te *TestEnvironment) transferAsset(
	sender tests.AccountWithMnemonic,
	payOut *common.PayoutDetails,
) (common.Hash, error) {
	te.logger.Debug("transfer asset ", "sender", sender.ID,
		"beneficiary", payOut.Beneficiary, "asset id", payOut.AssetID, "token id", payOut.TokenID,
		"amount", payOut.Amount,
	)

	//	var callDataParams any

	callDataParams := &common.TransferParams{
		Beneficiary: payOut.Beneficiary,
		Amount:      payOut.Amount,
	}

	// TODO: Need to improve this logic
	//	callDataParams = &common.TransferParams{
	//		Beneficiary: payOut.Beneficiary,
	//		Amount:      payOut.Amount,
	//	}
	// }

	callData, err := polo.PolorizeDocument(callDataParams, polo.DocStructs())
	te.Suite.NoError(err)

	payload := func() []byte {
		ap := &common.AssetActionPayload{
			AssetID:  payOut.AssetID,
			Callsite: common.TransferEndpoint,
			Calldata: callData.Bytes(),
		}

		encoded, _ := ap.Bytes()

		return encoded
	}()

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         sender.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, sender.ID, 0),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Funds: []common.IxFund{
			{
				AssetID: payOut.AssetID,
				Amount:  big.NewInt(1),
			},
		},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetAction,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       sender.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       payOut.Beneficiary,
				LockType: common.MutateLock,
			},
			{
				ID:       payOut.AssetID.AsIdentifier(),
				LockType: common.NoLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       sender.ID,
			KeyID:    0,
			Mnemonic: sender.Mnemonic,
		},
	})

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. obtain balances of sender and receiver before and after the transfer.
// 3. Ensure the sender's balance is decreased by the transfer amount.
// 4. Ensure the receiver's balance is increased by the transfer amount.
func validateAssetTransfer(
	te *TestEnvironment,
	isFailure bool,
	sender identifiers.Identifier,
	payload *common.PayoutDetails,
	ixHash common.Hash,
) {
	if isFailure {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

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

	MAS0AssetID := createAndMint(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		sender.ID,
		nil,
	), &common.MintParams{
		Beneficiary: sender.ID,
		Amount:      initialAmount,
	})

	testcases := []struct {
		name     string
		sender   tests.AccountWithMnemonic
		payout   *common.PayoutDetails
		postTest func(
			te *TestEnvironment,
			isSuccess bool,
			sender identifiers.Identifier,
			payload *common.PayoutDetails,
			ixHash common.Hash,
		)
		expectedError error
		isFailure     bool
	}{
		{
			name:   "transfer MAS0 asset",
			sender: sender,
			payout: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			postTest: validateAssetTransfer,
		},
		{
			name:   "amount is invalid",
			sender: sender,
			payout: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(0),
			},
			isFailure: true,
			postTest:  validateAssetTransfer,
		},
		{
			name:   "asset ID doesn't exist",
			sender: sender,
			payout: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     tests.GetRandomAssetID(te.T(), tests.RandomIdentifierWithZeroVariant(te.T())),
				Amount:      big.NewInt(100),
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.transferAsset(test.sender, test.payout)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.isFailure, test.sender.ID, test.payout, ixHash)
		})
	}
}

func (te *TestEnvironment) TestAssetTransfer_checkFuelDeduction() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	receiver := accs[1]
	initialAmount := big.NewInt(1000)

	MAS0AssetID := createAndMint(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		sender.ID,
		nil,
	), &common.MintParams{
		Beneficiary: sender.ID,
		Amount:      initialAmount,
	})

	testcases := []struct {
		name          string
		payout        *common.PayoutDetails
		expectedError error
	}{
		{
			name: "transfer non fuel token",
			payout: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(88),
			},
		},
		{
			name: "transfer fuel token",
			payout: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     common.KMOITokenAssetID,
				Amount:      big.NewInt(66),
			},
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			preTransferFuelAmount := getBalance(te, sender.ID, common.KMOITokenAssetID, -1)

			ixHash, err := te.transferAsset(sender, test.payout)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

			postTransferFuelAmount := getBalance(te, sender.ID, common.KMOITokenAssetID, -1)

			if test.payout.AssetID == common.KMOITokenAssetID {
				require.Equal(te.T(),
					preTransferFuelAmount-postTransferFuelAmount,
					receipt.FuelUsed.ToUint64()+test.payout.Amount.Uint64(),
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
