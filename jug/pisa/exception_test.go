package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestException(t *testing.T) {
	trace := []string{"function1()", "function2()"}
	data := "Something went wrong"
	class := CustomExceptionClass{datatype: TypeString}

	except := exception(class, trace, data)

	if except.Class != TypeString.String() {
		t.Errorf("Expected exception class to be 'string', but got '%s'", except.Class)
	}

	if except.Data != data {
		t.Errorf("Expected exception data to be '%s', but got '%s'", data, except.Data)
	}

	if len(except.Trace) != 2 {
		t.Errorf("Expected exception trace to have length 2, but got %d", len(except.Trace))
	}
}

func TestExceptionf(t *testing.T) {
	trace := []string{"function1()", "function2()"}
	format := "Something went wrong: %v"
	arg := "argument"
	expectedData := "Something went wrong: argument"
	class := CustomExceptionClass{datatype: TypeString}

	except := exceptionf(class, trace, format, arg)

	if except.Class != TypeString.String() {
		t.Errorf("Expected exception class to be 'string', but got '%s'", except.Class)
	}

	if except.Data != expectedData {
		t.Errorf("Expected exception data to be '%s', but got '%s'", expectedData, except.Data)
	}

	if len(except.Trace) != 2 {
		t.Errorf("Expected exception trace to have length 2, but got %d", len(except.Trace))
	}
}

func TestExceptionString(t *testing.T) {
	except := &Exception{Class: "TestException", Data: "test data", Trace: []string{"frame1", "frame2"}}
	str := except.String()

	require.Equal(t, "pisa.Exception [TestException]\nerror: test data\n-| frame2\n--| frame1", str)
}

func TestExceptionWrap(t *testing.T) {
	except := &Exception{Class: "TestException", Data: "test data", Trace: []string{"frame1", "frame2"}}

	except = except.Wrap("frame3")

	expectedTrace := []string{"frame1", "frame2", "frame3"}
	if len(except.Trace) != len(expectedTrace) {
		t.Errorf("Wrap() did not append frame to trace. Expected trace length: %v, Got: %v",
			len(expectedTrace), len(except.Trace),
		)
	}

	for i, frame := range expectedTrace {
		if except.Trace[i] != frame {
			t.Errorf("Wrap() did not append frame to trace. Expected trace: %v, Got: %v", expectedTrace, except.Trace)

			break
		}
	}
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
