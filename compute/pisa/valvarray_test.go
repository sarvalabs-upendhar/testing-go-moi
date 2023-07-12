package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVarrayValue_Copy(t *testing.T) {
	tests := []struct {
		varray  *VarrayValue
		element RegisterValue
	}{
		{
			must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), StringValue("bar"))),
			StringValue("boo"),
		},
		{
			must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
			StringValue("hoo"),
		},
		{
			must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), nil, StringValue("bar"))),
			StringValue("koo"),
		},
	}

	for _, test := range tests {
		clone := test.varray.Copy().(*VarrayValue) //nolint:forcetypeassert
		require.Equal(t, test.varray, clone)

		except := clone.Append(test.element)
		require.Nil(t, except)
		require.NotEqual(t, test.varray, clone)
	}
}

func TestVarrayValue_Data(t *testing.T) {
	tests := []struct {
		array *VarrayValue
		res   []byte
	}{
		{
			must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), StringValue("bar"))),
			[]byte{0xe, 0x2f, 0x6, 0x36, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72},
		},
		{
			must(newVarrayValue(VarrayDatatype{PrimitiveString}, nil)),
			[]byte{0xe, 0xf},
		},
		{
			must(newVarrayFromValues(VarrayDatatype{PrimitiveString}, StringValue("foo"), nil, StringValue("bar"))),
			[]byte{0xe, 0x2f, 0x6, 0x36, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72},
		},
	}

	for _, test := range tests {
		data := test.array.Data()
		require.Equal(t, test.res, data)
	}
}
