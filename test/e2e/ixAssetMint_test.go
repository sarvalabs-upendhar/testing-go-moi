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
func (te *TestEnvironment) mintAsset(
	acc tests.AccountWithMnemonic,
	assetID identifiers.AssetID,
	params *common.MintParams,
) (common.Hash, error) {
	te.logger.Debug("mint asset ",
		"sender", acc.ID, "asset", assetID, "amount", params.Amount)

	action, err := common.GetAssetActionPayload(assetID, common.MintEndpoint, params)
	te.Suite.NoError(err)

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
			{
				ID:       params.Beneficiary,
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
	assetID identifiers.AssetID,
	payload *common.MintParams,
	ixHash common.Hash,
	isSuccess bool,
) {
	if !isSuccess {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

	beneficiary := payload.Beneficiary

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	beneficiaryHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, beneficiary)

	beneficiaryPrevBal := getBalance(te, beneficiary, assetID, int64(beneficiaryHeight-1))
	beneficiaryCurBal := getBalance(te, beneficiary, assetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64(), beneficiaryCurBal-beneficiaryPrevBal)
}

func (te *TestEnvironment) TestAssetMint() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	beneficiary := accs[1]
	initialAmount := big.NewInt(1000)

	// TODO CONSIDER ADDING THESE UNDER PRE-TEST
	MAS0AssetID := createAsset(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		sender.ID,
		nil,
	))

	testcases := []struct {
		name     string
		sender   tests.AccountWithMnemonic
		assetID  identifiers.AssetID
		params   *common.MintParams
		postTest func(
			te *TestEnvironment,
			assetID identifiers.AssetID,
			payload *common.MintParams,
			ixHash common.Hash,
			isSuccess bool,
		)
		expectedError error
		isSuccess     bool
	}{
		{
			name:    "mint MAS0 asset",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.MintParams{
				Beneficiary: beneficiary.ID,
				Amount:      big.NewInt(100),
			},
			isSuccess: true,
			postTest:  validateAssetMint,
		},
		{
			name:    "sender should be asset manager",
			sender:  beneficiary,
			assetID: MAS0AssetID,
			params: &common.MintParams{
				Beneficiary: beneficiary.ID,
				Amount:      big.NewInt(1),
			},
			isSuccess: false,
			postTest:  validateAssetMint,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.mintAsset(test.sender, test.assetID, test.params)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.assetID, test.params, ixHash, test.isSuccess)
		})
	}
}
