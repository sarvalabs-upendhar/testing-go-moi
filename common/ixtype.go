package common

import "fmt"

type IxOpType int

const (
	IxInvalid IxOpType = iota
	IxParticipantCreate
	IxAssetTransfer
	TxFuelSupply // TODO: Remove this
	IxAssetCreate
	IxAssetApprove
	IxAssetRevoke
	IxAssetMint
	IxAssetBurn

	IxLogicDeploy
	IxLogicInvoke
	IxLogicEnlist
	TxLogicInteract
	TxLogicUpgrade
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
