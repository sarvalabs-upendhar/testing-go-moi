package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArrayValue_Copy(t *testing.T) {
	tests := []struct {
		array  *ArrayValue
		modify RegisterValue
	}{
		{
			must(newArrayFromValues(ArrayDatatype{PrimitiveString, 2}, StringValue("foo"), StringValue("bar"))),
			StringValue("boo"),
		},
		{
			must(newArrayValue(ArrayDatatype{PrimitiveString, 2}, nil)),
			StringValue("hoo"),
		},
		{
			must(newArrayFromValues(ArrayDatatype{PrimitiveString, 3}, StringValue("foo"), nil, StringValue("bar"))),
			StringValue("koo"),
		},
	}

	for _, test := range tests {
		clone := test.array.Copy().(*ArrayValue) //nolint:forcetypeassert
		require.Equal(t, test.array, clone)

		except := clone.Set(U64Value(0), test.modify)
		require.Nil(t, except)
		require.NotEqual(t, test.array, clone)
	}
}

func TestArrayValue_Data(t *testing.T) {
	tests := []struct {
		array *ArrayValue
		res   []byte
	}{
		{
			must(newArrayFromValues(ArrayDatatype{PrimitiveString, 2}, StringValue("foo"), StringValue("bar"))),
			[]byte{0xe, 0x2f, 0x6, 0x36, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72},
		},
		{
			must(newArrayValue(ArrayDatatype{PrimitiveString, 2}, nil)),
			[]byte{0xe, 0x2f, 0x0, 0x0},
		},
		{
			must(newArrayFromValues(ArrayDatatype{PrimitiveString, 3}, StringValue("foo"), nil, StringValue("bar"))),
			[]byte{0xe, 0x3f, 0x6, 0x30, 0x36, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72},
		},
	}

	for _, test := range tests {
		data := test.array.Data()
		require.Equal(t, test.res, data)
	}
}
