package types

import "github.com/sarvalabs/moichain/types"

const GuardianSLot = 2

type Guardians map[string]Guardian

type Guardian struct {
	GuardianOperator string        `polo:"GuardianOperator"`
	KramaID          string        `polo:"KramaID"`
	DeviceID         string        `polo:"DeviceID"`
	PublicKey        []byte        `polo:"PublicKey"`
	IncentiveWallet  types.Address `polo:"IncentiveWallet"`
	ExtraData        []byte        `polo:"ExtraData"`
}
