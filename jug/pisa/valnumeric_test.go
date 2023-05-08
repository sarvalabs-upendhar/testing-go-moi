package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestU64Methods(t *testing.T) {
	runtime := NewRuntime()

	t.Run("Abs [0x10]", func(t *testing.T) {
		tests := []struct {
			input U64Value
			res   U64Value
			err   *Exception
		}{
			{10, 10, nil},
			{100, 100, nil},
			{5, 5, nil},
		}
		method := methodsU64()[0x10]
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
}

func TestI64Methods(t *testing.T) {
	runtime := NewRuntime()

	t.Run("Abs [0x10]", func(t *testing.T) {
		tests := []struct {
			input I64Value
			res   I64Value
			err   *Exception
		}{
			{-10, 10, nil},
			{100, 100, nil},
			{-5, 5, nil},
		}
		method := methodsI64()[0x10]
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
}
