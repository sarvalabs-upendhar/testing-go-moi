package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state"
)

type OpRunner func(
	*common.IxOp, *engineio.RuntimeContext,
	*FuelTank, *state.Transition,
) *common.IxOpResult

func lookupOpRunner(kind common.IxOpType) OpRunner {
	return opRunnerLookup[kind]
}

var opRunnerLookup = map[common.IxOpType]OpRunner{
	common.IxParticipantCreate: RunParticipantCreate,
	common.IxAccountConfigure:  RunAccountConfigure,
	common.IxAccountInherit:    RunAccountInherit,
	common.IxAssetCreate:       RunAssetCreate,
	common.IxAssetAction:       RunLogicInvoke,

	// common.IxGuardianRegister: RunGuardianRegister,
	// common.IxGuardianStake:    RunGuardianStake,
	// common.IxGuardianUnstake:  RunGuardianUnstake,
	// common.IxGuardianWithdraw: RunGuardianWithdraw,
	// common.IxGuardianClaim:    RunGuardianClaim,
	common.IxLogicDeploy: RunLogicDeploy,
	common.IxLogicInvoke: RunLogicInvoke,
	// common.IxLogicEnlist:       RunLogicEnlist,
}
