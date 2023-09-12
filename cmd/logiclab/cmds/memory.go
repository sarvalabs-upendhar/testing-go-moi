package cmds

import (
	"fmt"

	"github.com/manishmeganathan/symbolizer"
)

func HelpMemory() string {
	return `
@>Memory Variables<@ are session memory values that can be used for holding values that can be used during 
logic calls with the [@>invoke<@] and [@>deploy<@] commands. Memory variables are flushed after each session.
They can be manipulated with the [@>get<@] and [@>set<@] commands and follow the [@>argument<@] value rules.
`
}

// GetMemoryCommand generates a Command runner to get the
// value of an identifier from the environment memory
func GetMemoryCommand(ident string) Command {
	return func(env *Environment) string {
		value, ok := env.memory[ident]
		if !ok {
			return fmt.Sprintf("no value for '%v'", ident)
		}

		return fmt.Sprintf("%v: %v", ident, env.format(value))
	}
}

// SetMemoryCommand generates a Command runner to set the value
// of an identifier to a given value in the environment memory
func SetMemoryCommand(ident string, value any) Command {
	return func(env *Environment) string {
		env.memory[ident] = value

		return fmt.Sprintf("%v: %v", ident, env.format(value))
	}
}

// WipeMemoryCommand generates a Command runner to remove
// the value of an identifier in the environment memory
func WipeMemoryCommand(ident string) Command {
	return func(env *Environment) string {
		delete(env.memory, ident)

		return fmt.Sprintf("%v wiped", ident)
	}
}

func HelpGet() string {
	return `
The @>get<@ command can be used to get the value for [@>memory<@] variables for a given identifier.
It can also be used to get the value of [@>config<@] values or [@>designated<@] participants.
Entities like [@>users<@] and [@>logics<@] can also be inspected using this command.

usage:
@>get [identifier]<@
@>get [entity].[identifier]<@
@>get [prefix].[identifier]<@

example:
>>> get addr1 
addr1: 0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c
`
}

func parseGetCommand(parser *symbolizer.Parser) Command {
	// Read the identifier
	parser.Advance()
	prefix := parser.Cursor()

	// Simple Identifier (not a prefix)
	if !parser.IsPeek(symbolizer.TokenKind('.')) {
		return GetMemoryCommand(prefix.Literal)
	}

	switch prefix.Kind {
	case TokenConfig:
		return parseGetConfig(parser)
	case TokenDesignated:
		return parseGetDesignated(parser)
	case TokenUsers:
		return parseGetUser(parser)
	case TokenLogics:
		return parseGetLogic(parser)
	}

	return InvalidCommandErrorf("unsupported prefix '%v'", prefix.Literal)
}

func HelpSet() string {
	return `
The @>set<@ command can be used to set the value for [@>memory<@] values for a given identifier.
It can also be used to set the value of config values [@>config<@] or designated participants [@>designated<@]

usage:
@>set [identifier] [value]<@
@>set [prefix].[identifier] [value]<@

examples:
>>> set addr1 0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c
addr1: 0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c
`
}

func parseSetCommand(parser *symbolizer.Parser) Command {
	// Read the identifier
	parser.Advance()

	// Simple Identifier (not a prefix)
	if !parser.IsPeek(symbolizer.TokenKind('.')) {
		// Read the identifier
		ident := parser.Cursor().Literal

		// Read the argument
		parser.Advance()
		// Parse the argument into a value
		argument, err := parseValue(parser)
		if err != nil {
			return InvalidCommandErrorf("invalid argument value: %v", err)
		}

		return SetMemoryCommand(ident, argument)
	}

	switch parser.Cursor().Kind {
	case TokenConfig:
		return parseSetConfig(parser)
	case TokenDesignated:
		return parseSetDesignated(parser)
	}

	return InvalidCommandErrorf("unsupported prefix '%v'", parser.Cursor().Literal)
}

func HelpWipe() string {
	return `
The @>wipe<@ command can be used to remove the value of [@>memory<@] variables for a given identifier.
It can also be used to unset the value of [@>designated<@] participants and 
remove entities like [@>users<@] and [@>logics<@] from the lab inventory.

usage:
@>wipe [identifier]<@
@>wipe [entity].[identifier]<@
@>wipe [prefix].[identifier]<@

example:
>>> wipe addr1 
wiped addr1
`
}

func parseWipeCommand(parser *symbolizer.Parser) Command {
	// Read the identifier
	parser.Advance()
	prefix := parser.Cursor()

	// Simple Identifier (not a prefix)
	if !parser.IsPeek(symbolizer.TokenKind('.')) {
		return WipeMemoryCommand(prefix.Literal)
	}

	switch prefix.Kind {
	case TokenConfig:
		return InvalidCommandErrorf("cannot wipe config values")
	case TokenDesignated:
		return parseWipeDesignated(parser)
	case TokenUsers:
		return parseWipeUser(parser)
	case TokenLogics:
		return parseWipeLogic(parser)
	}

	return InvalidCommandErrorf("unsupported prefix '%v'", prefix)
}
