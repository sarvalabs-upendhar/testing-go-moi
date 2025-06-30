package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

type OpRunner func(
	*common.IxOp, *common.ExecutionContext,
	*FuelTank, *state.Transition,
) *common.IxOpResult

func lookupOpRunner(kind common.IxOpType) OpRunner {
	return opRunnerLookup[kind]
}

var opRunnerLookup = map[common.IxOpType]OpRunner{
	common.IxParticipantCreate: RunParticipantCreate,
	common.IXAccountConfigure:  RunAccountConfigure,
	common.IXAccountInherit:    RunAccountInherit,
	common.IxAssetTransfer:     RunAssetTransfer,
	common.IxAssetCreate:       RunAssetCreate,
	common.IxAssetApprove:      RunAssetApprove,
	common.IxAssetRevoke:       RunAssetRevoke,
	common.IxAssetMint:         RunAssetMint,
	common.IxAssetBurn:         RunAssetBurn,
	common.IxAssetLockup:       RunAssetLockup,
	common.IxAssetRelease:      RunAssetRelease,
	common.IxGuardianRegister:  RunGuardianRegister,
	common.IxGuardianStake:     RunGuardianStake,
	common.IxGuardianUnstake:   RunGuardianUnstake,
	common.IxGuardianWithdraw:  RunGuardianWithdraw,
	common.IxGuardianClaim:     RunGuardianClaim,
	common.IxLogicDeploy:       RunLogicDeploy,
	common.IxLogicInvoke:       RunLogicInvoke,
	common.IxLogicEnlist:       RunLogicEnlist,
}
