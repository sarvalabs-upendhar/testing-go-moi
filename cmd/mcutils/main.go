package main

import (
	"os"

	"github.com/sarvalabs/go-moi/cmd/mcutils/internal"
	"github.com/spf13/cobra"
)

func main() {
	NewRootCommand().Execute()
}

type Command struct {
	baseCmd *cobra.Command
}

func NewRootCommand() *Command {
	rc := &Command{
		baseCmd: &cobra.Command{
			Use:   "mcutils",
			Short: "Utility tool for creating and managing MOI protocol test network.",
		},
	}

	rc.RegisterSubCommands()

	return rc
}

func (rc *Command) RegisterSubCommands() {
	rc.baseCmd.AddCommand(
		internal.GetInitCommand(),
		internal.GetAccountCommand(),
		internal.GetGenesisCommand(),
		internal.GetFaucetCommand(),
	)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (rc *Command) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
