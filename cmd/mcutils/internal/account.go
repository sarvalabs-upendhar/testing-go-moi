package internal

import (
	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/common/tests"
)

func GetAccountCommand() *cobra.Command {
	testAccountCmd := &cobra.Command{
		Use:   "account",
		Short: "Creates test accounts.",
		Run:   runAccountCommand,
	}

	parseAccountFlags(testAccountCmd)

	return testAccountCmd
}

func parseAccountFlags(cmd *cobra.Command) {
	cmd.Flags().IntVar(
		&accountCount,
		"count",
		0,
		"Count gives number of test accounts.",
	)
	cmd.Flags().StringVar(
		&writeAccountsFilePath,
		"accounts-path",
		"accounts.json",
		"Path to accounts.json file.",
	)

	_ = cmd.MarkFlagRequired("count")
}

func runAccountCommand(cmd *cobra.Command, args []string) {
	accounts, err := cmdCommon.GetAccountsWithMnemonic(accountCount)
	if err != nil {
		cmdCommon.Err(err)
	}

	err = tests.WriteToAccountsFile(writeAccountsFilePath, accounts)
	if err != nil {
		cmdCommon.Err(err)
	}
}
