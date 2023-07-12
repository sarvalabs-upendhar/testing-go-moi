package genesis

import (
	"math"
	"strconv"
	"strings"

	"github.com/sarvalabs/moichain/cmd/common"
	common2 "github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/utils"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/common/hexutil"
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
		"Asset information. Format: <symbol:dimension:standard:isLogical:isMintable:operator>",
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
		behaviourNodes, randomNodes = getContextNodes(
			instancesFilePath,
			common.DefaultBehaviouralCount,
			common.DefaultRandomCount,
		)
	}

	genesis.AddAssetInfo(common2.AssetAccountSetupArgs{
		AssetInfo:          info,
		BehaviouralContext: utils.KramaIDFromString(behaviourNodes),
		RandomContext:      utils.KramaIDFromString(randomNodes),
	})

	if err = common.WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		common.Err(err)
	}
}

// parseAssetInfo decodes an asset information string into an asset information struct using the delimiter `:`
func parseAssetInfoAndAllocations(assetInfo string, allocations []string) (*common2.AssetCreationArgs, error) {
	params := strings.Split(assetInfo, ":")
	if len(params) != AssetInfoParamsNumber {
		return nil, errors.New("invalid asset info params")
	}

	symbol := strings.TrimSpace(params[0])
	if symbol == "" {
		return nil, common2.ErrInvalidAssetInfoParams
	}

	dimension, err := strconv.ParseUint(strings.TrimSpace(params[1]), 10, 8)
	if err != nil || dimension > math.MaxUint8 {
		return nil, common2.ErrInvalidAssetInfoParams
	}

	standard, err := strconv.ParseUint(strings.TrimSpace(params[2]), 10, 16)
	if err != nil || standard > math.MaxUint16 {
		return nil, common2.ErrInvalidAssetInfoParams
	}

	isLogical, err := strconv.ParseBool(strings.TrimSpace(params[3]))
	if err != nil {
		return nil, common2.ErrInvalidAssetInfoParams
	}

	isStateFul, err := strconv.ParseBool(strings.TrimSpace(params[4]))
	if err != nil {
		return nil, common2.ErrInvalidAssetInfoParams
	}

	operator := strings.TrimSpace(params[5])
	if operator == "" {
		return nil, common2.ErrInvalidAssetInfoParams
	}

	info := &common2.AssetCreationArgs{
		Symbol:      symbol,
		Dimension:   hexutil.Uint8(dimension),
		Standard:    hexutil.Uint16(standard),
		IsLogical:   isLogical,
		IsStateful:  isStateFul,
		Operator:    common2.HexToAddress(operator),
		Allocations: make([]common2.Allocation, 0),
	}

	for _, alloc := range allocations {
		info.Allocations = append(info.Allocations, *parseAllocation(alloc))
	}

	return info, nil
}

// parseAllocation decodes allocation string into allocation struct using delimiter `:`
func parseAllocation(allocation string) *common2.Allocation {
	if delimiterIdx := strings.Index(allocation, ":"); delimiterIdx != -1 {
		// <address>:<balance>
		valueRaw := allocation[delimiterIdx+1:]

		balance, err := parseUint256orHex(&valueRaw)
		if err != nil {
			common.Err(errors.Wrapf(err, "failed to parse amount"))
		}

		return &common2.Allocation{
			Address: common2.HexToAddress(allocation[:delimiterIdx]),
			Amount:  (*hexutil.Big)(balance),
		}
	}

	common.Err(errors.New("failed to parse allocation"))

	return nil
}
