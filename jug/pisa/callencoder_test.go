package pisa

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func makefields(fields []*engineio.TypeField) *engineio.TypeFields {
	// Ensure that there are less than 256 field expressions
	// This is an internal call so, is alright to panic
	if len(fields) > 256 {
		panic("cannot have more than 256 fields for FieldTable")
	}

	// Create a blank field table
	table := &engineio.TypeFields{
		Table:   make(map[uint8]*engineio.TypeField, len(fields)),
		Symbols: make(map[string]uint8, len(fields)),
	}

	for position, field := range fields {
		table.Table[uint8(position)] = field
		table.Symbols[field.Name] = uint8(position)
	}

	return table
}

func TestCallEncoder_EncodeInputs(t *testing.T) {
	tests := []struct {
		fields  *engineio.TypeFields
		inputs  map[string]any
		encoded polo.Document
		err     string
	}{
		{
			makefields([]*engineio.TypeField{
				{Name: "a", Type: engineio.TypeString},
				{Name: "b", Type: engineio.TypeI64},
				{Name: "c", Type: engineio.TypeBool},
				{Name: "d", Type: engineio.TypeBytes},
				{Name: "e", Type: engineio.NewVarrayType(engineio.TypeU64)},
				{Name: "f", Type: engineio.NewMappingType(engineio.PrimitiveString, engineio.TypeString)},
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
			},
			"",
		},

		{
			makefields([]*engineio.TypeField{
				{Name: "a", Type: engineio.TypeString},
				{Name: "b", Type: engineio.NewVarrayType(engineio.TypeU64)},
			}),
			map[string]any{
				"a": "hello",
				"b": []byte{0xab, 0xc1, 0x23},
			},
			nil,
			"invalid input data for 'b': incompatible wire: unexpected wiretype 'word'. expected one of: {null, pack, document}",
		},

		{
			makefields([]*engineio.TypeField{
				{Name: "a", Type: engineio.NewMappingType(engineio.PrimitiveString, engineio.TypeString)},
			}),
			map[string]any{
				"a": map[string]any{"name": "Manish", "age": 23},
			},
			nil,
			"invalid input data for 'a': call encoding does not support object types",
		},
	}

	for _, test := range tests {
		callEncoder := CallEncoder(engineio.CallFields{Inputs: test.fields})

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
		fields  *engineio.TypeFields
		outputs polo.Document
		decoded map[string]any
		err     string
	}{
		{
			makefields([]*engineio.TypeField{
				{Name: "a", Type: engineio.TypeString},
				{Name: "b", Type: engineio.TypeI64},
				{Name: "c", Type: engineio.TypeBool},
				{Name: "d", Type: engineio.TypeBytes},
				{Name: "e", Type: engineio.NewVarrayType(engineio.TypeU64)},
				{Name: "f", Type: engineio.NewMappingType(engineio.PrimitiveString, engineio.TypeString)},
			}),
			polo.Document{
				"a": polo.Raw{6, 104, 101, 108, 108, 111},
				"b": polo.Raw{4, 3, 21},
				"c": polo.Raw{2},
				"d": polo.Raw{6, 171, 193, 35},
				"e": polo.Raw{14, 47, 3, 19, 123, 1, 200},
				"f": polo.Raw{14, 79, 6, 22, 70, 86, 97, 102, 111, 111, 98, 98, 97, 114},
			},
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
			makefields([]*engineio.TypeField{
				{Name: "a", Type: engineio.TypeString},
				{Name: "b", Type: engineio.NewVarrayType(engineio.TypeU64)},
			}),
			polo.Document{
				"a": polo.Raw{6, 104, 101, 108, 108, 111},
				"b": polo.Raw{4, 3, 21},
			},
			nil,
			"invalid output data for 'b': incompatible wire: unexpected wiretype 'negint'. " +
				"expected one of: {null, pack, document}",
		},
	}

	for _, test := range tests {
		callEncoder := CallEncoder(engineio.CallFields{Outputs: test.fields})

		doc, err := callEncoder.DecodeOutputs(test.outputs)
		require.Equal(t, test.decoded, doc)

		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.err)
		}
	}
}
