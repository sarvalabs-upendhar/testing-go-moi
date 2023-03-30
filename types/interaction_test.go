package types_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func TestCopyIxInput(t *testing.T) {
	testcases := []struct {
		name  string
		input types.IxInput
	}{
		{
			name:  "IxInput copied successfully",
			input: createInputWithTestData(t),
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
				reflect.ValueOf(expectedIxInput.PerceivedProofs).Pointer(),
				reflect.ValueOf(inputCopy.PerceivedProofs).Pointer(),
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
			compute: createComputeWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedCompute := test.compute

			computeCopy := test.compute.Copy()

			require.Equal(t, expectedCompute, computeCopy)

			require.NotEqual(t,
				reflect.ValueOf(expectedCompute.Hash).Pointer(),
				reflect.ValueOf(computeCopy.Hash).Pointer(),
			)

			require.NotEqual(t,
				reflect.ValueOf(expectedCompute.ComputeNodes).Pointer(),
				reflect.ValueOf(computeCopy.ComputeNodes).Pointer(),
			)
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
			trust: createTrustWithTestData(t),
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
