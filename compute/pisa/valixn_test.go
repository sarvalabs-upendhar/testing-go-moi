package pisa

import (
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/sarvalabs/go-moi/common"
	"github.com/stretchr/testify/assert"
)

func TestInteractionValue(t *testing.T) {
	value := InteractionValue{driver: NewDebugIxnDriver(
		common.IxLogicInvoke,
		big.NewInt(1), big.NewInt(1000),
		"Hello", []byte{0, 1, 2, 3},
	)}

	t.Run("RegisterValue Implementation", func(t *testing.T) {
		// Test Type()
		assert.Equal(t, BuiltinDatatype{
			name:   "Interaction",
			fields: makefields([]*TypeField{}),
		}, value.Type(), "InteractionValue Type should be TypeInteraction")

		// Test Copy()
		clone := value.Copy()
		assert.Equal(t, value, clone, "Copy of Interaction should be equal to original")

		// Test Norm()
		norm := value.Norm()
		assert.Equal(t, map[string]any{}, norm, "Normalized value of Interaction should be equal to map[string]->any")

		// Test Data()
		assert.Equal(t, []byte{}, value.Data(), "POLO encoded bytes of Interaction should match expected value")
	})

	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("FuelPrice [0x10]", func(t *testing.T) {
			method := runtime.builtinClasses["Interaction"].methods[0x10]

			tests := []struct {
				res *U256Value
				err *Exception
			}{
				{res: &U256Value{uint256.NewInt(1)}, err: nil},
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

		t.Run("FuelLimit [0x11]", func(t *testing.T) {
			method := runtime.builtinClasses["Interaction"].methods[0x11]

			tests := []struct {
				res *U256Value
				err *Exception
			}{
				{res: &U256Value{uint256.NewInt(1000)}, err: nil},
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

		t.Run("InteractionType [0x12]", func(t *testing.T) {
			method := runtime.builtinClasses["Interaction"].methods[0x12]

			tests := []struct {
				res StringValue
				err *Exception
			}{
				{res: "IxLogicInvoke", err: nil},
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
