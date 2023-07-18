package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogicContext(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new EnvironmentValue
		value := LogicContextValue{}

		// Test Type()
		assert.Equal(t, BuiltinDatatype{name: "LogicContext", fields: makefields([]*TypeField{{"addr", PrimitiveAddress}})}, value.Type(), "LogicContextValue Type should be TypeLogicContext") //nolint:lll

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of LogicContext should be equal to original")

		// Test Size()
		assert.Equal(t, U64Value(len(LogicContextType.(BuiltinDatatype).fields.Table)), value.Size(), "Size of LogicContextValue must be same as that of the fields table") //nolint:lll, forcetypeassert
	})
}

func TestParticipantContext(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new EnvironmentValue
		value := ParticipantContextValue{}

		// Test Type()
		assert.Equal(t, BuiltinDatatype{name: "ParticipantContext", fields: makefields([]*TypeField{{"addr", PrimitiveAddress}})}, value.Type(), "ParticipantContextValue Type should be TypeParticipantContext") //nolint:lll

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of Participant should be equal to original")

		// Test Size()
		assert.Equal(t, U64Value(len(ParticipantContextType.(BuiltinDatatype).fields.Table)), value.Size(), "Size of ParticipantContextValue must be same as that of the fields table") //nolint:lll, forcetypeassert
	})
}
