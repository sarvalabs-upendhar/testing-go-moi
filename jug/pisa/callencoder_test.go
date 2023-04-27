package pisa

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

type mockRefProvider map[string]any

func (m mockRefProvider) GetReference(ref engineio.ReferenceVal) (any, bool) {
	val, ok := m[string(ref)]

	return val, ok
}

func TestCallEncoder_EncodeInputs(t *testing.T) {
	tests := []struct {
		fields  *TypeFields
		inputs  map[string]any
		encoded []byte
		refs    engineio.ReferenceProvider
		err     string
	}{
		{
			makefields([]*TypeField{
				{Name: "a", Type: TypeString},
				{Name: "b", Type: TypeI64},
				{Name: "c", Type: TypeBool},
				{Name: "d", Type: TypeBytes},
				{Name: "e", Type: NewVarrayType(TypeU64)},
				{Name: "f", Type: NewMappingType(PrimitiveString, TypeString)},
			}),
			map[string]any{
				"a": "hello",
				"b": int64(-789),
				"c": true,
				"d": []byte{0xab, 0xc1, 0x23},
				"e": []any{uint64(123), uint64(456)},
				"f": map[any]any{"a": "foo", "b": "bar"},
			},
			polo.Document{
				"a": polo.Raw{6, 104, 101, 108, 108, 111},
				"b": polo.Raw{4, 3, 21},
				"c": polo.Raw{2},
				"d": polo.Raw{6, 171, 193, 35},
				"e": polo.Raw{14, 47, 3, 19, 123, 1, 200},
				"f": polo.Raw{14, 79, 6, 22, 70, 86, 97, 102, 111, 111, 98, 98, 97, 114},
			}.Bytes(),
			nil,
			"",
		},

		{
			makefields([]*TypeField{
				{Name: "a", Type: TypeString},
				{Name: "b", Type: NewVarrayType(TypeU64)},
			}),
			map[string]any{
				"a": "hello",
				"b": []byte{0xab, 0xc1, 0x23},
			},
			nil,
			nil,
			"invalid input data for 'b': incompatible wire: unexpected wiretype 'word'. expected one of: {null, pack, document}",
		},

		{
			makefields([]*TypeField{
				{Name: "a", Type: NewMappingType(PrimitiveString, TypeString)},
			}),
			map[string]any{
				"a": map[any]any{"foo": "bar", "boo": "far"},
			},
			polo.Document{
				"a": polo.Raw{14, 95, 6, 54, 102, 150, 1, 98, 111, 111, 102, 97, 114, 102, 111, 111, 98, 97, 114},
			}.Bytes(),
			nil,
			"",
		},

		{
			makefields([]*TypeField{{Name: "a", Type: TypeString}}),
			map[string]any{
				"a": engineio.ReferenceVal("clone-data"),
			},
			polo.Document{
				"a": polo.Raw{6, 111, 114, 105, 103, 105, 110, 97, 108, 45, 100, 97, 116, 97},
			}.Bytes(),
			mockRefProvider{"clone-data": "original-data"},
			"",
		},

		{
			makefields([]*TypeField{{Name: "a", Type: TypeString}}),
			map[string]any{
				"a": engineio.ReferenceVal("clone-data"),
			},
			nil,
			mockRefProvider{"clone": "original-data"},
			"invalid input data for 'a': unable to resolve reference 'ref<clone-data>'",
		},

		{
			makefields([]*TypeField{{Name: "a", Type: TypeString}}),
			map[string]any{
				"a": engineio.ReferenceVal("clone-data"),
			},
			nil,
			nil,
			"invalid input data for 'a': encountered reference value without a ref provider",
		},

		{
			makefields([]*TypeField{{Name: "a", Type: TypeString}}),
			nil,
			nil,
			nil,
			"",
		},

		{
			makefields([]*TypeField{
				{Name: "foo", Type: NewClassType("MyClass", makefields([]*TypeField{
					{Name: "a", Type: TypeString},
					{Name: "b", Type: TypeI64},
				}))},
			}),
			map[string]any{
				"foo": map[string]any{
					"a": "hello",
					"b": 345,
				},
			},
			polo.Document{
				"foo": polo.Document{
					"a": must(polo.Polorize("hello")),
					"b": must(polo.Polorize(345)),
				}.Bytes(),
			}.Bytes(),
			nil,
			"",
		},
	}

	for _, test := range tests {
		callEncoder := CallEncoder(CallFields{Inputs: test.fields})

		doc, err := callEncoder.EncodeInputs(test.inputs, test.refs)
		require.Equal(t, test.encoded, doc)

		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.err)
		}
	}
}

func TestCallEncoder_DecodeOutputs(t *testing.T) {
	tests := []struct {
		fields  *TypeFields
		outputs []byte
		decoded map[string]any
		err     string
	}{
		{
			makefields([]*TypeField{
				{Name: "a", Type: TypeString},
				{Name: "b", Type: TypeI64},
				{Name: "c", Type: TypeBool},
				{Name: "d", Type: TypeBytes},
				{Name: "e", Type: NewVarrayType(TypeU64)},
				{Name: "f", Type: NewMappingType(PrimitiveString, TypeString)},
			}),
			polo.Document{
				"a": polo.Raw{6, 104, 101, 108, 108, 111},
				"b": polo.Raw{4, 3, 21},
				"c": polo.Raw{2},
				"d": polo.Raw{6, 171, 193, 35},
				"e": polo.Raw{14, 47, 3, 19, 123, 1, 200},
				"f": polo.Raw{14, 79, 6, 22, 70, 86, 97, 102, 111, 111, 98, 98, 97, 114},
			}.Bytes(),
			map[string]any{
				"a": "hello",
				"b": int64(-789),
				"c": true,
				"d": []byte{0xab, 0xc1, 0x23},
				"e": []any{uint64(123), uint64(456)},
				"f": map[any]any{"a": "foo", "b": "bar"},
			},
			"",
		},

		{
			makefields([]*TypeField{
				{Name: "a", Type: TypeString},
				{Name: "b", Type: NewVarrayType(TypeU64)},
			}),
			polo.Document{
				"a": polo.Raw{6, 104, 101, 108, 108, 111},
				"b": polo.Raw{4, 3, 21},
			}.Bytes(),
			nil,
			"invalid output data for 'b': incompatible wire: unexpected wiretype 'negint'. " +
				"expected one of: {null, pack, document}",
		},

		{
			makefields([]*TypeField{
				{Name: "a", Type: TypeString},
				{Name: "b", Type: NewVarrayType(TypeU64)},
			}),
			nil,
			nil,
			"",
		},
	}

	for _, test := range tests {
		callEncoder := CallEncoder(CallFields{Outputs: test.fields})

		doc, err := callEncoder.DecodeOutputs(test.outputs)
		require.Equal(t, test.decoded, doc)

		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.err)
		}
	}
}
