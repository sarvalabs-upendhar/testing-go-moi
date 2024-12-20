package e2e

import (
	"context"
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

func (te *TestEnvironment) revokeAsset(
	sender tests.AccountWithMnemonic,
	assetActionPayload *common.AssetActionPayload,
) (common.Hash, error) {
	te.logger.Debug("revoke asset ", "sender", sender.Addr,
		"beneficiary", assetActionPayload.Beneficiary, "asset id", assetActionPayload.AssetID,
	)

	payload, err := assetActionPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, sender.Addr),
		Sender:    sender.Addr,
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Funds: []common.IxFund{
			{
				AssetID: assetActionPayload.AssetID,
				Amount:  big.NewInt(1),
			},
		},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetRevoke,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  sender.Addr,
				LockType: common.MutateLock,
			},
			{
				Address:  assetActionPayload.Beneficiary,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, sender.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// validateAssetRevoke verifies that an asset revoke operation was successful.
// 1. Ensure the receipt for the interaction was generated successfully.
// 2. Check that the sender's asset mandate no longer includes the revoked asset for the specified beneficiary.
func validateAssetRevoke(
	te *TestEnvironment,
	sender identifiers.Address,
	payload *common.AssetActionPayload,
	ixHash common.Hash,
) {
	// Verify the receipt was generated successfully for the interaction hash.
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	// Retrieves all the mandates associated with the sender.
	mandates := moiclient.GetMandates(te.T(), te.moiClient, sender, -1)

	// Verify that the revoked mandate does not exist in the mandates list.
	for _, mandate := range mandates {
		if mandate.AssetID == payload.AssetID.String() &&
			mandate.Address == payload.Beneficiary {
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

	MAS0AssetID := createAsset(te, sender, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		nil,
	))

	approveAsset(te, sender, &common.AssetActionPayload{
		Beneficiary: receiver.Addr,
		AssetID:     MAS0AssetID,
		Amount:      big.NewInt(100),
		Timestamp:   time.Now().Add(1 * time.Hour).Unix(),
	})

	testcases := []struct {
		name               string
		sender             tests.AccountWithMnemonic
		assetActionPayload *common.AssetActionPayload
		postTest           func(
			te *TestEnvironment,
			sender identifiers.Address,
			payload *common.AssetActionPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "revoke MAS0 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.Addr,
				AssetID:     MAS0AssetID,
			},
			postTest: validateAssetRevoke,
		},
		{
			name:   "invalid ix participants",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: sender.Addr,
				AssetID:     MAS0AssetID,
			},
			expectedError: common.ErrInvalidIxParticipants,
		},
		{
			name:   "beneficiary is sarga account",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: common.SargaAddress,
				AssetID:     MAS0AssetID,
			},
			expectedError: common.ErrGenesisAccount,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.revokeAsset(test.sender, test.assetActionPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.Addr, test.assetActionPayload, ixHash)
		})
	}
}
