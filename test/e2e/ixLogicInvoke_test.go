package e2e

import (
	"context"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

func (te *TestEnvironment) logicInvoke(
	acc tests.AccountWithMnemonic,
	logicPayload *common.LogicPayload,
) (common.Hash, error) {
	te.logger.Debug("invoke logic ",
		"sender", acc.ID,
		"logicID", logicPayload.LogicID,
		"callsite", logicPayload.Callsite,
		"calldata", logicPayload.Calldata,
	)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         acc.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, acc.ID, 0),
		},
		FuelPrice:    DefaultFuelPrice,
		FuelLimit:    DefaultFuelLimit,
		IxOps:        []common.IxOpRaw{},
		Participants: []common.IxParticipant{},
	}

	tests.AddIxOp(te.T(), ixData, common.IxLogicInvoke, common.KMOITokenAssetID, logicPayload)

	tests.AppendDefaultParticipants(te.T(), ixData)

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       acc.ID,
			KeyID:    0,
			Mnemonic: acc.Mnemonic,
		},
	})

	ixHash, err := te.moiClient.SendInteractions(context.Background(), sendIX)
	if err != nil {
		return common.NilHash, err
	}

	te.logger.Debug("Logic Invoked fired", "ix-hash", ixHash)

	return ixHash, nil
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

	state := moiclient.GetTokenLedgerState(
		te.T(), te.moiClient,
		payload.LogicID, []identifiers.Identifier{sender, receiver},
	)
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

	ixHash, err := te.logicDeploy(
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
		checkReceipt  func(t *testing.T, client *moiclient.Client, ixHash common.Hash) *args.RPCReceipt
	}{
		{
			name:   "valid logic invoke",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  ledgerLogicID,
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			},
			postTest: validateLogicInvoke,
		},
		{
			name:   "invalid logic id",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  identifiers.Nil,
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			},
			expectedError: common.ErrInvalidIdentifier,
		},
		{
			name:   "empty call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  ledgerLogicID,
				Callsite: "Transfer",
				Calldata: make(polo.Document).Bytes(),
			},
			checkReceipt: checkForReceiptFailure,
		},
		{
			name:   "empty callsite",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  ledgerLogicID,
				Callsite: "",
				Calldata: invokeCalldata,
			},
			expectedError: common.ErrInvalidCallSite,
		},
		{
			name:   "logic isn't registered",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  identifiers.RandomLogicIDv0(),
				Callsite: "Transfer",
				Calldata: invokeCalldata,
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name:   "invalid callsite",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  ledgerLogicID,
				Callsite: "abcd",
				Calldata: []byte{},
			},
			checkReceipt: checkForReceiptFailure,
		},
		{
			name:   "invalid call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				LogicID:  ledgerLogicID,
				Callsite: "Transfer",
				Calldata: []byte{1, 2, 3},
			},
			checkReceipt: checkForReceiptFailure,
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

			if test.checkReceipt != nil {
				test.checkReceipt(te.T(), te.moiClient, ixHash)

				return
			}

			test.postTest(te, test.sender.ID, receiver, test.logicPayload, ixHash)
		})
	}
}
