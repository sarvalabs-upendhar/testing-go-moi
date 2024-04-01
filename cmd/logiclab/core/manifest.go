package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/opcode"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-moi/common"
)

func PrintManifest(manifest *engineio.Manifest, encoding engineio.Encoding) string {
	switch encoding {
	case engineio.POLO:
		// Generate POLO data
		data, err := manifest.Encode(engineio.POLO)
		if err != nil {
			return fmt.Sprintf("unable to polo serialize manifest: %v", err)
		}

		// Encode as hex string and attach the 0x prefix
		return "0x" + hex.EncodeToString(data)

	case engineio.JSON:
		// Generate the indented JSON data
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Sprintf("unable to json marshal manifest: %v", err)
		}

		return common.BytesToHex(data)

	case engineio.YAML:
		// Create an encoding buffer
		var b bytes.Buffer
		// Create a new YAML encoder and set indent level
		encoder := yaml.NewEncoder(&b)
		encoder.SetIndent(2)

		// Generate the indented YAML data
		if err := encoder.Encode(manifest); err != nil {
			return fmt.Sprintf("unable to yaml marshal manifest: %v", err)
		}

		// Get the hex-encoded string directly from the buffer
		hexEncodedString := common.BytesToHex(b.Bytes())

		return hexEncodedString

	default:
		return fmt.Sprintf("[unimplemented] unsupported manifest encoding: %v", encoding)
	}
}

func ConvertManifestCodeform(original *engineio.Manifest, encoding engineio.Encoding, codeform string) string {
	switch codeform {
	case "BIN":
		// Generate BIN data of opcodes
		manifest, err := ConvertToBinCodeform(original)
		if err != nil {
			return fmt.Sprintf("%v", err)
		}

		return PrintManifest(manifest, encoding)

	case "HEX":
		// Generate HEX data of opcodes
		manifest, err := ConvertToHexCodeform(original)
		if err != nil {
			return fmt.Sprintf("%v", err)
		}

		return PrintManifest(manifest, encoding)

	case "ASM":
		// Generate ASM data of opcodes
		manifest, err := ConvertToAsmCodeform(original)
		if err != nil {
			return fmt.Sprintf("%v", err)
		}

		return PrintManifest(manifest, encoding)

	default:
		panic("unhandled manifest codeform conversion")
	}
}

func ConvertToBinCodeform(manifest *engineio.Manifest) (*engineio.Manifest, error) {
	for _, element := range manifest.Elements {
		switch element.Kind {
		case pisa.RoutineElement:
			routine, ok := element.Data.(*pisa.RoutineSchema)
			if !ok {
				return manifest, fmt.Errorf("unable to extract element data")
			}

			if routine.Executes.Hex != "" {
				bin, err := opcode.Hex2Bin(routine.Executes.Hex)
				if err != nil {
					return nil, err
				}

				routine.Executes.Bin = bin
				routine.Executes.Hex = ""
			}

			if len(routine.Executes.Asm) != 0 {
				bin, err := opcode.Asm2Bin(routine.Executes.Asm)
				if err != nil {
					return nil, err
				}

				routine.Executes.Bin = bin
				routine.Executes.Hex = ""
				routine.Executes.Asm = nil
			}

		case pisa.MethodElement:
			method, ok := element.Data.(*pisa.MethodSchema)
			if !ok {
				return manifest, fmt.Errorf("unable to extract element data")
			}

			if method.Executes.Hex != "" {
				bin, err := opcode.Hex2Bin(method.Executes.Hex)
				if err != nil {
					return nil, err
				}

				method.Executes.Bin = bin
				method.Executes.Hex = ""
			}

			if len(method.Executes.Asm) != 0 {
				bin, err := opcode.Asm2Bin(method.Executes.Asm)
				if err != nil {
					return nil, err
				}

				method.Executes.Bin = bin
				method.Executes.Hex = ""
				method.Executes.Asm = nil
			}
		}
	}

	return manifest, nil
}

func ConvertToHexCodeform(manifest *engineio.Manifest) (*engineio.Manifest, error) {
	for _, element := range manifest.Elements {
		switch element.Kind {
		case pisa.RoutineElement:
			routine, ok := element.Data.(*pisa.RoutineSchema)
			if !ok {
				return manifest, fmt.Errorf("unable to extract element data")
			}

			if !bytes.Equal(routine.Executes.Bin, []byte{}) {
				hexcode := opcode.Bin2Hex(routine.Executes.Bin)
				if hexcode == "" {
					return manifest, fmt.Errorf("error: failed to encode hex string")
				}

				routine.Executes.Bin = []byte("")
				routine.Executes.Hex = hexcode
			}
		case pisa.MethodElement:
			method, ok := element.Data.(*pisa.MethodSchema)
			if !ok {
				return manifest, fmt.Errorf("unable to extract element data")
			}

			if !bytes.Equal(method.Executes.Bin, []byte{}) {
				hexcode := opcode.Bin2Hex(method.Executes.Bin)
				if hexcode == "" {
					return manifest, fmt.Errorf("error: failed to encode hex string")
				}

				method.Executes.Bin = []byte("")
				method.Executes.Hex = hexcode
			}
		}
	}

	return manifest, nil
}

func ConvertToAsmCodeform(manifest *engineio.Manifest) (*engineio.Manifest, error) {
	for _, element := range manifest.Elements {
		switch element.Kind {
		case pisa.RoutineElement:
			routine, ok := element.Data.(*pisa.RoutineSchema)
			if !ok {
				return manifest, fmt.Errorf("unable to extract element data")
			}

			if routine.Executes.Hex != "" {
				manifest, err := ConvertToBinCodeform(manifest)
				if err != nil {
					return manifest, fmt.Errorf("error: %w", err)
				}
			}

			if !bytes.Equal(routine.Executes.Bin, []byte{}) {
				asm, err := opcode.Bin2Asm(routine.Executes.Bin)
				if err != nil {
					return manifest, err
				}

				routine.Executes.Bin = []byte("")
				routine.Executes.Hex = ""
				routine.Executes.Asm = asm
			}
		case pisa.MethodElement:
			method, ok := element.Data.(*pisa.MethodSchema)
			if !ok {
				return manifest, fmt.Errorf("unable to extract element data")
			}

			if method.Executes.Hex != "" {
				manifest, err := ConvertToBinCodeform(manifest)
				if err != nil {
					return manifest, fmt.Errorf("error: %w", err)
				}
			}

			if !bytes.Equal(method.Executes.Bin, []byte{}) {
				asm, err := opcode.Bin2Asm(method.Executes.Bin)
				if err != nil {
					return manifest, err
				}

				method.Executes.Bin = []byte("")
				method.Executes.Hex = ""
				method.Executes.Asm = asm
			}
		}
	}

	return manifest, nil
}

func EncodingFromString(encoding string) engineio.Encoding {
	// todo: return error for invalid option
	switch encoding {
	case "POLO":
		return engineio.POLO
	case "JSON":
		return engineio.JSON
	case "YAML":
		return engineio.YAML
	default:
		return engineio.POLO
	}
}
