package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// CallEncoder implements the engineio.CallEncoder interface for PISA
type CallEncoder CallFields

// EncodeInputs will encode the given input data into a polo.Document based on the callfield's
// input field table. Will error if field objects are missing or are of incorrect type.
// Implements the engineio.CallEncoder interface for CallEncoder
func (encoder CallEncoder) EncodeInputs(inputs map[string]any, refs engineio.ReferenceProvider) ([]byte, error) {
	// If inputs are nil, return nil bytes
	if inputs == nil {
		return nil, nil
	}

	encoded := make(polo.Document)

	// Iterate through each object in the given inputs
	for name, object := range inputs {
		// Obtain the typefield for the input identifier, error if not found
		typefield := encoder.Inputs.Lookup(name)
		if typefield == nil {
			return nil, errors.Errorf("invalid input data for '%v': no such field", name)
		}

		// Encode the object, recursively
		data, err := engineio.EncodeValues(object, refs)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the input field. This confirms the given object is acceptable for the type field
		if _, err = NewRegisterValue(typefield.Type, data); err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		encoded.SetRaw(typefield.Name, data)
	}

	return encoded.Bytes(), nil
}

// DecodeOutputs will decode the given polo.Document of outputs into a map of objects based on the
// callfield's output field table. Will error if the field objects are missing or of incorrect type.
// Implements the engineio.CallEncoder interface for CallEncoder
func (encoder CallEncoder) DecodeOutputs(outputs []byte) (map[string]any, error) {
	// If output bytes are nil, return nil doc
	if outputs == nil {
		return nil, nil
	}

	// Decode the outputs into a polo.Document
	document := make(polo.Document)
	if err := polo.Depolorize(&document, outputs); err != nil {
		return nil, errors.Wrap(err, "could not decode outputs into polo document")
	}

	decoded := make(map[string]any)

	// Iterate through each field defined in the callfield's output table
	for name, data := range document {
		// Obtain the typefield for the input identifier, error if not found
		typefield := encoder.Outputs.Lookup(name)
		if typefield == nil {
			return nil, errors.Errorf("invalid output data for '%v': no such field", name)
		}

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the output field. This confirms that the data is acceptable for the type field
		value, err := NewRegisterValue(typefield.Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid output data for '%v'", typefield.Name)
		}

		// Convert the value into its normalized form
		decoded[typefield.Name] = value.Norm()
	}

	return decoded, nil
}
