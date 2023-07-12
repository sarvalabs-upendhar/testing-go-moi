package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type mockInvalidDatatype struct{}

func (m mockInvalidDatatype) Kind() DatatypeKind     { return DatatypeKind(10) }
func (m mockInvalidDatatype) Copy() Datatype         { panic("do not implement") }
func (m mockInvalidDatatype) String() string         { panic("do not implement") }
func (m mockInvalidDatatype) Equals(_ Datatype) bool { panic("do not implement") }

func TestDatatypeSerialization(t *testing.T) {
	tests := []Datatype{
		PrimitiveBool,
		PrimitiveString,
		PrimitiveI64,
		PrimitiveAddress,

		ArrayDatatype{PrimitiveString, 32},
		ArrayDatatype{MapDatatype{PrimitiveString, PrimitiveU256}, 8},

		VarrayDatatype{PrimitiveString},
		VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}},

		MapDatatype{PrimitiveString, PrimitiveString},
		MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveU64}},
		MapDatatype{PrimitiveString, PrimitiveBool},

		ClassDatatype{
			name: "Person",
			fields: makefields([]*TypeField{
				{"Name", PrimitiveString},
				{"Age", PrimitiveU64},
			}),
		},
	}

	for _, test := range tests {
		encoded, err := EncodeDatatype(test)
		require.NoError(t, err, test.Kind())

		decoded, err := DecodeDatatype(encoded)
		require.NoError(t, err)

		require.Equal(t, test, decoded)
	}
}

func TestDatatypeKind_String(t *testing.T) {
	for kind, str := range datatypeKindToString {
		require.Equal(t, str, kind.String())
	}

	require.PanicsWithValue(t, "unknown DatatypeKind variant", func() {
		_ = DatatypeKind(len(datatypeKindToString)).String()
	})
}

func TestDatatypeKind_IsCollection(t *testing.T) {
	require.False(t, Primitive.IsCollection())
	require.True(t, Array.IsCollection())
	require.True(t, Varray.IsCollection())
	require.True(t, Mapping.IsCollection())
	require.False(t, BuiltinClass.IsCollection())
	require.False(t, Class.IsCollection())
}

func TestPrimitive_String(t *testing.T) {
	for primitive, str := range primitiveToString {
		require.Equal(t, str, primitive.String())
	}

	require.PanicsWithValue(t, "unknown PrimitiveDatatype variant", func() {
		_ = PrimitiveDatatype(MaxPrimitiveKind + 1).String()
	})
}

func TestPrimitive_Declarable(t *testing.T) {
	for primitive := range primitiveToString {
		if primitive > 0 {
			require.True(t, primitive.Declarable())
		} else {
			require.False(t, primitive.Declarable())
		}
	}
}

func TestPrimitive_Numeric(t *testing.T) {
	for primitive := range primitiveToString {
		if primitive == PrimitiveI64 || primitive == PrimitiveU64 ||
			primitive == PrimitiveU256 || primitive == PrimitiveI256 {
			require.True(t, primitive.Numeric())
		} else {
			require.False(t, primitive.Numeric())
		}
	}
}

func TestDatatype_String(t *testing.T) {
	tests := []struct {
		datatype Datatype
		stringer string
	}{
		{PrimitiveBool, "bool"},
		{PrimitiveBytes, "bytes"},
		{PrimitiveString, "string"},
		{PrimitiveI64, "i64"},
		{PrimitiveU64, "u64"},
		{PrimitiveAddress, "address"},
		{PrimitiveU256, "u256"},

		{ArrayDatatype{PrimitiveString, 32}, "[32]string"},
		{ArrayDatatype{MapDatatype{PrimitiveString, PrimitiveU256}, 8}, "[8]map[string]u256"},
		{VarrayDatatype{PrimitiveString}, "[]string"},
		{VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}}, "[][4]i64"},
		{MapDatatype{PrimitiveString, PrimitiveString}, "map[string]string"},
		{MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveU64}}, "map[string][]u64"},
	}

	for _, test := range tests {
		require.Equal(t, test.stringer, test.datatype.String())
	}
}

func TestDatatype_Copy(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		tests := []PrimitiveDatatype{
			PrimitiveBool,
			PrimitiveString,
			PrimitiveI64,
			PrimitiveAddress,
		}

		for _, test := range tests {
			clone := test.Copy()

			require.Equal(t, test, clone)
			require.True(t, test.Equals(clone))

			primitive, _ := clone.(PrimitiveDatatype)
			primitive++

			require.NotEqual(t, test, primitive)
		}
	})

	t.Run("array", func(t *testing.T) {
		tests := []ArrayDatatype{
			{PrimitiveString, 32},
			{MapDatatype{PrimitiveString, PrimitiveU256}, 8},
		}

		for _, test := range tests {
			clone := test.Copy()

			require.Equal(t, test, clone)
			require.True(t, test.Equals(clone))

			array, _ := clone.(ArrayDatatype)
			array.size += 10

			require.NotEqual(t, test, array)
		}
	})

	t.Run("varray", func(t *testing.T) {
		tests := []struct {
			original VarrayDatatype
			modified Datatype
		}{
			{
				VarrayDatatype{PrimitiveString},
				PrimitiveU256,
			},
			{
				VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}},
				ArrayDatatype{PrimitiveI64, 8},
			},
		}

		for _, test := range tests {
			clone := test.original.Copy()
			varray, _ := clone.(VarrayDatatype)

			require.Equal(t, test.original, varray)
			require.True(t, test.original.Equals(varray))

			varray.elem = test.modified

			require.NotEqual(t, test.original, varray)
		}
	})

	t.Run("mapping", func(t *testing.T) {
		tests := []struct {
			original MapDatatype
			modified Datatype
		}{
			{
				MapDatatype{PrimitiveString, PrimitiveString},
				PrimitiveU256,
			},
			{
				MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveU64}},
				VarrayDatatype{PrimitiveI64},
			},
		}

		for _, test := range tests {
			clone := test.original.Copy()
			mapping, _ := clone.(MapDatatype)

			require.Equal(t, test.original, mapping)
			require.True(t, test.original.Equals(mapping))

			mapping.val = test.modified

			require.NotEqual(t, test.original, mapping)
		}
	})

	t.Run("class", func(t *testing.T) {
		tests := []struct {
			original ClassDatatype
			modified *TypeField
		}{
			{
				ClassDatatype{
					name: "Person",
					fields: makefields([]*TypeField{
						{"Name", PrimitiveString},
						{"Age", PrimitiveU64},
					}),
				},
				&TypeField{"Name", PrimitiveAddress},
			},
		}

		for _, test := range tests {
			clone := test.original.Copy()
			class, _ := clone.(ClassDatatype)

			require.Equal(t, test.original, class)
			require.True(t, test.original.Equals(class))

			class.fields.Insert(0, test.modified)
			require.NotEqual(t, test.original, class)
		}
	})
}

func TestDatatype_Equals(t *testing.T) {
	tests := []struct {
		a, b  Datatype
		equal bool
	}{
		{PrimitiveBool, PrimitiveString, false},
		{PrimitiveI64, PrimitiveU64, false},
		{PrimitiveAddress, PrimitiveAddress, true},
		{PrimitiveU256, ArrayDatatype{PrimitiveString, 32}, false},
		{PrimitiveBytes, VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}}, false},
		{PrimitiveU64, MapDatatype{PrimitiveString, PrimitiveString}, false},

		{ArrayDatatype{PrimitiveString, 32}, PrimitiveAddress, false},
		{ArrayDatatype{PrimitiveString, 32}, ArrayDatatype{PrimitiveString, 32}, true},
		{ArrayDatatype{PrimitiveString, 32}, ArrayDatatype{PrimitiveString, 12}, false},
		{ArrayDatatype{PrimitiveI64, 32}, ArrayDatatype{PrimitiveU64, 12}, false},
		{ArrayDatatype{PrimitiveBytes, 32}, VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}}, false},
		{ArrayDatatype{PrimitiveU64, 32}, MapDatatype{PrimitiveString, PrimitiveString}, false},

		{VarrayDatatype{PrimitiveU64}, PrimitiveAddress, false},
		{VarrayDatatype{PrimitiveString}, ArrayDatatype{PrimitiveString, 10}, false},
		{VarrayDatatype{PrimitiveString}, VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}}, false},
		{VarrayDatatype{PrimitiveString}, VarrayDatatype{PrimitiveString}, true},
		{VarrayDatatype{PrimitiveString}, VarrayDatatype{PrimitiveBool}, false},
		{VarrayDatatype{PrimitiveBool}, MapDatatype{PrimitiveString, PrimitiveString}, false},

		{MapDatatype{PrimitiveString, PrimitiveString}, PrimitiveAddress, false},
		{MapDatatype{PrimitiveString, PrimitiveU64}, ArrayDatatype{PrimitiveString, 10}, false},
		{MapDatatype{PrimitiveString, PrimitiveAddress}, ArrayDatatype{PrimitiveU64, 12}, false},
		{MapDatatype{PrimitiveString, PrimitiveBool}, MapDatatype{PrimitiveString, PrimitiveBool}, true},
		{
			MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveBool}},
			MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveI64}},
			false,
		},
		{
			MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveI64}},
			MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveI64}},
			true,
		},
	}

	for _, test := range tests {
		require.Equal(t, test.equal, test.a.Equals(test.b))
	}
}

type mockClassdefProvider struct {
	classdefs map[string]ClassDatatype
}

func (m mockClassdefProvider) GetClassDatatype(name string) (ClassDatatype, bool) {
	class, ok := m.classdefs[name]

	return class, ok
}

func TestParseDatatype(t *testing.T) {
	tests := []struct {
		symbol   string
		err      string
		datatype Datatype
	}{
		{"bool", "", PrimitiveBool},
		{"bytes", "", PrimitiveBytes},
		{"string", "", PrimitiveString},
		{"i64", "", PrimitiveI64},
		{"u64", "", PrimitiveU64},
		{"address", "", PrimitiveAddress},
		{"u256", "", PrimitiveU256},
		{"i256", "", PrimitiveI256},

		{"[32]string", "", ArrayDatatype{PrimitiveString, 32}},
		{"[8]map[string]u256", "", ArrayDatatype{MapDatatype{PrimitiveString, PrimitiveU256}, 8}},
		{"[]string", "", VarrayDatatype{PrimitiveString}},
		{"[][4]i64", "", VarrayDatatype{ArrayDatatype{PrimitiveI64, 4}}},
		{"map[string]string", "", MapDatatype{PrimitiveString, PrimitiveString}},
		{"map[string][]u64", "", MapDatatype{PrimitiveString, VarrayDatatype{PrimitiveU64}}},

		{"[Type1", "invalid type data for array: missing end of enclosure: ']'", nil},
		{"[20000000000000000000]address", "invalid type data for array: size must be a 64-bit unsigned integer", nil},
		{"[56String]address", "invalid type data for array: size must be a 64-bit unsigned integer", nil},
		{"[6]addr", "invalid type data for array: invalid element type: invalid class reference: 'addr' not found", nil},
		{"[]big", "invalid type data for sequence: invalid element type: invalid class reference: 'big' not found", nil},
		{"map{string, string}", "invalid type data for hashmap: missing start of enclosure: '['", nil},
		{"map[StringType]", "invalid type data for hashmap: invalid key type: must be a valid primitive", nil},
		{
			"map[u64]StringType",
			"invalid type data for hashmap: invalid value type: invalid class reference: 'StringType' not found",
			nil,
		},
	}

	provider := mockClassdefProvider{classdefs: map[string]ClassDatatype{}}

	for _, test := range tests {
		_, err := ParseDatatype(test.symbol, provider)
		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.err)
		}
	}
}
