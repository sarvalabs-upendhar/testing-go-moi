package kbft

import (
	"bytes"

	"github.com/sarvalabs/go-moi-identifiers"
)

type ValidatorSet struct {
	Validators       []*Validator `json:"validators"`
	Operator         *Validator   `json:"generator"`
	ContextIndices   []int32
	observers        []string
	totalVotingPower []int64
}

func NewValidatorSet(v []*Validator, operatorIndex int32, vp []int64, i []int32, observers []string) *ValidatorSet {
	vs := new(ValidatorSet)
	vs.Validators = v
	vs.observers = observers
	vs.ContextIndices = i
	vs.Operator = v[operatorIndex]
	vs.totalVotingPower = vp

	return vs
}

func (valSet *ValidatorSet) HasAddress(id identifiers.Identifier) bool {
	for _, val := range valSet.Validators {
		if bytes.Equal(val.ID.Bytes(), id.Bytes()) {
			return true
		}
	}

	return false
}

func (valSet *ValidatorSet) GetByIndex(index int32) (id identifiers.Identifier, val *Validator) {
	if index < 0 || int(index) >= len(valSet.Validators) {
		return identifiers.Identifier{}, nil
	}

	val = valSet.Validators[index]

	return val.ID, val
}

func (valSet *ValidatorSet) GetByAddress(id identifiers.Identifier) (index int32, val *Validator) {
	for idx, val := range valSet.Validators {
		if bytes.Equal(val.ID.Bytes(), id.Bytes()) {
			return int32(idx), val.Copy()
		}
	}

	return -1, nil
}
