package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

//nolint:dup
func (te *TestEnvironment) configureAccount(
	senderID identifiers.Identifier,
	senderKeyID uint64,
	accountConfigurePayload *common.AccountConfigurePayload,
	accKeys []moiclient.AccountKeyWithMnemonic,
) (common.Hash, error) {
	te.logger.Debug("configure account ", "sender", senderID)

	payload, err := accountConfigurePayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         senderID,
			KeyID:      senderKeyID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, senderID, senderKeyID),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAccountConfigure,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       senderID,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, accKeys)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func (te *TestEnvironment) TestIXAccountConfigure() {
	newAcc := tests.RandomIdentifierWithZeroVariant(te.T())
	acc := te.chooseRandomAccount()

	action, err := common.GetAssetActionPayload(
		common.KMOITokenAssetID,
		common.TransferEndpoint,
		&common.TransferParams{
			Beneficiary: newAcc,
			Amount:      big.NewInt(InitialKMOITokens * 0.8),
		})
	require.NoError(te.T(), err)

	keysWithMnemonic, err := cmdCommon.GetAccountsWithMnemonic(4)
	require.NoError(te.T(), err)

	createParticipant(te, acc, &common.ParticipantCreatePayload{
		ID: newAcc,
		KeysPayload: []common.KeyAddPayload{
			{
				PublicKey: keysWithMnemonic[0].PublicKey,
				Weight:    1000,
			},
			{
				PublicKey: keysWithMnemonic[1].PublicKey,
				Weight:    400,
			},
			{
				PublicKey: keysWithMnemonic[2].PublicKey,
				Weight:    400,
			},
			{
				PublicKey: keysWithMnemonic[3].PublicKey,
				Weight:    1001,
			},
		},
		Value: action,
	})

	ixHash, err := te.configureAccount(newAcc, 0,
		&common.AccountConfigurePayload{
			Revoke: []common.KeyRevokePayload{
				{
					KeyID: 3,
				},
			},
		}, []moiclient.AccountKeyWithMnemonic{
			{
				ID:       newAcc,
				KeyID:    0,
				Mnemonic: keysWithMnemonic[0].Mnemonic,
			},
		})
	require.NoError(te.T(), err)
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	testcases := []struct {
		name                    string
		senderID                identifiers.Identifier
		senderKeyID             uint64
		accountConfigurePayload *common.AccountConfigurePayload
		signers                 []moiclient.AccountKeyWithMnemonic
		notaryAccounts          []identifiers.Identifier
		expectedError           error
	}{
		{
			name:        "add account keys",
			senderID:    newAcc,
			senderKeyID: 0,
			accountConfigurePayload: &common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						PublicKey: tests.RandomIdentifierWithZeroVariant(te.T()).Bytes(),
						Weight:    500,
					},
					{
						PublicKey: tests.RandomIdentifierWithZeroVariant(te.T()).Bytes(),
						Weight:    500,
					},
				},
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    0,
					Mnemonic: keysWithMnemonic[0].Mnemonic,
				},
			},
		},
		{
			name:        "revoke account keys",
			senderID:    newAcc,
			senderKeyID: 0,
			accountConfigurePayload: &common.AccountConfigurePayload{
				Revoke: []common.KeyRevokePayload{
					{
						KeyID: 1,
					},
					{
						KeyID: 2,
					},
				},
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    0,
					Mnemonic: keysWithMnemonic[0].Mnemonic,
				},
			},
		},
		{
			name:        "ixn from revoked sender key failed",
			senderID:    newAcc,
			senderKeyID: 3,
			accountConfigurePayload: &common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						PublicKey: tests.RandomIdentifierWithZeroVariant(te.T()).Bytes(),
						Weight:    500,
					},
				},
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    3,
					Mnemonic: keysWithMnemonic[3].Mnemonic,
				},
			},
			expectedError: common.ErrInvalidWeight,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			prevAccKeys, err := te.moiClient.AccountKeys(context.Background(), &args.GetAccountKeysArgs{
				ID: test.senderID,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			ixHash, err := te.configureAccount(test.senderID, test.senderKeyID,
				test.accountConfigurePayload, test.signers)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

			accountKeys, err := te.moiClient.AccountKeys(context.Background(), &args.GetAccountKeysArgs{
				ID: test.senderID,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			if test.accountConfigurePayload.Add != nil {
				require.Equal(te.T(), len(prevAccKeys)+len(test.accountConfigurePayload.Add), len(accountKeys))

				start := len(accountKeys) - len(test.accountConfigurePayload.Add)

				for _, addKey := range test.accountConfigurePayload.Add {
					require.Equal(te.T(), addKey.PublicKey, accountKeys[start].PublicKey.Bytes())
					require.Equal(te.T(), addKey.Weight, accountKeys[start].Weight.ToUint64())
					require.Equal(te.T(), addKey.SignatureAlgorithm, accountKeys[start].SignatureAlgorithm.ToUint64())

					start++
				}

				return
			}

			for _, revokeKey := range test.accountConfigurePayload.Revoke {
				require.Equal(te.T(), true, accountKeys[revokeKey.KeyID].Revoked)
			}
		})
	}
}
