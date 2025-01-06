package e2e

import (
	"context"
	"fmt"
	"math/big"

	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/moiclient"
)

// TODO Decide how to throw errors , use suite or require package
func (te *TestEnvironment) createAssetWithMultiSig(
	senderAddr identifiers.Address,
	senderKeyID uint64,
	assetCreatePayload *common.AssetCreatePayload,
	accountKeys []moiclient.AccountKeyWithMnemonic,
	notaryAccount []identifiers.Address,
) (common.Hash, error) {
	te.logger.Debug("create asset ",
		"sender", senderAddr, "symbol", assetCreatePayload.Symbol, "supply", assetCreatePayload.Supply)

	payload, err := assetCreatePayload.Bytes()
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
				Type:    common.IxAssetCreate,
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

	for _, addr := range notaryAccount {
		ixData.Participants = append(ixData.Participants, common.IxParticipant{
			Address:  addr,
			LockType: common.MutateLock,
			Notary:   true,
		})
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, accountKeys)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func (te *TestEnvironment) TestMultiSig() {
	newAcc := tests.RandomAddress(te.T())
	acc := te.chooseRandomAccount()

	accounts, err := cmdCommon.GetAccountsWithMnemonic(4)
	require.NoError(te.T(), err)

	createParticipant(te, acc, &common.ParticipantCreatePayload{
		Address: newAcc,
		KeysPayload: []common.KeyAddPayload{
			{
				PublicKey: accounts[0].Addr.Bytes(),
				Weight:    400,
			},
			{
				PublicKey: accounts[1].Addr.Bytes(),
				Weight:    400,
			},
			{
				PublicKey: accounts[2].Addr.Bytes(),
				Weight:    200,
			},
			{
				PublicKey: accounts[3].Addr.Bytes(),
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
		senderAddr         identifiers.Address
		senderKeyID        uint64
		assetCreatePayload *common.AssetCreatePayload
		signers            []moiclient.AccountKeyWithMnemonic
		notaryAccounts     []identifiers.Address
		expectedError      error
	}{
		{
			name:               "finalized ixn with multisig successfully",
			senderAddr:         newAcc,
			senderKeyID:        2,
			assetCreatePayload: assetCreatePayload,
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					Addr:     newAcc,
					KeyID:    0,
					Mnemonic: accounts[0].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
			},
		},
		{
			name:               "sender key signature not found",
			senderAddr:         newAcc,
			senderKeyID:        0,
			assetCreatePayload: assetCreatePayload,
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					Addr:     newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    3,
					Mnemonic: accounts[3].Mnemonic,
				},
			},
			expectedError: common.ErrInvalidSenderSignature,
		},
		{
			name:               "notary participant signature not found",
			senderAddr:         newAcc,
			senderKeyID:        1,
			assetCreatePayload: assetCreatePayload,
			signers: []moiclient.AccountKeyWithMnemonic{
				{
					Addr:     newAcc,
					KeyID:    0,
					Mnemonic: accounts[0].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
			},
			notaryAccounts: []identifiers.Address{te.accounts[1].Addr},
			expectedError:  errors.New("invalid notary participant signature"),
		},
		{
			name:        "finalize ixn with notary signature successfully",
			senderAddr:  newAcc,
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
					Addr:     newAcc,
					KeyID:    0,
					Mnemonic: accounts[0].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    1,
					Mnemonic: accounts[1].Mnemonic,
				},
				{
					Addr:     newAcc,
					KeyID:    2,
					Mnemonic: accounts[2].Mnemonic,
				},
				{
					Addr:     te.accounts[1].Addr,
					KeyID:    0,
					Mnemonic: te.accounts[1].Mnemonic,
				},
			},
			notaryAccounts: []identifiers.Address{te.accounts[1].Addr},
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			prevSequenceID, err := te.moiClient.InteractionCount(context.Background(), &args.InteractionCountArgs{
				Address: test.senderAddr,
				KeyID:   test.senderKeyID,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			ixHash, err := te.createAssetWithMultiSig(
				test.senderAddr, test.senderKeyID, test.assetCreatePayload, test.signers, test.notaryAccounts)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			fmt.Println("ix hash", ixHash)

			checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

			sequenceID, err := te.moiClient.InteractionCount(context.Background(), &args.InteractionCountArgs{
				Address: test.senderAddr,
				KeyID:   test.senderKeyID,
				Options: args.TesseractNumberOrHash{
					TesseractNumber: &args.LatestTesseractHeight,
				},
			})
			require.NoError(te.T(), err)

			require.Equal(te.T(), prevSequenceID.ToUint64()+1, sequenceID.ToUint64())
		})
	}
}
