package common

import "fmt"

type IxOpType int

const (
	IxInvalid IxOpType = iota
	IxParticipantCreate
	IxAssetTransfer
	IxFuelSupply // TODO: Remove this
	IxAssetCreate
	IxAssetApprove
	IxAssetRevoke
	IxAssetMint
	IxAssetBurn
	IxAssetLockup
	IxAssetRelease

	IxLogicDeploy
	IxLogicInvoke
	IxLogicEnlist
	IxLogicInteract
	IxLogicUpgrade
)

var txTypeToString = map[IxOpType]string{
	IxInvalid:           "IxInvalid",
	IxParticipantCreate: "IxParticipantCreate",
	IxAssetTransfer:     "IxAssetTransfer",
	IxAssetCreate:       "IxAssetCreate",
	IxAssetApprove:      "IxAssetApprove",
	IxAssetRevoke:       "IxAssetRevoke",
	IxAssetMint:         "IxAssetMint",
	IxAssetBurn:         "IxAssetBurn",
	IxAssetLockup:       "IxAssetLockup",
	IxAssetRelease:      "IxAssetRelease",
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
