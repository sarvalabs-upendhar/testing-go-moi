package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/compute/engineio"
)

func TestRoutine_name(t *testing.T) {
	r := Routine{Name: "testRoutine"}
	require.Equal(t, "testRoutine", r.name())
}

func TestRoutine_ptr(t *testing.T) {
	r := Routine{Ptr: 123}
	require.Equal(t, engineio.ElementPtr(123), r.ptr())
}

func TestRoutineMethod_code(t *testing.T) {
	rm := RoutineMethod{Code: MethodCode(0x0)}
	require.Equal(t, MethodCode(0x0), rm.code())
}

func TestRoutineMethod_datatype(t *testing.T) {
	rm := RoutineMethod{Datatype: PrimitiveString}
	require.Equal(t, PrimitiveString, rm.datatype())
}
