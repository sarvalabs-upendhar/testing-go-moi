package genesis

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/spf13/cobra"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
)

var consensusNodes []string

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
		&participantID,
		"participant-id",
		"",
		"Identifier of the account.",
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
		&consensusNodes,
		"consensus-nodes",
		[]string{},
		"List of krama ids. Format: <kramaID1,kramaID2,...>",
	)

	if err := cobra.MarkFlagRequired(cmd.Flags(), "participant-id"); err != nil {
		cmdcommon.Err(err)
	}
}

func initAccount() {
	genesis, err := common.ReadGenesisFile(genesisFilePath)
	if err != nil {
		cmdcommon.Err(err)
	}

	if len(consensusNodes) == 0 {
		consensusNodes = getContextNodes(
			instancesFilePath,
			common.ConsensusNodesSize,
		)
	}

	decodedParticipantID, err := identifiers.NewParticipantIDFromHex(participantID)
	if err != nil {
		panic(err)
	}

	genesis.AddAccount(common.AccountSetupArgs{
		ID:             decodedParticipantID.AsIdentifier(),
		AccType:        common.AccountType(accountType),
		MoiID:          moiID,
		ConsensusNodes: utils.KramaIDFromString(consensusNodes),
	})

	if err = cmdcommon.WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		cmdcommon.Err(err)
	}
}
