package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func TestRoutine_exported(t *testing.T) {
	r := Routine{Name: "PublicRoutine"}
	assert.True(t, r.exported())

	r = Routine{Name: "privateRoutine"}
	assert.False(t, r.exported())
}

func TestRoutine_mutable(t *testing.T) {
	r := Routine{Name: "MutableRoutine!"}
	assert.True(t, r.mutable())

	r = Routine{Name: "PureRoutine"}
	assert.False(t, r.mutable())
}

func TestRoutine_payable(t *testing.T) {
	r := Routine{Name: "Expensive$"}
	assert.True(t, r.payable())

	r = Routine{Name: "Free"}
	assert.False(t, r.payable())
}

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
