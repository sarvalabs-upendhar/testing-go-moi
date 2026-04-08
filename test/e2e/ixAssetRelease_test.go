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
	assetID identifiers.AssetID,
	params *common.ReleaseParams,
) (common.Hash, error) {
	te.logger.Debug("release asset ", "sender", sender.ID, "benefactor", params.Benefactor,
		"beneficiary", params.Beneficiary, "asset id", assetID,
	)

	action, err := common.GetAssetActionPayload(assetID, common.ReleaseEndpoint, params)
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
		Funds:     []common.IxFund{},
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
				ID:       params.Beneficiary, // FIXME: Beneficiary can be sender in some scenarios
				LockType: common.MutateLock,
			},
			{
				ID:       params.Benefactor,
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

func validateAssetRelease(
	te *TestEnvironment,
	sender identifiers.Identifier,
	assetID identifiers.AssetID,
	payload *common.ReleaseParams,
	ixHash common.Hash,
	isFailure bool,
) {
	if isFailure {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	lockups := moiclient.GetLockups(te.T(), te.moiClient, sender, -1)

	for _, lockup := range lockups {
		if lockup.AssetID == assetID &&
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

	MAS0AssetID := createAndMint(te, benefactor, createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		initialAmount,
		common.MAS0,
		benefactor.ID,
		nil,
	), &common.MintParams{
		Beneficiary: benefactor.ID,
		Amount:      initialAmount,
	})

	lockupAsset(te, benefactor, &common.PayoutDetails{
		Beneficiary: sender.ID,
		AssetID:     MAS0AssetID,
		Amount:      big.NewInt(100),
	})

	testcases := []struct {
		name     string
		sender   tests.AccountWithMnemonic
		assetID  identifiers.AssetID
		params   *common.ReleaseParams
		postTest func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			assetID identifiers.AssetID,
			params *common.ReleaseParams,
			ixHash common.Hash,
			isFailure bool,
		)
		expectedError error
		isFailure     bool
	}{
		{
			name:    "release MAS0 asset",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.ReleaseParams{
				Benefactor:  benefactor.ID,
				Beneficiary: receiver.ID,
				Amount:      big.NewInt(100),
			},
			postTest: validateAssetRelease,
		},
		{
			name:    "lockup not found",
			sender:  sender,
			assetID: MAS0AssetID,
			params: &common.ReleaseParams{
				Benefactor:  sender.ID, // no lockup available for sender
				Beneficiary: benefactor.ID,
				Amount:      big.NewInt(100),
			},
			postTest:  validateAssetRelease,
			isFailure: true,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.releaseAsset(test.sender, test.assetID, test.params)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.assetID, test.params, ixHash, test.isFailure)
		})
	}
}
