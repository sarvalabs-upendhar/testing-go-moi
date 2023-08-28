package e2e

import (
	"context"
	"testing"

	"github.com/pkg/errors"
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
	seeder              = common.HexToAddress("0xffcd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34")
	receiver            = common.HexToAddress("0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c")

	ledgerManifestFile = "./../../compute/manifests/ledger.yaml"
	ledgerManifest     = "0x" + common.BytesToHex(tests.ReadManifest(&testing.T{}, ledgerManifestFile))
	deployCalldata     = "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29ec4" +
		"42dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"
)

func (te *TestEnvironment) deployLogic(
	acc tests.AccountWithMnemonic,
	logicPayload *common.LogicPayload,
) (common.Hash, error) {
	te.logger.Info("deploy logic ",
		"sender", acc.Addr, "manifest", logicPayload.Manifest,
		"call site", logicPayload.Callsite, "call data", logicPayload.Calldata)

	payload, err := logicPayload.Bytes()
	te.Suite.NoError(err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxLogicDeploy,
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, acc.Addr),
		Sender:    acc.Addr,
		FuelPrice: DefaulFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Payload:   payload,
	}

	sendIX := moiclient.CreateSendIXFromSendIXArgs(te.T(), sendIXArgs, acc.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. encode expected manifest to be of type JSON
// 3. fetch actual logic manifest of deployed logic
// 4. make sure expected logic manifest and actual logic manifest matches
// 5. fetch ledger state and ensure it matches call data of logic payload
func validateTokenLedgerLogicDeploy(
	te *TestEnvironment,
	sender common.Address,
	payload *common.LogicPayload,
	ixHash common.Hash,
) {
	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	expectedManifest, err := moiclient.GetLogicManifestByEncodingType(te.T(), payload.Manifest, "JSON")
	require.NoError(te.T(), err)

	logicID := moiclient.GetLogicID(te.T(), te.moiClient, sender, rpcargs.LatestTesseractHeight)

	actualManifest, err := te.moiClient.LogicManifest(context.Background(), &rpcargs.LogicManifestArgs{
		LogicID:  logicID,
		Encoding: "JSON",
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	require.NoError(te.T(), err)

	require.Equal(te.T(), expectedManifest, actualManifest)

	state := moiclient.GetTokenLedgerState(te.T(), te.moiClient, logicID)

	require.Equal(te.T(), "MOI-Token", state.Name)
	require.Equal(te.T(), "MOI", state.Symbol)
	require.Equal(te.T(), initialSeederAmount, state.Supply.Uint64())

	senderBalance, ok := state.Balances[seeder]
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
			sender common.Address,
			payload *common.LogicPayload,
			ixHash common.Hash,
		)
		checkReceiptSuccess func(t *testing.T, client *moiclient.Client, ixHash common.Hash) *rpcargs.RPCReceipt
		expectedError       error
	}{
		{
			name:   "valid logic deploy",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seeder!",
				Calldata: common.Hex2Bytes(deployCalldata),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			postTest: validateTokenLedgerLogicDeploy,
		},
		{
			name:   "empty manifest",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seeder!",
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
				Calldata: common.Hex2Bytes(deployCalldata),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			checkReceiptSuccess: checkForReceiptSuccess, // TODO check if this can be structured in better way
		},
		{
			name:   "empty call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seeder!",
				Calldata: make(polo.Document).Bytes(),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			expectedError: errors.New("failed to validate logic deploy"),
		},
		{
			name:   "invalid call site",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "random!",
				Calldata: common.Hex2Bytes(deployCalldata),
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			expectedError: errors.New("failed to validate logic deploy"),
		},
		{
			name:   "invalid call data",
			sender: sender,
			logicPayload: &common.LogicPayload{
				Callsite: "Seeder!",
				Calldata: []byte{1, 2, 3},
				Manifest: common.Hex2Bytes(ledgerManifest),
			},
			expectedError: errors.New("failed to validate logic deploy"),
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.deployLogic(
				test.sender,
				test.logicPayload,
			)

			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}
			require.NoError(te.T(), err)

			if test.checkReceiptSuccess != nil {
				test.checkReceiptSuccess(te.T(), te.moiClient, ixHash)

				return
			}

			test.postTest(te, test.sender.Addr, test.logicPayload, ixHash)
		})
	}
}
