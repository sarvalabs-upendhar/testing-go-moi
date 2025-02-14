package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

//nolint:dupl
func (te *TestEnvironment) mintAsset(
	acc tests.AccountWithMnemonic,
	assetSupplyPayload *common.AssetSupplyPayload,
) (common.Hash, error) {
	te.logger.Debug("mint asset ",
		"sender", acc.ID, "asset", assetSupplyPayload.AssetID, "amount", assetSupplyPayload.Amount)

	payload, err := assetSupplyPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         acc.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, acc.ID, 0),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Funds: []common.IxFund{
			{
				AssetID: assetSupplyPayload.AssetID,
				Amount:  assetSupplyPayload.Amount,
			},
		},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetMint,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       acc.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       assetSupplyPayload.AssetID.AsIdentifier(),
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       acc.ID,
			KeyID:    0,
			Mnemonic: acc.Mnemonic,
		},
	})

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. make sure asset minted on senders side by payload amount
func validateAssetMint(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload common.AssetSupplyPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, payload.AssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.AssetID, args.LatestTesseractHeight)

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

	transferAsset(te, sender, &common.AssetActionPayload{
		Beneficiary: nonOperator.ID,
		AssetID:     MAS0AssetID,
		Amount:      big.NewInt(100),
	})

	testcases := []struct {
		name               string
		sender             tests.AccountWithMnemonic
		assetSupplyPayload *common.AssetSupplyPayload
		postTest           func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			payload common.AssetSupplyPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "mint MAS0 asset",
			sender: sender,
			assetSupplyPayload: &common.AssetSupplyPayload{
				AssetID: MAS0AssetID,
				Amount:  big.NewInt(100),
			},
			postTest: validateAssetMint,
		},
		{
			name:   "cannot mint MAS1 asset",
			sender: sender,
			assetSupplyPayload: &common.AssetSupplyPayload{
				AssetID: MAS1AssetID,
				Amount:  big.NewInt(1),
			},
			expectedError: common.ErrMintOrBurnNonFungibleToken,
		},
		{
			name:   "amount is invalid",
			sender: nonOperator,
			assetSupplyPayload: &common.AssetSupplyPayload{
				AssetID: MAS0AssetID,
				Amount:  big.NewInt(-1),
			},
			expectedError: common.ErrInvalidValue,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.mintAsset(test.sender, test.assetSupplyPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, *test.assetSupplyPayload, ixHash)
		})
	}
}
