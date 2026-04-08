package genesis

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
)

const (
	AssetInfoParamsNumber = 6
)

func GetPremineCommand() *cobra.Command {
	premineCmd := &cobra.Command{
		Use:   "premine",
		Short: "Allocates given asset info to given addresses with respective balances.",
		Run: func(cmd *cobra.Command, args []string) {
			addAsset()
		},
	}

	parsePremineFlags(premineCmd)

	return premineCmd
}

func parsePremineFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&assetInfo,
		"asset-info",
		"",
		"Asset information. Format: <symbol:dimension:standard:decimals:maxsupply:manager>",
	)
	cmd.Flags().StringSliceVar(
		&allocations,
		"allocations",
		[]string{},
		"Balance allocation of addresses. Format: <address:balance,address:balance...>",
	)
	cmd.Flags().StringSliceVar(
		&consensusNodes,
		"consensus-nodes",
		[]string{},
		"List of krama ids. Format: <kramaID1,kramaID2,...>",
	)

	if err := cobra.MarkFlagRequired(cmd.Flags(), "asset-info"); err != nil {
		cmdcommon.Err(err)
	}

	if err := cobra.MarkFlagRequired(cmd.Flags(), "allocations"); err != nil {
		cmdcommon.Err(err)
	}
}

func addAsset() {
	genesis, err := common.ReadGenesisFile(genesisFilePath)
	if err != nil {
		cmdcommon.Err(err)
	}

	info, err := parseAssetInfoAndAllocations(assetInfo, allocations)
	if err != nil {
		cmdcommon.Err(err)
	}

	if len(consensusNodes) == 0 {
		consensusNodes = getContextNodes(
			instancesFilePath,
			common.ConsensusNodesSize,
		)
	}

	genesis.AddAssetInfo(common.AssetAccountSetupArgs{
		AssetInfo:      info,
		ConsensusNodes: utils.KramaIDFromString(consensusNodes),
	})

	if err = cmdcommon.WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		cmdcommon.Err(err)
	}
}

// parseAssetInfo decodes an asset information string into an asset information struct using the delimiter `:`
func parseAssetInfoAndAllocations(assetInfo string, allocations []string) (*common.AssetCreationArgs, error) {
	params := strings.Split(assetInfo, ":")
	if len(params) != AssetInfoParamsNumber {
		return nil, common.ErrInvalidAssetInfoParams
	}
	// <symbol:dimension:standard:decimals:maxsupply:manager>

	symbol := strings.TrimSpace(params[0])
	if symbol == "" {
		return nil, common.ErrInvalidAssetInfoParams
	}

	dimension, err := strconv.ParseUint(strings.TrimSpace(params[1]), 10, 8)
	if err != nil || dimension > math.MaxUint8 {
		return nil, common.ErrInvalidAssetInfoParams
	}

	standard, err := strconv.ParseUint(strings.TrimSpace(params[2]), 10, 16)
	if err != nil || standard > math.MaxUint16 {
		return nil, common.ErrInvalidAssetInfoParams
	}

	decimals, err := strconv.ParseUint(strings.TrimSpace(params[3]), 10, 8)
	if err != nil || decimals > math.MaxUint8 {
		return nil, common.ErrInvalidAssetInfoParams
	}

	maxSupply, success := new(big.Int).SetString(strings.TrimSpace(params[4]), 10)
	if !success {
		return nil, common.ErrInvalidAssetInfoParams
	}

	operator := strings.TrimSpace(params[5])
	if operator == "" {
		return nil, common.ErrInvalidAssetInfoParams
	}

	operatorID, _ := identifiers.NewParticipantIDFromHex(operator)

	info := &common.AssetCreationArgs{
		Symbol:      symbol,
		Dimension:   hexutil.Uint8(dimension),
		Standard:    hexutil.Uint16(standard),
		Decimals:    hexutil.Uint8(decimals),
		MaxSupply:   (hexutil.Big)(*maxSupply),
		Creator:     operatorID.AsIdentifier(),
		Manager:     operatorID.AsIdentifier(),
		Allocations: make([]common.Allocation, 0),
	}

	for _, alloc := range allocations {
		info.Allocations = append(info.Allocations, *parseAllocation(alloc))
	}

	return info, nil
}

// parseAllocation decodes allocation string into allocation struct using delimiter `:`
func parseAllocation(allocation string) *common.Allocation {
	if delimiterIdx := strings.Index(allocation, ":"); delimiterIdx != -1 {
		// <participantID>:<balance>
		valueRaw := allocation[delimiterIdx+1:]

		balance, err := parseUint256orHex(&valueRaw)
		if err != nil {
			cmdcommon.Err(errors.Wrapf(err, "failed to parse amount"))
		}

		id, _ := identifiers.NewParticipantIDFromHex(allocation[:delimiterIdx])

		return &common.Allocation{
			ID:     id.AsIdentifier(),
			Amount: (*hexutil.Big)(balance),
		}
	}

	cmdcommon.Err(errors.New("failed to parse allocation"))

	return nil
}
