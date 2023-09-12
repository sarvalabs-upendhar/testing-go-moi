package cmds

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// Command is a function form that accepts a logiclab Environment,
// performs some operations in it and then returns a string output.
type Command func(env *Environment) string

func HelpExit() string {
	return `
The @>exit<@ command can be used to close the LogicLab REPL. This closes the session, wipes all memory variables
and saves all entities to the [@>inventory<@] file. The @>ctrl-c (^C)<@ interrupt can also be used for the same effect
`
}

// ExitCommand generates a Command runner to
// abort the environment and close the REPL.
func ExitCommand() Command {
	return func(env *Environment) string {
		env.Abort()

		return ""
	}
}

// InvalidCommandError generates a Command runner
// to handle invalid commands declarations
func InvalidCommandError(err string) Command {
	return func(env *Environment) string {
		if err == "" {
			return "invalid command"
		}

		return fmt.Sprintf("invalid command: %v", err)
	}
}

// InvalidCommandErrorf generates a Command runner
// to handle invalid commands declarations
func InvalidCommandErrorf(format string, a ...any) Command {
	return InvalidCommandError(fmt.Sprintf(format, a...))
}

func RunFormula(env *Environment, fpath string, suppress bool) error {
	file, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a buffer to read the file contents
	reader := io.Reader(file)
	buffer := make([]byte, 1024*1024)

	var result string

	for {
		var size int

		if size, err = reader.Read(buffer); err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else {
				return err
			}
		}

		content := string(buffer[:size])
		lines := strings.Split(content, "\n")

		for _, line := range lines {
			// Trim leading and trailing whitespace
			line = strings.TrimSpace(line)

			// Skip empty lines
			if line == "" {
				continue
			}

			// Evaluate the command
			command := Parse(line)
			result = command(env)

			// Check if the abort flag is set
			if env.Aborted() {
				_ = env.Close("")

				break
			}

			// Print the input and output if not suppressed
			if !suppress {
				fmt.Println(">> ", line)
				fmt.Println(result)
			}
		}
	}

	// Print the last executed command's output
	// Suppression is only applied for intermediary commands
	if result != "" {
		fmt.Println(result)
	}

	return nil
}
