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
func (te *TestEnvironment) lockupAsset(
	sender tests.AccountWithMnemonic,
	payOut *common.PayoutDetails,
) (common.Hash, error) {
	te.logger.Debug("lockup asset ", "sender", sender.ID,
		"beneficiary", payOut.Beneficiary, "asset id", payOut.AssetID,
		"amount", payOut.Amount,
	)

	params := &common.LockupParams{
		Beneficiary: payOut.Beneficiary,
		Amount:      payOut.Amount,
	}

	action, err := common.GetAssetActionPayload(payOut.AssetID, common.LockupEndpoint, params)
	te.Suite.NoError(err)

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
				ID:       payOut.AssetID.AsIdentifier(),
				LockType: common.NoLock,
			},
			{
				ID:       payOut.Beneficiary,
				LockType: common.MutateLock,
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

// validateAssetLockup verifies that an asset lockup operation was successful.
// 1. Ensure the receipt for the interaction was generated successfully.
// 2. Retrieve the lockups associated with the sender.
// 3. Check if the lockup for the specified asset, beneficiary, and amount exists.
func validateAssetLockup(
	te *TestEnvironment,
	sender identifiers.Identifier,
	params *common.PayoutDetails,
	ixHash common.Hash,
	isSuccess bool,
) {
	if !isSuccess {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	lockups := moiclient.GetLockups(te.T(), te.moiClient, sender, -1)

	for _, lockup := range lockups {
		if lockup.AssetID == params.AssetID &&
			lockup.ID == params.Beneficiary &&
			lockup.Amount.ToInt().Cmp(params.Amount) == 0 {
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

	testcases := []struct {
		name     string
		sender   tests.AccountWithMnemonic
		payOut   *common.PayoutDetails
		postTest func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			params *common.PayoutDetails,
			ixHash common.Hash,
			isSuccess bool,
		)
		isSuccess     bool
		expectedError error
	}{
		{
			name:   "lockup MAS0 asset",
			sender: sender,
			payOut: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			isSuccess: true,
			postTest:  validateAssetLockup,
		},
		{
			name:   "amount is invalid",
			sender: sender,
			payOut: &common.PayoutDetails{
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(0),
			},
			isSuccess: false,
			postTest:  validateAssetLockup,
		},
		// TODO: Add more tests here
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.lockupAsset(test.sender, test.payOut)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.payOut, ixHash, test.isSuccess)
		})
	}
}
