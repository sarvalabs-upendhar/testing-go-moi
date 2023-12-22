package e2e

import (
	"context"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func (te *TestEnvironment) mintAsset(
	acc tests.AccountWithMnemonic,
	assetMintPayload *common.AssetMintOrBurnPayload,
) (common.Hash, error) {
	te.logger.Debug("mint asset ",
		"sender", acc.Addr, "asset", assetMintPayload.Asset, "amount", assetMintPayload.Amount)

	payload, err := assetMintPayload.Bytes()
	te.Suite.NoError(err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetMint,
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, acc.Addr),
		Sender:    acc.Addr,
		FuelPrice: DefaulFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Payload:   payload,
	}

	sendIX := moiclient.CreateSendIXFromSendIXArgs(te.T(), sendIXArgs, acc.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. make sure asset minted on senders side by payload amount
func validateAssetMint(
	te *TestEnvironment,
	sender common.Address,
	payload common.AssetMintOrBurnPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, payload.Asset, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.Asset, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64(), senderCurBal-senderPrevBal)
}

func (te *TestEnvironment) TestAssetMint() {
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

	transferAsset(te, sender, nonOperator.Addr, map[common.AssetID]*big.Int{
		MAS0AssetID: big.NewInt(100),
	})

	testcases := []struct {
		name             string
		sender           tests.AccountWithMnemonic
		assetMintPayload *common.AssetMintOrBurnPayload
		postTest         func(
			te *TestEnvironment,
			sender common.Address,
			payload common.AssetMintOrBurnPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "mint MAS0 asset",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  MAS0AssetID,
				Amount: big.NewInt(100),
			},
			postTest: validateAssetMint,
		},
		{
			name:   "cannot mint MAS1 asset",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  MAS1AssetID,
				Amount: big.NewInt(1),
			},
			expectedError: common.ErrMintNonFungibleToken,
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
			name:   "invalid assetID version",
			sender: sender,
			assetMintPayload: &common.AssetMintOrBurnPayload{
				Asset:  common.AssetID(""),
				Amount: big.NewInt(1),
			},
			expectedError: errors.New("invalid asset ID: insufficient length"),
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
			ixHash, err := te.mintAsset(test.sender, test.assetMintPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}
			require.NoError(te.T(), err)

			test.postTest(te, test.sender.Addr, *test.assetMintPayload, ixHash)
		})
	}
}
