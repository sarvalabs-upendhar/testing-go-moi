package genesis

import (
	"crypto/rand"
	"errors"

	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

func GetGenesisTestCommand() *cobra.Command {
	genesisTestCmd := &cobra.Command{
		Use:   "test",
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
	genesisTestCmd.Flags().StringSliceVar(
		&accAddresses,
		"addresses",
		[]string{},
		"addresses of accounts",
	)
	genesisTestCmd.Flags().IntVar(
		&behaviouralNodesCount,
		"behavioural-count",
		DefaultBehaviouralCount,
		"Number of behavioural krama ids per account/logic",
	)
	genesisTestCmd.Flags().IntVar(
		&randomNodesCount,
		"random-count",
		DefaultRandomCount,
		"Number of random krama ids per account/logic",
	)

	if err := genesisTestCmd.MarkFlagRequired("behavioural-count"); err != nil {
		common.Err(err)
	}

	if err := genesisTestCmd.MarkFlagRequired("random-count"); err != nil {
		common.Err(err)
	}

	if err := genesisTestCmd.MarkFlagRequired("addresses"); err != nil {
		common.Err(err)
	}
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

	kramaIDs, err := readKramaIDsFromInstancesFile()
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

	accCount := len(accAddresses)

	genesis := &lattice.GenesisV1{
		Accounts: make([]lattice.AccountInfoV1, 0, accCount),
	}

	genesis.AddSargaAccount(lattice.AccountInfoV1{
		Address:            types.SargaAddress.Hex(),
		AccountType:        types.SargaAccount,
		MoiID:              types.BytesToHex(getRandomMOIID()),
		BehaviouralContext: getKramaIDs(behaviouralNodesCount),
		RandomContext:      getKramaIDs(randomNodesCount),
	})

	for i := 0; i < accCount; i++ {
		genesis.AddAccount(
			lattice.AccountInfoV1{
				Address:            accAddresses[i],
				AccountType:        types.RegularAccount,
				MoiID:              types.BytesToHex(getRandomMOIID()),
				BehaviouralContext: getKramaIDs(behaviouralNodesCount),
				RandomContext:      getKramaIDs(randomNodesCount),
			},
		)
	}

	if err = writeToGenesisFile(genesis); err != nil {
		common.Err(err)
	}
}
