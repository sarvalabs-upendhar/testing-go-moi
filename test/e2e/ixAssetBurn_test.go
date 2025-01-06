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
func (te *TestEnvironment) burnAsset(
	acc tests.AccountWithMnemonic,
	assetSupplyPayload *common.AssetSupplyPayload,
) (common.Hash, error) {
	te.logger.Debug("burn asset ",
		"sender", acc.Addr, "asset", assetSupplyPayload.AssetID, "amount", assetSupplyPayload.Amount)

	payload, err := assetSupplyPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			Address:    acc.Addr,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, acc.Addr, 0),
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
				Type:    common.IxAssetBurn,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  acc.Addr,
				LockType: common.MutateLock,
			},
			{
				Address:  assetSupplyPayload.AssetID.Address(),
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			Addr:     acc.Addr,
			KeyID:    0,
			Mnemonic: acc.Mnemonic,
		},
	})

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. make sure asset burned on senders side by payload amount
func validateAssetBurn(
	te *TestEnvironment,
	sender identifiers.Address,
	payload common.AssetSupplyPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, payload.AssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, payload.AssetID, args.LatestTesseractHeight)

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

	transferAsset(te, sender, &common.AssetActionPayload{
		Beneficiary: nonOperator.Addr,
		AssetID:     MAS0AssetID,
		Amount:      big.NewInt(100),
	})

	testcases := []struct {
		name               string
		sender             tests.AccountWithMnemonic
		assetSupplyPayload *common.AssetSupplyPayload
		postTest           func(
			te *TestEnvironment,
			sender identifiers.Address,
			payload common.AssetSupplyPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "burn MAS0 asset",
			sender: sender,
			assetSupplyPayload: &common.AssetSupplyPayload{
				AssetID: MAS0AssetID,
				Amount:  big.NewInt(100),
			},
			postTest: validateAssetBurn,
		},
		{
			name:   "burn MAS1 asset",
			sender: sender,
			assetSupplyPayload: &common.AssetSupplyPayload{
				AssetID: MAS1AssetID,
				Amount:  big.NewInt(1),
			},
			expectedError: common.ErrMintOrBurnNonFungibleToken,
		},
		{
			name:   "amount is invalid",
			sender: sender,
			assetSupplyPayload: &common.AssetSupplyPayload{
				AssetID: MAS0AssetID,
				Amount:  big.NewInt(0),
			},
			expectedError: common.ErrInvalidValue,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.burnAsset(test.sender, test.assetSupplyPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.Addr, *test.assetSupplyPayload, ixHash)
		})
	}
}
