package common

import "fmt"

type IxType int

const (
	IxInvalid IxType = iota
	IxValueTransfer
	IxFuelSupply

	IxAssetCreate
	IxAssetApprove
	IxAssetRevoke
	IxAssetMint
	IxAssetBurn

	IxLogicDeploy
	IxLogicInvoke
	IxLogicEnlist
	IxLogicInteract
	IxLogicUpgrade
)

var ixTypeToString = map[IxType]string{
	IxInvalid:       "IxInvalid",
	IxValueTransfer: "IxValueTransfer",
	IxFuelSupply:    "IxFuelSupply",
	IxAssetCreate:   "IxAssetCreate",
	IxAssetApprove:  "IxAssetApprove",
	IxAssetRevoke:   "IxAssetRevoke",
	IxAssetMint:     "IxAssetMint",
	IxAssetBurn:     "IxAssetBurn",
	IxLogicDeploy:   "IxLogicDeploy",
	IxLogicInvoke:   "IxLogicInvoke",
}

func (ixtype IxType) String() string {
	str, ok := ixTypeToString[ixtype]
	if !ok {
		return fmt.Sprintf("unknown ixn: %d", ixtype)
	}

	return str
}

func (ixtype IxType) IxnID() int {
	return int(ixtype)
}
