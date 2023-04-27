package engineio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func TestIxnObject(t *testing.T) {
	// Test NewIxnObject function
	kind := types.IxLogicInvoke
	callsite := "TestCallsite"
	calldata := []byte{1, 2, 3}

	ixnObj := NewIxnObject(kind, callsite, calldata)
	require.NotNil(t, ixnObj, "NewIxnObject should return a non-nil object")

	// Test IxType function
	assert.Equal(t, kind, ixnObj.IxType(), "IxType function should return the correct IxType")

	// Test Callsite function
	assert.Equal(t, callsite, ixnObj.Callsite(), "Callsite function should return the correct callsite")

	// Test Calldata function
	assert.Equal(t, calldata, ixnObj.Calldata(), "Calldata function should return the correct calldata")
}

func TestEnvDriver(t *testing.T) {
	// Test NewEnvDriver function
	envDriver := NewEnvDriver()
	assert.Nil(t, envDriver, "NewEnvDriver should return a nil object")
}
