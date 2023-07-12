package kbft

import (
	"crypto"
	"crypto/ed25519"

	id "github.com/sarvalabs/moichain/common/kramaid"

	"github.com/sarvalabs/moichain/common"
)

type Validator struct {
	Address     common.Address `json:"address"`
	KipID       id.KramaID
	PubKey      crypto.PublicKey `json:"pub_key"`
	VotingPower int32            `json:"voting_power"`
}

func (v *Validator) Copy() *Validator {
	vCopy := *v

	return &vCopy
}

// PublicKey is an interface that represents a cryptographic public key
type PublicKey interface {
	Address() common.Address
}

// MOIPublicKey is a struct that represents a wrapper around a cryptographic public key.
// Implements the the PublicKey interface.
type MOIPublicKey struct {
	// Represents the wrapped key
	PubKey crypto.PublicKey
}

// Address is a method of KIPPublicKey that returns the Address from the public key
func (mpk *MOIPublicKey) Address() common.Address {
	if pubKey, ok := mpk.PubKey.(ed25519.PublicKey); ok {
		return common.BytesToAddress(pubKey)
	}

	return common.NilAddress
}
