package internal

import (
	"crypto/rand"
	"errors"
	"math/big"

	id "github.com/sarvalabs/moichain/common/kramaid"

	"github.com/sarvalabs/moichain/cmd/common"
	common2 "github.com/sarvalabs/moichain/common"

	"github.com/sarvalabs/moichain/common/hexutil"

	"github.com/sarvalabs/moichain/common/tests"

	"github.com/spf13/cobra"
)

func GetGenesisCommand() *cobra.Command {
	genesisTestCmd := &cobra.Command{
		Use:   "genesis",
		Short: "Create genesis file with accounts and their context nodes.",
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
		"Path to genesis.json file.",
	)
	genesisTestCmd.Flags().StringVar(
		&GuardianLogicPath,
		"guardian-path",
		"artifact.json",
		"Path to guardian-logic file, i.e artifact.json file.",
	)
	genesisTestCmd.Flags().StringVar(
		&readAccountsFilePath,
		"accounts-path",
		"accounts.json",
		"Path to accounts.json file.",
	)
	genesisTestCmd.Flags().StringVar(
		&readInstancesFilePath,
		"instances-path",
		"instances.json",
		"Path to instances.json file.",
	)
	genesisTestCmd.Flags().StringSliceVar(
		&accAddresses,
		"address-list",
		[]string{},
		"List of account address.",
	)
	genesisTestCmd.Flags().Uint64Var(
		&premineAmount,
		"premine-amount",
		0,
		"Amount of MOI Fuel tokens that need to be credited to each account.",
	)
	genesisTestCmd.Flags().IntVar(
		&behaviouralNodesCount,
		"behavioural-count",
		common.DefaultBehaviouralCount,
		"Number of behavioural krama ids per account.",
	)
	genesisTestCmd.Flags().IntVar(
		&randomNodesCount,
		"random-count",
		common.DefaultRandomCount,
		"Number of random krama ids per account.",
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

	kramaIDs, err := common.ReadKramaIDsFromInstancesFile(readInstancesFilePath)
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
		accAddresses, err = tests.GetAddressFromAccountsFile(readAccountsFilePath)
		if err != nil {
			common.Err(err)
		}
	}

	accCount := len(accAddresses)

	g := &common2.GenesisFile{
		Accounts: make([]common2.AccountSetupArgs, 0, accCount),
	}

	guardianArtifact, err := common.ReadArtifactFile(GuardianLogicPath)
	if err != nil {
		common.Err(err)
	}

	g.AddLogic(
		common2.LogicSetupArgs{
			Name:               guardianArtifact.Name,
			Callsite:           guardianArtifact.Callsite,
			Calldata:           guardianArtifact.Calldata,
			Manifest:           guardianArtifact.Manifest,
			BehaviouralContext: getKramaIDs(behaviouralNodesCount),
			RandomContext:      getKramaIDs(randomNodesCount),
		})

	g.AddSargaAccount(common2.AccountSetupArgs{
		Address:            common2.SargaAddress,
		AccType:            common2.SargaAccount,
		MoiID:              common2.BytesToHex(getRandomMOIID()),
		BehaviouralContext: getKramaIDs(behaviouralNodesCount),
		RandomContext:      getKramaIDs(randomNodesCount),
	})

	assetInfo := common2.AssetAccountSetupArgs{
		AssetInfo: &common2.AssetCreationArgs{
			Symbol:      common2.KMOITokenSymbol,
			Dimension:   0,
			Standard:    0,
			IsLogical:   false,
			IsStateful:  false,
			Operator:    common2.NilAddress,
			Allocations: make([]common2.Allocation, 0, accCount),
		},
		BehaviouralContext: getKramaIDs(behaviouralNodesCount),
		RandomContext:      getKramaIDs(randomNodesCount),
	}

	for i := 0; i < accCount; i++ {
		g.AddAccount(
			common2.AccountSetupArgs{
				Address:            common2.HexToAddress(accAddresses[i]),
				AccType:            common2.RegularAccount,
				MoiID:              common2.BytesToHex(getRandomMOIID()),
				BehaviouralContext: getKramaIDs(behaviouralNodesCount),
				RandomContext:      getKramaIDs(randomNodesCount),
			},
		)

		if premineAmount > 0 {
			assetInfo.AssetInfo.Allocations = append(assetInfo.AssetInfo.Allocations, common2.Allocation{
				Address: common2.HexToAddress(accAddresses[i]),
				Amount:  (*hexutil.Big)(new(big.Int).SetUint64(premineAmount)),
			})
		}
	}

	g.AddAssetInfo(assetInfo)

	if err = common.WriteToGenesisFile(genesisFilePath, g); err != nil {
		common.Err(err)
	}
}
