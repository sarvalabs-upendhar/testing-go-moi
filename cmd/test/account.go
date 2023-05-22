package test

import (
	cmdCommon "github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

func GetAccountCommand() *cobra.Command {
	testAccountCmd := &cobra.Command{
		Use:   "account",
		Short: "create test accounts",
		Run:   runAccountCommand,
	}

	parseAccountFlags(testAccountCmd)

	return testAccountCmd
}

func parseAccountFlags(cmd *cobra.Command) {
	cmd.Flags().IntVar(
		&count,
		"count",
		0,
		"No.of accounts",
	)
	cmd.Flags().StringVar(
		&accountsFilePath,
		"accounts-path",
		"accounts.json",
		"path to accounts file",
	)

	_ = cmd.MarkFlagRequired("count")
}

func runAccountCommand(cmd *cobra.Command, args []string) {
	accounts := make([]AccountWithMnemonic, 0, count)

	for i := 0; i < count; i++ {
		mnemonic := poi.GenerateRandMnemonic().String()

		_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, common.DefaultMOIWalletPath)
		if err != nil {
			cmdCommon.Err(err)
		}

		accounts = append(accounts,
			AccountWithMnemonic{
				Addr:     types.BytesToAddress(publicKey),
				Mnemonic: mnemonic,
			})
	}

	err := WriteToAccountsFile(accountsFilePath, accounts)
	if err != nil {
		cmdCommon.Err(err)
	}
}
