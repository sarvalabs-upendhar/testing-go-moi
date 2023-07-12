package internal

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

const (
	TokenExit symbolizer.TokenKind = -(iota + 10)
	TokenHelp

	TokenActor
	TokenAction
	TokenCallAction
	TokenMemoryAction

	TokenBig
	TokenManifest

	TokenConfig
	TokenConfigParam

	TokenEngine
	TokenFileEncoding
	TokenInstructFormat

	TokenPrepositionAs
	TokenPrepositionFrom
	TokenPrepositionInto
	TokenPrepositionWith

	TokenDesignate
	TokenDesignated

	TokenSlothash
	TokenCallEncode
	TokenCallDecode
	TokenErrDecode

	TokenLogic
	TokenParticipant
)

// keywords is a mapping of custom keywords to their
// symbolizer TokenKinds used to parse commands
var keywords = map[string]symbolizer.TokenKind{
	"exit": TokenExit,
	"help": TokenHelp,

	"designate":  TokenDesignate,
	"designated": TokenDesignated,

	"invoke": TokenCallAction,
	"deploy": TokenCallAction,

	"sender":   TokenActor,
	"receiver": TokenActor,

	"delete":   TokenAction,
	"inspect":  TokenAction,
	"register": TokenAction,
	"compile":  TokenAction,

	"set": TokenMemoryAction,
	"get": TokenMemoryAction,

	"config":   TokenConfig,
	"basefuel": TokenConfigParam,
	"hexbig":   TokenConfigParam,
	"hexbytes": TokenConfigParam,

	"POLO": TokenFileEncoding,
	"JSON": TokenFileEncoding,
	"YAML": TokenFileEncoding,

	"BIN": TokenInstructFormat,
	"HEX": TokenInstructFormat,

	"PISA": TokenEngine,

	"big":      TokenBig,
	"manifest": TokenManifest,

	"as":   TokenPrepositionAs,
	"from": TokenPrepositionFrom,
	"into": TokenPrepositionInto,
	"with": TokenPrepositionWith,

	"slothash":   TokenSlothash,
	"callencode": TokenCallEncode,
	"calldecode": TokenCallDecode,
	"errdecode":  TokenErrDecode,

	"logic":       TokenLogic,
	"participant": TokenParticipant,
}

func parseLogicCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(TokenAction) {
		if parser.IsPeek(symbolizer.TokenEoF) {
			return LogicListCommand()
		}

		return InvalidCommandError("invalid 'logic' command")
	}

	switch parser.Cursor().Literal {
	case "compile":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandError("invalid 'logic compile' command: missing name")
		}

		ident := parser.Cursor().Literal

		if !parser.ExpectPeek(TokenPrepositionFrom) {
			return InvalidCommandError("invalid 'logic compile' command: invalid syntax")
		}

		if !parser.ExpectPeek(TokenManifest) {
			return InvalidCommandError("invalid 'logic compile' command: missing manifest expression")
		}

		manifest, _, err := parseManifestExpression(parser)
		if err != nil {
			return InvalidCommandError(err.Error())
		}

		return LogicCompileFromManifestCommand(ident, manifest)

	case "inspect":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandError("invalid 'logic inspect' command: missing name")
		}

		return LogicInspectCommand(parser.Cursor().Literal)

	case "delete":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandError("invalid 'logic drop' command: missing name")
		}

		return LogicDeleteCommand(parser.Cursor().Literal)

	default:
		return InvalidCommandErrorf("invalid 'logic' command: '%v' is not supported", parser.Cursor().Literal)
	}
}

func parseParticipantCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(TokenAction) {
		if parser.IsPeek(symbolizer.TokenEoF) {
			return ParticipantListCommand()
		}

		return InvalidCommandError("invalid 'participant' command")
	}

	switch parser.Cursor().Literal {
	case "register":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandError("invalid 'participant register' command: missing username")
		}

		user := parser.Cursor().Literal
		parser.Advance()

		if parser.IsCursor(TokenPrepositionAs) {
			parser.Advance()

			address, err := parser.Cursor().Value()
			if err != nil {
				return InvalidCommandError("missing address")
			}

			value, ok := address.([]uint8)
			if !ok {
				return InvalidCommandError("invalid format not []uint8 ")
			}

			valueStr := string(value)

			return ParticipantRegisterCommand(user, valueStr)
		}

		return ParticipantRegisterCommand(user, "")

	case "inspect":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandError("invalid 'participant inspect' command: missing username")
		}

		return ParticipantInspectCommand(parser.Cursor().Literal)

	case "delete":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandError("invalid 'participant drop' command: missing username")
		}

		return ParticipantDeleteCommand(parser.Cursor().Literal)

	default:
		return InvalidCommandErrorf(
			"invalid 'participant' command: '%v' is not supported", parser.Cursor().Literal,
		)
	}
}

func parserManifestFilePath(parser *symbolizer.Parser) (*symbolizer.Parser, string, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, "", fmt.Errorf("invalid manifest expression: missing '('")
	}

	args, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, "", fmt.Errorf("%w invalid 'manifest' expression", err)
	}

	argParser := symbolizer.NewParser(args)

	pathPOLO, err := argParser.Cursor().Value()
	if err != nil {
		return nil, "", fmt.Errorf("invalid raw manifest expression %w", err)
	}

	value, ok := pathPOLO.(string)
	if ok {
		return argParser, value, nil
	} else {
		pathPOLOBytes, ok := pathPOLO.([]uint8)
		if !ok {
			return nil, "", fmt.Errorf("invalid 'pathPOLO' value: not []uint8")
		}
		pathPOLOStr := string(pathPOLOBytes)

		return argParser, pathPOLOStr, nil
	}
}

func parseManifestCommand(parser *symbolizer.Parser) Command {
	manifest, path, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	switch {
	case parser.IsCursor(TokenPrepositionAs):
		if !parser.ExpectPeek(TokenFileEncoding) {
			return InvalidCommandError("invalid 'manifest' command: missing encoding format for conversion")
		}

		return ManifestFileConvertCommand(manifest, parser.Cursor().Literal)

	case parser.IsCursor(TokenPrepositionInto):
		if !parser.ExpectPeek(TokenInstructFormat) {
			return InvalidCommandError("invalid 'manifest' command: missing instruction format for conversion")
		}

		extension := strings.TrimPrefix(filepath.Ext(path), ".")
		encoding := strings.ToUpper(extension)

		if encoding == "" {
			encoding = "POLO"
		}

		return ManifestInstructionConvertCommand(manifest, encoding, parser.Cursor().Literal)

	default:
		return InvalidCommandError("invalid 'manifest' command: missing valid preposition after manifest expression")
	}
}

func parseDesignateCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandErrorf("invalid 'designate' command: missing participant name")
	}

	name := parser.Cursor().Literal

	if !parser.ExpectPeek(TokenPrepositionAs) {
		return InvalidCommandError("invalid 'designate' command: invalid syntax")
	}

	if !parser.ExpectPeek(TokenActor) {
		return InvalidCommandError("invalid 'designate' command: missing actor")
	}

	actor := parser.Cursor().Literal

	return DesignateCommand(actor, name)
}

func parseDesignatedCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(TokenActor) {
		return InvalidCommandErrorf("invalid 'designated' command: missing actor")
	}

	switch parser.Cursor().Literal {
	case "sender":
		return DesignatedSenderCommand()
	case "receiver":
		return DesignatedReceiverCommand()
	default:
		return InvalidCommandErrorf("invalid 'designated' command: unsupported actor")
	}
}

func parseCallActionCommand(parser *symbolizer.Parser) Command {
	switch callkind := parser.Cursor().Literal; callkind {
	case "deploy", "invoke":
		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandErrorf("invalid '%v' command: missing logic identifier", callkind)
		}

		logic := parser.Cursor().Literal

		if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
			return InvalidCommandErrorf("invalid '%v' command: missing . after logic identifier", callkind)
		}

		if !parser.ExpectPeek(symbolizer.TokenIdent) {
			return InvalidCommandErrorf("invalid '%v' command: missing logic callsite", callkind)
		}

		callsite := parser.Cursor().Literal
		if parser.ExpectPeek(symbolizer.TokenKind('!')) || parser.ExpectPeek(symbolizer.TokenKind('$')) {
			callsite += parser.Cursor().Literal
		}

		parser.Advance()

		args, err := parser.Unwrap(symbolizer.EnclosureParens())
		if err != nil {
			return InvalidCommandErrorf("invalid '%v' command: malformed args: %v", callkind, err)
		}

		switch callkind {
		case "deploy":
			return LogicCallCommand(engineio.DeployerCallsite, logic, callsite, args)
		case "invoke":
			return LogicCallCommand(engineio.InvokableCallsite, logic, callsite, args)
		default:
			panic("unexpected mutation of callkind")
		}

	default:
		return InvalidCommandErrorf("invalid call command: '%v'", callkind)
	}
}

func parseMemoryActionCommand(parser *symbolizer.Parser) Command {
	action := parser.Cursor().Literal // set/get

	parser.Advance()
	ident := parser.Cursor().Literal // config or identifier

	switch action {
	case "get":
		if ident == "config" {
			return parseConfigCommand(parser, false)
		} else {
			return GetValueCommand(ident)
		}
	case "set":
		if ident == "config" {
			return parseConfigCommand(parser, true)
		}
	}

	parser.Advance()

	argument, err := parseValue(parser) // value for non config calls
	if err != nil {
		return InvalidCommandErrorf("invalid '%v' command: invalid argument value: %v", action, err)
	}

	return SetValueCommand(ident, argument) // all non config set calls
}

func parseConfigCommand(parser *symbolizer.Parser, set bool) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("invalid 'config' command: missing . after config identifier")
	}

	if !parser.ExpectPeek(TokenConfigParam) {
		return InvalidCommandErrorf("invalid 'config' command: missing valid config parameter")
	}

	option := parser.Cursor().Literal // keyword after [.]
	parser.Advance()

	if !set {
		return GetConfigCommand(option)
	}

	value, err := parseValue(parser) // value to set config
	if err != nil {                  // throw error if value is not present to set
		return InvalidCommandErrorf("invalid set command: invalid argument value %v", err)
	}

	return SetConfigCommand(option, value)
}

func parseSlothashCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenNumber) {
		return InvalidCommandError("invalid 'slothash' command: missing slot number")
	}

	token := parser.Cursor()
	value, _ := token.Value()

	slot, ok := value.(uint64)
	if !ok {
		return InvalidCommandError("invalid 'slothash' command: slot is not an uint64")
	}

	return SlothashCommand(slot)
}

func parseCallEncodeCommand(parser *symbolizer.Parser) Command {
	if parser.ExpectPeek(symbolizer.TokenIdent) {
		return CallgenMemoryCommand(parser.Cursor().Literal)
	}

	parser.Advance()

	value, err := parseValue(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid 'callencode' command: invalid argument value: %v", err)
	}

	return CallgenValueCommand(value)
}

func parseCallDecodeCommand(parser *symbolizer.Parser) Command {
	parser.Advance()
	data := parser.Cursor()

	if !parser.ExpectPeek(TokenPrepositionFrom) {
		return InvalidCommandError("invalid 'calldecode' command: missing from after data")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("invalid 'calldecode' command: missing logic identifier")
	}

	logic := parser.Cursor().Literal

	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandError("invalid 'calldecode' command: missing . after logic identifier")
	}

	parser.Advance()

	callsite := parser.Cursor().Literal
	if parser.ExpectPeek(symbolizer.TokenKind('!')) || parser.ExpectPeek(symbolizer.TokenKind('$')) {
		callsite += parser.Cursor().Literal
	}

	switch data.Kind {
	case symbolizer.TokenIdent:
		return CallDecodeMemoryCommand(data.Literal, logic, callsite)

	case symbolizer.TokenHexNumber:
		value, _ := data.Value()

		//nolint:forcetypeassert
		return CallDecodeValueCommand(value.([]byte), logic, callsite)

	default:
		return InvalidCommandError("invalid 'calldecode' command: invalid data")
	}
}

func parseErrDecodeCommand(parser *symbolizer.Parser) Command {
	parser.Advance()
	errdata := parser.Cursor()

	if !parser.ExpectPeek(TokenPrepositionFrom) {
		return InvalidCommandError("invalid 'errdecode' command: missing from after errdata")
	}

	if !parser.ExpectPeek(TokenEngine) {
		return InvalidCommandError("invalid 'errdecode' command: missing valid engine")
	}

	switch engine := parser.Cursor().Literal; engine {
	case "PISA":
		switch errdata.Kind {
		case symbolizer.TokenIdent:
			return ErrDecodePISAMemoryCommand(errdata.Literal)

		case symbolizer.TokenHexNumber:
			value, _ := errdata.Value()

			//nolint:forcetypeassert
			return ErrDecodePISAValueCommand(value.([]byte))

		default:
			return InvalidCommandError("invalid 'errdecode' command: invalid errdata")
		}

	default:
		return InvalidCommandErrorf("invalid 'errdecode' command: invalid engine '%v'", engine)
	}
}

func parseManifestExpression(parser *symbolizer.Parser) (*engineio.Manifest, string, error) {
	argParser, pathPOLO, err := parserManifestFilePath(parser)
	if err != nil {
		return nil, "", err
	}

	switch argParser.Cursor().Kind {
	case symbolizer.TokenString:
		manifest, err := engineio.ReadManifestFile(pathPOLO)
		if err != nil {
			return nil, "", fmt.Errorf("unable to read manifest: %w", err)
		}

		return manifest, pathPOLO, nil

	case symbolizer.TokenHexNumber:
		bytes, err := hex.DecodeString(fmt.Sprintf("%x", pathPOLO))
		if err != nil {
			return nil, "", errors.New("Error decoding manifest POLO")
		}

		manifest, err := ManifestPOLOConverter(bytes)
		if err != nil {
			return nil, "", fmt.Errorf("%w", err)
		}

		return manifest, "", nil

	default:
		return nil, "", errors.New("invalid missing path or polo string")
	}
}

func parseBigExpression(parser *symbolizer.Parser) (number *big.Int, err error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, errors.New("invalid big expression: missing '('")
	}

	expr, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, errors.Wrap(err, "invalid 'big' expression")
	}

	exprParser := symbolizer.NewParser(expr)

	switch exprParser.Cursor().Kind {
	// Hex Expression (0xABCD)
	case symbolizer.TokenHexNumber:
		hexnum := exprParser.Cursor().Literal

		num256, err := new(big.Int).SetString(hexnum, 0)
		if !err {
			return nil, errors.Errorf("invalid 'big' expression")
		}

		return num256, nil

	// Int Expression ([+/-]X)
	case symbolizer.TokenNumber:
		numeric := exprParser.Cursor().Literal

		switch {
		case strings.HasPrefix(numeric, "-"):
			defer func() {
				if err == nil {
					number = new(big.Int).Neg(number)
				}
			}()

			numeric = strings.TrimPrefix(numeric, "-")

			fallthrough

		case strings.HasPrefix(numeric, "+"):
			numeric = strings.TrimPrefix(numeric, "+")

			fallthrough

		default:
			num256, err := new(big.Int).SetString(numeric, 0)
			if !err {
				return nil, errors.Errorf("invalid 'big' expression")
			}

			return num256, nil
		}

	default:
		return nil, errors.New("invalid 'big' expression: unsupported value form")
	}
}

func parseValue(parser *symbolizer.Parser) (any, error) {
	switch parser.Cursor().Kind {
	// Reference Value
	case symbolizer.TokenIdent:
		return engineio.ReferenceVal(parser.Cursor().Literal), nil

	// List Value
	case symbolizer.TokenKind('['):
		unwrapped, err := parser.Unwrap(symbolizer.EnclosureSquare())
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert to value: list malformed")
		}

		values, err := parseUnkeyedValues(unwrapped, symbolizer.TokenKind(','))
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert to values: list malformed")
		}

		return values, nil

	// Map Value
	case symbolizer.TokenKind('{'):
		unwrapped, err := parser.Unwrap(symbolizer.EnclosureCurly())
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert to value: object malformed")
		}

		keyedValues, detected, err := parseKeyedValues(unwrapped, symbolizer.TokenKind(','))
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert to values: list malformed")
		}

		if detected == symbolizer.TokenIdent {
			values := make(map[string]any, len(keyedValues))

			for key, val := range keyedValues {
				values[key.(string)] = val //nolint:forcetypeassert
			}

			return values, nil
		} else {
			values := make(map[any]any, len(keyedValues))

			for key, val := range keyedValues {
				values[key] = val
			}

			return values, nil
		}

	// Big Value
	case TokenBig:
		return parseBigExpression(parser)

	default:
		if parser.Cursor().Kind.CanValue() {
			return parser.Cursor().Value()
		}

		return nil, errors.New("unsupported argument")
	}
}

func parseUnkeyedValues(input string, delim symbolizer.TokenKind) ([]any, error) {
	arguments := make([]any, 0)

	parser := symbolizer.NewParser(input,
		symbolizer.IgnoreWhitespaces(),
		symbolizer.Keywords(map[string]symbolizer.TokenKind{
			"big":  TokenBig,
			"true": symbolizer.TokenBoolean, "false": symbolizer.TokenBoolean,
			"TRUE": symbolizer.TokenBoolean, "FALSE": symbolizer.TokenBoolean,
			"True": symbolizer.TokenBoolean, "False": symbolizer.TokenBoolean,
		}),
	)

	for !parser.IsCursor(symbolizer.TokenEoF) {
		arg, err := parseValue(parser)
		if err != nil {
			return nil, err
		}

		arguments = append(arguments, arg)

		switch {
		case parser.IsCursor(delim):
			parser.Advance()
		case !(parser.ExpectPeek(delim) || parser.ExpectPeek(symbolizer.TokenEoF)):
			return nil, errors.Errorf("missing delimiter after argument")
		default:
			parser.Advance()
		}
	}

	return arguments, nil
}

func parseKeyedValues(input string, delim symbolizer.TokenKind) (map[any]any, symbolizer.TokenKind, error) {
	arguments := make(map[any]any)

	parser := symbolizer.NewParser(input,
		symbolizer.IgnoreWhitespaces(),
		symbolizer.Keywords(map[string]symbolizer.TokenKind{
			"big":  TokenBig,
			"true": symbolizer.TokenBoolean, "false": symbolizer.TokenBoolean,
			"TRUE": symbolizer.TokenBoolean, "FALSE": symbolizer.TokenBoolean,
			"True": symbolizer.TokenBoolean, "False": symbolizer.TokenBoolean,
		}),
	)

	detected := parser.Cursor().Kind

	for !parser.IsCursor(symbolizer.TokenEoF) {
		if !parser.IsCursor(detected) {
			return nil, detected, errors.New("inconsistent key kind")
		}

		var key any

		if detected == symbolizer.TokenIdent {
			key = parser.Cursor().Literal
		} else {
			var err error

			key, err = parseValue(parser)
			if err != nil {
				return nil, detected, errors.Wrapf(err, "malformed value for key")
			}

			if raw, ok := key.([]byte); ok {
				if len(raw) != 32 {
					return nil, detected, errors.New("malformed value for key: cannot use bytes")
				}

				key = common.BytesToAddress(raw)
			}
		}

		if !parser.ExpectPeek(symbolizer.TokenKind(':')) {
			return nil, detected, errors.New("missing colon after key")
		}

		parser.Advance()

		arg, err := parseValue(parser)
		if err != nil {
			return nil, detected, errors.Wrapf(err, "malformed value for key '%v'", key)
		}

		arguments[key] = arg

		switch {
		case parser.IsCursor(delim):
			parser.Advance()
		case !(parser.ExpectPeek(delim) || parser.ExpectPeek(symbolizer.TokenEoF)):
			return nil, detected, errors.Errorf("missing delimiter after argument for key '%v'", key)
		default:
			parser.Advance()
		}
	}

	return arguments, detected, nil
}

func parseArguments(args string) (map[string]any, error) {
	arguments := make(map[string]any)

	if args == "" {
		return arguments, nil
	}

	keyedArgs, detection, err := parseKeyedValues(args, symbolizer.TokenKind(','))
	if err != nil {
		return nil, errors.Wrap(err, "malformed arguments")
	}

	if detection != symbolizer.TokenIdent {
		return nil, errors.Errorf("malformed arguments: missing identifier keys")
	}

	for key, arg := range keyedArgs {
		arguments[key.(string)] = arg //nolint:forcetypeassert
	}

	return arguments, nil
}
