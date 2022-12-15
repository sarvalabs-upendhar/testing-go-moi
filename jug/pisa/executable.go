package pisa

import (
	"fmt"

	"github.com/sarvalabs/moichain/types"
)

// Executable defines an interface for executable objects such as Routines and Methods.
type Executable interface {
	Interface() CallFields
	Execute(*ExecutionContext, ValueTable) (ValueTable, error)
}

// CallFields represents the input/output symbols for a callable routine.
type CallFields struct {
	Inputs  FieldTable
	Outputs FieldTable
}

// Signature generates a signature from the RoutineFields symbols and their typedata.
// It is structured as '(input1, input2)->(output1, output2)', where the values are type data of each field
func (fields CallFields) Signature() string {
	return fmt.Sprintf("%v->%v", fields.Inputs.String(), fields.Outputs.String())
}

// SigHash generates a signature hash from the RoutineFields symbols and their typedata.
// The signature is hashed and the last 8 characters of the digest are returned as a string.
func (fields CallFields) SigHash() string {
	return types.GetHash([]byte(fields.Signature())).Hex()[:8]
}

// Methods is a collection of Method objects
type Methods [256]Method

// Method defines an extended Executable interface for type methods
type Method interface {
	Executable

	Datatype() *Datatype
	Builtin() bool
}
