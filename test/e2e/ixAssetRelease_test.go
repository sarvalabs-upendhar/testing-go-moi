package e2e

import (
	"context"
	"math/big"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func (te *TestEnvironment) releaseAsset(
	sender tests.AccountWithMnemonic,
	assetActionPayload *common.AssetActionPayload,
) (common.Hash, error) {
	te.logger.Debug("release asset ", "sender", sender.Addr, "benefactor", assetActionPayload.Benefactor,
		"beneficiary", assetActionPayload.Beneficiary, "asset id", assetActionPayload.AssetID,
	)

	payload, err := assetActionPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, sender.Addr),
		Sender:    sender.Addr,
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
				Address:  sender.Addr,
				LockType: common.MutateLock,
			},
			{
				Address:  assetActionPayload.Beneficiary,
				LockType: common.MutateLock,
			},
			{
				Address:  assetActionPayload.Benefactor,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, sender.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func validateAssetRelease(
	te *TestEnvironment,
	sender identifiers.Address,
	payload *common.AssetActionPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	lockups := moiclient.GetLockups(te.T(), te.moiClient, sender, -1)

	for _, lockup := range lockups {
		if lockup.AssetID == payload.AssetID.String() &&
			lockup.Address == payload.Beneficiary {
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
		Beneficiary: sender.Addr,
		AssetID:     MAS0AssetID,
		Amount:      big.NewInt(100),
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
			name:   "release MAS0 asset",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Benefactor:  benefactor.Addr,
				Beneficiary: receiver.Addr,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			postTest: validateAssetRelease,
		},
		{
			name:   "invalid ix participants",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Benefactor:  sender.Addr,
				Beneficiary: receiver.Addr,
				AssetID:     MAS0AssetID,
				Amount:      big.NewInt(100),
			},
			expectedError: common.ErrInvalidBenefactor,
		},
		{
			name:   "beneficiary is sarga account",
			sender: sender,
			assetActionPayload: &common.AssetActionPayload{
				Benefactor:  benefactor.Addr,
				Beneficiary: common.SargaAddress,
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

			test.postTest(te, test.sender.Addr, test.assetActionPayload, ixHash)
		})
	}
}
