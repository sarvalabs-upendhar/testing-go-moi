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

//nolint:dupl
func (te *TestEnvironment) approveAsset(
	sender tests.AccountWithMnemonic,
	assetActionPayload *common.AssetActionPayload,
) (common.Hash, error) {
	te.logger.Debug("approve asset ", "sender", sender.ID,
		"beneficiary", assetActionPayload.Beneficiary, "asset id", assetActionPayload.AssetID,
		"amount", assetActionPayload.Amount,
	)

	payload, err := assetActionPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         sender.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, sender.ID, 0),
		},
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
				Type:    common.IxAssetApprove,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       sender.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       assetActionPayload.Beneficiary,
				LockType: common.MutateLock,
			},
			{
				ID:       common.SargaAccountID,
				LockType: common.ReadLock,
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
	payload *common.AssetActionPayload,
	ixHash common.Hash,
) {
	// Verify that the receipt for the interaction hash was generated successfully.
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	// Retrieve all mandates associated with the sender.
	mandates := moiclient.GetMandates(te.T(), te.moiClient, sender, -1)

	for _, mandate := range mandates {
		if mandate.AssetID == payload.AssetID.String() &&
			mandate.ID == payload.Beneficiary &&
			mandate.Amount.ToInt().Cmp(payload.Amount) == 0 {
			return
		}
	}

	te.T().Fatalf("Expected mandate not found")
}

func (te *TestEnvironment) TestAssetApprove() {
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

	testcases := []struct {
		name               string
		sender             tests.AccountWithMnemonic
		assetActionPayload *common.AssetActionPayload
		postTest           func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			payload *common.AssetActionPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "approve MAS0 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
				Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),
			},
			postTest: validateAssetApprove,
		},
		{
			name:   "amount is invalid",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(-50),
				Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),
			},
			expectedError: common.ErrInvalidValue,
		},
		{
			name:   "timestamp is invalid",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(-50),
				Timestamp:   uint64(time.Now().Add(-1 * time.Hour).Unix()),
			},
			expectedError: common.ErrInvalidValue,
		},
		{
			name:   "beneficiary is sarga account",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: common.SargaAccountID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(1),
				Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),
			},
			expectedError: common.ErrGenesisAccount,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.approveAsset(test.sender, test.assetActionPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.assetActionPayload, ixHash)
		})
	}
}
