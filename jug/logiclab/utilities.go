package logiclab

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

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

// CallgenMemoryCommand generates a command runner to generate
// a doc-encoded calldata string from an object in the lab memory
func CallgenMemoryCommand(ident string) Command {
	return func(env *Environment) string {
		value, ok := env.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Sprintf("'%v' is not an object", ident)
		}

		calldata, err := generateCalldata(object, env)
		if err != nil {
			return fmt.Sprintf("could not encode '%v' into calldata: %v", ident, err)
		}

		return calldata
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

		calldata, err := generateCalldata(object, env)
		if err != nil {
			return fmt.Sprintf("could not encode value into calldata: %v", err)
		}

		return calldata
	}
}

// generateCalldata generates a calldata string (hex-encoded) for a given value
// object map[string]any. The values are doc-encoded with POLO for the given keys
func generateCalldata(object map[string]any, refs engineio.ReferenceProvider) (string, error) {
	doc := make(polo.Document)

	for key, val := range object {
		data, err := engineio.EncodeValues(val, refs)
		if err != nil {
			return "", err
		}

		doc.SetRaw(key, data)
	}

	return "0x" + hex.EncodeToString(doc.Bytes()), nil
}
