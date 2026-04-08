package e2e

import (
	"context"
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

func (te *TestEnvironment) revokeAsset(
	sender tests.AccountWithMnemonic,
	assetID identifiers.AssetID,
	params *common.RevokeParams,
) (common.Hash, error) {
	te.logger.Debug("revoke asset ", "sender", sender.ID,
		"beneficiary", params.Beneficiary, "asset id", assetID,
	)

	action, err := common.GetAssetActionPayload(assetID, common.RevokeEndpoint, params)
	te.Suite.Nil(err)

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
				LockType: common.MutateLock,
			},
			{
				ID:       assetID.AsIdentifier(),
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

// validateAssetRevoke verifies that an asset revoke operation was successful.
// 1. Ensure the receipt for the interaction was generated successfully.
// 2. Check that the sender's asset mandate no longer includes the revoked asset for the specified beneficiary.
func validateAssetRevoke(
	te *TestEnvironment,
	sender identifiers.Identifier,
	assetID identifiers.AssetID,
	params *common.RevokeParams,
	ixHash common.Hash,
	isSuccess bool,
) {
	if !isSuccess {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}
	// Verify the receipt was generated successfully for the interaction hash.
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	// Retrieves all the mandates associated with the sender.
	mandates := moiclient.GetMandates(te.T(), te.moiClient, sender, -1)

	// Verify that the revoked mandate does not exist in the mandates list.
	for _, mandate := range mandates {
		if mandate.AssetID == assetID &&
			mandate.ID == params.Beneficiary {
			te.T().Fatalf("Expected mandate to be revoked, but it still exists")
		}
	}
}

func (te *TestEnvironment) TestAssetRevoke() {
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

	approveAsset(te, sender, MAS0AssetID, &common.ApproveParams{
		Beneficiary: receiver.ID,
		Amount:      big.NewInt(100),
		ExpiresAt:   uint64(time.Now().Add(1 * time.Hour).Unix()),
	})

	testcases := []struct {
		name     string
		sender   tests.AccountWithMnemonic
		assetID  identifiers.AssetID
		params   *common.RevokeParams
		postTest func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			assetID identifiers.AssetID,
			params *common.RevokeParams,
			ixHash common.Hash,
			isSuccess bool,
		)
		expectedError error
		isSuccess     bool
	}{
		{
			name:    "revoke MAS0 asset",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.RevokeParams{
				Beneficiary: receiver.ID,
			},
			postTest:  validateAssetRevoke,
			isSuccess: true,
		},
		{
			name:    "invalid ix participants",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.RevokeParams{
				Beneficiary: sender.ID,
			},
			postTest:  validateAssetRevoke,
			isSuccess: false,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.revokeAsset(test.sender, test.assetID, test.params)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.assetID, test.params, ixHash, test.isSuccess)
		})
	}
}
