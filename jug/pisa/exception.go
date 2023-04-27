package pisa

import (
	"fmt"
	"strings"

	"github.com/sarvalabs/go-polo"
)

// ExceptionClass is denotes the Exception group
// for an error that occurs during execution
type ExceptionClass interface {
	Name() string
}

// Exception is the error object for PISA
type Exception struct {
	Class string
	Data  string
	Trace []string
}

func exception(class ExceptionClass, trace []string, data string) *Exception {
	return &Exception{Class: class.Name(), Data: data, Trace: trace}
}

func exceptionf(code ExceptionClass, trace []string, format string, a ...any) *Exception {
	return exception(code, trace, fmt.Sprintf(format, a...))
}

func (except Exception) Bytes() []byte {
	data, _ := polo.Polorize(except)

	return data
}

func (except Exception) String() string {
	var str strings.Builder

	str.WriteString(fmt.Sprintf("pisa.Exception [%v]\n", except.Class))
	str.WriteString(fmt.Sprintf("error: %v\n", except.Data))

	for i := len(except.Trace) - 1; i >= 0; i-- {
		str.WriteString(strings.Repeat("-", len(except.Trace)-i) + "| " + except.Trace[i])

		if i != 0 {
			str.WriteString("\n")
		}
	}

	return str.String()
}

func (except *Exception) Wrap(frame string) *Exception {
	return &Exception{
		Class: except.Class,
		Data:  except.Data,
		Trace: append(except.Trace, frame),
	}
}

type CustomExceptionClass struct {
	datatype *Datatype
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
