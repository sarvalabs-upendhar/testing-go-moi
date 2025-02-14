package kbft

import (
	"crypto"

	"github.com/sarvalabs/go-moi/common/identifiers"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
)

type Validator struct {
	ID          identifiers.Identifier `json:"id"`
	KipID       kramaid.KramaID
	PubKey      crypto.PublicKey `json:"pub_key"`
	VotingPower int32            `json:"voting_power"`
}

func (v *Validator) Copy() *Validator {
	vCopy := *v

	return &vCopy
}
