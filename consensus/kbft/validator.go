package kbft

import (
	"crypto"
	"crypto/ed25519"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
)

type Validator struct {
	Address     identifiers.Address `json:"address"`
	KipID       kramaid.KramaID
	PubKey      crypto.PublicKey `json:"pub_key"`
	VotingPower int32            `json:"voting_power"`
}

func (v *Validator) Copy() *Validator {
	vCopy := *v

	return &vCopy
}

// PublicKey is an interface that represents a cryptographic public key
type PublicKey interface {
	Address() identifiers.Address
}

// MOIPublicKey is a struct that represents a wrapper around a cryptographic public key.
// Implements the the PublicKey interface.
type MOIPublicKey struct {
	// Represents the wrapped key
	PubKey crypto.PublicKey
}

// Address is a method of KIPPublicKey that returns the Address from the public key
func (mpk *MOIPublicKey) Address() identifiers.Address {
	if pubKey, ok := mpk.PubKey.(ed25519.PublicKey); ok {
		return identifiers.NewAddressFromBytes(pubKey)
	}

	return identifiers.NilAddress
}
