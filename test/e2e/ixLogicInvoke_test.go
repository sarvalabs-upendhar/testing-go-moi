package e2e

import (
	"context"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
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
		"sender", acc.Addr,
		"logicID", logicPayload.Logic,
		"callsite", logicPayload.Callsite,
		"calldata", logicPayload.Calldata,
	)

	payload, err := logicPayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, acc.Addr),
		Sender:    acc.Addr,
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
				Address:  acc.Addr,
				LockType: common.MutateLock,
			},
			{
				Address:  logicPayload.Logic.Address(),
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, acc.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. fetch ledger state of logic
// 3. Ensure the sender's balance is decreased by the transfer amount.
// 4. Ensure the receiver's balance is increased by the transfer amount.
func validateLogicInvoke(
	te *TestEnvironment,
	sender identifiers.Address,
	receiver identifiers.Address,
	payload *common.LogicPayload,
	ixHash common.Hash,
) {
	// make sure interaction executed successfully
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	state := moiclient.GetTokenLedgerState(te.T(), te.moiClient, payload.Logic, []identifiers.Address{sender, receiver})
	senderBalance, ok := state.Balances[sender]
	require.True(te.T(), ok)

	require.Equal(te.T(), initialSeederAmount-transferAmount, senderBalance.Uint64())

	receiverBalance, ok := state.Balances[receiver]
	require.True(te.T(), ok)

	require.Equal(te.T(), transferAmount, receiverBalance.Uint64())
}

func (te *TestEnvironment) TestLogicInvoke() {
	sender := te.chooseRandomAccount()
	receiver, _ := identifiers.NewAddressFromHex("0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c")

	invokeCalldata := "0x0d6f0665a601a502616d6f756e74030f42407265636569766572060fafe52ec42a85db6" +
		"44d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c"

	ixHash, err := te.deployLogic(
		sender,
		&common.LogicPayload{
			Callsite: "Seed",
			Calldata: common.Hex2Bytes(deployCalldata),
			Manifest: common.Hex2Bytes(ledgerManifest),
		},
	)
	require.NoError(te.T(), err)

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	ledgerLogicID := moiclient.GetLogicID(te.T(), te.moiClient, 0, sender.Addr, args.LatestTesseractHeight)

	testcases := []struct {
		name         string
		sender       tests.AccountWithMnemonic
		logicPayload *common.LogicPayload
		postTest     func(
			te *TestEnvironment,
			sender identifiers.Address,
			receiver identifiers.Address,
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
				Calldata: hexutil.Bytes(common.Hex2Bytes(invokeCalldata)),
			},
			postTest: validateLogicInvoke,
		},
		{
			name:   "empty logic id",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic:    "",
				Callsite: "Transfer",
				Calldata: hexutil.Bytes(common.Hex2Bytes(invokeCalldata)),
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
				Calldata: common.Hex2Bytes(invokeCalldata),
			},
			expectedError: common.ErrEmptyCallSite,
		},
		{
			name:   "logic isn't registered",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Logic: identifiers.NewLogicIDv0(
					true,
					false,
					false,
					false,
					0,
					tests.RandomAddress(te.T()),
				),
				Callsite: "Transfer",
				Calldata: common.Hex2Bytes(invokeCalldata),
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

			test.postTest(te, test.sender.Addr, receiver, test.logicPayload, ixHash)
		})
	}
}
