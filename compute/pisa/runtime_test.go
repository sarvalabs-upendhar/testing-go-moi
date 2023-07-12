package pisa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

func TestRuntimeBuiltinIntegrity(t *testing.T) {
	runtime := NewRuntime()

	t.Run("Primitive", func(t *testing.T) {
		for primitive, methods := range runtime.primitiveMethods {
			t.Run(primitive.String(), func(t *testing.T) {
				for code, method := range methods {
					if method == nil {
						continue
					}

					t.Run(fmt.Sprintf("%v [%#x]", method.Name, code), func(t *testing.T) {
						testBuiltinMethodIntegrity(t, MethodCode(code), method, primitive)
					})
				}
			})
		}
	})

	t.Run("Builtin Library", func(t *testing.T) {
		for ptr, builtin := range runtime.builtinLibrary {
			require.Equal(t, ptr, builtin.Ptr)
		}
	})

	t.Run("Builtins Classes", func(t *testing.T) {
		for name, builtin := range runtime.builtinClasses {
			t.Run(name, func(t *testing.T) {
				require.Equal(t, name, builtin.datatype.name)

				for code, method := range builtin.methods {
					if method == nil {
						continue
					}

					t.Run(fmt.Sprintf("%v [%#x]", method.Name, code), func(t *testing.T) {
						testBuiltinMethodIntegrity(t, MethodCode(code), method, builtin.datatype)
					})
				}
			})
		}
	})
}

func testBuiltinMethodIntegrity(t *testing.T, code MethodCode, method *BuiltinMethod, datatype Datatype) {
	t.Helper()

	// Check that code of method matches its key
	require.Equal(t, code, method.Code)
	// Check that primitive and method type match
	require.Equal(t, datatype, method.Datatype)
	// Check that method ptr for primitive is not set
	require.Equal(t, uint64(0), method.Ptr)

	// handle special method integrity
	if code < MaxSpecialMethod {
		validator, ok := methodValidators[code]
		if !ok {
			panic(fmt.Sprintf("integrity test missing for code: %#x", code))
		}

		// Validate the special method
		err := validator(method)
		require.NoError(t, err)

		// Validate the builtin method cost
		require.Equal(t, engineio.NewFuel(expectedBuiltinCosts[code]), method.Cost)

		return
	}

	// handle type method integrity
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)
}

var expectedBuiltinCosts = map[MethodCode]uint64{
	// MethodBuild: 10,
	MethodThrow: 20,
	// MethodEmit: 20,
	MethodJoin: 20,
	MethodLt:   10,
	MethodGt:   10,
	MethodEq:   10,
	MethodBool: 10,
	MethodStr:  10,
	MethodAddr: 10,
	MethodLen:  10,
}
