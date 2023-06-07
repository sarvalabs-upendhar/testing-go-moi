package pisa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
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
						testBuiltinMethodIntegrity(t, code, method, primitive)
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
						testBuiltinMethodIntegrity(t, code, method, builtin.datatype)
					})
				}
			})
		}
	})
}

func testBuiltinMethodIntegrity(t *testing.T, code int, method *BuiltinMethod, datatype Datatype) {
	t.Helper()

	// Check that code of method matches its key
	require.Equal(t, MethodCode(code), method.Code)
	// Check that primitive and method type match
	require.Equal(t, datatype, method.Datatype)
	// Check that method ptr for primitive is not set
	require.Equal(t, uint64(0), method.Ptr)

	// handle special method integrity
	if code < 0xF {
		tester, ok := specialMethodTests[MethodCode(code)]
		if !ok {
			panic(fmt.Sprintf("integrity test missing for code: %#x", code))
		}

		tester(t, method)

		return
	}

	// handle type method integrity
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)
}

var specialMethodTests = map[MethodCode]func(t *testing.T, method *BuiltinMethod){
	MethodBuild: testBuildMethodIntegrity,
	MethodThrow: testThrowMethodIntegrity,
	MethodEmit:  testEmitMethodIntegrity,
	MethodJoin:  testJoinMethodIntegrity,
	MethodLt:    testLtMethodIntegrity,
	MethodGt:    testGtMethodIntegrity,
	MethodEq:    testEqMethodIntegrity,
	MethodBool:  testBoolMethodIntegrity,
	MethodStr:   testStrMethodIntegrity,
	MethodAddr:  testAddrMethodIntegrity,
	MethodLen:   testLenMethodIntegrity,
}

func testBuildMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __build__
	require.Equal(t, "__build__", method.name())
	// Code must be MethodBuild [0x0]
	require.Equal(t, MethodBuild, method.code())

	// todo: add cost check for __build__
	// require.Equal(t, x, method.Cost)

	// Must have at least 1 input
	require.GreaterOrEqual(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and it must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// Must have exactly 1 output, and it must be the same type as method
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, method.datatype(), method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testThrowMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __throw__
	require.Equal(t, "__throw__", method.name())
	// Code must be MethodThrow [0x1]
	require.Equal(t, MethodThrow, method.code())
	// Cost must be 20 FUEL
	require.Equal(t, engineio.NewFuel(20), method.Cost)

	// Must have exactly 1 input
	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and it must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// Must have exactly 1 output, and it must be a string
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveString, method.callfields().Outputs.Get(0).Type)
}

//nolint:wsl
func testEmitMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __emit__
	require.Equal(t, "__emit__", method.name())
	// Code must be MethodEmit [0x2]
	require.Equal(t, MethodEmit, method.code())

	// todo: add cost check for __emit__
	// require.Equal(t, x, method.Cost)

	// Must have exactly 1 input
	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and it must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// todo: add check for output type Log
}

func testJoinMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __join__
	require.Equal(t, "__join__", method.name())
	// Code must be MethodJoin [0x3]
	require.Equal(t, MethodJoin, method.code())
	// Cost must be 20 FUEL
	require.Equal(t, engineio.NewFuel(20), method.Cost)

	// Must have exactly 2 inputs
	require.Equal(t, uint8(2), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and both inputs must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)

	// Must have exactly 1 output, which must be the same type as the method
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, method.datatype(), method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testLtMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __lt__
	require.Equal(t, "__lt__", method.name())
	// Code must be MethodLt [0x4]
	require.Equal(t, MethodLt, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 2 inputs
	require.Equal(t, uint8(2), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and both inputs must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)

	// Must have exactly 1 output, which must be a Bool
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testGtMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __gt__
	require.Equal(t, "__gt__", method.name())
	// Code must be MethodGt [0x5]
	require.Equal(t, MethodGt, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 2 inputs
	require.Equal(t, uint8(2), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and both inputs must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)

	// Must have exactly 1 output, which must be a Bool
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testEqMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __eq__
	require.Equal(t, "__eq__", method.name())
	// Code must be MethodEq [0x6]
	require.Equal(t, MethodEq, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 2 inputs
	require.Equal(t, uint8(2), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, and both inputs must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)

	// Must have exactly 1 output, which must be a Bool
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testBoolMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __bool__
	require.Equal(t, "__bool__", method.name())
	// Code must be MethodBool [0x7]
	require.Equal(t, MethodBool, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 1 input
	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, which must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// Must have exactly 1 output, which must be a Bool
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testStrMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __str__
	require.Equal(t, "__str__", method.name())
	// Code must be MethodStr [0x8]
	require.Equal(t, MethodStr, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 1 input
	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, which must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// Must have exactly 1 output, which must be a String
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveString, method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testAddrMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __addr__
	require.Equal(t, "__addr__", method.name())
	// Code must be MethodAddr [0x9]
	require.Equal(t, MethodAddr, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 1 input
	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, which must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// Must have exactly 1 output, which must be a Address
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveAddress, method.callfields().Outputs.Get(0).Type)
}

//nolint:dupl
func testLenMethodIntegrity(t *testing.T, method *BuiltinMethod) {
	t.Helper()

	// Name must be __len__
	require.Equal(t, "__len__", method.name())
	// Code must be MethodLen [0xA]
	require.Equal(t, MethodLen, method.code())
	// Cost must be 10 FUEL
	require.Equal(t, engineio.NewFuel(10), method.Cost)

	// Must have exactly 1 input
	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	// Must have 'self' as the first input, which must be of the same as the method
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	// Must have exactly 1 output, which must be a U64
	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveU64, method.callfields().Outputs.Get(0).Type)
}
