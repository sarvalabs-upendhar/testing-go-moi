package genesis

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/spf13/cobra"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common/utils"
)

var (
	behaviourNodes []string
	randomNodes    []string
)

func GetInitAccountCommand() *cobra.Command {
	initAccountCmd := &cobra.Command{
		Use:   "init-account",
		Short: "Initializes the account.",
		Run: func(cmd *cobra.Command, args []string) {
			initAccount()
		},
	}

	parseInitAccountFlags(initAccountCmd)

	return initAccountCmd
}

func parseInitAccountFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&address,
		"address",
		"",
		"Address of the account.",
	)
	cmd.Flags().IntVar(
		&accountType,
		"account-type",
		int(common.RegularAccount),
		"Type of account. SargaAccount = 1, RegularAccount = 2, LogicAccount = 3, AssetAccount = 4",
	)
	cmd.Flags().StringVar(
		&moiID,
		"moi-id",
		"",
		"Moi-id of the participant.",
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

	if err := cobra.MarkFlagRequired(cmd.Flags(), "address"); err != nil {
		cmdcommon.Err(err)
	}
}

func initAccount() {
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

	genesis.AddAccount(common.AccountSetupArgs{
		Address:            common.HexToAddress(address),
		AccType:            common.AccountType(accountType),
		MoiID:              moiID,
		BehaviouralContext: utils.KramaIDFromString(behaviourNodes),
		RandomContext:      utils.KramaIDFromString(randomNodes),
	})

	if err = cmdcommon.WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		cmdcommon.Err(err)
	}
}
