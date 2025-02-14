package e2e

import (
	"context"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

//nolint:dupl
func (te *TestEnvironment) logicInvoke(
	acc tests.AccountWithMnemonic,
	logicPayload *common.LogicPayload,
) (common.Hash, error) {
	te.logger.Debug("invoke logic ",
		"sender", acc.ID,
		"logicID", logicPayload.Logic,
		"callsite", logicPayload.Callsite,
		"calldata", logicPayload.Calldata,
	)

	payload, err := logicPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         acc.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, acc.ID, 0),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxLogicInvoke,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       acc.ID,
				LockType: common.MutateLock,
			},
			{
				ID:       logicPayload.Logic.AsIdentifier(),
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       acc.ID,
			KeyID:    0,
			Mnemonic: acc.Mnemonic,
		},
	})

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. fetch ledger state of logic
// 3. Ensure the sender's balance is decreased by the transfer amount.
// 4. Ensure the receiver's balance is increased by the transfer amount.
func validateLogicInvoke(
	te *TestEnvironment,
	sender identifiers.Identifier,
	receiver identifiers.Identifier,
	payload *common.LogicPayload,
	ixHash common.Hash,
) {
	// make sure interaction executed successfully
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	state := moiclient.GetTokenLedgerState(te.T(), te.moiClient, payload.Logic, []identifiers.Identifier{sender, receiver})
	senderBalance, ok := state.Balances[sender]
	require.True(te.T(), ok)

	require.Equal(te.T(), initialSeederAmount-transferAmount, senderBalance.Uint64())

	receiverBalance, ok := state.Balances[receiver]
	require.True(te.T(), ok)

	require.Equal(te.T(), transferAmount, receiverBalance.Uint64())
}

func (te *TestEnvironment) TestLogicInvoke() {
	sender := te.chooseRandomAccount()
	receiver := identifiers.RandomParticipantIDv0().AsIdentifier()

	invokeCalldata := DocGen(te.T(), map[string]any{
		"amount":   transferAmount,
		"receiver": receiver,
	}).Bytes()

	ixHash, err := te.deployLogic(
		sender,
		&common.LogicPayload{
			Callsite: "Seed",
			Calldata: DeployCallData.Bytes(),
			Manifest: common.Hex2Bytes(ledgerManifest),
		},
	)
	require.NoError(te.T(), err)

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	ledgerLogicID := moiclient.GetLogicID(te.T(), te.moiClient, 0, sender.ID, args.LatestTesseractHeight)

	testcases := []struct {
		name         string
		sender       tests.AccountWithMnemonic
		logicPayload *common.LogicPayload
		postTest     func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			receiver identifiers.Identifier,
			payload *common.LogicPayload,
			ixHash common.Hash,
		)
		expectedError error
	}{
		{
			name:   "valid logic invoke",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    ledgerLogicID,
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			},
			postTest: validateLogicInvoke,
		},
		{
			name:   "empty logic id",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    identifiers.Nil,
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			},
			expectedError: common.ErrMissingLogicID,
		},
		{
			name:   "empty call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    ledgerLogicID,
				Callsite: "Transfer",
				Calldata: make(polo.Document).Bytes(),
			},
			expectedError: errors.New("failed to validate logic invoke"),
		},
		{
			name:   "empty callsite",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    ledgerLogicID,
				Callsite: "",
				Calldata: invokeCalldata,
			},
			expectedError: common.ErrEmptyCallSite,
		},
		{
			name:   "logic isn't registered",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    identifiers.RandomLogicIDv0(),
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name:   "invalid callsite",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    ledgerLogicID,
				Callsite: "abcd",
				Calldata: []byte{},
			},
			expectedError: errors.New("failed to validate logic invoke"),
		},
		{
			name:   "invalid call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    ledgerLogicID,
				Callsite: "Transfer",
				Calldata: []byte{1, 2, 3},
			},
			expectedError: errors.New("failed to validate logic invoke"),
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.logicInvoke(
				test.sender,
				test.logicPayload,
			)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)

			test.postTest(te, test.sender.ID, receiver, test.logicPayload, ixHash)
		})
	}
}
