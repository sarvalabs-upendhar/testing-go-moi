package cmds

import (
	"fmt"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/sarvalabs/go-moi-engineio"
)

func HelpLogics() string {
	return `
@>Logics<@ are entities in LogicLab that represent executable code in logic interactions.
The list of all compiled logics can be accessed with the @>logics<@ command.

They can be created with the [@>compile<@] command using a raw [@>manifest<@] or file and have a unique address, 
along with a full context state. If a logic is stateful, a [@>deploy<@] must occur before it is ready for use.
Logic information can be accessed with [@>get<@] command and they can be removed with the [@>wipe<@] command.

usage:
@>logics<@: list all registered users 
@>get logics.[name]<@: inspect a specific logic
@>wipe logics.[name]<@: remove a specific logic
`
}

// LogicsCommand generates a Command runner
// to print details of all compiled logics
func LogicsCommand() Command {
	return func(env *Environment) string {
		var (
			idx  = 1
			list strings.Builder
		)

		for name, logicID := range env.inventory.Logics {
			list.WriteString(fmt.Sprintf("%v] %v [@>%v<@]", idx, name, logicID))

			if idx++; idx <= len(env.inventory.Logics) {
				list.WriteString("\n")
			}
		}

		if idx == 1 {
			list.WriteString("no logics found")
		}

		return Colorize(list.String())
	}
}

func HelpCompile() string {
	return `
The @>compile<@ command can be used to compile and create [@>logics<@] with LogicLab.
They can be compiled with a valid [@>manifest<@] expression and have a randomly generated address.

usage:
@>compile [name] from manifest(...)<@

examples:
>>> compile Ledger from manifest("./jug/manifests/ledger.yaml")     
logic 'Ledger' [0800007140e42388a825992f5f07c7711718384b0ef228b36f46511503295e1dc38931] compiled with 100 FUEL
`
}

// CompileLogicCommand generates a Command runner to compile
// a new Logic with the given name and manifest object
func CompileLogicCommand(name string, manifest *engineio.Manifest) Command {
	return func(env *Environment) string {
		// Check if a logic with name already exists
		if _, exists := env.inventory.Logics[name]; exists {
			return fmt.Sprintf("logic %v already exists", name)
		}

		// Compile the manifest into a Logic
		logic, fuel, err := CompileManifest(name, manifest, env.inventory.Config.BaseFuel)
		if err != nil {
			return fmt.Sprintf("logic could not be compiled: %v", err)
		}

		// Add the logic to the inventory
		env.inventory.AddLogic(logic)

		return fmt.Sprintf("logic '%v' [%v] compiled with %v FUEL", name, logic.Logic.ID, fuel)
	}
}

func parseCompileCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing name for compile")
	}

	ident := parser.Cursor().Literal

	if !parser.ExpectPeek(TokenPrepositionFrom) {
		return InvalidCommandError("missing from preposition")
	}

	if !parser.ExpectPeek(TokenManifest) {
		return InvalidCommandError("missing manifest expression")
	}

	manifest, _, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	return CompileLogicCommand(ident, manifest)
}

func parseGetLogic(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after logics prefix")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing logic name")
	}

	name := parser.Cursor().Literal

	return func(env *Environment) string {
		// Find the logic in the inventory
		logic, exists := env.inventory.FindLogic(name)
		if !exists {
			return fmt.Sprintf("logic %v does not exist", name)
		}

		return Colorize(logic.String())
	}
}

func parseWipeLogic(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandErrorf("missing . after logics prefix")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing logic name")
	}

	name := parser.Cursor().Literal

	return func(env *Environment) string {
		// Check if a logic with name exists
		if exists := env.inventory.LogicExists(name); !exists {
			return fmt.Sprintf("logic %v does not exist", name)
		}

		// Remove the logic from the inventory
		env.inventory.RemoveLogic(name)

		return fmt.Sprintf("wiped logic '%v'", name)
	}
}
