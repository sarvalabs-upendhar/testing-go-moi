package pisa

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstantValue(t *testing.T) {
	tests := []struct {
		constant Constant
		value    RegisterValue
		except   *Exception
	}{
		{
			constant: Constant{Type: PrimitiveString, Data: must(polo.Polorize("hello!"))},
			value:    StringValue("hello!"),
			except:   nil,
		},
		{
			constant: Constant{Type: PrimitiveAddress, Data: must(polo.Polorize([32]byte{}))},
			value:    AddressValue{},
			except:   nil,
		},
		{
			constant: Constant{Type: PrimitiveU64, Data: []byte{0x6, 0x65}},
			value:    nil,
			except:   exception(ValueError, "malformed constant: data does not decode to a uint64"),
		},
	}

	for _, test := range tests {
		value, except := test.constant.value()

		require.Equal(t, test.except, except)
		require.Equal(t, test.value, value)
	}
}

func TestPtrValue(t *testing.T) {
	// Create a new PtrValue
	ptr := PtrValue(12345)

	// Test Type()
	assert.Equal(t, TypePtr, ptr.Type(), "PtrValue Type should be TypePtr")

	// Test Copy()
	clone := ptr.Copy()
	assert.Equal(t, ptr, clone, "Copy of PtrValue should be equal to original")

	// Test Norm()
	norm := ptr.Norm()
	assert.Equal(t, uint64(ptr), norm, "Normalized value of PtrValue should be equal to uint64 value of original")

	// Test Data()
	data := ptr.Data()
	expectedData := []byte{0x3, 0x30, 0x39}
	assert.Equal(t, expectedData, data, "POLO encoded bytes of PtrValue should match expected value")
}

func TestNullValue(t *testing.T) {
	// Test Type method
	assert.Equal(t, TypeNull, NullValue{}.Type(), "Type method should return TypeNull")

	// Test Copy method
	nullVal := NullValue{}
	copyVal := nullVal.Copy().(NullValue) //nolint:forcetypeassert
	assert.Equal(t, nullVal, copyVal, "Copy method should return a new NullValue instance that is equal to the original")

	// Test Norm method
	assert.Nil(t, NullValue{}.Norm(), "Norm method should return nil for NullValue")

	// Test Data method
	assert.Equal(t, []byte{0}, NullValue{}.Data(), "Data method should return []byte{0} for NullValue")
}

func TestIsNullValue(t *testing.T) {
	// Test with a TypeNull value
	assert.True(t, IsNullValue(NullValue{}))

	// Test with a non-TypeNull value
	assert.False(t, IsNullValue(PtrValue(12345)))

	// Test with a non-TypeNull value
	assert.False(t, IsNullValue(StringValue("hello")))
}
