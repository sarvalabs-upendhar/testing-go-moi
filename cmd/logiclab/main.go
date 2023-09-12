package main

import (
	"fmt"

	"github.com/chzyer/readline"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-pisa"

	"github.com/sarvalabs/go-moi/cmd/logiclab/cmds"
	"github.com/sarvalabs/go-moi/crypto"
)

const (
	PROMPT = "\u001b[32m>>> \u001b[0m"
	DIVIDE = "======================================================"
	STRIKE = "------------------------------------------------------"

	FIGLET = "" + DIVIDE + "\n" +
		"\u001b[32m" +
		"dP                   oo          dP          dP       \n" +
		"88                               88          88       \n" +
		"88 .d8888b. .d8888b. dP .d8888b. 88 .d8888b. 88d888b. \n" +
		"88 88'  `88 88'  `88 88 88'  `\"\" 88 88'  `88 88'  `88 \n" +
		"88 88.  .88 88.  .88 88 88.  ... 88 88.  .88 88.  .88 \n" +
		"dP `88888P' `8888P88 dP `88888P' dP `88888P8 88Y8888' \n" +
		"                 .88                                  \n" +
		"             d8888P                                   \n" +
		"\u001b[0m" +
		DIVIDE + "\n"

	CLOSER = "\r" + DIVIDE + "\nClosing LogicLab REPL"
	LAUNCH = FIGLET +
		"LogicLab Initialized @ \u001B[32m%v\u001B[0m\n" +
		"LogicLab Documentation: %v\n" +
		"\u001B[32mStarting REPL ...\u001B[0m (use 'exit' or 'ctrl-c' to close)\n" +
		DIVIDE

	DOCS = "https://cocolang.dev/tour"
)

func init() {
	engineio.RegisterRuntime(pisa.NewRuntime(), crypto.Cryptographer(0))
}

func main() {
	CliRoot().Execute()
}

// REPL start the REPL (Read-Evaluate-Print Loop) for LogicLab Commands.
func REPL(env *cmds.Environment, term *readline.Instance) error {
	for {
		// READ
		line, err := term.Readline()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) {
				return env.Close(CLOSER)
			}

			fmt.Println("failed to readline:", err)
		}

		// skip empty line eval
		if line == "" {
			continue
		}

		// EVALUATE
		command := cmds.Parse(line)
		result := command(env)

		// handle aborted environment
		if env.Aborted() {
			return env.Close(CLOSER)
		}

		// PRINT
		println(result)
	}
}
