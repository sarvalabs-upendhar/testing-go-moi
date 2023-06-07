package types_test

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common/tests"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func createTestIXInput(t *testing.T, ixType types.IxType, payload []byte) types.IxInput {
	t.Helper()

	return types.IxInput{
		Type:     ixType,
		Nonce:    4,
		Sender:   tests.RandomAddress(t),
		Receiver: tests.RandomAddress(t),
		Payer:    tests.RandomAddress(t),
		TransferValues: map[types.AssetID]*big.Int{
			tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
			tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
		},
		PerceivedValues: map[types.AssetID]*big.Int{
			tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(99),
			tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(111),
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(23),
		Payload:   payload,
	}
}

func TestNewInteraction(t *testing.T) {
	assetPayload := types.AssetCreatePayload{
		Type: types.AssetKindFile,
	}

	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	logicPayload := types.LogicPayload{
		Callsite: "hello",
	}

	rawLogicPayload, err := logicPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ixInput     types.IxInput
		sign        []byte
		expectedIX  *types.Interaction
		expectedErr error
	}{
		{
			name:    "value transfer ix",
			ixInput: createTestIXInput(t, types.IxValueTransfer, nil),
			sign:    []byte{1, 2, 3},
		},
		{
			name:    "asset create ix",
			ixInput: createTestIXInput(t, types.IxAssetCreate, rawAssetPayload),
			sign:    []byte{1, 2, 3},
		},
		{
			name:    "deploy logic ix",
			ixInput: createTestIXInput(t, types.IxLogicDeploy, rawLogicPayload),
			sign:    []byte{1, 2, 3},
		},
		{
			name:    "invoke logic ix",
			ixInput: createTestIXInput(t, types.IxLogicInvoke, rawLogicPayload),
			sign:    []byte{1, 2, 3},
		},
		{
			name:        "invalid ix",
			ixInput:     createTestIXInput(t, types.IxInvalid, rawLogicPayload),
			sign:        []byte{1, 2, 3},
			expectedErr: types.ErrInvalidInteractionType,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixData := types.IxData{
				Input: test.ixInput,
			}

			ix, err := types.NewInteraction(ixData, test.sign)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)

			// check if ix data copied properly
			require.Equal(t, test.ixInput.Type, ix.Type())
			require.Equal(t, test.ixInput.Nonce, ix.Nonce())
			require.Equal(t, test.ixInput.Sender, ix.Sender())
			require.Equal(t, test.ixInput.Payer, ix.Payer())
			require.Equal(t, test.ixInput.TransferValues, ix.TransferValues())
			require.Equal(t, test.ixInput.PerceivedValues, ix.PerceivedValues())
			require.Equal(t, test.ixInput.FuelPrice, ix.FuelPrice())
			require.Equal(t, test.ixInput.FuelLimit, ix.FuelLimit())
			require.Equal(t, test.ixInput.Payload, ix.Payload())

			if test.ixInput.Type == types.IxValueTransfer {
				require.Equal(t, test.ixInput.Receiver, ix.Receiver())
			}

			require.Equal(t, test.sign, ix.Signature())

			data, err := polo.Polorize(ixData)
			require.NoError(t, err)

			require.Equal(t, types.GetHash(data), ix.Hash())

			size, err := ix.Size()
			require.NoError(t, err)

			require.Equal(t, uint64(len(data)+len(ix.Signature())), size)

			// check for payload
			if test.ixInput.Type == types.IxAssetCreate {
				payload, err := ix.GetAssetPayload()
				require.NoError(t, err)

				require.Equal(t, assetPayload, *payload.Create)
			}

			if test.ixInput.Type == types.IxLogicDeploy || test.ixInput.Type == types.IxLogicInvoke {
				payload, err := ix.GetLogicPayload()
				require.NoError(t, err)

				require.Equal(t, logicPayload, *payload)
			}
		})
	}
}

func TestCopyIxInput(t *testing.T) {
	testcases := []struct {
		name  string
		input types.IxInput
	}{
		{
			name:  "IxInput copied successfully",
			input: tests.CreateIXInputWithTestData(t, types.IxAssetCreate, []byte{187, 1, 29, 103}, []byte{187, 1, 29, 103}),
		},
		{
			name:  "copy ix input with nil perceived proofs ",
			input: tests.CreateIXInputWithTestData(t, types.IxAssetCreate, []byte{187, 1, 29, 103}, nil),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedIxInput := test.input

			inputCopy := test.input.Copy()

			require.Equal(t, expectedIxInput, inputCopy)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxInput.TransferValues).Pointer(),
				reflect.ValueOf(inputCopy.TransferValues).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxInput.PerceivedValues).Pointer(),
				reflect.ValueOf(inputCopy.PerceivedValues).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxInput.FuelLimit).Pointer(),
				reflect.ValueOf(inputCopy.FuelLimit).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(test.input.FuelPrice).Pointer(),
				reflect.ValueOf(inputCopy.FuelPrice).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxInput.Payload).Pointer(),
				reflect.ValueOf(inputCopy.Payload).Pointer(),
			)

			if test.input.PerceivedProofs != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedIxInput.PerceivedProofs).Pointer(),
					reflect.ValueOf(inputCopy.PerceivedProofs).Pointer(),
				)
			}
		})
	}
}

func TestCopyIxCompute(t *testing.T) {
	testcases := []struct {
		name    string
		compute types.IxCompute
	}{
		{
			name:    "IxCompute copied successfully",
			compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.GetTestKramaIDs(t, 2)),
		},
		{
			name:    "copy ix compute with nil hash and zero nodes",
			compute: tests.CreateComputeWithTestData(t, types.NilHash, nil),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedCompute := test.compute

			computeCopy := test.compute.Copy()

			require.Equal(t, expectedCompute, computeCopy)

			if len(test.compute.ComputeNodes) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(expectedCompute.ComputeNodes).Pointer(),
					reflect.ValueOf(computeCopy.ComputeNodes).Pointer(),
				)
			}
		})
	}
}

func TestCopyIxTrust(t *testing.T) {
	testcases := []struct {
		name  string
		trust types.IxTrust
	}{
		{
			name:  "IxTrust copied successfully",
			trust: tests.CreateTrustWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedTrust := test.trust

			trustCopy := test.trust.Copy()

			require.Equal(t, expectedTrust, trustCopy)

			require.NotEqual(t,
				reflect.ValueOf(expectedTrust.TrustNodes).Pointer(),
				reflect.ValueOf(trustCopy.TrustNodes).Pointer(),
			)
		})
	}
}

func TestCopyIxData(t *testing.T) {
	testcases := []struct {
		name   string
		ixData types.IxData
	}{
		{
			name:   "IxData object copied successfully",
			ixData: createIxDataWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedIxData := test.ixData

			dataCopy := test.ixData.Copy()

			require.Equal(t, expectedIxData, dataCopy)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxData.Input.FuelLimit).Pointer(),
				reflect.ValueOf(dataCopy.Input.FuelLimit).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxData.Input.TransferValues).Pointer(),
				reflect.ValueOf(dataCopy.Input.TransferValues).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxData.Compute.ComputeNodes).Pointer(),
				reflect.ValueOf(dataCopy.Compute.ComputeNodes).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedIxData.Trust.TrustNodes).Pointer(),
				reflect.ValueOf(dataCopy.Trust.TrustNodes).Pointer(),
			)
		})
	}
}
