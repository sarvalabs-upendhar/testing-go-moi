package logiclab

import (
	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/engineio"
)

const (
	TokenExit symbolizer.TokenKind = -(iota + 10)
	TokenHelp

	TokenActor
	TokenAction
	TokenCallAction
	TokenMemoryAction

	TokenEngine
	TokenEncoding
	TokenPreposition

	TokenDesignate
	TokenDesignated
	TokenCallgen
	TokenSlothash
	TokenErrDecode

	TokenLogic
	TokenManifest
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

	"POLO": TokenEncoding,
	"JSON": TokenEncoding,
	"YAML": TokenEncoding,

	"PISA": TokenEngine,

	"as":   TokenPreposition,
	"from": TokenPreposition,
	"into": TokenPreposition,
	"with": TokenPreposition,

	"callgen":   TokenCallgen,
	"slothash":  TokenSlothash,
	"errdecode": TokenErrDecode,

	"logic":       TokenLogic,
	"manifest":    TokenManifest,
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

		if !parser.ExpectPeek(TokenPreposition) {
			return InvalidCommandError("invalid 'logic compile' command: invalid syntax")
		}

		if parser.Cursor().Literal != "from" {
			return InvalidCommandError("invalid 'logic compile' command: invalid syntax")
		}

		if !parser.ExpectPeek(TokenManifest) {
			return InvalidCommandError("invalid 'logic compile' command: missing manifest expression")
		}

		path, err := parseManifestExpression(parser)
		if err != nil {
			return InvalidCommandError(err.Error())
		}

		return LogicCompileFromManifestCommand(ident, path)

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

		return ParticipantRegisterCommand(parser.Cursor().Literal)

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

func parseManifestCommand(parser *symbolizer.Parser) Command {
	path, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	if parser.Cursor().Literal != "as" {
		return InvalidCommandError("invalid 'manifest' command: missing encoding")
	}

	if !parser.ExpectPeek(TokenEncoding) {
		return InvalidCommandError("invalid 'manifest' command: invalid syntax")
	}

	encoding := parser.Cursor().Literal

	return ManifestPrintCommand(path, encoding)
}

func parseDesignateCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandErrorf("invalid 'designate' command: missing participant name")
	}

	name := parser.Cursor().Literal

	if !parser.ExpectPeek(TokenPreposition) {
		return InvalidCommandError("invalid 'designate' command: invalid syntax")
	}

	if parser.Cursor().Literal != "as" {
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
			return CallCommand(engineio.DeployerCallsite, logic, callsite, args)
		case "invoke":
			return CallCommand(engineio.InvokableCallsite, logic, callsite, args)
		default:
			panic("unexpected mutation of callkind")
		}

	default:
		return InvalidCommandErrorf("invalid call command: '%v'", callkind)
	}
}

func parseMemoryActionCommand(parser *symbolizer.Parser) Command {
	action := parser.Cursor().Literal

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandErrorf("invalid '%v' command: missing identifier", action)
	}

	ident := parser.Cursor().Literal
	if action == "get" {
		return GetValueCommand(ident)
	}

	parser.Advance()

	argument, err := parseValue(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid '%v' command: invalid argument value: %v", action, err)
	}

	return SetValueCommand(ident, argument)
}

func parseCallgenCommand(parser *symbolizer.Parser) Command {
	if parser.ExpectPeek(symbolizer.TokenIdent) {
		return CallgenMemoryCommand(parser.Cursor().Literal)
	}

	parser.Advance()

	value, err := parseValue(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid 'callgen' command: invalid argument value: %v", err)
	}

	return CallgenValueCommand(value)
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

func parseErrDecodeCommand(parser *symbolizer.Parser) Command {
	parser.Advance()
	errdata := parser.Cursor()

	if !parser.ExpectPeek(TokenPreposition) {
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

func parseManifestExpression(parser *symbolizer.Parser) (string, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return "", errors.New("invalid manifest expression: missing '('")
	}

	args, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return "", errors.Wrap(err, "invalid 'manifest' expression")
	}

	argParser := symbolizer.NewParser(args)
	if argParser.Cursor().Kind != symbolizer.TokenString {
		return "", errors.New("invalid 'manifest' expression: missing file path")
	}

	path, _ := argParser.Cursor().Value()

	return path.(string), nil //nolint:forcetypeassert
}

func parseValue(parser *symbolizer.Parser) (any, error) {
	switch parser.Cursor().Kind {
	case symbolizer.TokenIdent:
		return engineio.ReferenceVal(parser.Cursor().Literal), nil

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
