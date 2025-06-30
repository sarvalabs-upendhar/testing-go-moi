package common

import "fmt"

type IxOpType int

const (
	IxInvalid IxOpType = iota
	IxParticipantCreate
	IXAccountConfigure
	IXAccountInherit
	IxAssetTransfer
	IxFuelSupply // TODO: Remove this
	IxAssetCreate
	IxAssetApprove
	IxAssetRevoke
	IxAssetMint
	IxAssetBurn
	IxAssetLockup
	IxAssetRelease

	IxGuardianRegister
	IxGuardianStake
	IxGuardianUnstake
	IxGuardianWithdraw
	IxGuardianClaim

	IxLogicDeploy
	IxLogicInvoke
	IxLogicEnlist
	IxLogicInteract
	IxLogicUpgrade
)

var txTypeToString = map[IxOpType]string{
	IxInvalid:           "IxInvalid",
	IxParticipantCreate: "IxParticipantCreate",
	IXAccountConfigure:  "IxAccountConfigure",
	IXAccountInherit:    "IxAccountInherit",
	IxAssetTransfer:     "IxAssetTransfer",
	IxAssetCreate:       "IxAssetCreate",
	IxAssetApprove:      "IxAssetApprove",
	IxAssetRevoke:       "IxAssetRevoke",
	IxAssetMint:         "IxAssetMint",
	IxAssetBurn:         "IxAssetBurn",
	IxAssetLockup:       "IxAssetLockup",
	IxAssetRelease:      "IxAssetRelease",
	IxGuardianRegister:  "IxGuardianRegister",
	IxGuardianStake:     "IxGuardianStake",
	IxGuardianUnstake:   "IxGuardianUnstake",
	IxGuardianWithdraw:  "IxGuardianWithdraw",
	IxGuardianClaim:     "IxGuardianClaim",
	IxLogicDeploy:       "IxLogicDeploy",
	IxLogicInvoke:       "IxLogicInvoke",
	IxLogicEnlist:       "IxLogicEnlist",
}

func (ixType IxOpType) String() string {
	str, ok := txTypeToString[ixType]
	if !ok {
		return fmt.Sprintf("unknown ixn: %d", ixType)
	}

	return str
}

func (ixType IxOpType) TxnID() int {
	return int(ixType)
}
