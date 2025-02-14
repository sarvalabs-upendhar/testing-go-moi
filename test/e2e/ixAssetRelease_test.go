package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func (te *TestEnvironment) releaseAsset(
	sender tests.AccountWithMnemonic,
	assetActionPayload *common.AssetActionPayload,
) (common.Hash, error) {
	te.logger.Debug("release asset ", "sender", sender.ID, "benefactor", assetActionPayload.Benefactor,
		"beneficiary", assetActionPayload.Beneficiary, "asset id", assetActionPayload.AssetID,
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
		Funds:     []common.IxFund{},
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetRelease,
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
				ID:       assetActionPayload.Benefactor,
				LockType: common.MutateLock,
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

func validateAssetRelease(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload *common.AssetActionPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	lockups := moiclient.GetLockups(te.T(), te.moiClient, sender, -1)

	for _, lockup := range lockups {
		if lockup.AssetID == payload.AssetID.String() &&
			lockup.ID == payload.Beneficiary {
			te.T().Fatalf("Expected lockup to be released, but it still exists")
		}
	}
}

func (te *TestEnvironment) TestAssetRelease() {
	accs, err := te.chooseRandomUniqueAccounts(3)
	require.NoError(te.T(), err)

	sender := accs[0]
	receiver := accs[1]
	benefactor := accs[2]
	initialAmount := big.NewInt(1000)

	MAS0AssetID := createAsset(te, benefactor, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		nil,
	))

	lockupAsset(te, benefactor, &common.AssetActionPayload{
		Beneficiary: sender.ID,
		AssetID:     MAS0AssetID,
		Amount:      big.NewInt(100),
	})

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
			name:   "release MAS0 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Benefactor:  benefactor.ID,
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			postTest: validateAssetRelease,
		},
		{
			name:   "invalid ix participants",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Benefactor:  sender.ID,
				Beneficiary: receiver.ID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			expectedError: common.ErrInvalidBenefactor,
		},
		{
			name:   "beneficiary is sarga account",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Benefactor:  benefactor.ID,
				Beneficiary: common.SargaAccountID,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			expectedError: common.ErrGenesisAccount,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.releaseAsset(test.sender, test.assetActionPayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.assetActionPayload, ixHash)
		})
	}
}
