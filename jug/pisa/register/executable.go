package register

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

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

// CallFields represents the input/output symbols for a callable routine.
type CallFields struct {
	Inputs  *FieldTable
	Outputs *FieldTable
}

// EncodeInputs will encode the given input data into a polo.Document based on the callfield's
// input field table. Will error if field objects are missing or are of incorrect type.
// Implements the engineio.CallEncoder interface for CallFields
func (fields CallFields) EncodeInputs(inputs map[string]any) (polo.Document, error) {
	encoded := make(polo.Document)

	// Iterate through each field defined in the callfield's input table
	for _, typefield := range fields.Inputs.Table {
		// Obtain the type object for the field from the inputs, error if not found
		object, ok := inputs[typefield.Name]
		if !ok {
			return nil, errors.Errorf("missing input data for '%v'", typefield.Name)
		}

		// Polorize the object, reflectively
		data, err := polo.Polorize(object)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the input field. This confirms the given object is acceptable for the type field
		if _, err = NewValue(typefield.Type, data); err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		encoded.SetRaw(typefield.Name, data)
	}

	return encoded, nil
}

// DecodeOutputs will decode the given polo.Document of outputs into a map of objects based on the
// callfield's output field table. Will error if the field objects are missing or of incorrect type.
// Implements the engineio.CallEncoder interface for CallFields
func (fields CallFields) DecodeOutputs(outputs polo.Document) (map[string]any, error) {
	decoded := make(map[string]any)

	// Iterate through each field defined in the callfield's output table
	for _, typefield := range fields.Outputs.Table {
		// Obtain the data for the field in the output document
		data := outputs.GetRaw(typefield.Name)

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the output field. This confirms that the data is acceptable for the type field
		value, err := NewValue(typefield.Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid output data for '%v'", typefield.Name)
		}

		// Convert the value into its normalized form
		decoded[typefield.Name] = value.Norm()
	}

	return decoded, nil
}
