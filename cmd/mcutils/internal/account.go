package internal

import (
	"log"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto/poi"
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
	accounts := make([]tests.AccountWithMnemonic, 0, accountCount)

	for i := 0; i < accountCount; i++ {
		mnemonic := poi.GenerateRandMnemonic().String()

		_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, config.DefaultMOIIDPath)
		if err != nil {
			cmdCommon.Err(err)
		}

		log.Println(publicKey)

		log.Println("MOIID address", common.BytesToAddress(publicKey))

		_, publicKey, err = poi.GetPrivateKeyAtPath(mnemonic, config.DefaultMoiWalletPath)
		if err != nil {
			cmdCommon.Err(err)
		}

		accounts = append(accounts,
			tests.AccountWithMnemonic{
				Addr:     common.BytesToAddress(publicKey),
				Mnemonic: mnemonic,
			})
	}

	err := tests.WriteToAccountsFile(writeAccountsFilePath, accounts)
	if err != nil {
		cmdCommon.Err(err)
	}
}
