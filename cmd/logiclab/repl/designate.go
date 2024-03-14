package repl

import (
	"fmt"

	"github.com/manishmeganathan/symbolizer"
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

	return func(repl *Repl) string {
		var username string

		switch actor.Kind {
		case TokenSender:
			username = repl.env.Sender
		case TokenReceiver:
			username = repl.env.Receiver
		default:
			return fmt.Sprintf("invalid designated parameter: %v", actor.Literal)
		}

		return fmt.Sprintf("designated.%v: %v", actor.Literal, username)
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

	return func(repl *Repl) string {
		switch actor.Kind {
		case TokenSender:
			if err = repl.env.SetDefaultSender(string(username)); err != nil {
				return fmt.Sprintf("could not set user as sender: %v", err)
			}
		case TokenReceiver:
			if err = repl.env.SetDefaultReceiver(string(username)); err != nil {
				return fmt.Sprintf("could not set user as receiver: %v", err)
			}
		default:
			return fmt.Sprintf("invalid designated parameter: %v", actor.Literal)
		}

		return fmt.Sprintf("designated.%v: %v", actor.Literal, username.String())
	}
}

func parseWipeDesignated(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after designated prefix")
	}

	parser.Advance()
	actor := parser.Cursor()

	return func(repl *Repl) string {
		switch actor.Kind {
		case TokenSender:
			repl.env.Sender = ""
		case TokenReceiver:
			repl.env.Receiver = ""
		default:
			return fmt.Sprintf("invalid designated parameter: %v", actor.Literal)
		}

		return fmt.Sprintf("wiped designated.%v", actor.Literal)
	}
}
