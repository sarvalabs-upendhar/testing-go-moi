package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-pisa/datatypes"
	"github.com/sarvalabs/go-pisa/logic"
)

func TestCallEncoder_EncodeInputs(t *testing.T) {
	tests := []struct {
		fields  *datatypes.TypeFields
		inputs  map[string]any
		encoded []byte
		err     string
	}{
		{
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "a", Type: datatypes.PrimitiveString},
				{Name: "b", Type: datatypes.PrimitiveI64},
				{Name: "c", Type: datatypes.PrimitiveBool},
				{Name: "d", Type: datatypes.PrimitiveBytes},
				{Name: "e", Type: datatypes.NewVarray(datatypes.PrimitiveU64)},
				{Name: "f", Type: datatypes.NewMapping(datatypes.PrimitiveString, datatypes.PrimitiveString)},
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
			"",
		},

		{
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "a", Type: datatypes.PrimitiveString},
				{Name: "b", Type: datatypes.NewVarray(datatypes.PrimitiveU64)},
			}),
			map[string]any{
				"a": "hello",
				"b": []byte{0xab, 0xc1, 0x23},
			},
			nil,
			"invalid input data for 'b': incompatible wire: unexpected wiretype 'word'. expected one of: {null, pack, document}",
		},

		{
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "a", Type: datatypes.NewMapping(datatypes.PrimitiveString, datatypes.PrimitiveString)},
			}),
			map[string]any{
				"a": map[any]any{"foo": "bar", "boo": "far"},
			},
			polo.Document{
				"a": polo.Raw{14, 95, 6, 54, 102, 150, 1, 98, 111, 111, 102, 97, 114, 102, 111, 111, 98, 97, 114},
			}.Bytes(),
			"",
		},

		{
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "foo", Type: datatypes.NewClass(
					"MyClass",
					datatypes.NewFields([]*datatypes.TypeField{
						{Name: "a", Type: datatypes.PrimitiveString},
						{Name: "b", Type: datatypes.PrimitiveI64},
					}),
				)},
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
			"",
		},
	}

	for _, test := range tests {
		callEncoder := CallEncoder(logic.CallFields{Inputs: test.fields})

		doc, err := callEncoder.EncodeInputs(test.inputs)
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
		fields  *datatypes.TypeFields
		outputs []byte
		decoded map[string]any
		err     string
	}{
		{
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "a", Type: datatypes.PrimitiveString},
				{Name: "b", Type: datatypes.PrimitiveI64},
				{Name: "c", Type: datatypes.PrimitiveBool},
				{Name: "d", Type: datatypes.PrimitiveBytes},
				{Name: "e", Type: datatypes.NewVarray(datatypes.PrimitiveU64)},
				{Name: "f", Type: datatypes.NewMapping(datatypes.PrimitiveString, datatypes.PrimitiveString)},
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
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "a", Type: datatypes.PrimitiveString},
				{Name: "b", Type: datatypes.NewVarray(datatypes.PrimitiveU64)},
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
			datatypes.NewFields([]*datatypes.TypeField{
				{Name: "a", Type: datatypes.PrimitiveString},
				{Name: "b", Type: datatypes.NewVarray(datatypes.PrimitiveU64)},
			}),
			nil,
			nil,
			"",
		},
	}

	for _, test := range tests {
		callEncoder := CallEncoder(logic.CallFields{Outputs: test.fields})

		doc, err := callEncoder.DecodeOutputs(test.outputs)
		require.Equal(t, test.decoded, doc)

		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.err)
		}
	}
}
