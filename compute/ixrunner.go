package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

type IxRunner func(*common.Interaction, *engineio.FuelTank, state.ObjectMap) (*common.Receipt, error)

func LookupIxRunner(kind common.IxType) (IxRunner, bool) {
	runner, ok := ixRunnerLookup[kind]
	return runner, ok //nolint:nlreturn
}

var ixRunnerLookup = map[common.IxType]IxRunner{
	common.IxValueTransfer: RunAssetTransfer,
	common.IxAssetCreate:   RunAssetCreate,
	common.IxAssetMint:     RunAssetMint,
	common.IxAssetBurn:     RunAssetBurn,
	common.IxLogicDeploy:   RunLogicDeploy,
	common.IxLogicInvoke:   RunLogicInvoke,
}
