package pisa

import (
	"reflect"
	"sort"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/register"
)

// CallEncoder implements the engineio.CallEncoder interface for PISA
type CallEncoder engineio.CallFields

// EncodeInputs will encode the given input data into a polo.Document based on the callfield's
// input field table. Will error if field objects are missing or are of incorrect type.
// Implements the engineio.CallEncoder interface for CallEncoder
func (encoder CallEncoder) EncodeInputs(inputs map[string]any) (polo.Document, error) {
	encoded := make(polo.Document)

	// Iterate through each object in the given inputs
	for name, object := range inputs {
		// Obtain the typefield for the input identifier, error if not found
		typefield := encoder.Inputs.Lookup(name)
		if typefield == nil {
			return nil, errors.Errorf("invalid input data for '%v': no such field", name)
		}

		// Encode the object, recursively
		data, err := encodeValues(object)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the input field. This confirms the given object is acceptable for the type field
		if _, err = register.NewValue(typefield.Type, data); err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		encoded.SetRaw(typefield.Name, data)
	}

	return encoded, nil
}

// DecodeOutputs will decode the given polo.Document of outputs into a map of objects based on the
// callfield's output field table. Will error if the field objects are missing or of incorrect type.
// Implements the engineio.CallEncoder interface for CallEncoder
func (encoder CallEncoder) DecodeOutputs(outputs polo.Document) (map[string]any, error) {
	decoded := make(map[string]any)

	// Iterate through each field defined in the callfield's output table
	for name, data := range outputs {
		// Obtain the typefield for the input identifier, error if not found
		typefield := encoder.Outputs.Lookup(name)
		if typefield == nil {
			return nil, errors.Errorf("invalid output data for '%v': no such field", name)
		}

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the output field. This confirms that the data is acceptable for the type field
		value, err := register.NewValue(typefield.Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid output data for '%v'", typefield.Name)
		}

		// Convert the value into its normalized form
		decoded[typefield.Name] = value.Norm()
	}

	return decoded, nil
}

// encodeValues encodes a value into a bytes, recursively resolving any internal type data
func encodeValues(value any) ([]byte, error) {
	switch val := value.(type) {
	// Object Type (ClassType)
	case map[string]any:
		document := make(polo.Document)

		// For each field in the object
		for field, v := range val {
			// Encode field value
			data, err := encodeValues(v)
			if err != nil {
				return nil, err
			}

			document.SetRaw(field, data)
		}

		return document.Bytes(), nil

	// Map Type (MapType)
	case map[any]any:
		// Create a new Polorizer
		polorizer := polo.NewPolorizer()

		// Reflect the value object and sort its keys
		reflected := reflect.ValueOf(val)
		keys := reflected.MapKeys()
		sort.Slice(keys, register.MapSorter(keys))

		// Iterate over the sorted keys and encode
		// each key-value pair to the polorizer
		for _, key := range keys {
			// Encode key value
			kdata, err := encodeValues(key.Interface())
			if err != nil {
				return nil, err
			}

			// Encode val value
			vdata, err := encodeValues(reflected.MapIndex(key).Interface())
			if err != nil {
				return nil, err
			}

			// Write both key and val data into polorizer
			_ = polorizer.PolorizeAny(kdata)
			_ = polorizer.PolorizeAny(vdata)
		}

		return polorizer.Bytes(), nil

	// List Type (ArrayType & VarrayType)
	case []any:
		// Create a new Polorizer
		polorizer := polo.NewPolorizer()

		// For each element in the list
		for _, elem := range val {
			// Encode element value
			data, err := encodeValues(elem)
			if err != nil {
				return nil, err
			}

			// Write element data into polorizer
			_ = polorizer.PolorizeAny(data)
		}

		return polorizer.Bytes(), nil

	// Simple Type
	default:
		data, err := polo.Polorize(val)
		if err != nil {
			return nil, err
		}

		return data, nil
	}
}
