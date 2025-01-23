package internal

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/spf13/cobra"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
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
	genesisTestCmd.Flags().Uint64Var(
		&premineAmount,
		"premine-amount",
		0,
		"Amount of MOI Fuel tokens that need to be credited to each account.",
	)
	genesisTestCmd.Flags().IntVar(
		&consensusNodesCount,
		"consensus-count",
		common.ConsensusNodesSize,
		"Number of consensus krama ids per account.",
	)
}

func getRandomMOIID() []byte {
	randomMoiID := make([]byte, 32)

	_, err := rand.Read(randomMoiID)
	if err != nil {
		cmdcommon.Err(err)
	}

	return randomMoiID
}

func createTestGenesisFile() {
	var kidTracker int

	kramaIDs, err := common.ReadKramaIDsFromInstancesFile(readInstancesFilePath)
	if err != nil {
		cmdcommon.Err(err)
	}

	totalIDs := len(kramaIDs)

	if consensusNodesCount > totalIDs {
		cmdcommon.Err(errors.New("insufficient krama IDs in instances file"))
	}

	// fetch krama ids in round robin manner
	getKramaIDs := func(count int) []kramaid.KramaID {
		ids := make([]kramaid.KramaID, 0, count)

		for i := 0; i < count; i++ {
			kid := kramaIDs[kidTracker]
			kidTracker = (kidTracker + 1) % totalIDs

			ids = append(ids, kramaid.KramaID(kid))
		}

		return ids
	}

	accounts, err := tests.GetAccountsFromFile(readAccountsFilePath)
	if err != nil {
		cmdcommon.Err(err)
	}

	accCount := len(accounts)

	g := &common.GenesisFile{
		Accounts: make([]common.AccountSetupArgs, 0, accCount),
	}

	guardianArtifact, err := cmdcommon.ReadArtifactFile(GuardianLogicPath)
	if err != nil {
		cmdcommon.Err(err)
	}

	g.AddLogic(
		common.LogicSetupArgs{
			Name:           guardianArtifact.Name,
			Callsite:       guardianArtifact.Callsite,
			Calldata:       guardianArtifact.Calldata,
			Manifest:       guardianArtifact.Manifest,
			ConsensusNodes: getKramaIDs(consensusNodesCount),
		})

	g.AddSargaAccount(common.AccountSetupArgs{
		ID:             common.SargaAccountID,
		AccType:        common.SargaAccount,
		MoiID:          common.BytesToHex(getRandomMOIID()),
		ConsensusNodes: getKramaIDs(consensusNodesCount),
	})

	assetInfo := common.AssetAccountSetupArgs{
		AssetInfo: &common.AssetCreationArgs{
			Symbol:      common.KMOITokenSymbol,
			Dimension:   0,
			Standard:    0,
			IsLogical:   false,
			IsStateful:  false,
			Operator:    identifiers.Nil,
			Allocations: make([]common.Allocation, 0, accCount),
		},
		ConsensusNodes: getKramaIDs(consensusNodesCount),
	}

	for i := 0; i < accCount; i++ {
		participantID, err := identifiers.NewParticipantIDFromHex(accounts[i].ID.String())
		if err != nil {
			panic(err)
		}

		g.AddAccount(
			common.AccountSetupArgs{
				ID: participantID.AsIdentifier(),
				Keys: []common.KeyArgs{
					{
						PublicKey:          accounts[i].PublicKey,
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				AccType:        common.RegularAccount,
				MoiID:          common.BytesToHex(getRandomMOIID()),
				ConsensusNodes: getKramaIDs(consensusNodesCount),
			},
		)

		if premineAmount > 0 {
			assetInfo.AssetInfo.Allocations = append(assetInfo.AssetInfo.Allocations, common.Allocation{
				ID:     participantID.AsIdentifier(),
				Amount: (*hexutil.Big)(new(big.Int).SetUint64(premineAmount)),
			})
		}
	}

	g.AddAssetInfo(assetInfo)

	if err = cmdcommon.WriteToGenesisFile(genesisFilePath, g); err != nil {
		cmdcommon.Err(err)
	}
}
