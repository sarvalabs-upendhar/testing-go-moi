package cmds

import (
	"fmt"
	"strings"

	"github.com/manishmeganathan/symbolizer"
)

// CommandHelp is a function form that returns the help doc string
type CommandHelp func() string

func parseHelpCommand(parser *symbolizer.Parser) Command {
	// Read the keyword
	parser.Advance()
	keyword := parser.Cursor().Literal
	// Lookup a command helper for the given keyword
	helper, exists := cmdHelp[keyword]

	return func(env *Environment) string {
		if !exists {
			return fmt.Sprintf("no help found for '%v'", keyword)
		}

		// Trim leading & trailing spaces
		cleaned := strings.TrimSpace(helper())
		// Colorize and return
		return Colorize(cleaned)
	}
}

func HelpHelp() string {
	//nolint:lll
	return `
The @>help<@ command can be used to view help documentation for commands and concepts.
When accessing the help docs, colored words in square brackets indicate other valid help strings.
Use the [@>exit<@] command to close the LogicLab REPL.

getting started:
- [@>register<@] command: create new [@>users<@]
- [@>compile<@] command:  create new [@>logics<@]
- [@>get<@] [@>set<@] [@>wipe<@]: access, modify and clear values in the session [@>memory<@], [@>config<@] or [@>designated<@] users
- [@>convert<@] command: convert [@>manifest<@] files between different file and code forms
- [@>invoke<@] [@>deploy<@]:  perform deploy or invoke logic calls
`
}

var cmdHelp = map[string]CommandHelp{
	"":     HelpHelp,
	"exit": HelpExit,

	"set":  HelpSet,
	"get":  HelpGet,
	"wipe": HelpWipe,

	"memory":     HelpMemory,
	"config":     HelpConfig,
	"designated": HelpDesignated,

	"manifest": HelpManifest,
	"convert":  HelpConvert,

	"argument":  HelpArgument,
	"inventory": HelpInventory,

	"users":   HelpUsers,
	"logics":  HelpLogics,
	"engines": HelpEngines,

	"register": HelpRegister,
	"compile":  HelpCompile,

	"callencode": HelpCallencode,
	"calldecode": HelpCalldecode,
	"errdecode":  HelpErrdecode,
	"slothash":   HelpSlothash,

	"invoke": HelpInvoke,
	"deploy": HelpDeploy,
}
