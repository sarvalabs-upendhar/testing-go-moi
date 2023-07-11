package main

import (
	"os"

	"github.com/sarvalabs/moichain/cmd/moipod/internal"
	"github.com/sarvalabs/moichain/cmd/moipod/internal/genesis"
	"github.com/sarvalabs/moichain/cmd/moipod/internal/server"
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
			Use:   "moipod",
			Short: "MOI Pod is a golang client implementation of MOI protocol.",
		},
	}

	rc.RegisterSubCommands()

	return rc
}

func (rc *Command) RegisterSubCommands() {
	rc.baseCmd.AddCommand(
		genesis.GetCommand(),
		server.GetServerCommand(),
		internal.GetRegisterCommand(),
		internal.GetBootNodeCommand(),
	)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (rc *Command) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
