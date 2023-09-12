package cmds

import (
	"fmt"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

var (
	ErrNoDesignatedSender   = errors.New("no designated sender")
	ErrNoDesignatedReceiver = errors.New("no designated receiver")
)

func HelpDesignated() string {
	return `
@>Designated<@ participants are special users who are used as the default @>sender<@ and/or @>receiver<@ for logic 
calls when applicable. The referenced [@>users<@] must already exist in the lab [@>inventory<@]. Designated users
can be accessed and modified with the [@>get<@] and [@>set<@] commands and are persisted across multiple sessions.

usage:
@>get designated.[actor]<@
@>set designated.[actor] [username]<@
@>wipe designated.[actor]<@

example:
>>> get designated.receiver 
designated.receiver: manish

>>> set designated.sender rahul
designated.sender: rahul

>>> wipe designated.sender 
wiped designated.sender
`
}

func parseGetDesignated(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after designated prefix")
	}

	parser.Advance()
	actor := parser.Cursor()

	return func(env *Environment) string {
		var username string

		switch actor.Kind {
		case TokenSender:
			username = env.inventory.Sender
		case TokenReceiver:
			username = env.inventory.Receiver
		default:
			return fmt.Sprintf("invalid designated parameter: %v", actor.Literal)
		}

		return fmt.Sprintf("designated.%v: %v", actor, username)
	}
}

func parseSetDesignated(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after designated prefix")
	}

	parser.Advance()
	actor := parser.Cursor()
	parser.Advance()

	// Parse the username into a value
	argument, err := parseValue(parser)
	if err != nil {
		return InvalidCommandErrorf("invalid argument value: %v", err)
	}

	username, ok := argument.(Identifier)
	if !ok {
		return InvalidCommandErrorf("value for designated.%v must be an ident", actor)
	}

	return func(env *Environment) string {
		switch actor.Kind {
		case TokenSender:
			env.inventory.Sender = string(username)
		case TokenReceiver:
			env.inventory.Receiver = string(username)
		default:
			return fmt.Sprintf("invalid designated parameter: %v", actor.Literal)
		}

		return fmt.Sprintf("designated.%v: %v", actor, username)
	}
}

func parseWipeDesignated(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after designated prefix")
	}

	parser.Advance()
	actor := parser.Cursor()

	return func(env *Environment) string {
		switch actor.Kind {
		case TokenSender:
			env.inventory.Sender = ""
		case TokenReceiver:
			env.inventory.Receiver = ""
		default:
			return fmt.Sprintf("invalid designated parameter: %v", actor.Literal)
		}

		return fmt.Sprintf("wiped designated.%v", actor)
	}
}

func fetchDesignatedSenderState(env *Environment) (*core.UserAccount, error) {
	if env.inventory.Sender == "" {
		return nil, ErrNoDesignatedSender
	}

	return fetchSenderState(env, env.inventory.Sender)
}

func fetchSenderState(env *Environment, username string) (*core.UserAccount, error) {
	// Find the user in the inventory
	user, exists := env.inventory.FindUser(username)
	if !exists {
		return nil, errors.Errorf("user '%v' not found", username)
	}

	return user, nil
}
