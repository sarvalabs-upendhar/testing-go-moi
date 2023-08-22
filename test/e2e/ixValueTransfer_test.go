package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func (te *TestEnvironment) transferAsset(
	sender tests.AccountWithMnemonic,
	receiver common.Address,
	transferValues map[common.AssetID]*big.Int,
) (common.Hash, error) {
	te.logger.Info("transfer asset ", "sender", sender.Addr,
		"receiver", receiver, "transfer values", transferValues)

	sendIXArgs := &common.SendIXArgs{
		Type:           common.IxValueTransfer,
		Nonce:          moiclient.GetLatestNonce(te.T(), te.moiClient, sender.Addr),
		Sender:         sender.Addr,
		Receiver:       receiver,
		TransferValues: transferValues,
		FuelPrice:      DefaulFuelPrice,
		FuelLimit:      DefaultFuelLimit,
	}

	sendIX := moiclient.CreateSendIXFromSendIXArgs(te.T(), sendIXArgs, sender.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. obtain balances of sender and receiver before and after the transfer.
// 3. Ensure the sender's balance is decreased by the transfer amount.
// 4. Ensure the receiver's balance is increased by the transfer amount.
func validateAssetTransfer(
	te *TestEnvironment,
	sender common.Address,
	receiver common.Address,
	assetID common.AssetID,
	amount *big.Int,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, assetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, assetID, args.LatestTesseractHeight)

	receiverCurBal := getBalance(te, receiver, assetID, args.LatestTesseractHeight)

	require.Equal(te.T(), amount.Uint64(), senderPrevBal-senderCurBal)
	require.Equal(te.T(), amount.Uint64(), receiverCurBal)
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
		name           string
		sender         tests.AccountWithMnemonic
		receiver       common.Address
		transferValues map[common.AssetID]*big.Int
		postTest       func(
			te *TestEnvironment,
			sender common.Address,
			receiver common.Address,
			assetID common.AssetID,
			amount *big.Int,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:     "transfer MAS0 asset",
			sender:   sender,
			receiver: receiver.Addr,
			transferValues: map[common.AssetID]*big.Int{
				MAS0AssetID: big.NewInt(100),
			},
			postTest: validateAssetTransfer,
		},
		{
			name:     "transfer MAS1 asset",
			sender:   sender,
			receiver: receiver.Addr,
			transferValues: map[common.AssetID]*big.Int{
				MAS1AssetID: big.NewInt(1),
			},
			postTest: validateAssetTransfer,
		},
		{
			name:     "insufficient balance",
			sender:   sender,
			receiver: receiver.Addr,
			transferValues: map[common.AssetID]*big.Int{
				MAS0AssetID: initialAmount.Add(initialAmount, big.NewInt(1)),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:           "empty transfer values",
			sender:         sender,
			receiver:       receiver.Addr,
			transferValues: map[common.AssetID]*big.Int{},
			expectedError:  common.ErrEmptyTransferValues,
		},
		{
			name:     "asset ID doesn't exist",
			sender:   sender,
			receiver: receiver.Addr,
			transferValues: map[common.AssetID]*big.Int{
				tests.GetRandomAssetID(te.T(), tests.RandomAddress(te.T())): big.NewInt(100),
			},
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.transferAsset(test.sender, test.receiver, test.transferValues)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}
			require.NoError(te.T(), err)

			for assetID, amount := range test.transferValues {
				test.postTest(te, test.sender.Addr, test.receiver, assetID, amount, ixHash)

				break
			}
		})
	}
}
