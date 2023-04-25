package types

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	AddressLength = 32
)

var (
	NilAddress      Address
	SargaLogicID, _ = NewLogicIDv0(true, false, false,
		false, 0, SargaAddress)
	GenesisIxHash = GetHash([]byte("Genesis Interaction"))
)

var (
	SargaAddress        = CreateAddressFromString("sargaAccount")
	StakingContractAddr = CreateAddressFromString("staking-contract")
	GenesisLogicAddrs   = []Address{StakingContractAddr}
)

// Address represents the 32 byte address of an MOI account.
type Address [AddressLength]byte

func (a Address) IsNil() bool {
	return a == NilAddress
}

func (a Address) String() string {
	if a == NilAddress {
		return ""
	}

	return a.Hex()
}
func (a Address) Bytes() []byte { return a[:] }

// SetBytes sets the address to the value of b.
func (a *Address) SetBytes(b []byte) {
	if len(b) > len(a) {
		b = b[len(b)-AddressLength:]
	}

	copy(a[AddressLength-len(b):], b)
}

// MarshalText implements the custom json marshaller
func (a Address) MarshalText() ([]byte, error) {
	result := make([]byte, len(a)*2+2)
	copy(result[:2], "0x")
	hex.Encode(result[2:], a.Bytes())

	return result, nil
}

// UnmarshalText sets the address to the value of text.
func (a *Address) UnmarshalText(text []byte) error {
	if !(len(text) >= 2 && text[0] == byte('0') && (text[1] == byte('X') || text[1] == byte('x'))) {
		return ErrInvalidAddress
	}

	text = text[2:]

	if len(text) != AddressLength*2 {
		return fmt.Errorf("invalid address length: %d", len(text)/2)
	}

	_, err := hex.Decode(a[:], text)

	return err
}

// Hex return the Hex representation of the Address
func (a Address) Hex() string {
	return "0x" + hex.EncodeToString(a[:])
}

// BytesToAddress returns the address from b
func BytesToAddress(b []byte) Address {
	var a Address

	a.SetBytes(b)

	return a
}

// HexToAddress converts string to Address
func HexToAddress(s string) Address {
	return BytesToAddress(FromHex(s))
}

func Contains(addresses []Address, target Address) bool {
	for _, addr := range addresses {
		if addr == target {
			return true
		}
	}

	return false
}

func CreateAddressFromString(name string) Address {
	return BytesToAddress(GetHash([]byte(name)).Bytes())
}

func NewAccountAddress(nonce uint64, address Address) Address {
	rawBytes := make([]byte, 40)
	binary.BigEndian.PutUint64(rawBytes, nonce)
	copy(rawBytes[8:], address.Bytes())

	GetHash(rawBytes)

	return BytesToAddress(GetHash(rawBytes).Bytes())
}
