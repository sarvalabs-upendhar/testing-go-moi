package pisa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentValue(t *testing.T) {
	// Create a new EnvironmentValue
	value := EnvironmentValue{NewDebugEnvDriver(time.Now().Unix(), "Test")}

	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Test Type()
		assert.Equal(t, BuiltinDatatype{
			name:   "Environment",
			fields: makefields([]*TypeField{}),
		}, value.Type(),
			"EnvironmentValue Type should be TypeEnvironment",
		)

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of Environment should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, map[string]any{}, norm, "Normalized value of Environment should be equal to map[string]->any")

		// Test Data()
		assert.Equal(t, []byte{}, value.Data(), "POLO encoded bytes of EnvironmentValue should match expected value")
	})

	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("Time [0x10]", func(t *testing.T) {
			method := runtime.builtinClasses["Environment"].methods[0x10]

			tests := []struct {
				res I64Value
				err *Exception
			}{
				{res: I64Value(time.Now().Unix()), err: nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: value})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("ClusterID [0x11]", func(t *testing.T) {
			method := runtime.builtinClasses["Environment"].methods[0x11]

			tests := []struct {
				res StringValue
				err *Exception
			}{
				{StringValue("Test"), nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: value})

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})
	})
}
