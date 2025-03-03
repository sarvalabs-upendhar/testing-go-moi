package e2e

import (
	"context"
	"fmt"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

// TODO Decide how to throw errors , use suite or require package
func (te *TestEnvironment) createAssetWithMultiSig(
	senderID identifiers.Identifier,
	senderKeyID uint64,
	assetCreatePayload *common.AssetCreatePayload,
	accountKeys []moiclient.AccountKeyWithMnemonic,
	notaryAccount []identifiers.Identifier,
) (common.Hash, error) {
	te.logger.Debug("create asset ",
		"sender", senderID, "symbol", assetCreatePayload.Symbol, "supply", assetCreatePayload.Supply)

	payload, err := assetCreatePayload.Bytes()
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
				Type:    common.IxAssetCreate,
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

	for _, id := range notaryAccount {
		ixData.Participants = append(ixData.Participants, common.IxParticipant{
			ID:       id,
			LockType: common.MutateLock,
			Notary:   true,
		})
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, accountKeys)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func (te *TestEnvironment) TestMultiSig() {
	newAcc := tests.RandomIdentifierWithZeroVariant(te.T())
	acc := te.chooseRandomAccount()

	accounts, err := cmdCommon.GetAccountsWithMnemonic(4)
	require.NoError(te.T(), err)

	createParticipant(te, acc, &common.ParticipantCreatePayload{
		ID: newAcc,
		KeysPayload: []common.KeyAddPayload{
			{
				PublicKey: accounts[0].PublicKey,
				Weight:    400,
			},
			{
				PublicKey: accounts[1].PublicKey,
				Weight:    400,
			},
			{
				PublicKey: accounts[2].PublicKey,
				Weight:    200,
			},
			{
				PublicKey: accounts[3].PublicKey,
				Weight:    400,
			},
		},
		Amount: big.NewInt(100000),
	})

	assetCreatePayload := createAssetCreatePayload(
		tests.GetRandomUpperCaseString(te.T(), 8),
		big.NewInt(1000),
		common.MAS0,
		func(payload *common.AssetCreatePayload) {
			payload.Dimension = 1
			payload.IsStateFul = true
			payload.IsLogical = true
		},
	)

	testcases := []struct {
		name               string
		senderID           identifiers.Identifier
		senderKeyID        uint64
		assetCreatePayload *common.AssetCreatePayload
		signers            []moiclient.AccountKeyWithMnemonic
		notaryAccounts     []identifiers.Identifier
		expectedError      error
	}{
		{
			name:               "finalized ixn with multisig successfully",
			senderID:           newAcc,
			senderKeyID:        2,
			assetCreatePayload: assetCreatePayload,
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    0,
					Mnemonic: accounts[0].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
			},
		},
		{
			name:               "sender key signature not found",
			senderID:           newAcc,
			senderKeyID:        0,
			assetCreatePayload: assetCreatePayload,
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    3,
					Mnemonic: accounts[3].Mnemonic,
				},
			},
			expectedError: common.ErrInvalidSenderSignature,
		},
		{
			name:               "notary participant signature not found",
			senderID:           newAcc,
			senderKeyID:        1,
			assetCreatePayload: assetCreatePayload,
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    0,
					Mnemonic: accounts[0].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
			},
			notaryAccounts: []identifiers.Identifier{te.accounts[1].ID},
			expectedError:  errors.New("invalid notary participant signature"),
		},
		{
			name:        "finalize ixn with notary signature successfully",
			senderID:    newAcc,
			senderKeyID: 1,
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1),
				common.MAS1,
				func(payload *common.AssetCreatePayload) {
					payload.Dimension = 1
				},
			),
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					ID:       newAcc,
					KeyID:    0,
					Mnemonic: accounts[0].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					ID:       newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
				{
					ID:       te.accounts[1].ID,
					KeyID:    0,
					Mnemonic: te.accounts[1].Mnemonic,
				},
			},
			notaryAccounts: []identifiers.Identifier{te.accounts[1].ID},
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			prevSequenceID, err := te.moiClient.InteractionCount(context.Background(), &args.InteractionCountArgs{
				ID:    test.senderID,
				KeyID: test.senderKeyID,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			ixHash, err := te.createAssetWithMultiSig(
				test.senderID, test.senderKeyID, test.assetCreatePayload, test.signers, test.notaryAccounts)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			fmt.Println("ix hash", ixHash)

			checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

			sequenceID, err := te.moiClient.InteractionCount(context.Background(), &args.InteractionCountArgs{
				ID:    test.senderID,
				KeyID: test.senderKeyID,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			require.Equal(te.T(), prevSequenceID.ToUint64()+1, sequenceID.ToUint64())
		})
	}
}
