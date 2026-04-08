package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

//nolint:dup
func (te *TestEnvironment) approveAsset(
	sender tests.AccountWithMnemonic,
	assetID identifiers.AssetID,
	params *common.ApproveParams,
) (common.Hash, error) {
	te.logger.Debug("approve asset ", "sender", sender.ID,
		"beneficiary", params.Beneficiary, "asset id", assetID,
		"amount", params.Amount,
	)

	action, err := common.GetAssetActionPayload(assetID, common.ApproveEndpoint, params)
	require.NoError(te.T(), err)

	payload, err := action.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         sender.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, sender.ID, 0),
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
				ID:       sender.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       params.Beneficiary,
				LockType: common.NoLock,
			},
			{
				ID:       assetID.AsIdentifier(),
				LockType: common.NoLock,
			},
			{
				ID:       common.SargaAccountID,
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

// validateAssetApprove verifies that an asset approval operation was successful.
// 1. Ensure the receipt for the interaction was generated successfully.
// 2. Retrieve the mandates associated with the sender.
// 3. Check that the mandate for the specified asset, beneficiary, and amount exists.
func validateAssetApprove(
	te *TestEnvironment,
	sender identifiers.Identifier,
	assetID identifiers.AssetID,
	params *common.ApproveParams,
	ixHash common.Hash,
	isSuccess bool,
) {
	te.logger.Debug("validating asset approval ", "sender", sender,
		"beneficiary", params.Beneficiary, "asset id", assetID,
		"amount", params.Amount, "ixhash", ixHash,
	)

	if !isSuccess {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

	// Verify that the receipt for the interaction hash was generated successfully.
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	// Retrieve all mandates associated with the sender.
	mandates := moiclient.GetMandates(te.T(), te.moiClient, sender, -1)

	for _, mandate := range mandates {
		if mandate.AssetID == assetID &&
			mandate.ID == params.Beneficiary &&
			mandate.Amount.ToInt().Cmp(params.Amount) == 0 {
			return
		}
	}

	te.T().Fatalf("Expected mandate not found")
}

func (te *TestEnvironment) TestAssetApprove() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	beneficiary := accs[1]

	initialAmount := big.NewInt(10000)

	MAS0AssetID := createAndMint(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		sender.ID,
		nil,
	), &common.MintParams{Beneficiary: sender.ID, Amount: big.NewInt(100)})

	testcases := []struct {
		name     string
		sender   tests.AccountWithMnemonic
		AssetID  identifiers.AssetID
		params   *common.ApproveParams
		postTest func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			assetID identifiers.AssetID,
			params *common.ApproveParams,
			ixHash common.Hash,
			isSuccess bool,
		)
		expectedError error
		isSuccess     bool
	}{
		{
			name:    "approve MAS0 asset",
			sender:  sender,
			AssetID: MAS0AssetID,
			params: &common.ApproveParams{
				Beneficiary: beneficiary.ID,
				Amount:      big.NewInt(100),
			},
			isSuccess: true,
			postTest:  validateAssetApprove,
		},
		{
			name:    "amount is invalid",
			sender:  sender,
			AssetID: MAS0AssetID,
			params: &common.ApproveParams{
				Amount:      big.NewInt(0),
				Beneficiary: beneficiary.ID,
			},
			isSuccess: false,
			postTest:  validateAssetApprove,
		},
		{
			name:    "beneficiary is not available",
			sender:  sender,
			AssetID: MAS0AssetID,
			params: &common.ApproveParams{
				Beneficiary: identifiers.RandomParticipantIDv0().AsIdentifier(),
				Amount:      big.NewInt(1),
			},
			isSuccess:     false,
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.approveAsset(test.sender, test.AssetID, test.params)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.AssetID, test.params, ixHash, test.isSuccess)
		})
	}
}
