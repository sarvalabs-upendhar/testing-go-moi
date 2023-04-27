package pisa

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func TestNewRegisterValue(t *testing.T) {
	t.Run("unsupported_primitive", func(t *testing.T) {
		require.PanicsWithValue(t, "unsupported datatype for value generation: ptr", func() {
			_, _ = NewRegisterValue(TypePtr, nil)
		})
	})

	t.Run("unsupported_type", func(t *testing.T) {
		require.PanicsWithValue(t, "unsupported datatype for value generation: DatatypeKind(10)", func() {
			_, _ = NewRegisterValue(&Datatype{Kind: DatatypeKind(10)}, nil)
		})
	})
}

func TestNewRegisterSet(t *testing.T) {
	tests := []struct {
		name   string
		fields *TypeFields
		values polo.Document
		output RegisterSet
		err    string
	}{
		{
			name:   "empty_values_with_fields",
			fields: makefields([]*TypeField{{"foo", TypeString}}),
			values: nil,
			output: nil,
			err:    "missing input values",
		},
		{
			name:   "empty_values_without_fields",
			fields: makefields([]*TypeField{}),
			values: nil,
			output: make(RegisterSet),
			err:    "",
		},
		{
			name: "missing_field_data",
			fields: makefields([]*TypeField{
				{"foo", TypeAddress},
				{"boo", TypeString},
			}),
			values: polo.Document{
				"foo": must(polo.Polorize(types.Address{10, 10, 10, 10})),
			},
			output: nil,
			err:    "missing data for 'boo'",
		},
		{
			name: "malformed_field_data",
			fields: makefields([]*TypeField{
				{"foo", TypeAddress},
				{"boo", TypeString},
			}),
			values: polo.Document{
				"foo": must(polo.Polorize(types.Address{10, 10, 10, 10})),
				"boo": must(polo.Polorize(5000)),
			},
			output: nil,
			err:    "malformed data for 'boo': not string",
		},
		{
			name: "well_formed",
			fields: makefields([]*TypeField{
				{"foo", TypeAddress},
				{"boo", TypeString},
			}),
			values: polo.Document{
				"foo": must(polo.Polorize(types.Address{10, 10, 10, 10})),
				"boo": must(polo.Polorize("hello!")),
			},
			output: RegisterSet{
				0: AddressValue(types.Address{10, 10, 10, 10}),
				1: StringValue("hello!"),
			},
			err: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			regs, err := NewRegisterSet(test.fields, test.values)

			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.output, regs)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func TestRegisterSet_Get(t *testing.T) {
	// create a sample RegisterSet instance
	registers := RegisterSet{
		0: U64Value(42),
		1: StringValue("hello"),
	}

	// test Get method for existing address
	val := registers.Get(0)
	assert.Equal(t, U64Value(42), val)

	// test Get method for non-existing address
	val = registers.Get(2)
	assert.Equal(t, NullValue{}, val)
}

func TestRegisterSet_Set(t *testing.T) {
	// create a sample RegisterSet instance
	registers := RegisterSet{
		0: U64Value(42),
		1: StringValue("hello"),
	}

	// call Set method and check the output
	registers.Set(2, StringValue("world"))
	assert.Equal(t, 3, len(registers))
	assert.Equal(t, StringValue("world"), registers.Get(2))

	// test overwrite of existing RegisterValue
	registers.Set(0, U64Value(12345))
	assert.Equal(t, 3, len(registers))
	assert.Equal(t, U64Value(12345), registers.Get(0))
}

func TestRegisterSet_Unset(t *testing.T) {
	// create a sample RegisterSet instance
	registers := RegisterSet{
		0: U64Value(42),
		1: StringValue("hello"),
	}

	// call Unset method and check the output
	registers.Unset(0)
	assert.Equal(t, 1, len(registers))
	assert.Equal(t, NullValue{}, registers.Get(0))
}
