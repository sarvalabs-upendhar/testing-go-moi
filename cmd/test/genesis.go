package test

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/sarvalabs/moichain/common/hexutil"

	id "github.com/sarvalabs/moichain/mudra/kramaid"

	"github.com/sarvalabs/moichain/cmd/genesis"
	"github.com/sarvalabs/moichain/common/tests"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/types"
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
	genesisTestCmd.Flags().Uint64Var(
		&premineAmount,
		"premine-amount",
		0,
		"amount of moi tokens that need to credited for each account",
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
	getKramaIDs := func(count int) []id.KramaID {
		ids := make([]id.KramaID, 0, count)

		for i := 0; i < count; i++ {
			kid := kramaIDs[kidTracker]
			kidTracker = (kidTracker + 1) % totalIDs

			ids = append(ids, id.KramaID(kid))
		}

		return ids
	}

	if len(accAddresses) == 0 {
		accAddresses, err = tests.GetAddressFromAccountsFile(accountsFilePath)
		if err != nil {
			common.Err(err)
		}
	}

	accCount := len(accAddresses)

	g := &types.GenesisFile{
		Accounts: make([]types.AccountSetupArgs, 0, accCount),
	}

	g.AddSargaAccount(types.AccountSetupArgs{
		Address:            types.SargaAddress,
		AccType:            types.SargaAccount,
		MoiID:              types.BytesToHex(getRandomMOIID()),
		BehaviouralContext: getKramaIDs(behaviouralNodesCount),
		RandomContext:      getKramaIDs(randomNodesCount),
	})

	assetInfo := types.AssetAccountSetupArgs{
		AssetInfo: &types.AssetCreationArgs{
			Symbol:      types.MOITokenSymbol,
			Dimension:   0,
			Standard:    0,
			IsLogical:   false,
			IsStateful:  false,
			Owner:       types.NilAddress,
			Allocations: make([]types.Allocation, 0, accCount),
		},
		BehaviouralContext: getKramaIDs(behaviouralNodesCount),
		RandomContext:      getKramaIDs(randomNodesCount),
	}

	for i := 0; i < accCount; i++ {
		g.AddAccount(
			types.AccountSetupArgs{
				Address:            types.HexToAddress(accAddresses[i]),
				AccType:            types.RegularAccount,
				MoiID:              types.BytesToHex(getRandomMOIID()),
				BehaviouralContext: getKramaIDs(behaviouralNodesCount),
				RandomContext:      getKramaIDs(randomNodesCount),
			},
		)

		if premineAmount > 0 {
			assetInfo.AssetInfo.Allocations = append(assetInfo.AssetInfo.Allocations, types.Allocation{
				Address: types.HexToAddress(accAddresses[i]),
				Amount:  (*hexutil.Big)(new(big.Int).SetUint64(premineAmount)),
			})
		}
	}

	g.AddAssetInfo(assetInfo)

	if err = genesis.WriteToGenesisFile(genesisFilePath, g); err != nil {
		common.Err(err)
	}
}
