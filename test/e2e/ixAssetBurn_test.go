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

//nolint:dup
func (te *TestEnvironment) burnAsset(
	acc tests.AccountWithMnemonic,
	assetID identifiers.AssetID,
	params *common.BurnParams,
) (common.Hash, error) {
	te.logger.Debug("burn asset ",
		"sender", acc.ID, "asset", assetID, "amount", params.Amount)

	action, err := common.GetAssetActionPayload(assetID, common.BurnEndpoint, params)
	require.NoError(te.T(), err)

	payload, err := action.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         acc.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, acc.ID, 0),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetAction,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       acc.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       assetID.AsIdentifier(),
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
// 2. make sure asset burned on senders side by payload amount
func validateAssetBurn(
	te *TestEnvironment,
	sender identifiers.Identifier,
	assetID identifiers.AssetID,
	params *common.BurnParams,
	ixHash common.Hash,
	isSuccess bool,
) {
	if !isSuccess {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, assetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, assetID, args.LatestTesseractHeight)

	require.Equal(te.T(), params.Amount.Uint64(), senderPrevBal-senderCurBal)
}

func (te *TestEnvironment) TestAssetBurn() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]

	initialAmount := big.NewInt(1000)

	// TODO CONSIDER ADDING THESE UNDER PRE-TEST
	MAS0AssetID := createAndMint(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		sender.ID,
		nil,
	),
		&common.MintParams{Beneficiary: sender.ID, Amount: initialAmount})

	testcases := []struct {
		name      string
		sender    tests.AccountWithMnemonic
		assetID   identifiers.AssetID
		params    *common.BurnParams
		isSuccess bool
		postTest  func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			assetID identifiers.AssetID,
			payload *common.BurnParams,
			ixHash common.Hash,
			isSuccess bool,
		)
		expectedError error
	}{
		{
			name:    "burn MAS0 asset",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.BurnParams{
				Amount: big.NewInt(100),
			},
			postTest:  validateAssetBurn,
			isSuccess: true,
		},

		{
			name:    "amount is invalid",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.BurnParams{
				Amount: big.NewInt(0),
			},
			isSuccess: false,
			postTest:  validateAssetBurn,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.burnAsset(test.sender, test.assetID, test.params)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.assetID, test.params, ixHash, test.isSuccess)
		})
	}
}
