package repl

import (
	"math/big"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-moi-identifiers"
)

func HelpArgument() string {
	return `
@>Argument Value Rules<@ are applied when parsing values for logic calls as well as for the [@>set<@] command.
Once a value has been stored to the session memory it can be used in various commands with its identifier.

supported formats:
- @>Integer<@ (Ex: 100, -934343, 329429352)
- @>String<@ (Ex: "Hello", "Fahrenheit 451")
- @>Boolean<@ (Ex: true, True, TRUE, false, False, FALSE)
- @>Bytes/Address<@ (Ex: 0xf6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34)
- @>Lists<@ (Ex: [256, 2345], ["foo", "bar"])
- @>Mappings<@ (Ex: {"a": 123, "b": 345}, {456: "foo", 123: "bar"}) // value keys
- @>Objects<@ (Ex: {a: 123, b: 345}, {name: "Darius", age: 45})     // ident keys
`
}

type Identifier = engineio.ReferenceVal

func parseValue(parser *symbolizer.Parser) (any, error) {
	switch parser.Cursor().Kind {
	// Identifier Value
	case symbolizer.TokenIdent:
		return Identifier(parser.Cursor().Literal), nil

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

				key = identifiers.NewAddressFromBytes(raw)
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

func Colorize(str string) string {
	str = strings.ReplaceAll(str, "@>", "\u001b[32m")
	str = strings.ReplaceAll(str, "<@", "\u001b[0m")

	return str
}
