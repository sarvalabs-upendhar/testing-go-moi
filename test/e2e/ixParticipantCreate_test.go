package e2e

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

func (te *TestEnvironment) createParticipant(
	sender tests.AccountWithMnemonic,
	participantCreatePayload *common.ParticipantCreatePayload,
) (common.Hash, error) {
	te.logger.Debug("register participant ", "sender", sender.ID,
		"address", participantCreatePayload.ID, "amount", participantCreatePayload.Amount,
	)

	payload, err := participantCreatePayload.Bytes()
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
				Type:    common.IxParticipantCreate,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       sender.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       participantCreatePayload.ID,
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

func checkForKeys(t *testing.T, keysPayload []common.KeyAddPayload, rpcAccountKeys []args.RPCAccountKey) {
	t.Helper()

	require.Equal(t, len(keysPayload), len(rpcAccountKeys))

	for _, key := range keysPayload {
		found := false

		for _, rpcAccountKey := range rpcAccountKeys {
			if bytes.Equal(rpcAccountKey.PublicKey, key.PublicKey) {
				require.Equal(t, key.Weight, rpcAccountKey.Weight.ToUint64())

				found = true

				break
			}
		}

		require.True(t, found)
	}
}

func validateParticipantCreate(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload *common.ParticipantCreatePayload,
	ixHash common.Hash,
) {
	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, common.KMOITokenAssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, common.KMOITokenAssetID, args.LatestTesseractHeight)

	receiverCurBal := getBalance(te, payload.ID, common.KMOITokenAssetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64()+receipt.FuelUsed.ToUint64(), senderPrevBal-senderCurBal)
	require.Equal(te.T(), payload.Amount.Uint64(), receiverCurBal)

	accountKeys, err := te.moiClient.AccountKeys(context.Background(), &args.GetAccountKeysArgs{
		ID: payload.ID,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	require.NoError(te.T(), err)

	checkForKeys(te.T(), payload.KeysPayload, accountKeys)
}

func (te *TestEnvironment) TestParticipantCreate() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	receiver := accs[1]

	id := tests.RandomIdentifierWithZeroVariant(te.T())

	testcases := []struct {
		name                     string
		sender                   tests.AccountWithMnemonic
		participantCreatePayload *common.ParticipantCreatePayload
		postTest                 func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			payload *common.ParticipantCreatePayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "participant registered successfully",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				ID: id,
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          id.Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(10),
			},
			postTest: validateParticipantCreate,
		},
		{
			name:   "register participants with multiple keys",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				ID: tests.RandomIdentifierWithZeroVariant(te.T()),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          id.Bytes(),
						Weight:             200,
						SignatureAlgorithm: 0,
					},
					{
						PublicKey:          tests.RandomIdentifierWithZeroVariant(te.T()).Bytes(),
						Weight:             800,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(10),
			},
			postTest: validateParticipantCreate,
		},
		{
			name:   "invalid weight of keys",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				ID: id,
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          id.Bytes(),
						Weight:             600,
						SignatureAlgorithm: 0,
					},
					{
						PublicKey:          tests.RandomIdentifierWithZeroVariant(te.T()).Bytes(),
						Weight:             299,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(10),
			},
			expectedError: common.ErrInvalidWeight,
		},
		{
			name:   "insufficient funds",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				ID: receiver.ID,
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          receiver.ID.Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(1000000000000),
			},
			expectedError: common.ErrInsufficientFunds,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.createParticipant(test.sender, test.participantCreatePayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, test.participantCreatePayload, ixHash)
		})
	}
}
