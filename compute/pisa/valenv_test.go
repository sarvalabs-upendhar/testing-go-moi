package pisa

import (
	"math/big"
	"testing"
	"time"

	"github.com/holiman/uint256"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/stretchr/testify/assert"
)

func TestEnvironmentValue(t *testing.T) {
	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Create a new EnvironmentValue
		value := EnvironmentValue{engineio.NewEnvObject(time.Now().Unix(), big.NewInt(1))}

		// Test Type()
		assert.Equal(t, BuiltinDatatype{name: "Environment", fields: makefields([]*TypeField{})}, value.Type(), "EnvironmentValue Type should be TypeEnvironment") //nolint:lll

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of Environment should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, map[string]any{}, norm, "Normalized value of Environment should be equal to mpa[string]->any")

		// Test Data()
		assert.Equal(t, []byte{}, value.Data(), "POLO encoded bytes of StringValue should match expected value")
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
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}                                                                    //nolint:lll
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: EnvironmentValue{driver: engineio.NewEnvObject(time.Now().Unix(), big.NewInt(1))}}) //nolint:lll

				if test.err != nil {
					assert.Equal(t, test.err, except)
				} else {
					assert.Nil(t, except)
					assert.Equal(t, test.res, outputs.Get(0))
				}
			}
		})

		t.Run("FuelPrice [0x11]", func(t *testing.T) {
			method := runtime.builtinClasses["Environment"].methods[0x11]

			tests := []struct {
				res *U256Value
				err *Exception
			}{
				{res: &U256Value{value: uint256.NewInt(1)}, err: nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}                                                                    //nolint:lll
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: EnvironmentValue{driver: engineio.NewEnvObject(time.Now().Unix(), big.NewInt(1))}}) //nolint:lll

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
