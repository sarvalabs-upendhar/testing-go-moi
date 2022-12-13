package ixpool

import "github.com/sarvalabs/moichain/types"

type MockStateManager struct {
	acc map[types.Address]uint64
}

func (mc *MockStateManager) GetLatestNonce(addr types.Address) (uint64, error) {
	return mc.acc[addr], nil
}

func (mc *MockStateManager) IsAccountRegistered(addr types.Address) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (mc *MockStateManager) IsLogicRegistered(logicID types.LogicID) error {
	// TODO implement me
	panic("implement me")
}
