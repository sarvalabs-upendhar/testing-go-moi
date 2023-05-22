package genesis

import (
	"math"
	"strconv"
	"strings"

	"github.com/sarvalabs/moichain/cmd/common"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

const (
	AssetInfoParamsNumber = 6
)

func GetPremineCommand() *cobra.Command {
	premineCmd := &cobra.Command{
		Use:   "premine",
		Short: "allocates given asset info to given addresses with respective balances",
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
		"format: <symbol:dimension:standard:isLogical:isMintable:owner>",
	)
	cmd.Flags().StringSliceVar(
		&allocations,
		"allocations",
		[]string{},
		"format: <address:balance,address:balance...>",
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

	assetInfo, err := parseAssetInfoAndAllocations(assetInfo, allocations)
	if err != nil {
		common.Err(err)
	}

	genesis.AddAssetInfo(*assetInfo)

	if err = WriteToGenesisFile(genesisFilePath, genesis); err != nil {
		common.Err(err)
	}
}

// parseAssetInfo decodes an asset information string into an asset information struct using the delimiter `:`
func parseAssetInfoAndAllocations(assetInfo string, allocations []string) (*lattice.AssetInfoV1, error) {
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

	isMintable, err := strconv.ParseBool(strings.TrimSpace(params[4]))
	if err != nil {
		return nil, types.ErrInvalidAssetInfoParams
	}

	owner := strings.TrimSpace(params[5])
	if owner == "" {
		return nil, types.ErrInvalidAssetInfoParams
	}

	info := &lattice.AssetInfoV1{
		Symbol:      symbol,
		Dimension:   uint8(dimension),
		Standard:    uint16(standard),
		IsLogical:   isLogical,
		IsMintable:  isMintable,
		Owner:       owner,
		Allocations: make([]lattice.AllocationV1, 0),
	}

	for _, alloc := range allocations {
		info.Allocations = append(info.Allocations, *parseAllocation(alloc))
	}

	return info, nil
}

// parseAllocation decodes allocation string into allocation struct using delimiter `:`
func parseAllocation(allocation string) *lattice.AllocationV1 {
	if delimiterIdx := strings.Index(allocation, ":"); delimiterIdx != -1 {
		// <address>:<balance>
		valueRaw := allocation[delimiterIdx+1:]

		balance, err := parseUint256orHex(&valueRaw)
		if err != nil {
			common.Err(errors.Wrapf(err, "failed to parse amount"))
		}

		return &lattice.AllocationV1{
			Address: allocation[:delimiterIdx],
			Balance: balance.Uint64(),
		}
	}

	common.Err(errors.New("failed to parse allocation"))

	return nil
}
