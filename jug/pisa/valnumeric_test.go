package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestU64Value(t *testing.T) {
	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveU64][0x10]

			tests := []struct {
				input U64Value
				res   U64Value
				err   *Exception
			}{
				{10, 10, nil},
				{100, 100, nil},
				{5, 5, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

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

func TestI64Value(t *testing.T) {
	t.Run("Methods", func(t *testing.T) {
		runtime := NewRuntime()

		t.Run("Abs [0x10]", func(t *testing.T) {
			method := runtime.primitiveMethods[PrimitiveI64][0x10]

			tests := []struct {
				input I64Value
				res   I64Value
				err   *Exception
			}{
				{-10, 10, nil},
				{100, 100, nil},
				{-5, 5, nil},
			}

			for _, test := range tests {
				scope := &callscope{engine: &Engine{callstack: make(callstack, 0), runtime: &runtime}}
				outputs, except := method.Builtin.runner(scope.engine, RegisterSet{0: test.input})

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
