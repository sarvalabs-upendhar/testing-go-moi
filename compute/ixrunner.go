package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

type IxRunner func(*common.Interaction, *common.ExecutionContext, *FuelTank, *state.Transition) *common.Receipt

func lookupIxRunner(kind common.IxType) IxRunner {
	return ixRunnerLookup[kind]
}

var ixRunnerLookup = map[common.IxType]IxRunner{
	common.IxValueTransfer: RunAssetTransfer,
	common.IxAssetCreate:   RunAssetCreate,
	common.IxAssetMint:     RunAssetMint,
	common.IxAssetBurn:     RunAssetBurn,
	common.IxLogicDeploy:   RunLogicDeploy,
	common.IxLogicInvoke:   RunLogicInvoke,
}
