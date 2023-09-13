package cmds

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/opcode"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/state"
)

func HelpManifest() string {
	return `
@>Manifests<@ are composite deployment artifacts for [@>logics<@] and are usually found as JSON, YAML or POLO files.
Logic Manifest Spec: https://sarvalabs.notion.site/Logic-Manifest-Standard-93f5fee1af8d4c3cad155b9827b97930?pvs=4 

LogicLab can manipulate Manifests to [@>deploy<@] as a logic or [@>convert<@] to other formats using the manifest
expression which accepts either a relative filepath (string) or a raw manifest bytes (hex-encoded polo data). 
Using the expression directly will print the manifest back in its hex-encoded polo form.

supported expression formats:
@>manifest("[filepath]")<@
@>manifest([hex-encoded polo])<@
`
}

func parseManifestCommand(parser *symbolizer.Parser) Command {
	manifest, _, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	return func(env *Environment) string {
		return PrintManifest(manifest, engineio.POLO)
	}
}

// returns a manifest, its filepath (empty for raw manifest expressions) and an error
func parseManifestExpression(parser *symbolizer.Parser) (*engineio.Manifest, string, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, "", errors.New("invalid manifest expression: missing '('")
	}

	// Parse the expression contents inside the ()
	inner, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid manifest expression")
	}

	parserInner := symbolizer.NewParser(inner)
	// Check the inner value is valid
	if !parserInner.Cursor().Kind.CanValue() {
		return nil, "", errors.New("invalid manifest expression")
	}

	// Parse the value within the parenthesis
	value, err := parserInner.Cursor().Value()
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid manifest expression")
	}

	switch parserInner.Cursor().Kind {
	case symbolizer.TokenString:
		// value is a string
		fpath, _ := value.(string)

		// Read the manifest at the given filepath
		manifest, err := engineio.ReadManifestFile(fpath)
		if err != nil {
			return nil, "", fmt.Errorf("invalid manifest file: %w", err)
		}

		return manifest, fpath, nil

	case symbolizer.TokenHexNumber:
		// value is some bytes
		raw, _ := value.([]byte)

		manifest, err := engineio.NewManifest(raw, engineio.POLO)
		if err != nil {
			return nil, "", errors.Wrap(err, "invalid raw manifest")
		}

		return manifest, "", nil

	default:
		return nil, "", errors.New("invalid manifest expression")
	}
}

func HelpConvert() string {
	return `
The @>convert<@ can be used to convert a [@>manifest<@] into other encoding formats 
(with the @>as<@ preposition) or code formats (with the @>into<@ preposition). 

Supported Encoding Values: @>JSON<@, @>YAML<@ and @>POLO<@
Supported Codeform Values: @>BIN<@, @>HEX<@ and @>ASM<@

usage:
@>convert manifest(...) as [encoding]<@
@>convert manifest(...) into [codeform]<@

examples:
>> convert manifest("./manifests/ledger.yaml") as JSON
// prints JSON object

>> convert manifest("./manifests/ledger.yaml") as POLO
// prints hex encoded string of POLO bytes

>> convert manifest("./manifests/ledger.yaml") into HEX
// prints manifest with HEX code
`
}

func parseConvertCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(TokenManifest) {
		return InvalidCommandError("missing manifest expression for convert")
	}

	manifest, fpath, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	switch parser.Cursor().Kind {
	// Convert Encoding Format [JSON, YAML, POLO]
	case TokenPrepositionAs:
		if !parser.ExpectPeek(TokenManifestEncoding) {
			return InvalidCommandError("missing encoding format for convert")
		}

		return ConvertManifestEncoding(manifest, EncodingFromString(parser.Cursor().Literal))

	// Convert Codeform [BIN, HEX, ASM]
	case TokenPrepositionInto:
		if !parser.ExpectPeek(TokenManifestCodeform) {
			return InvalidCommandError("missing codeform for convert")
		}

		extension := strings.TrimPrefix(filepath.Ext(fpath), ".")
		encoding := strings.ToUpper(extension)

		return ConvertManifestCodeform(manifest, EncodingFromString(encoding), parser.Cursor().Literal)

	default:
		return InvalidCommandErrorf("invalid preposition after manifest expr for convert")
	}
}

func ConvertManifestEncoding(manifest *engineio.Manifest, encoding engineio.Encoding) Command {
	return func(env *Environment) string {
		return PrintManifest(manifest, encoding)
	}
}

func ConvertManifestCodeform(original *engineio.Manifest, encoding engineio.Encoding, codeform string) Command {
	return func(env *Environment) string {
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
}

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

		return string(data)

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

		return b.String()

	default:
		return fmt.Sprintf("[unimplemented] unsupported manifest encoding: %v", encoding)
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

// CompileManifest reads and compiles a manifest file at the given path into a Logic with the given name.
func CompileManifest(
	name string, manifest *engineio.Manifest, fuel engineio.EngineFuel,
) (
	*core.LogicAccount, engineio.EngineFuel, error,
) {
	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	if !ok {
		return nil, 0, errors.Errorf("unsupported manifest engine: %v", manifest.Header().LogicEngine())
	}

	// Compile the manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(fuel, manifest)
	if err != nil {
		return nil, consumed, err
	}

	// Create a new account state
	logicState := core.NewAccountState(core.RandomAddress())
	// Create a new LogicObject from the LogicDescriptor
	logicObject := state.NewLogicObject(logicState.Address, descriptor)

	// If the logic ID has no persistent state, it can be marked
	// as ready, otherwise it requires a deploy to occur first
	id, _ := logicObject.ID.Identifier()
	ready := !id.PersistentState()

	return &core.LogicAccount{
		Name:     name,
		Ready:    ready,
		Logic:    logicObject,
		State:    logicState,
		Manifest: manifest,
	}, consumed, nil
}

func EncodingFromString(encoding string) engineio.Encoding {
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
