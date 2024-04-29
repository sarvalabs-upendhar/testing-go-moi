package pisa

import (
	"reflect"
	"sort"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/logic"
	"github.com/sarvalabs/go-pisa/values"
	"github.com/sarvalabs/go-polo"
)

// CallEncoder is a tool for  inputs and decoding outputs for a specific callsite
type CallEncoder logic.CallFields

// NewCallEncoder generates a new CallEncoder from a logic.Driver for a given callsite.
// Returns an error if the callsite does not exist on the logic.
func NewCallEncoder(driver logic.Driver, callsite string) (CallEncoder, error) {
	ptr, ok := driver.Callsite(callsite)
	if !ok {
		return CallEncoder{}, errors.Errorf("callsite '%v' not found", callsite)
	}

	// Get LogicElement from the LogicDriver
	element, ok := driver.Element(ptr)
	if !ok {
		return CallEncoder{}, errors.Errorf("cannot find logic element for callsite '%v'", callsite)
	}

	// Decode the element data into a Routine object
	routine := new(pisa.Routine)
	if err := polo.Depolorize(routine, element.Data); err != nil {
		return CallEncoder{}, errors.Wrap(err, "failed to decode callsite element into Routine")
	}

	// Return the routine callfields as a CallEncoder
	return CallEncoder(routine.CallFields), nil
}

// EncodeInputs will encode the given input data into a polo.Document based on the callfield's
// input field table. Will error if field objects are missing or are of incorrect type.
// Implements the engineio.CallEncoder interface for CallEncoder
func (encoder CallEncoder) EncodeInputs(inputs map[string]any) ([]byte, error) {
	// If inputs are nil, return nil bytes
	if inputs == nil {
		return nil, nil
	}

	encoded := make(polo.Document)

	// Iterate through each field defined in the callfield's input table
	for typefield := range encoder.Inputs.Iter() {
		object, ok := inputs[typefield.Name]
		if !ok {
			return nil, errors.Errorf("missing input data for '%v'", typefield.Name)
		}

		// Encode the object, recursively
		data, err := EncodeValues(object)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid input data for '%v'", typefield.Name)
		}

		// Attempt to create a values.Register object from the encoded data for the type specified
		// in the input field. This confirms the given object is acceptable for the type field
		if _, err = values.NewMemoryRegister(typefield.Type, data); err != nil {
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
	for typefield := range encoder.Outputs.Iter() {
		data, ok := document[typefield.Name]
		if !ok {
			return nil, errors.Errorf("missing output data for '%v'", typefield.Name)
		}

		// Attempt to create a register.Value object from the encoded data for the type specified
		// in the output field. This confirms that the data is acceptable for the type field
		value, err := values.NewMemoryRegister(typefield.Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid output data for '%v'", typefield.Name)
		}

		// Convert the value into its normalized form
		decoded[typefield.Name] = value.Norm()
	}

	return decoded, nil
}

// EncodeDocument encodes a value into a POLO Document, recursivel
func EncodeDocument(value map[string]any) (polo.Document, error) {
	document := make(polo.Document)

	// For each field in the object
	for key, val := range value {
		// Encode field value
		data, err := EncodeValues(val)
		if err != nil {
			return nil, err
		}

		document.SetRaw(key, data)
	}

	return document, nil
}

// EncodeValues encodes a value into some POLO bytes, recursively resolving any internal type data.
func EncodeValues(value any) ([]byte, error) {
	switch val := value.(type) {
	// Object (ClassType)
	case map[string]any:
		document, err := EncodeDocument(val)
		if err != nil {
			return nil, err
		}

		return document.Bytes(), nil

	// Map (MapType)
	case map[any]any:
		// Create a new Polorizer
		polorizer := polo.NewPolorizer()

		// Reflect the value object and sort its keys
		reflected := reflect.ValueOf(val)
		keys := reflected.MapKeys()
		sort.Slice(keys, polo.ValueSort(keys))

		// Iterate over the sorted keys and encode
		// each key-value pair to the polorizer
		for _, key := range keys {
			// Encode key value
			kdata, err := EncodeValues(key.Interface())
			if err != nil {
				return nil, err
			}

			// Encode val value
			vdata, err := EncodeValues(reflected.MapIndex(key).Interface())
			if err != nil {
				return nil, err
			}

			// Write both key and val data into polorizer
			_ = polorizer.PolorizeAny(kdata)
			_ = polorizer.PolorizeAny(vdata)
		}

		return polorizer.Bytes(), nil

	// List (ArrayType & VarrayType)
	case []any:
		// Create a new Polorizer
		polorizer := polo.NewPolorizer()

		// For each element in the list
		for _, elem := range val {
			// Encode element value
			data, err := EncodeValues(elem)
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
