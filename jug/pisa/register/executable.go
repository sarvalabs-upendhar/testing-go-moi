package register

import (
	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

type ExecutionScope interface {
	Throw(object *exception.Object)
	ExceptionThrown() bool
	GetException() *exception.Object
}

type Executable interface {
	Interface() CallFields
	Execute(ExecutionScope, ValueTable) ValueTable
}
