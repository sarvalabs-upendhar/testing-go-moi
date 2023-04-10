package engineio

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatatypeKind_String(t *testing.T) {
	for kind, str := range datatypeKindToString {
		require.Equal(t, str, kind.String())
	}

	require.PanicsWithValue(t, "unknown DatatypeKind variant", func() {
		_ = DatatypeKind(len(datatypeKindToString)).String()
	})
}

func TestDatatypeKind_IsCollection(t *testing.T) {
	for kind := range datatypeKindToString {
		if kind == PrimitiveType || kind == ClassType {
			require.False(t, kind.IsCollection())
		} else {
			require.True(t, kind.IsCollection())
		}
	}
}

func TestPrimitive_String(t *testing.T) {
	for primitive, str := range primitiveToString {
		require.Equal(t, str, primitive.String())
	}

	require.PanicsWithValue(t, "unknown Primitive variant", func() {
		_ = Primitive(MaxPrimitive + 1).String()
	})
}

func TestPrimitive_Datatype(t *testing.T) {
	for primitive := range primitiveToString {
		require.Equal(t, &Datatype{Prim: primitive}, primitive.Datatype())
	}
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
		if primitive == PrimitiveI64 || primitive == PrimitiveU64 || primitive == PrimitiveBigInt {
			require.True(t, primitive.Numeric())
		} else {
			require.False(t, primitive.Numeric())
		}
	}
}

func TestDatatype_String(t *testing.T) {
	tests := []struct {
		datatype *Datatype
		stringer string
	}{
		{TypeBool, "bool"},
		{TypeBytes, "bytes"},
		{TypeString, "string"},
		{TypeI64, "int64"},
		{TypeU64, "uint64"},
		{TypeAddress, "address"},
		{TypeBigInt, "bigint"},

		{NewArrayType(32, TypeString), "[32]string"},
		{NewArrayType(8, NewMappingType(PrimitiveString, TypeBigInt)), "[8]map[string]bigint"},
		{NewVarrayType(TypeString), "[]string"},
		{NewVarrayType(NewArrayType(4, TypeI64)), "[][4]int64"},
		{NewMappingType(PrimitiveString, TypeString), "map[string]string"},
		{NewMappingType(PrimitiveString, NewVarrayType(TypeU64)), "map[string][]uint64"},
	}

	for _, test := range tests {
		require.Equal(t, test.stringer, test.datatype.String())
	}

	require.PanicsWithValue(t, "unsupported string conversion for Datatype", func() {
		_ = Datatype{Kind: DatatypeKind(10)}.String()
	})
}

func TestDatatype_Copy(t *testing.T) {
	tests := []*Datatype{
		TypeBool,
		TypeString,
		TypeI64,
		TypeAddress,

		NewArrayType(32, TypeString),
		NewArrayType(8, NewMappingType(PrimitiveString, TypeBigInt)),
		NewVarrayType(TypeString),
		NewVarrayType(NewArrayType(4, TypeI64)),
		NewMappingType(PrimitiveString, TypeString),
		NewMappingType(PrimitiveString, NewVarrayType(TypeU64)),
		NewMappingType(PrimitiveString, TypeBool), NewMappingType(PrimitiveString, TypeBool),
	}

	for _, test := range tests {
		clone := test.Copy()

		require.Equal(t, test, clone)
		require.True(t, test.Equals(clone))
		require.NotEqual(t, reflect.ValueOf(test).Pointer(), reflect.ValueOf(clone).Pointer())
	}
}

func TestDatatype_Equals(t *testing.T) {
	tests := []struct {
		a, b  *Datatype
		equal bool
	}{
		{TypeBool, TypeString, false},
		{TypeI64, TypeU64, false},
		{TypeAddress, TypeAddress, true},
		{TypeBigInt, NewArrayType(32, TypeString), false},
		{TypeBytes, NewVarrayType(NewArrayType(4, TypeI64)), false},
		{TypeU64, NewMappingType(PrimitiveString, TypeString), false},

		{NewArrayType(32, TypeString), TypeAddress, false},
		{NewArrayType(32, TypeString), NewArrayType(32, TypeString), true},
		{NewArrayType(32, TypeString), NewArrayType(12, TypeString), false},
		{NewArrayType(32, TypeI64), NewArrayType(12, TypeU64), false},
		{NewArrayType(32, TypeBytes), NewVarrayType(NewArrayType(4, TypeI64)), false},
		{NewArrayType(32, TypeU64), NewMappingType(PrimitiveString, TypeString), false},

		{NewVarrayType(TypeU64), TypeAddress, false},
		{NewVarrayType(TypeString), NewArrayType(10, TypeString), false},
		{NewVarrayType(TypeString), NewVarrayType(NewArrayType(4, TypeI64)), false},
		{NewVarrayType(TypeString), NewVarrayType(TypeString), true},
		{NewVarrayType(TypeString), NewVarrayType(TypeBool), false},
		{NewVarrayType(TypeBool), NewMappingType(PrimitiveString, TypeString), false},

		{NewMappingType(PrimitiveString, TypeString), TypeAddress, false},
		{NewMappingType(PrimitiveString, TypeU64), NewArrayType(10, TypeString), false},
		{NewMappingType(PrimitiveString, TypeAddress), NewArrayType(12, TypeU64), false},
		{NewMappingType(PrimitiveString, TypeBool), NewMappingType(PrimitiveString, TypeBool), true},
		{
			NewMappingType(PrimitiveString, NewVarrayType(TypeBool)),
			NewMappingType(PrimitiveString, NewVarrayType(TypeI64)),
			false,
		},
		{
			NewMappingType(PrimitiveString, NewVarrayType(TypeI64)),
			NewMappingType(PrimitiveString, NewVarrayType(TypeI64)),
			true,
		},
	}

	for _, test := range tests {
		require.Equal(t, test.equal, test.a.Equals(test.b))
	}

	require.PanicsWithValue(t, "cannot check type equality for unknown datatype kind", func() {
		_ = Datatype{Kind: DatatypeKind(10)}.Equals(&Datatype{Kind: DatatypeKind(10)})
	})
}

func TestParseDatatype(t *testing.T) {
	tests := []struct {
		symbol   string
		err      string
		datatype *Datatype
	}{
		{"bool", "", TypeBool},
		{"bytes", "", TypeBytes},
		{"string", "", TypeString},
		{"int64", "", TypeI64},
		{"uint64", "", TypeU64},
		{"address", "", TypeAddress},
		{"bigint", "", TypeBigInt},

		{"[32]string", "", NewArrayType(32, TypeString)},
		{"[8]map[string]bigint", "", NewArrayType(8, NewMappingType(PrimitiveString, TypeBigInt))},
		{"[]string", "", NewVarrayType(TypeString)},
		{"[][4]int64", "", NewVarrayType(NewArrayType(4, TypeI64))},
		{"map[string]string", "", NewMappingType(PrimitiveString, TypeString)},
		{"map[string][]uint64", "", NewMappingType(PrimitiveString, NewVarrayType(TypeU64))},

		{"[Type1", "invalid type data for array: missing end of enclosure: ']'", nil},
		{"[20000000000000000000]address", "invalid type data for array: size must be a 64-bit unsigned integer", nil},
		{"[56String]address", "invalid type data for array: size must be a 64-bit unsigned integer", nil},
		{"[6]addr", "invalid type data for array: invalid element type: not a datatype", nil},
		{"[]big", "invalid type data for sequence: invalid element type: not a datatype", nil},
		{"map{string, string}", "invalid type data for hashmap: missing start of enclosure: '['", nil},
		{"map[StringType]", "invalid type data for hashmap: invalid key type: must be a valid primitive", nil},
		{"map[uint64]StringType", "invalid type data for hashmap: invalid value type: not a datatype", nil},
	}

	for _, test := range tests {
		_, err := ParseDatatype(test.symbol)
		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.err)
		}
	}
}
