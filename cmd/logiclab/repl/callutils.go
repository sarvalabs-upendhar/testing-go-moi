package repl

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"

	"github.com/manishmeganathan/symbolizer"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

func HelpCallencode() string {
	return `
The @>callencode<@ command can be used to encode calldata for logic calls.
The generated calldata is a POLO doc-encoded hex string of the call object.
The input object can be provided as a literal or accessed from the session [@>memory<@].

usage:
@>callencode [identifier]<@
@>callencode [object]<@

examples:
>> set A 500
>> set B "manish"
>> set C {name: A, value: B}
>> callencode C
0x0d5f064576c5016e616d650301f476616c7565066d616e697368

>> callencode {name: A, value: B}
0x0d5f064576c5016e616d650301f476616c7565066d616e697368
`
}

// CallencodeMemoryCommand generates a command runner to generate
// a doc-encoded calldata string from an object in the lab memory
func CallencodeMemoryCommand(ident string) Command {
	return func(repl *Repl) string {
		value, ok := repl.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		return CallencodeValueCommand(value)(repl)
	}
}

// CallencodeValueCommand generates a command runner to generate
// a doc-encoded calldata string from a given value object
func CallencodeValueCommand(value any) Command {
	return func(repl *Repl) string {
		object, ok := value.(map[string]any)
		if !ok {
			return "value is not an object"
		}

		doc := make(polo.Document)

		for key, val := range object {
			// todo: fix
			data, err := pisa.EncodeValues(val)
			if err != nil {
				return fmt.Sprintf("could not encode value into calldata: %v", err)
			}

			doc.SetRaw(key, data)
		}

		return "0x" + hex.EncodeToString(doc.Bytes())
	}
}

func parseCallencodeCommand(parser *symbolizer.Parser) Command {
	if parser.ExpectPeek(symbolizer.TokenIdent) {
		return CallencodeMemoryCommand(parser.Cursor().Literal)
	}

	parser.Advance()

	value, err := parseValue(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid 'callencode' command: invalid argument value: %v", err)
	}

	return CallencodeValueCommand(value)
}

func HelpCalldecode() string {
	return `
The @>calldecode<@ command can be used to decode the raw output data from logic calls.
The output data can be provided as raw bytes or accessed from the session [@>memory<@].

usage:
@>calldecode [identifier] from [name].[callsite]<@
@>calldecode [object data] from [name].[callsite]<@

examples:
>> set A 0x0d2f06256f6b02
>> calldecode A from Ledger.Mint!
// Outputs ||| ok: true

>> calldecode 0x0d2f06256f6b02 from Ledger.Mint!
// Outputs ||| ok: true
`
}

func CalldecodeMemoryCommand(ident, name, site string) Command {
	return func(repl *Repl) string {
		value, ok := repl.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		data, ok := value.([]byte)
		if !ok {
			return fmt.Sprintf("'%v' is not an hex value", ident)
		}

		return CalldecodeValueCommand(data, name, site)(repl)
	}
}

func CalldecodeValueCommand(data []byte, name, site string) Command {
	return func(repl *Repl) string {
		// Find the logic from the inventory
		logic, err := repl.env.FetchLogic(name)
		if err != nil {
			return fmt.Sprintf("logic '%v' does not exist: %v", name, err)
		}

		// Get the callsite from the logic, error if not found
		callsite, ok := logic.Object.GetCallsite(site)
		if !ok {
			return fmt.Sprintf("logic '%v' does not have callsite '%v'", name, site)
		}

		// Obtain the runtime for the logic engine in the header
		engine, ok := engineio.FetchEngine(logic.Object.Engine())
		if !ok {
			return "failed to get runtime for logic"
		}

		// Generate the call encoder for the callsite
		encoder, err := engine.GetCallEncoderFromLogicDriver(logic.Object, callsite)
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

func parseCalldecodeCommand(parser *symbolizer.Parser) Command {
	parser.Advance()
	calldata := parser.Cursor()

	if !parser.ExpectPeek(TokenPrepositionFrom) {
		return InvalidCommandError("missing from preposition")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing logic name for calldecode")
	}

	logic := parser.Cursor().Literal

	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandError("missing . after logic name")
	}

	parser.Advance()

	callsite := parser.Cursor().Literal
	if parser.ExpectPeek(symbolizer.TokenKind('!')) || parser.ExpectPeek(symbolizer.TokenKind('$')) {
		callsite += parser.Cursor().Literal
	}

	switch calldata.Kind {
	case symbolizer.TokenIdent:
		return CalldecodeMemoryCommand(calldata.Literal, logic, callsite)

	case symbolizer.TokenHexNumber:
		value, _ := calldata.Value()

		return CalldecodeValueCommand(value.([]byte), logic, callsite) //nolint:forcetypeassert

	default:
		return InvalidCommandError("invalid calldata")
	}
}

func HelpErrdecode() string {
	return `
The @>errdecode<@ command can be used to decode the error data returned by logic calls.
The given error data is decoded for the error object of the respective [@>engines<@] and printed.
The error data can be provided as raw bytes or accessed from the session memory.

usage:
@>errdecode [error data] from [engine]<@
@>errdecode [identifier] from [engine]<@

examples:
>> errdecode 0x0e4f0666ce01737472696e6768656c6c6f213f068602
726f6f742e7365747570205b3078305d726f6f742e446f205b3078305d202e2e2e205b3078323a205448524f57203078305d from PISA
// prints error object

>> set err 0x0e4f0666ce01737472696e6768656c6c6f213f068602726f6
f742e7365747570205b3078305d726f6f742e446f205b3078305d202e2e2e205b3078323a205448524f57203078305d
>> errdecode err from PISA
// prints error object
`
}

// ErrdecodeValueCommand generates a command runner to
// decode the error object from the given error bytes data.
func ErrdecodeValueCommand(errdata []byte, engine engineio.EngineKind) Command {
	return func(repl *Repl) string {
		runtime, _ := engineio.FetchEngine(engine)

		errorObject, err := runtime.DecodeErrorResult(errdata)
		if err != nil {
			return fmt.Sprintf("failed to decode error data into ErrorResult: %v", err)
		}

		return errorObject.String()
	}
}

// ErrdecodeMemoryCommand generates a command runner
// to decode the error object from the error bytes
// data at the given identifier in the lab memory
func ErrdecodeMemoryCommand(ident string, engine engineio.EngineKind) Command {
	return func(repl *Repl) string {
		value, ok := repl.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		errdata, ok := value.([]byte)
		if !ok {
			return fmt.Sprintf("'%v' is not an hex value", ident)
		}

		return ErrdecodeValueCommand(errdata, engine)(repl)
	}
}

func parseErrdecodeCommand(parser *symbolizer.Parser) Command {
	parser.Advance()
	errdata := parser.Cursor()

	if !parser.ExpectPeek(TokenPrepositionFrom) {
		return InvalidCommandError("missing from preposition")
	}

	if !parser.ExpectPeek(TokenEngineKind) {
		return InvalidCommandError("missing valid engine")
	}

	engine := engineio.EngineKindFromString(parser.Cursor().Literal)

	switch errdata.Kind {
	case symbolizer.TokenIdent:
		return ErrdecodeMemoryCommand(errdata.Literal, engine)

	case symbolizer.TokenHexNumber:
		value, _ := errdata.Value()

		return ErrdecodeValueCommand(value.([]byte), engine) //nolint:forcetypeassert

	default:
		return InvalidCommandError("invalid error data")
	}
}

func HelpStorageKey() string {
	return `
The @>storagekey<@ command can be used to generate storage slot keys.
The storage slot represents then key for positional information in a [logics] state.
Currently it only supports a simple slot hashing by accepting a uint8 slot and
returning its hash, but this will be extended when PISA's storage layer is complete.

usage:
@>storagekey [slot] mapkey[value]<@
@>storagekey [slot] mapkey[value] arridx[value]<@
@>storagekey [slot] mapkey[value] arridx[value] clsfld[value]<@

examples:
>> storagekey 0 clsfld(1)
89eb0d6a8a691dae2cd15ed0369931ce0a949ecafa5c3f93f8121833646e15c4
>> storagekey 0 mapkey("foo") arridx(1)
300464d4748307d603e3807009362bfec9fd1ed997c4f3ec1789d073b0c1c88a
>> storagekey 0 mapkey(5) arridx(1) clsfld(5)
11d68688e315ef4b7cc63fe4652023cb7a830724c5df94abf33dd13c4f84298a
`
}

// parseStorageKeyCommand generates a command runner to generate
// the storage key for a given storage path.
func parseStorageKeyCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenNumber) {
		return InvalidCommandError("missing slot number for storage key")
	}

	token := parser.Cursor()
	value, _ := token.Value()

	slot, ok := value.(uint64)
	if !ok {
		return InvalidCommandError("slot is not an uint64")
	}

	if slot > math.MaxUint8 {
		return InvalidCommandError("slot number is too large")
	}

	if parser.IsPeek(symbolizer.TokenEoF) {
		return func(repl *Repl) string {
			return hex.EncodeToString(pisa.GenerateStorageKey(uint8(slot)))
		}
	}

	if !parser.ExpectPeek(TokenStorageKeyAccessor) {
		return InvalidCommandError("invalid storage accessor")
	}

	accessors := make([]pisa.Accessor, 0)

	for !parser.IsCursor(symbolizer.TokenEoF) {
		if !parser.IsCursor(TokenStorageKeyAccessor) {
			return InvalidCommandError("invalid storage accessor")
		}

		switch parser.Cursor().Literal {
		case "arridx":
			accessor, err := parseAccessorArrIdx(parser)
			if err != nil {
				return InvalidCommandError(err.Error())
			}

			accessors = append(accessors, accessor)
		case "mapkey":
			accessor, err := parseAccessorMapKey(parser)
			if err != nil {
				return InvalidCommandError(err.Error())
			}

			accessors = append(accessors, accessor)
		case "clsfld":
			accessor, err := parseAccessorClsFld(parser)
			if err != nil {
				return InvalidCommandError(err.Error())
			}

			accessors = append(accessors, accessor)
		default:
			panic("unimplemented accessor")
		}
	}

	return func(repl *Repl) string {
		storageKey := pisa.GenerateStorageKey(uint8(slot), accessors...)

		return hex.EncodeToString(storageKey)
	}
}

func parseAccessorMapKey(parser *symbolizer.Parser) (pisa.Accessor, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, errors.New("invalid mapkey expression: missing '('")
	}

	// Parse the expression contents inside the ()
	inner, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, errors.Wrap(err, "invalid mapkey expression")
	}

	parserInner := symbolizer.NewParser(inner)
	// Check the inner value is valid
	if !parserInner.Cursor().Kind.CanValue() {
		return nil, errors.New("invalid mapkey expression")
	}

	// Parse the value within the parenthesis
	value, err := parserInner.Cursor().Value()
	if err != nil {
		return nil, errors.Wrap(err, "invalid mapkey expression")
	}

	serial, err := polo.Polorize(value)
	if err != nil {
		return nil, errors.Wrap(err, "invalid mapkey expression: failed serialization")
	}

	hashed := blake2b.Sum256(serial)

	return pisa.MapKey(hashed), nil
}

func parseAccessorArrIdx(parser *symbolizer.Parser) (pisa.Accessor, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, errors.New("invalid arridx expression: missing '('")
	}

	// Parse the expression contents inside the ()
	inner, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, errors.Wrap(err, "invalid arridx expression")
	}

	parserInner := symbolizer.NewParser(inner)
	// Check the inner value is valid
	if !parserInner.Cursor().Kind.CanValue() {
		return nil, errors.New("invalid arridx expression")
	}

	// Parse the value within the parenthesis
	value, err := parserInner.Cursor().Value()
	if err != nil {
		return nil, errors.Wrap(err, "invalid arridx expression")
	}

	idx, ok := value.(uint64)
	if !ok {
		return nil, errors.New("invalid arridx expression: idx is not an uint64")
	}

	return pisa.ArrIdx(idx), nil
}

func parseAccessorClsFld(parser *symbolizer.Parser) (pisa.Accessor, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, errors.New("invalid clsfld expression: missing '('")
	}

	// Parse the expression contents inside the ()
	inner, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, errors.Wrap(err, "invalid clsfld expression")
	}

	parserInner := symbolizer.NewParser(inner)
	// Check the inner value is valid
	if !parserInner.Cursor().Kind.CanValue() {
		return nil, errors.New("invalid clsfld expression")
	}

	// Parse the value within the parenthesis
	value, err := parserInner.Cursor().Value()
	if err != nil {
		return nil, errors.Wrap(err, "invalid clsfld expression")
	}

	fld, ok := value.(uint64)
	if !ok {
		return nil, errors.New("invalid clsfld expression: fld is not an uint64")
	}

	if fld > math.MaxUint8 {
		return nil, errors.New("invalid clsfld expression: fld is too large")
	}

	return pisa.ClsFld(fld), nil
}
