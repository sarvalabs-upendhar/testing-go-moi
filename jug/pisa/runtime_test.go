package pisa

import (
	"fmt"
	"testing"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/stretchr/testify/require"
)

func TestPrimitiveMethodIntegrity(t *testing.T) {
	runtime := NewRuntime()

	for primitive, methods := range runtime.bmethods {
		t.Run(primitive.String(), func(t *testing.T) {
			for code, method := range methods {
				if method == nil {
					continue
				}

				t.Run(fmt.Sprintf("%v [%#x]", method.Name, code), func(t *testing.T) {
					// Check that code of method matches its key
					require.Equal(t, MethodCode(code), method.Code)
					// Check that primitive and method type match
					require.Equal(t, primitive, method.Datatype)
					// Check that method ptr for primitive is not set
					require.Equal(t, engineio.ElementPtr(0), method.Ptr)

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
				})
			}
		})
	}
}

var specialMethodTests = map[MethodCode]func(t *testing.T, method Method){
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

func testBuildMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__build__", method.name())
	require.Equal(t, MethodBuild, method.code())
}

func testThrowMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__throw__", method.name())
	require.Equal(t, MethodThrow, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, PrimitiveString, method.callfields().Outputs.Get(0).Type)
}

func testEmitMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__emit__", method.name())
	require.Equal(t, MethodEmit, method.code())
}

func testJoinMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__join__", method.name())
	require.Equal(t, MethodJoin, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)
	require.Equal(t, method.datatype(), method.callfields().Outputs.Get(0).Type)
}

func testLtMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__lt__", method.name())
	require.Equal(t, MethodLt, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, "other", method.callfields().Inputs.Get(1).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

func testGtMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__gt__", method.name())
	require.Equal(t, MethodGt, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, "other", method.callfields().Inputs.Get(1).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

func testEqMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__eq__", method.name())
	require.Equal(t, MethodEq, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, "other", method.callfields().Inputs.Get(1).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(1).Type)
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

func testBoolMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__bool__", method.name())
	require.Equal(t, MethodBool, method.code())

	require.Equal(t, uint8(1), method.callfields().Inputs.Size())
	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, uint8(1), method.callfields().Outputs.Size())
	require.Equal(t, PrimitiveBool, method.callfields().Outputs.Get(0).Type)
}

func testStrMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__str__", method.name())
	require.Equal(t, MethodStr, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, PrimitiveString, method.callfields().Outputs.Get(0).Type)
}

func testAddrMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__addr__", method.name())
	require.Equal(t, MethodAddr, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, PrimitiveAddress, method.callfields().Outputs.Get(0).Type)
}

func testLenMethodIntegrity(t *testing.T, method Method) {
	t.Helper()

	require.Equal(t, "__len__", method.name())
	require.Equal(t, MethodLen, method.code())

	require.Equal(t, "self", method.callfields().Inputs.Get(0).Name)
	require.Equal(t, method.datatype(), method.callfields().Inputs.Get(0).Type)

	require.Equal(t, PrimitiveU64, method.callfields().Outputs.Get(0).Type)
}
