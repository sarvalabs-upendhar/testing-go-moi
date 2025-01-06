package e2e

import (
	"context"
	"fmt"
	"math/big"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

func (te *TestEnvironment) configureAccount(
	senderAddr identifiers.Address,
	senderKeyID uint64,
	accountConfigurePayload *common.AccountConfigurePayload,
	accKeys []moiclient.AccountKeyWithMnemonic,
) (common.Hash, error) {
	te.logger.Debug("configure account ", "sender", senderAddr)

	payload, err := accountConfigurePayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			Address:    senderAddr,
			KeyID:      senderKeyID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, senderAddr, senderKeyID),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IXAccountConfigure,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  senderAddr,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, accKeys)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func (te *TestEnvironment) TestIXAccountConfigure() {
	newAcc := tests.RandomAddress(te.T())
	acc := te.chooseRandomAccount()

	keysWithMnemonic, err := cmdCommon.GetAccountsWithMnemonic(4)
	require.NoError(te.T(), err)

	createParticipant(te, acc, &common.ParticipantCreatePayload{
		Address: newAcc,
		KeysPayload: []common.KeyAddPayload{
			{
				PublicKey: keysWithMnemonic[0].Addr.Bytes(),
				Weight:    1000,
			},
			{
				PublicKey: keysWithMnemonic[1].Addr.Bytes(),
				Weight:    400,
			},
			{
				PublicKey: keysWithMnemonic[2].Addr.Bytes(),
				Weight:    400,
			},
			{
				PublicKey: keysWithMnemonic[3].Addr.Bytes(),
				Weight:    1001,
			},
		},
		Amount: big.NewInt(InitialKMOITokens * 0.8),
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
				Addr:     newAcc,
				KeyID:    0,
				Mnemonic: keysWithMnemonic[0].Mnemonic,
			},
		})
	require.NoError(te.T(), err)
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	testcases := []struct {
		name                    string
		senderAddr              identifiers.Address
		senderKeyID             uint64
		accountConfigurePayload *common.AccountConfigurePayload
		signers                 []moiclient.AccountKeyWithMnemonic
		notaryAccounts          []identifiers.Address
		expectedError           error
	}{
		{
			name:        "add account keys",
			senderAddr:  newAcc,
			senderKeyID: 0,
			accountConfigurePayload: &common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						PublicKey: tests.RandomAddress(te.T()).Bytes(),
						Weight:    500,
					},
					{
						PublicKey: tests.RandomAddress(te.T()).Bytes(),
						Weight:    500,
					},
				},
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					Addr:     newAcc,
					KeyID:    0,
					Mnemonic: keysWithMnemonic[0].Mnemonic,
				},
			},
		},
		{
			name:        "revoke account keys",
			senderAddr:  newAcc,
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
					Addr:     newAcc,
					KeyID:    0,
					Mnemonic: keysWithMnemonic[0].Mnemonic,
				},
			},
		},
		{
			name:        "ixn from revoked sender key failed",
			senderAddr:  newAcc,
			senderKeyID: 3,
			accountConfigurePayload: &common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						PublicKey: tests.RandomAddress(te.T()).Bytes(),
						Weight:    500,
					},
				},
			},
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					Addr:     newAcc,
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
				Address: test.senderAddr,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			fmt.Printf("before %+v\n", prevAccKeys)

			ixHash, err := te.configureAccount(test.senderAddr, test.senderKeyID,
				test.accountConfigurePayload, test.signers)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

			accountKeys, err := te.moiClient.AccountKeys(context.Background(), &args.GetAccountKeysArgs{
				Address: test.senderAddr,
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
