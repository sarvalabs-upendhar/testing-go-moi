package genesis

import (
	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

var (
	behaviourNodes []string
	randomNodes    []string
)

func GetInitAccountCommand() *cobra.Command {
	initAccountCmd := &cobra.Command{
		Use:   "init-account",
		Short: "initialize account",
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
		"address of the account",
	)
	cmd.Flags().IntVar(
		&accountType,
		"account-type",
		int(types.RegularAccount),
		"address of the account",
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
		behaviourNodes, randomNodes = getContextNodes(instancesFilePath, DefaultBehaviouralCount, DefaultRandomCount)
	}

	genesis.AddAccount(lattice.AccountInfoV1{
		Address:            address,
		AccountType:        types.AccountType(accountType),
		MoiID:              moiID,
		BehaviouralContext: behaviourNodes,
		RandomContext:      randomNodes,
	})

	if err = WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		common.Err(err)
	}
}
