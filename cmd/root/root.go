package root

import (
	"os"

	"github.com/sarvalabs/moichain/cmd/register"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/cmd/bootnode"
	"github.com/sarvalabs/moichain/cmd/genesis"
	"github.com/sarvalabs/moichain/cmd/logiclab"
	"github.com/sarvalabs/moichain/cmd/server"
	"github.com/sarvalabs/moichain/cmd/test"
)

type Command struct {
	baseCmd *cobra.Command
}

func NewRootCommand() *Command {
	rc := &Command{
		baseCmd: &cobra.Command{
			Use:   "moichain",
			Short: "Moichain is a context aware blockchain protocol",
		},
	}

	rc.RegisterSubCommands()

	return rc
}

func (rc *Command) RegisterSubCommands() {
	rc.baseCmd.AddCommand(
		server.GetServerCommand(),
		bootnode.GetCommand(),
		test.GetCommand(),
		logiclab.GetCommand(),
		genesis.GetCommand(),
		register.GetCommand(),
	)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (rc *Command) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
