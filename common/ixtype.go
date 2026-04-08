package common

import "fmt"

type IxOpType int

const (
	IxInvalid IxOpType = iota
	IxParticipantCreate
	IxAccountConfigure
	IxAccountInherit

	IxAssetCreate
	IxAssetAction

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
	IxAccountConfigure:  "IxAccountConfigure",
	IxAccountInherit:    "IxAccountInherit",
	IxAssetCreate:       "IxAssetCreate",
	IxAssetAction:       "IxAssetAction",
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
