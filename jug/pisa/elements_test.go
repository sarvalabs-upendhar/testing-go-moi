package pisa

import (
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func TestElementSerialization(t *testing.T) {
	t.Run("StateSchema", func(t *testing.T) {
		state := &StateSchema{
			Kind: engineio.PersistentState,
			Fields: []TypefieldSchema{
				{Slot: 0, Label: "A", Type: "string"},
				{Slot: 1, Label: "B", Type: "uint64"},
			},
		}

		encoded, err := polo.Polorize(state)
		require.NoError(t, err)

		decoded := new(StateSchema)
		err = polo.Depolorize(decoded, encoded)
		require.NoError(t, err)

		require.Equal(t, state, decoded)
	})

	t.Run("ClassSchema", func(t *testing.T) {
		class := &ClassSchema{
			Name: "CustomClass",
			Fields: []TypefieldSchema{
				{Slot: 0, Label: "A", Type: "string"},
				{Slot: 1, Label: "B", Type: "uint64"},
			},
		}

		encoded, err := polo.Polorize(class)
		require.NoError(t, err)

		decoded := new(ClassSchema)
		err = polo.Depolorize(decoded, encoded)
		require.NoError(t, err)

		require.Equal(t, class, decoded)
	})

	t.Run("RoutineSchema", func(t *testing.T) {
		routine := &RoutineSchema{
			Name: "FunctionA",
			Kind: engineio.InvokableCallsite,
			Accepts: []TypefieldSchema{
				{Slot: 0, Label: "A", Type: "string"},
				{Slot: 1, Label: "B", Type: "uint64"},
			},
			Returns: []TypefieldSchema{
				{Slot: 0, Label: "A", Type: "bool"},
				{Slot: 1, Label: "B", Type: "address"},
			},
		}

		encoded, err := polo.Polorize(routine)
		require.NoError(t, err)

		decoded := new(RoutineSchema)
		err = polo.Depolorize(decoded, encoded)
		require.NoError(t, err)

		require.Equal(t, routine, decoded)
	})

	t.Run("TypedefSchema", func(t *testing.T) {
		typedef := TypedefSchema("map[string]string")

		encoded, err := polo.Polorize(typedef)
		require.NoError(t, err)

		decoded := new(TypedefSchema)
		err = polo.Depolorize(decoded, encoded)
		require.NoError(t, err)

		require.Equal(t, typedef, *decoded)
	})

	t.Run("ConstantSchema", func(t *testing.T) {
		constant := &ConstantSchema{
			Type:  "uint64",
			Value: "0x030a0b",
		}

		encoded, err := polo.Polorize(constant)
		require.NoError(t, err)

		decoded := new(ConstantSchema)
		err = polo.Depolorize(decoded, encoded)
		require.NoError(t, err)

		require.Equal(t, constant, decoded)
	})
}
