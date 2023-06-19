package genesis

import (
	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
	"github.com/spf13/cobra"
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
		int(types.RegularAccount),
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
		common.Err(err)
	}
}

func initAccount() {
	genesis, err := readGenesisFile()
	if err != nil {
		common.Err(err)
	}

	if len(behaviourNodes) == 0 && len(randomNodes) == 0 {
		behaviourNodes, randomNodes = getContextNodes(
			instancesFilePath,
			common.DefaultBehaviouralCount,
			common.DefaultRandomCount,
		)
	}

	genesis.AddAccount(types.AccountSetupArgs{
		Address:            types.HexToAddress(address),
		AccType:            types.AccountType(accountType),
		MoiID:              moiID,
		BehaviouralContext: utils.KramaIDFromString(behaviourNodes),
		RandomContext:      utils.KramaIDFromString(randomNodes),
	})

	if err = common.WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		common.Err(err)
	}
}
