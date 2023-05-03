package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestException(t *testing.T) {
	data := "Something went wrong"
	class := CustomExceptionClass{datatype: TypeString}

	except := exception(class, data)

	if except.Class != TypeString.String() {
		t.Errorf("Expected exception class to be 'string', but got '%s'", except.Class)
	}

	if except.Error != data {
		t.Errorf("Expected exception data to be '%s', but got '%s'", data, except.Error)
	}
}

func TestExceptionf(t *testing.T) {
	format := "Something went wrong: %v"
	arg := "argument"

	expectedData := "Something went wrong: argument"
	class := CustomExceptionClass{datatype: TypeString}

	except := exceptionf(class, format, arg)

	if except.Class != TypeString.String() {
		t.Errorf("Expected exception class to be 'string', but got '%s'", except.Class)
	}

	if except.Error != expectedData {
		t.Errorf("Expected exception data to be '%s', but got '%s'", expectedData, except.Error)
	}
}

func TestException_Traced(t *testing.T) {
	data := "Something went wrong"
	class := CustomExceptionClass{datatype: TypeString}
	trace := []string{"function1()", "function2()"}

	except := exception(class, data).traced(trace)

	if except.Class != TypeString.String() {
		t.Errorf("Expected exception class to be 'string', but got '%s'", except.Class)
	}

	if except.Error != data {
		t.Errorf("Expected exception data to be '%s', but got '%s'", data, except.Error)
	}

	if len(except.Trace) != 2 {
		t.Errorf("Expected exception trace to have length 2, but got %d", len(except.Trace))
	}
}

func TestExceptionString(t *testing.T) {
	except := &Exception{Class: "TestException", Error: "test data", Trace: []string{"frame1", "frame2"}}
	str := except.String()

	require.Equal(t, "pisa.Exception [TestException]\nerror: test data\n-| frame2\n--| frame1", str)
}

func TestCustomExceptionClass(t *testing.T) {
	datatype := &Datatype{Ident: "MyCustomError"}

	err := &CustomExceptionClass{datatype: datatype}
	assert.Equal(t, datatype.String(), err.Name())
}

func TestBuiltinExceptionClass(t *testing.T) {
	cases := []struct {
		err BuiltinExceptionClass
	}{
		{err: Ok},
		{err: FuelError},
		{err: InitError},
		{err: CallError},
		{err: RuntimeError},
		{err: ReferenceError},
		{err: NotImplementedError},
		{err: TypeError},
		{err: ValueError},
		{err: AccessError},
		{err: OverflowError},
		{err: DivideByZeroError},
	}

	for _, c := range cases {
		err := c.err
		name := err.Name()

		require.True(t, len(name) > 0)
		assert.Contains(t, name, "builtin.")
	}
}
