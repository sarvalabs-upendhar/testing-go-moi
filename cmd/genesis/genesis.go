package genesis

import (
	"github.com/spf13/cobra"
)

const (
	DefaultBehaviouralCount = 1
	DefaultRandomCount      = 0
)

func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:   "genesis",
		Short: "creates genesis file",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	parseFlags(genesisCmd)
	setupSubCommands(genesisCmd)

	return genesisCmd
}

func setupSubCommands(cmd *cobra.Command) {
	cmd.AddCommand(
		GetGenesisTestCommand(),
		GetInitAccountCommand(),
		GetPreDeployCommand(),
		GetPremineCommand(),
	)
}

func parseFlags(genesisCmd *cobra.Command) {
	genesisCmd.PersistentFlags().StringVar(
		&genesisFilePath,
		"genesis-path",
		"genesis.json",
		"path to genesis file",
	)
	genesisCmd.PersistentFlags().StringVar(
		&instancesFilePath,
		"instances-path",
		"instances.json",
		"path to instances file",
	)
}
