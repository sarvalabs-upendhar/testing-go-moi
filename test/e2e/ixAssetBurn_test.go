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

func (te *TestEnvironment) burnAsset(
	acc tests.AccountWithMnemonic,
	assetMintPayload *common.AssetMintOrBurnPayload,
) (common.Hash, error) {
	te.logger.Debug("burn asset ",
		"sender", acc.Addr, "asset", assetMintPayload.Asset, "amount", assetMintPayload.Amount)

	payload, err := assetMintPayload.Bytes()
	te.Suite.NoError(err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetBurn,
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, acc.Addr),
		Sender:    acc.Addr,
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Payload:   payload,
	}

	sendIX := moiclient.CreateSendIXFromSendIXArgs(te.T(), sendIXArgs, acc.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. make sure asset burned on senders side by payload amount
func validateAssetBurn(
	te *TestEnvironment,
	sender identifiers.Address,
	payload common.AssetMintOrBurnPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, payload.Asset, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.Asset, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64(), senderPrevBal-senderCurBal)
}

func (te *TestEnvironment) TestAssetBurn() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	nonOperator := accs[1]

	initialAmount := big.NewInt(1000)

	// TODO CONSIDER ADDING THESE UNDER PRE-TEST
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

	transferAsset(te, sender, nonOperator.Addr, map[identifiers.AssetID]*big.Int{
		MAS0AssetID: big.NewInt(100),
	})

	testcases := []struct {
		name             string
		sender           tests.AccountWithMnemonic
		assetMintPayload *common.AssetMintOrBurnPayload
		postTest         func(
			te *TestEnvironment,
			sender identifiers.Address,
			payload common.AssetMintOrBurnPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "burn MAS0 asset",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  MAS0AssetID,
				Amount: big.NewInt(100),
			},
			postTest: validateAssetBurn,
		},
		{
			name:   "burn MAS1 asset",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  MAS1AssetID,
				Amount: big.NewInt(1),
			},
			postTest: validateAssetBurn,
		},
		{
			name:   "asset not found",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  tests.GetRandomAssetID(te.T(), tests.RandomAddress(te.T())),
				Amount: big.NewInt(1),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient balance",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  MAS0AssetID,
				Amount: initialAmount.Add(initialAmount, big.NewInt(1)),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "operator address mismatch (sender is not the asset operator)",
			sender: nonOperator,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  MAS0AssetID,
				Amount: big.NewInt(1),
			},
			expectedError: common.ErrOperatorMismatch,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.burnAsset(test.sender, test.assetMintPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}
			require.NoError(te.T(), err)

			test.postTest(te, test.sender.Addr, *test.assetMintPayload, ixHash)
		})
	}
}
