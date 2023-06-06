package genesis

import (
	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

func GetPreDeployCommand() *cobra.Command {
	preDeployCmd := &cobra.Command{
		Use:   "predeploy",
		Short: "add genesis logics",
		Run:   runPreDeployCommand,
	}

	setupPreDeploySubCommands(preDeployCmd)
	parsePreDeployFlags(preDeployCmd)

	return preDeployCmd
}

func runPreDeployCommand(cmd *cobra.Command, args []string) {
	artifact, err := readArtifactFile(artifact)
	if err != nil {
		common.Err(err)
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
		"path to logic artifact json file",
	)
	cmd.Flags().StringVar(
		&moiID,
		"moi-id",
		"",
		"moi-id of the participant",
	)
	cmd.Flags().StringSliceVar(
		&behaviourNodes,
		"behaviour-nodes",
		[]string{},
		"list of krama ids format<kramaID1,kramaID2,...>",
	)
	cmd.Flags().StringSliceVar(
		&randomNodes,
		"random-nodes",
		[]string{},
		"list of krama ids format<kramaID1,kramaID2,...>",
	)
}

// addGenesisLogic takes path to logic file and appends it to current set of logics
func addGenesisLogic(artifact *Artifact) {
	genesis, err := readGenesisFile()
	if err != nil {
		common.Err(err)
	}

	if len(behaviourNodes) == 0 && len(randomNodes) == 0 {
		behaviourNodes, randomNodes = getContextNodes(instancesFilePath, DefaultBehaviouralCount, DefaultRandomCount)
	}

	genesis.AddLogic(types.GenesisLogic{
		Name:               artifact.Name,
		Callsite:           artifact.Callsite,
		Calldata:           artifact.Calldata,
		Manifest:           artifact.Manifest,
		BehaviouralContext: behaviourNodes,
		RandomContext:      randomNodes,
	})

	if err = WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		common.Err(err)
	}
}

//
// var stakingCmd = &cobra.Command{
//	Use:   "staking-contract",
//	Short: "update genesis logic with staking contract",
//	Run: func(cmd *cobra.Command, args []string) {
//		calldata := "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29ec4" +
//			"42dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"
//
//		m, err := ReadManifest("./jug/manifests/erc20.json")
//		if err != nil {
//			common.Err(err)
//		}
//
//		manifest := "0x" + types.BytesToHex(m)
//
//		artifact := &Artifact{
//			Name:     "staking-contract",
//			Callsite: "Seeder!",
//			Calldata: hexutil.Bytes(types.Hex2Bytes(calldata)),
//			Manifest: hexutil.Bytes(types.Hex2Bytes(manifest)),
//		}
//
//		addGenesisLogic(artifact)
//	},
//}
