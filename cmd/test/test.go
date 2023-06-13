package test

import (
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	testnetCmd := &cobra.Command{
		Use:   "test",
		Short: "Creates the test directories and configurations required to start a test network",
	}

	setupSubCommands(testnetCmd)

	return testnetCmd
}

func setupSubCommands(cmd *cobra.Command) {
	cmd.AddCommand(GetInitCommand())
	cmd.AddCommand(GetAccountCommand())
	cmd.AddCommand(GetGenesisCommand())
	cmd.AddCommand(GetFaucetCommand())
}
