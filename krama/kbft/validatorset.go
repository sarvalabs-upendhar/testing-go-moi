package kbft

import (
	"bytes"

	"gitlab.com/sarvalabs/moichain/types"
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

func (valSet *ValidatorSet) HasAddress(addrs types.Address) bool {
	for _, val := range valSet.Validators {
		if bytes.Equal(val.Address.Bytes(), addrs.Bytes()) {
			return true
		}
	}

	return false
}

func (valSet *ValidatorSet) GetByIndex(index int32) (addrs types.Address, val *Validator) {
	if index < 0 || int(index) >= len(valSet.Validators) {
		return types.Address{}, nil
	}

	val = valSet.Validators[index]

	return val.Address, val
}

func (valSet *ValidatorSet) GetByAddress(add types.Address) (index int32, val *Validator) {
	for idx, val := range valSet.Validators {
		if bytes.Equal(val.Address.Bytes(), add.Bytes()) {
			return int32(idx), val.Copy()
		}
	}

	return -1, nil
}
