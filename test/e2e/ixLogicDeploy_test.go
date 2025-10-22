package e2e

import (
	"context"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/compute/exlogics/tokenledger"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

var (
	initialSeederAmount = uint64(100000000)
	transferAmount      = uint64(1000000)

	ledgerManifest = func() string {
		path := "./../../compute/exlogics/tokenledger/tokenledger.yaml"

		engineio.RegisterEngine(pisa.NewEngine())

		manifest, err := engineio.NewManifestFromFile(path)
		if err != nil {
			panic(err)
		}

		encoded, err := manifest.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return "0x" + common.BytesToHex(encoded)
	}()

	inputs = tokenledger.InputSeed{
		Symbol: "MOI",
		Supply: 100000000,
	}

	DeployCallData, _ = polo.PolorizeDocument(inputs, polo.DocStructs())
)

func (te *TestEnvironment) logicDeploy(
	acc tests.AccountWithMnemonic,
	logicPayload *common.LogicPayload,
) (common.Hash, error) {
	te.logger.Debug("deploy logic ",
		"sender", acc.ID,
		"call site", logicPayload.Callsite, "call data", logicPayload.Calldata)

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
				Type:    common.IxLogicDeploy,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       acc.ID,
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

	ixHash, err := te.moiClient.SendInteractions(context.Background(), sendIX)
	if err != nil {
		return common.NilHash, err
	}

	te.logger.Debug("Deploy Interaction fired", "ix-hash", ixHash)

	return ixHash, nil
}

// 1. check if receipt generated for ix successfully
// 2. encode expected manifest to be of type JSON
// 3. fetch actual logic manifest of deployed logic
// 4. make sure expected logic manifest and actual logic manifest matches
// 5. fetch ledger state and ensure it matches call data of logic payload
func validateLogicDeploy(
	te *TestEnvironment,
	sender identifiers.Identifier,
	payload *common.LogicPayload,
	txnID int,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	expectedManifest, err := moiclient.GetLogicManifestByEncodingType(te.T(), payload.Manifest, "JSON")
	require.NoError(te.T(), err)

	logicID := moiclient.GetLogicID(te.T(), te.moiClient, txnID, sender, rpcargs.LatestTesseractHeight)

	te.logger.Debug(
		"Validating logic deploy",
		"ixHash", ixHash,
		"call site", payload.Callsite,
		"logic ID", logicID)

	actualManifest, err := te.moiClient.LogicManifest(context.Background(), &rpcargs.LogicManifestArgs{
		LogicID:  logicID.AsIdentifier(),
		Encoding: "JSON",
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})

	require.NoError(te.T(), err)
	require.Equal(te.T(), expectedManifest, actualManifest)

	state := moiclient.GetTokenLedgerState(te.T(), te.moiClient, logicID, []identifiers.Identifier{sender})

	require.Equal(te.T(), "MOI", state.Symbol)
	require.Equal(te.T(), initialSeederAmount, state.Supply.Uint64())

	senderBalance, ok := state.Balances[sender]
	require.True(te.T(), ok)

	require.Equal(te.T(), initialSeederAmount, senderBalance.Uint64())
}

func (te *TestEnvironment) TestLogicDeploy() {
	sender := te.chooseRandomAccount()

	testcases := []struct {
		name         string
		sender       tests.AccountWithMnemonic
		logicPayload *common.LogicPayload
		postTest     func(
			te *TestEnvironment,
			sender identifiers.Identifier,
			payload *common.LogicPayload,
			txnID int,
			ixHash common.Hash,
		)
		checkReceipt  func(t *testing.T, client *moiclient.Client, ixHash common.Hash) *rpcargs.RPCReceipt
		expectedError error
	}{
		{
			name:   "valid ledger logic deploy",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seed",
				Calldata: DeployCallData.Bytes(),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			postTest: validateLogicDeploy,
		},
		{
			name:   "empty manifest",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seed",
				Calldata: common.Hex2Bytes(ledgerManifest),
				Manifest: []byte{},
			},
			expectedError: common.ErrEmptyManifest,
		},
		{
			name:   "empty callsite",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "",
				Calldata: DeployCallData.Bytes(),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			checkReceipt: checkForReceiptSuccess, // TODO check if this can be structured in better way
		},
		{
			name:   "empty call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seed",
				Calldata: make(polo.Document).Bytes(),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			checkReceipt: checkForReceiptFailure,
		},
		{
			name:   "invalid call site",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "random",
				Calldata: DeployCallData.Bytes(),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			checkReceipt: checkForReceiptFailure,
		},
		{
			name:   "invalid call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seed",
				Calldata: []byte{1, 2, 3},
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			checkReceipt: checkForReceiptFailure,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.logicDeploy(
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

			test.postTest(te, test.sender.ID, test.logicPayload, 0, ixHash)
		})
	}
}
