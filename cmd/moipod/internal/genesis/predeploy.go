package genesis

import (
	"github.com/spf13/cobra"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
)

func GetPreDeployCommand() *cobra.Command {
	preDeployCmd := &cobra.Command{
		Use:   "predeploy",
		Short: "Adds genesis logics.",
		Run:   runPreDeployCommand,
	}

	setupPreDeploySubCommands(preDeployCmd)
	parsePreDeployFlags(preDeployCmd)

	return preDeployCmd
}

func runPreDeployCommand(cmd *cobra.Command, args []string) {
	artifact, err := cmdcommon.ReadArtifactFile(artifact)
	if err != nil {
		cmdcommon.Err(err)
	}

	addGenesisLogic(artifact)
}

func setupPreDeploySubCommands(cmd *cobra.Command) {
	cmd.AddCommand()
}

func parsePreDeployFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&artifact,
		"artifact-path",
		"artifact.json",
		"Path to logic artifact.json file.",
	)
	cmd.Flags().StringSliceVar(
		&behaviourNodes,
		"behaviour-nodes",
		[]string{},
		"List of krama ids. Format: <kramaID1,kramaID2,...>",
	)
	cmd.Flags().StringSliceVar(
		&randomNodes,
		"random-nodes",
		[]string{},
		"List of krama ids. Format: <kramaID1,kramaID2,...>",
	)
}

// addGenesisLogic takes path to logic file and appends it to current set of logics
func addGenesisLogic(artifact *cmdcommon.Artifact) {
	genesis, err := common.ReadGenesisFile(genesisFilePath)
	if err != nil {
		cmdcommon.Err(err)
	}

	if len(behaviourNodes) == 0 && len(randomNodes) == 0 {
		behaviourNodes, randomNodes = getContextNodes(
			instancesFilePath,
			cmdcommon.DefaultBehaviouralCount,
			cmdcommon.DefaultRandomCount,
		)
	}

	genesis.AddLogic(common.LogicSetupArgs{
		Name:               artifact.Name,
		Callsite:           artifact.Callsite,
		Calldata:           artifact.Calldata,
		Manifest:           artifact.Manifest,
		BehaviouralContext: utils.KramaIDFromString(behaviourNodes),
		RandomContext:      utils.KramaIDFromString(randomNodes),
	})

	if err = cmdcommon.WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		cmdcommon.Err(err)
	}
}
