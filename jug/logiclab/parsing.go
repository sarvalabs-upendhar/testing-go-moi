package logiclab

import (
	"encoding/hex"
	"strconv"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/engineio"
)

const (
	TokenBooleanTrue symbolizer.TokenKind = -(iota + 8)
	TokenBooleanFalse
)

const (
	TokenExit symbolizer.TokenKind = -(iota + 10)
	TokenHelp

	TokenDesignate
	TokenDesignated

	TokenActor
	TokenAction
	TokenCallAction
	TokenMemoryAction
	TokenPreposition

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

	"as":   TokenPreposition,
	"from": TokenPreposition,
	"into": TokenPreposition,
	"with": TokenPreposition,

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
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
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
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
			return InvalidCommandError("invalid 'logic inspect' command: missing name")
		}

		return LogicInspectCommand(parser.Cursor().Literal)

	case "delete":
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
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
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
			return InvalidCommandError("invalid 'participant register' command: missing username")
		}

		return ParticipantRegisterCommand(parser.Cursor().Literal)

	case "inspect":
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
			return InvalidCommandError("invalid 'participant inspect' command: missing username")
		}

		return ParticipantInspectCommand(parser.Cursor().Literal)

	case "delete":
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
			return InvalidCommandError("invalid 'participant drop' command: missing username")
		}

		return ParticipantDeleteCommand(parser.Cursor().Literal)

	default:
		return InvalidCommandErrorf(
			"invalid 'participant' command: '%v' is not supported", parser.Cursor().Literal,
		)
	}
}

func parseDesignateCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
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
		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
			return InvalidCommandErrorf("invalid '%v' command: missing logic identifier", callkind)
		}

		logic := parser.Cursor().Literal

		if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
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

	if !parser.ExpectPeek(symbolizer.TokenIdentifier) {
		return InvalidCommandErrorf("invalid '%v' command: missing identifier", action)
	}

	ident := parser.Cursor().Literal
	if action == "get" {
		return GetValueCommand(ident)
	}

	parser.Advance()

	argument, err := parseArg(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid '%v' command: invalid argument", action)
	}

	return SetValueCommand(ident, argument)
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

	return argParser.Cursor().Literal, nil
}

func parseArg(parser *symbolizer.Parser) (any, error) {
	switch cursor := parser.Cursor(); cursor.Kind {
	case symbolizer.TokenIdentifier:
		return MemoryVar(cursor.Literal), nil

	case symbolizer.TokenString:
		return cursor.Literal, nil

	case symbolizer.TokenNumber:
		number, err := strconv.ParseUint(cursor.Literal, 10, 64)
		if err != nil {
			return nil, err
		}

		return number, nil

	case symbolizer.TokenHexNumber:
		data, err := hex.DecodeString(cursor.Literal)
		if err != nil {
			return nil, err
		}

		return data, nil

	case TokenBooleanFalse:
		return false, nil

	case TokenBooleanTrue:
		return true, nil

	case symbolizer.TokenKind('-'):
		if !parser.IsPeek(symbolizer.TokenNumber) {
			return nil, errors.New("negative sign missing number")
		}

		parser.Advance()
		literal := "-" + parser.Cursor().Literal

		number, err := strconv.ParseInt(literal, 10, 64)
		if err != nil {
			return nil, err
		}

		return number, nil

	default:
		return nil, errors.New("unsupported argument")
	}
}

func parseArguments(args string) (map[string]any, error) {
	arguments := make(map[string]any)
	if args == "" {
		return arguments, nil
	}

	parser := symbolizer.NewParser(args,
		symbolizer.IgnoreWhitespaces(),
		symbolizer.Keywords(map[string]symbolizer.TokenKind{
			"true":  TokenBooleanTrue,
			"True":  TokenBooleanTrue,
			"TRUE":  TokenBooleanTrue,
			"false": TokenBooleanFalse,
			"False": TokenBooleanFalse,
			"FALSE": TokenBooleanFalse,
		}),
	)

	for !parser.IsCursor(symbolizer.TokenEoF) {
		if !parser.IsCursor(symbolizer.TokenIdentifier) {
			return nil, errors.New("missing identifier")
		}

		label := parser.Cursor().Literal

		if !parser.ExpectPeek(symbolizer.TokenKind(':')) {
			return nil, errors.New("missing colon after identifier")
		}

		parser.Advance()

		arg, err := parseArg(parser)
		if err != nil {
			return nil, err
		}

		arguments[label] = arg

		if !parser.ExpectPeek(symbolizer.TokenKind(',')) {
			if parser.ExpectPeek(symbolizer.TokenEoF) {
				break
			}

			return nil, errors.New("missing comma after argument")
		}

		parser.Advance()
	}

	return arguments, nil
}
