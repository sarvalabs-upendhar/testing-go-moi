package repl

import (
	"fmt"
	"strings"

	"github.com/manishmeganathan/symbolizer"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

func HelpEngines() string {
	return `The @>engines<@ command can be used to list all supported engines and their versions`
}

func EnginesCommand() Command {
	return func(repl *Repl) string {
		engines := []engineio.EngineKind{
			engineio.PISA,
		}

		var str strings.Builder

		for idx, engine := range engines {
			runtime, _ := engineio.FetchEngine(engine)
			str.WriteString(Colorize(fmt.Sprintf("%v] @>%s<@ (%v)", idx+1, engine, runtime.Version())))
		}

		return str.String()
	}
}

func HelpConfig() string {
	return `
The @>LogicLab<@ configuration can be accessed and modified with the [@>get<@] and [@>set<@] commands by using
the 'config' prefix for the identifier. Supported values for the config parameter identifiers are listed below.
The configuration stored within the environment's '[@>inventory<@] file. It should only be modified while the
lab is not active, otherwise it will revert to the in-memory value when the lab environment is closed.

Config Parameters
-----------------
basefuel: [uint] The determines how much FUEL is used for logic calls (default: 10000)
hexbigint: [bool] This determines whether arbitrary sized numbers are represented as a hex string.
hexbytes: [bool] This determines whether bytes data are represented as a hex string.

usage:
@>get config.[param]<@
@>set config.[param] [argument]<@

example:
>>> get config.basefuel
config.basefuel: 10000

>>> set config.basefuel 50000
config.basefuel: 50000
`
}

func parseGetConfig(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after config prefix")
	}

	if !parser.ExpectPeek(TokenConfigParam) {
		return InvalidCommandErrorf("invalid config parameter: %v", parser.Peek().Literal)
	}

	param := parser.Cursor().Literal

	return func(repl *Repl) string {
		var value string

		switch param {
		case "basefuel":
			value = repl.FormatValue(repl.env.CallFuel)
		case "hexbigint":
			value = repl.FormatValue(repl.env.Config.HexBigInt)
		case "hexbytes":
			value = repl.FormatValue(repl.env.Config.HexBytes)
		default:
			return fmt.Sprintf("[unimplemented] cannot get config.%v", param)
		}

		return fmt.Sprintf("config.%v: %v", param, value)
	}
}

func parseSetConfig(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after config prefix")
	}

	if !parser.ExpectPeek(TokenConfigParam) {
		return InvalidCommandErrorf("invalid config parameter: %v", parser.Peek().Literal)
	}

	param := parser.Cursor().Literal
	parser.Advance()

	// Parse the argument into a value
	argument, err := parseValue(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid argument value: %v", err)
	}

	return func(repl *Repl) string {
		switch param {
		case "basefuel":
			value, ok := argument.(uint64)
			if !ok {
				return "value for config.basefuel must be a uint"
			}

			repl.env.CallFuel = value

		case "hexbigint":
			value, ok := argument.(bool)
			if !ok {
				return "value for config.hexbigint must be a bool"
			}

			repl.env.Config.HexBigInt = value

		case "hexbytes":
			value, ok := argument.(bool)
			if !ok {
				return "value for config.hexbytes must be a bool"
			}

			repl.env.Config.HexBytes = value

		default:
			return fmt.Sprintf("[unimplemented] cannot set config.%v", param)
		}

		return fmt.Sprintf("config.%v: %v", param, repl.FormatValue(argument))
	}
}
