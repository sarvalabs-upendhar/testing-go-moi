package repl

import (
	"fmt"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/sarvalabs/go-moi-identifiers"
)

func HelpUsers() string {
	return `
@>Users<@ are entities in LogicLab that represent participants in logic interactions.
The list of all registered users can be accessed with the @>users<@ command.

They can be created with the [@>register<@] command and have a unique address and full 
context state. They can be [@>designated<@] as the default sender or receiver for logic calls.
User information can be accessed with [@>get<@] command and they can be removed with the [@>wipe<@] command.

usage:
@>users<@: list all registered users 
@>get users.[username]<@: inspect a specific user
@>wipe users.[username]<@: remove a specific user
`
}

// UsersCommand generates a Command runner
// to print details of all registered users
func UsersCommand() Command {
	return func(repl *Repl) string {
		var (
			idx  = 1
			list strings.Builder
		)

		for username, address := range repl.env.Users {
			list.WriteString(fmt.Sprintf("%v] %v [@>%v<@]", idx, username, repl.FormatValue(address)))

			if idx++; idx <= len(repl.env.Users) {
				list.WriteString("\n")
			}
		}

		if idx == 1 {
			list.WriteString("no users found")
		}

		return Colorize(list.String())
	}
}

func HelpRegister() string {
	return `
The @>register<@ command can be used to register and create [@>users<@] with LogicLab.
They can be registered with a specific address or a randomly generated if not provided.

usage:
@>register [username]<@
@>register [username] as [address]<@

examples:
>>> register rahul
user 'rahul' created with address '0xb1107436395807a00c0d673134d48956315a0c65af620a95a6ada9470fef276e'

>>> register manish as 0xb1107436395807a00c0d673134d48956315a0c65af620a95a6ada9470fef276e
user 'manish' created with address '0xb1107436395807a00c0d673134d48956315a0c65af620a95a6ada9470fef276e'
`
}

// RegisterUserCommand generates a Command runner
// to register a new User with the given username
func RegisterUserCommand(username string, address identifiers.Address) Command {
	return func(repl *Repl) string {
		// Check if a user with username already exists
		if exists := repl.env.UserExists(username); exists {
			return fmt.Sprintf("user %v already exists", username)
		}

		// Register a new user with the environment
		if err := repl.env.RegisterUser(username, address); err != nil {
			return fmt.Sprintf("user %v could not be created: %v", username, err)
		}

		return fmt.Sprintf("user '%v' created with address '%v'", username, repl.env.Users[username])
	}
}

func parseRegisterCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing username for register")
	}

	username := parser.Cursor().Literal

	// Register a user with a random address
	if parser.IsPeek(symbolizer.TokenEoF) {
		return RegisterUserCommand(username, identifiers.NilAddress)
	}

	if !parser.ExpectPeek(TokenPrepositionAs) {
		return InvalidCommandErrorf("invalid register command")
	}

	parser.Advance()

	value, err := parser.Cursor().Value()
	if err != nil {
		return InvalidCommandError("missing address for register")
	}

	addr, ok := value.([]byte)
	if !ok {
		return InvalidCommandError("invalid address for register: must be bytes")
	}

	// Register a user with the given address
	return RegisterUserCommand(username, identifiers.NewAddressFromBytes(addr))
}

func parseGetUser(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after users prefix")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing username")
	}

	username := parser.Cursor().Literal

	return func(repl *Repl) string {
		// Find the user in the inventory
		addr, exists := repl.env.Users[username]
		if !exists {
			return fmt.Sprintf("user '%v' does not exist", username)
		}

		return Colorize(fmt.Sprintf("%v\t[@>%v<@]", username, addr))
	}
}

func parseWipeUser(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after users prefix")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing username")
	}

	username := parser.Cursor().Literal

	return func(repl *Repl) string {
		// Check if a user with username exists
		if exists := repl.env.UserExists(username); !exists {
			return fmt.Sprintf("user %v does not exist", username)
		}

		// Remove the user from the inventory
		if err := repl.env.RemoveUser(username); err != nil {
			return fmt.Sprintf("could not remove user: %v", err)
		}

		return fmt.Sprintf("wiped user '%v'", username)
	}
}
