package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

type TxRunner func(
	*common.IxOp, *common.ExecutionContext,
	*FuelTank, *state.Transition,
) *common.IxOpResult

func lookupTxRunner(kind common.IxOpType) TxRunner {
	return txRunnerLookup[kind]
}

var txRunnerLookup = map[common.IxOpType]TxRunner{
	common.IxParticipantCreate: RunParticipantCreate,
	common.IxAssetTransfer:     RunAssetTransfer,
	common.IxAssetCreate:       RunAssetCreate,
	common.IxAssetMint:         RunAssetMint,
	common.IxAssetBurn:         RunAssetBurn,
	common.IxLogicDeploy:       RunLogicDeploy,
	common.IxLogicInvoke:       RunLogicInvoke,
	common.IxLogicEnlist:       RunLogicEnlist,
}
