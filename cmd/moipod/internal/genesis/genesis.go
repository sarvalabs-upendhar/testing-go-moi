package genesis

import (
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:   "genesis",
		Short: "Used to create the genesis file.",
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
		"Path to genesis.json file.",
	)
	genesisCmd.PersistentFlags().StringVar(
		&instancesFilePath,
		"instances-path",
		"instances.json",
		"Path to instances.json file.",
	)
}
