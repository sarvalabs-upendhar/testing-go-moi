package genesis

import (
	"math"
	"strconv"
	"strings"

	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/moichain/cmd/common"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
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
		"Asset information. Format: <symbol:dimension:standard:isLogical:isMintable:owner>",
	)
	cmd.Flags().StringSliceVar(
		&allocations,
		"allocations",
		[]string{},
		"Balance allocation of addresses. Format: <address:balance,address:balance...>",
	)
	cmd.Flags().StringSliceVar(
		&behaviourNodes,
		"behaviour-nodes",
		[]string{},
		"List of krama ids. Format: <kramaID1,kramaID2,...>",
	)
	cmd.Flags().StringSliceVar(
		&randomNodes,
		"random-nodes",
		[]string{},
		"List of krama ids. Format: <kramaID1,kramaID2,...>",
	)

	if err := cobra.MarkFlagRequired(cmd.Flags(), "asset-info"); err != nil {
		common.Err(err)
	}

	if err := cobra.MarkFlagRequired(cmd.Flags(), "allocations"); err != nil {
		common.Err(err)
	}
}

func addAsset() {
	genesis, err := readGenesisFile()
	if err != nil {
		common.Err(err)
	}

	info, err := parseAssetInfoAndAllocations(assetInfo, allocations)
	if err != nil {
		common.Err(err)
	}

	if len(behaviourNodes) == 0 && len(randomNodes) == 0 {
		behaviourNodes, randomNodes = getContextNodes(instancesFilePath, DefaultBehaviouralCount, DefaultRandomCount)
	}

	genesis.AddAssetInfo(types.AssetAccountSetupArgs{
		AssetInfo:          info,
		BehaviouralContext: utils.KramaIDFromString(behaviourNodes),
		RandomContext:      utils.KramaIDFromString(randomNodes),
	})

	if err = WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		common.Err(err)
	}
}

// parseAssetInfo decodes an asset information string into an asset information struct using the delimiter `:`
func parseAssetInfoAndAllocations(assetInfo string, allocations []string) (*types.AssetCreationArgs, error) {
	params := strings.Split(assetInfo, ":")
	if len(params) != AssetInfoParamsNumber {
		return nil, errors.New("invalid asset info params")
	}

	symbol := strings.TrimSpace(params[0])
	if symbol == "" {
		return nil, types.ErrInvalidAssetInfoParams
	}

	dimension, err := strconv.ParseUint(strings.TrimSpace(params[1]), 10, 8)
	if err != nil || dimension > math.MaxUint8 {
		return nil, types.ErrInvalidAssetInfoParams
	}

	standard, err := strconv.ParseUint(strings.TrimSpace(params[2]), 10, 16)
	if err != nil || standard > math.MaxUint16 {
		return nil, types.ErrInvalidAssetInfoParams
	}

	isLogical, err := strconv.ParseBool(strings.TrimSpace(params[3]))
	if err != nil {
		return nil, types.ErrInvalidAssetInfoParams
	}

	isStateFul, err := strconv.ParseBool(strings.TrimSpace(params[4]))
	if err != nil {
		return nil, types.ErrInvalidAssetInfoParams
	}

	owner := strings.TrimSpace(params[5])
	if owner == "" {
		return nil, types.ErrInvalidAssetInfoParams
	}

	info := &types.AssetCreationArgs{
		Symbol:      symbol,
		Dimension:   hexutil.Uint8(dimension),
		Standard:    hexutil.Uint16(standard),
		IsLogical:   isLogical,
		IsStateful:  isStateFul,
		Owner:       types.HexToAddress(owner),
		Allocations: make([]types.Allocation, 0),
	}

	for _, alloc := range allocations {
		info.Allocations = append(info.Allocations, *parseAllocation(alloc))
	}

	return info, nil
}

// parseAllocation decodes allocation string into allocation struct using delimiter `:`
func parseAllocation(allocation string) *types.Allocation {
	if delimiterIdx := strings.Index(allocation, ":"); delimiterIdx != -1 {
		// <address>:<balance>
		valueRaw := allocation[delimiterIdx+1:]

		balance, err := parseUint256orHex(&valueRaw)
		if err != nil {
			common.Err(errors.Wrapf(err, "failed to parse amount"))
		}

		return &types.Allocation{
			Address: types.HexToAddress(allocation[:delimiterIdx]),
			Amount:  (*hexutil.Big)(balance),
		}
	}

	common.Err(errors.New("failed to parse allocation"))

	return nil
}
