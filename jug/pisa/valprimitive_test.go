package pisa

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/moichain/types"
)

func TestBoolValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new BoolValue
		value := BoolValue(true)

		// Test Type()
		assert.Equal(t, TypeBool, value.Type(), "BoolValue Type should be TypeBool")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of BoolValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, true, norm, "Normalized value of BoolValue should be equal to bool value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{0x2}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of BoolValue should match expected value")
	})

	t.Run("Helpers", func(t *testing.T) {
		// Create a new BoolValue
		value := BoolValue(true)

		// Test And()
		assert.False(t, bool(value.And(false)))
		assert.True(t, bool(value.And(true)))

		// Test Or()
		assert.True(t, bool(value.Or(false)))
		assert.True(t, bool(value.Or(true)))

		// Test Not()
		assert.False(t, bool(value.Not()))
	})
}

func TestStringValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new StringValue
		value := StringValue("foobar")

		// Test Type()
		assert.Equal(t, TypeString, value.Type(), "StringValue Type should be TypeString")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of StringValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, "foobar", norm, "Normalized value of StringValue should be equal to string value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{0x6, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of StringValue should match expected value")
	})

	t.Run("Helpers", func(t *testing.T) {
		// Create a new BoolValue
		value := StringValue("boofar")
		value2 := StringValue("-")

		// Test Concat()
		assert.Equal(t, StringValue("boofar-boofar"), value.Concat(value2).Concat(value))

		// Test HasPrefix()
		prefix1 := StringValue("boo")
		assert.True(t, bool(value.HasPrefix(prefix1)))
		prefix2 := StringValue("hello")
		assert.False(t, bool(value.HasPrefix(prefix2)))
	})
}

func TestBytesValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new BytesValue
		value := BytesValue{0x10, 0x20, 0x30}

		// Test Type()
		assert.Equal(t, TypeBytes, value.Type(), "BytesValue Type should be TypeBytes")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of BytesValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, []byte{0x10, 0x20, 0x30}, norm,
			"Normalized value of BytesValue should be equal to string value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{0x6, 0x10, 0x20, 0x30}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of BytesValue should match expected value")
	})
}

func TestAddressValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new AddressValue
		value := AddressValue{0x10}

		// Test Type()
		assert.Equal(t, TypeAddress, value.Type(), "AddressValue Type should be TypeAddress")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of AddressValue should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, [32]byte{0x10}, norm,
			"Normalized value of AddressValue should be equal to [32]byte value of original")

		// Test Data()
		data := value.Data()
		expectedData := []byte{
			0x6, 0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		}
		assert.Equal(t, expectedData, data, "POLO encoded bytes of AddressValue should match expected value")
	})
}

func randomAddressValue(t *testing.T) AddressValue {
	t.Helper()

	address := make([]byte, 32)
	_, _ = rand.Read(address)

	return AddressValue(types.BytesToAddress(address))
}
