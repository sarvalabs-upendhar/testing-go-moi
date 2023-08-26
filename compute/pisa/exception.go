package pisa

import (
	"fmt"
	"strings"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

// ExceptionClass is denotes the Exception group
// for an error that occurs during execution
type ExceptionClass interface {
	Name() string
}

// Exception is the error object for PISA
type Exception struct {
	Class string
	Error string
	Trace []string
}

func exception(class ExceptionClass, data string) *Exception {
	return &Exception{Class: class.Name(), Error: data}
}

func exceptionf(code ExceptionClass, format string, a ...any) *Exception {
	return exception(code, fmt.Sprintf(format, a...))
}

func (except *Exception) traced(trace []string) *Exception {
	return &Exception{
		Class: except.Class,
		Error: except.Error,
		Trace: trace,
	}
}

func (except Exception) Engine() engineio.EngineKind { return engineio.PISA }

func (except Exception) Bytes() []byte {
	data, _ := polo.Polorize(except)

	return data
}

func (except Exception) String() string {
	var str strings.Builder

	str.WriteString(fmt.Sprintf("pisa.Exception [%v]\n", except.Class))
	str.WriteString(fmt.Sprintf("error: %v\n", except.Error))

	for i := len(except.Trace) - 1; i >= 0; i-- {
		str.WriteString(strings.Repeat("-", len(except.Trace)-i) + "| " + except.Trace[i])

		if i != 0 {
			str.WriteString("\n")
		}
	}

	return str.String()
}

type CustomExceptionClass struct {
	datatype Datatype
}

func (err CustomExceptionClass) Name() string {
	return err.datatype.String()
}

type BuiltinExceptionClass int

const (
	Ok BuiltinExceptionClass = iota
	FuelError
	InitError
	CallError

	RuntimeError
	ReferenceError
	NotImplementedError

	TypeError
	ValueError
	AccessError

	OverflowError
	DivideByZeroError
)

var builtinExceptionToString = map[BuiltinExceptionClass]string{
	Ok:                  "Ok",
	FuelError:           "FuelError",
	InitError:           "InitError",
	CallError:           "CallError",
	RuntimeError:        "RuntimeError",
	ReferenceError:      "ReferenceError",
	NotImplementedError: "NotImplementedError",
	TypeError:           "TypeError",
	ValueError:          "ValueError",
	AccessError:         "AccessError",
	OverflowError:       "OverflowError",
	DivideByZeroError:   "DivideByZeroError",
}

func (err BuiltinExceptionClass) Name() string {
	str, ok := builtinExceptionToString[err]
	if !ok {
		panic("unknown ErrorCode variant")
	}

	return "builtin." + str
}

func exceptionNullRegister(register byte) *Exception {
	return exceptionf(ValueError, "$%v is null", register)
}

func exceptionInvalidDatatype[K string | DatatypeKind | PrimitiveDatatype](kind K, register byte) *Exception {
	return exceptionf(TypeError, "not a %v: $%v", kind, register)
}

func exceptionInvalidOperationForType(op string, datatype Datatype) *Exception {
	return exceptionf(ValueError, "cannot %v with %v registers", op, datatype)
}

func exceptionOutOfBounds() *Exception {
	return exception(AccessError, "index out of bounds")
}
