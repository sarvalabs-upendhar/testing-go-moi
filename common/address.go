package common

import (
	"encoding/binary"

	"github.com/sarvalabs/go-moi-identifiers"
)

const KMOITokenSymbol = "KMOI"

var (
	GenesisIxHash       = GetHash([]byte("Genesis Interaction"))
	StakingContractAddr = CreateAddressFromString("staking-contract")
	GenesisLogicAddrs   = []identifiers.Address{StakingContractAddr, GuardianLogicAddr}
)

var (
	SargaAddress = CreateAddressFromString("sargaAccount")
	SargaLogicID = identifiers.NewLogicIDv0(true, false, false, false, 0, SargaAddress)
)

var (
	GuardianLogicAddr = CreateAddressFromString("guardian-registry")
	GuardianLogicID   = identifiers.NewLogicIDv0(true, false, false, false, 0, GuardianLogicAddr)
)

var (
	KMOITokenAddress = CreateAddressFromString(KMOITokenSymbol)
	KMOITokenAssetID = identifiers.NewAssetIDv0(false, false, 0, 0, KMOITokenAddress)
)

func ContainsAddress(addresses []identifiers.Address, target identifiers.Address) bool {
	for _, addr := range addresses {
		if addr == target {
			return true
		}
	}

	return false
}

func CreateAddressFromString(name string) identifiers.Address {
	hash := GetHash([]byte(name)).Bytes()

	return identifiers.NewAddressFromBytes(hash)
}

func NewAccountAddress(nonce uint64, address identifiers.Address) identifiers.Address {
	rawBytes := make([]byte, 40)
	binary.BigEndian.PutUint64(rawBytes, nonce)
	copy(rawBytes[8:], address.Bytes())

	hash := GetHash(rawBytes).Bytes()

	return identifiers.NewAddressFromBytes(hash)
}
