package e2e

import (
	"context"
	"math/big"

	"github.com/sarvalabs/go-moi-identifiers"
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
	te.logger.Debug("register participant ", "sender", sender.Addr,
		"address", participantCreatePayload.Address, "amount", participantCreatePayload.Amount,
	)

	payload, err := participantCreatePayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, sender.Addr),
		Sender:    sender.Addr,
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
				Address:  sender.Addr,
				LockType: common.MutateLock,
			},
			{
				Address:  participantCreatePayload.Address,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, sender.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

func validateParticipantCreate(
	te *TestEnvironment,
	sender identifiers.Address,
	payload *common.ParticipantCreatePayload,
	ixHash common.Hash,
) {
	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	senderHeight := moiclient.GetLatestHeight(te.T(), te.moiClient, sender)

	senderPrevBal := getBalance(te, sender, common.KMOITokenAssetID, int64(senderHeight-1))
	senderCurBal := getBalance(te, sender, common.KMOITokenAssetID, args.LatestTesseractHeight)

	receiverCurBal := getBalance(te, payload.Address, common.KMOITokenAssetID, args.LatestTesseractHeight)

	require.Equal(te.T(), payload.Amount.Uint64()+receipt.FuelUsed.ToUint64(), senderPrevBal-senderCurBal)
	require.Equal(te.T(), payload.Amount.Uint64(), receiverCurBal)
}

func (te *TestEnvironment) TestParticipantCreate() {
	accs, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender := accs[0]
	receiver := accs[1]

	addr := tests.RandomAddress(te.T())

	testcases := []struct {
		name                     string
		sender                   tests.AccountWithMnemonic
		participantCreatePayload *common.ParticipantCreatePayload
		postTest                 func(
			te *TestEnvironment,
			sender identifiers.Address,
			payload *common.ParticipantCreatePayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "participant registered successfully",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				Address: addr,
				Amount:  big.NewInt(10),
			},
			postTest: validateParticipantCreate,
		},
		{
			name:   "participant already registered",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				Address: receiver.Addr,
				Amount:  big.NewInt(1),
			},
			expectedError: common.ErrAlreadyRegistered,
		},
		{
			name:   "insufficient funds",
			sender: sender,
			participantCreatePayload: &common.ParticipantCreatePayload{
				Address: receiver.Addr,
				Amount:  big.NewInt(1000000000000),
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

			test.postTest(te, test.sender.Addr, test.participantCreatePayload, ixHash)
		})
	}
}
