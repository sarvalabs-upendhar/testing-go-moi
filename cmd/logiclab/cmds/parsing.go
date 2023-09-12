package cmds

import (
	"github.com/manishmeganathan/symbolizer"
	engineio "github.com/sarvalabs/go-moi-engineio"
)

const (
	TokenExit symbolizer.TokenKind = -(iota + 10)
	TokenHelp

	TokenGet
	TokenSet
	TokenWipe

	TokenPrepositionAs
	TokenPrepositionFrom
	TokenPrepositionInto
	TokenPrepositionWith

	TokenConfig
	TokenConfigParam

	TokenDesignated
	TokenSender
	TokenReceiver

	TokenBig
	TokenManifest

	TokenUsers
	TokenRegister

	TokenLogics
	TokenCompile

	TokenEngines
	TokenEngineKind

	TokenDeploy
	TokenInvoke

	TokenCallencode
	TokenCalldecode
	TokenErrdecode
	TokenSlothash

	TokenConvert
	TokenManifestEncoding
	TokenManifestCodeform
)

// keywords is a mapping of custom keywords to their
// symbolizer TokenKinds used to parse commands
var keywords = map[string]symbolizer.TokenKind{
	"exit": TokenExit,
	"help": TokenHelp,

	"get":  TokenGet,
	"set":  TokenSet,
	"wipe": TokenWipe,

	"as":   TokenPrepositionAs,
	"from": TokenPrepositionFrom,
	"into": TokenPrepositionInto,
	"with": TokenPrepositionWith,

	"config":    TokenConfig,
	"basefuel":  TokenConfigParam,
	"hexbigint": TokenConfigParam,
	"hexbytes":  TokenConfigParam,

	"designated": TokenDesignated,
	"sender":     TokenSender,
	"receiver":   TokenReceiver,

	"big":      TokenBig,
	"manifest": TokenManifest,

	"users":    TokenUsers,
	"register": TokenRegister,

	"logics":  TokenLogics,
	"compile": TokenCompile,

	"engines": TokenEngines,
	"PISA":    TokenEngineKind,

	"callencode": TokenCallencode,
	"calldecode": TokenCalldecode,
	"errdecode":  TokenErrdecode,
	"slothash":   TokenSlothash,

	"convert": TokenConvert,
	"POLO":    TokenManifestEncoding,
	"JSON":    TokenManifestEncoding,
	"YAML":    TokenManifestEncoding,
	"BIN":     TokenManifestCodeform,
	"HEX":     TokenManifestCodeform,
	"ASM":     TokenManifestCodeform,

	"deploy": TokenDeploy,
	"invoke": TokenInvoke,
}

// Parse parses an input command string into a Command runner
func Parse(cmd string) Command {
	parser := symbolizer.NewParser(cmd,
		symbolizer.IgnoreWhitespaces(),
		symbolizer.Keywords(keywords),
	)

	switch parser.Cursor().Kind {
	case TokenExit:
		return ExitCommand()
	case TokenHelp:
		return parseHelpCommand(parser)

	case TokenGet:
		return parseGetCommand(parser)
	case TokenSet:
		return parseSetCommand(parser)
	case TokenWipe:
		return parseWipeCommand(parser)

	case TokenManifest:
		return parseManifestCommand(parser)
	case TokenConvert:
		return parseConvertCommand(parser)

	case TokenUsers:
		return UsersCommand()
	case TokenLogics:
		return LogicsCommand()
	case TokenEngines:
		return EnginesCommand()

	case TokenRegister:
		return parseRegisterCommand(parser)
	case TokenCompile:
		return parseCompileCommand(parser)

	case TokenCallencode:
		return parseCallencodeCommand(parser)
	case TokenCalldecode:
		return parseCalldecodeCommand(parser)
	case TokenErrdecode:
		return parseErrdecodeCommand(parser)
	case TokenSlothash:
		return parseSlothashCommand(parser)

	case TokenDeploy:
		return parseLogicCall(parser, engineio.DeployerCallsite)
	case TokenInvoke:
		return parseLogicCall(parser, engineio.InvokableCallsite)

	default:
		return InvalidCommandError("")
	}
}
