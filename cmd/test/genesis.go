package test

import (
	"crypto/rand"
	"errors"

	"github.com/sarvalabs/moichain/cmd/genesis"

	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

func GetGenesisCommand() *cobra.Command {
	genesisTestCmd := &cobra.Command{
		Use:   "genesis",
		Short: "create genesis file with accounts and their context nodes",
		Run:   runGenesisTestCommand,
	}

	parseGenesisTestFlags(genesisTestCmd)

	return genesisTestCmd
}

func runGenesisTestCommand(cmd *cobra.Command, args []string) {
	createTestGenesisFile()
}

func parseGenesisTestFlags(genesisTestCmd *cobra.Command) {
	genesisTestCmd.Flags().StringVar(
		&genesisFilePath,
		"genesis-path",
		"genesis.json",
		"path to genesis file",
	)
	genesisTestCmd.Flags().StringVar(
		&accountsFilePath,
		"accounts-path",
		"accounts.json",
		"path to accounts file",
	)
	genesisTestCmd.Flags().StringVar(
		&instancesFilePath,
		"instances-path",
		"instances.json",
		"path to instances file",
	)
	genesisTestCmd.Flags().StringSliceVar(
		&accAddresses,
		"addresses",
		[]string{},
		"list of account address",
	)
	genesisTestCmd.Flags().IntVar(
		&behaviouralNodesCount,
		"behavioural-count",
		genesis.DefaultBehaviouralCount,
		"Number of behavioural krama ids per account/logic",
	)
	genesisTestCmd.Flags().IntVar(
		&randomNodesCount,
		"random-count",
		genesis.DefaultRandomCount,
		"Number of random krama ids per account/logic",
	)
}

func getRandomMOIID() []byte {
	randomMoiID := make([]byte, 32)

	_, err := rand.Read(randomMoiID)
	if err != nil {
		common.Err(err)
	}

	return randomMoiID
}

func createTestGenesisFile() {
	var kidTracker int

	kramaIDs, err := genesis.ReadKramaIDsFromInstancesFile(instancesFilePath)
	if err != nil {
		common.Err(err)
	}

	totalIDs := len(kramaIDs)

	if behaviouralNodesCount+randomNodesCount > totalIDs {
		common.Err(errors.New("insufficient krama IDs in instances file"))
	}

	// fetch krama ids in round robin manner
	getKramaIDs := func(count int) []string {
		ids := make([]string, 0, count)

		for i := 0; i < count; i++ {
			id := kramaIDs[kidTracker]
			kidTracker = (kidTracker + 1) % totalIDs

			ids = append(ids, id)
		}

		return ids
	}

	if len(accAddresses) == 0 {
		accAddresses, err = GetAddressFromAccountsFile(accountsFilePath)
		if err != nil {
			common.Err(err)
		}
	}

	accCount := len(accAddresses)

	g := &lattice.GenesisV1{
		Accounts: make([]lattice.AccountInfoV1, 0, accCount),
	}

	g.AddSargaAccount(lattice.AccountInfoV1{
		Address:            types.SargaAddress.Hex(),
		AccountType:        types.SargaAccount,
		MoiID:              types.BytesToHex(getRandomMOIID()),
		BehaviouralContext: getKramaIDs(behaviouralNodesCount),
		RandomContext:      getKramaIDs(randomNodesCount),
	})

	for i := 0; i < accCount; i++ {
		g.AddAccount(
			lattice.AccountInfoV1{
				Address:            accAddresses[i],
				AccountType:        types.RegularAccount,
				MoiID:              types.BytesToHex(getRandomMOIID()),
				BehaviouralContext: getKramaIDs(behaviouralNodesCount),
				RandomContext:      getKramaIDs(randomNodesCount),
			},
		)
	}

	if err = genesis.WriteToGenesisFile(genesisFilePath, g); err != nil {
		common.Err(err)
	}
}
