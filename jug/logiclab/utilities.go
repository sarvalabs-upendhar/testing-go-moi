package logiclab

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/sarvalabs/go-polo"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
)

// ManifestPrintCommand generates a command runner to print a
// manifest file at the given path in the expected output encoding.
// - POLO: POLO Hex String
// - JSON: JSON Indented String
// - YAML: YAML Indented String
func ManifestPrintCommand(path, encoding string) Command {
	return func(env *Environment) string {
		// Read manifest file from path
		manifest, err := engineio.ReadManifestFile(path)
		if err != nil {
			return fmt.Sprintf("unable to read manifest: %v", err)
		}

		switch encoding {
		case "POLO":
			// Generate POLO data
			data, err := manifest.Encode(engineio.POLO)
			if err != nil {
				return fmt.Sprintf("unable to polo serialize manifest: %v", err)
			}

			// Encode as hex string and attach the 0x prefix
			return "0x" + hex.EncodeToString(data)

		case "JSON":
			// Generate the indented JSON data
			data, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return fmt.Sprintf("unable to json marshal manifest: %v", err)
			}

			return string(data)

		case "YAML":
			// Create an encoding buffer
			var b bytes.Buffer
			// Create a new YAML encoder and set indent level
			encoder := yaml.NewEncoder(&b)
			encoder.SetIndent(2)

			// Generate the indented YAML data
			if err = encoder.Encode(manifest); err != nil {
				return fmt.Sprintf("unable to yaml marshal manifest: %v", err)
			}

			return b.String()

		default:
			panic("unhandled manifest print encoding")
		}
	}
}

// SlothashCommand generates a command runner to generate
// the slothash for a given storage path. Currently, only
// accepts a single uint8 slot number but will be extended.
func SlothashCommand(slot uint64) Command {
	return func(env *Environment) string {
		if slot > math.MaxUint8 {
			return "slot number is too large"
		}

		return hex.EncodeToString(pisa.SlotHash(uint8(slot)))
	}
}

// ErrDecodePISAValueCommand generates a command runner to
// decode the error object for PISA from the given error bytes data.
func ErrDecodePISAValueCommand(errdata []byte) Command {
	return func(env *Environment) string {
		exception := new(pisa.Exception)

		if err := polo.Depolorize(exception, errdata); err != nil {
			return fmt.Sprintf("failed to decode error data into pisa.Exception: %v", err)
		}

		return exception.String()
	}
}

// ErrDecodePISAMemoryCommand generates a command runner to
// decode the error object for PISA from the error bytes data
// at the given identifier in the lab memory
func ErrDecodePISAMemoryCommand(ident string) Command {
	return func(env *Environment) string {
		value, ok := env.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		errdata, ok := value.([]byte)
		if !ok {
			return fmt.Sprintf("'%v' is not an hex value", ident)
		}

		return ErrDecodePISAValueCommand(errdata)(env)
	}
}

func CallDecodeMemoryCommand(ident, name, site string) Command {
	return func(env *Environment) string {
		value, ok := env.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		data, ok := value.([]byte)
		if !ok {
			return fmt.Sprintf("'%v' is not an hex value", ident)
		}

		return CallDecodeValueCommand(data, name, site)(env)
	}
}

func CallDecodeValueCommand(data []byte, name, site string) Command {
	return func(env *Environment) string {
		// Find the logic from the inventory
		logic, exists := env.inventory.FindLogic(name)
		if !exists {
			return fmt.Sprintf("logic '%v' does not exist", name)
		}

		// Get the callsite from the logic, error if not found
		callsite, ok := logic.Object.GetCallsite(site)
		if !ok {
			return fmt.Sprintf("logic '%v' does not have callsite '%v'", name, callsite)
		}

		// Obtain the runtime for the logic engine in the header
		runtime, ok := engineio.FetchEngineRuntime(logic.Object.Engine())
		if !ok {
			return "failed to get runtime for logic"
		}

		// Generate the call encoder for the callsite
		encoder, err := runtime.GetCallEncoder(callsite, logic.Object)
		if err != nil {
			return fmt.Sprintf("failed to generate call encoder for callsite '%v'", callsite)
		}

		// Decode the outputs with the CallEncoder
		outputs, err := encoder.DecodeOutputs(data)
		if err != nil {
			return fmt.Sprintf("failed decode data for callsite '%v': %v", callsite, err)
		}

		var str strings.Builder

		str.WriteString("Outputs ||| ")

		for label, object := range outputs {
			str.WriteString(fmt.Sprintf("%v: %v ", label, object))
		}

		return str.String()
	}
}

// CallgenMemoryCommand generates a command runner to generate
// a doc-encoded calldata string from an object in the lab memory
func CallgenMemoryCommand(ident string) Command {
	return func(env *Environment) string {
		value, ok := env.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		return CallgenValueCommand(value)(env)
	}
}

// CallgenValueCommand generates a command runner to generate
// a doc-encoded calldata string from a given value object
func CallgenValueCommand(value any) Command {
	return func(env *Environment) string {
		object, ok := value.(map[string]any)
		if !ok {
			return "value is not an object"
		}

		doc := make(polo.Document)

		for key, val := range object {
			data, err := engineio.EncodeValues(val, env)
			if err != nil {
				return fmt.Sprintf("could not encode value into calldata: %v", err)
			}

			doc.SetRaw(key, data)
		}

		return "0x" + hex.EncodeToString(doc.Bytes())
	}
}
