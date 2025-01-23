package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

//nolint:dupl
func (te *TestEnvironment) lockupAsset(
	sender tests.AccountWithMnemonic,
	assetActionPayload *common.AssetActionPayload,
) (common.Hash, error) {
	te.logger.Debug("lockup asset ", "sender", sender.ID,
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
				Type:    common.IxAssetLockup,
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

// validateAssetLockup verifies that an asset lockup operation was successful.
// 1. Ensure the receipt for the interaction was generated successfully.
// 2. Retrieve the lockups associated with the sender.
// 3. Check if the lockup for the specified asset, beneficiary, and amount exists.
func validateAssetLockup(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload *common.AssetActionPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	lockups := moiclient.GetLockups(te.T(), te.moiClient, sender, -1)

	for _, lockup := range lockups {
		if lockup.AssetID == payload.AssetID.String() &&
			lockup.ID == payload.Beneficiary &&
			lockup.Amount.ToInt().Cmp(payload.Amount) == 0 {
			return
		}
	}

	te.T().Fatalf("Expected lockup not found")
}

func (te *TestEnvironment) TestAssetLockup() {
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
			name:   "lockup MAS0 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			postTest: validateAssetLockup,
		},
		{
			name:   "amount is invalid",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(-50),
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
			},
			expectedError: common.ErrGenesisAccount,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.lockupAsset(test.sender, test.assetActionPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.assetActionPayload, ixHash)
		})
	}
}
