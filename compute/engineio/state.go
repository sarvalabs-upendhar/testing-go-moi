package engineio

import (
	"encoding/hex"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

// StateDriver represents an interface for accessing and manipulating state information of an account.
// It is bounded to a particular account and can only mutate within applicable portions
// of the state within the bounds of the logic's namespace
type StateDriver interface {
	Address() identifiers.Address
	LogicID() identifiers.LogicID

	GetStorageEntry([]byte) ([]byte, bool)
	SetStorageEntry([]byte, []byte) bool
}

func NewDebugStateDriver(t *testing.T, address identifiers.Address, logicID identifiers.LogicID) StateDriver {
	t.Helper()

	return &DebugStateDriver{
		address:    address,
		logicID:    logicID,
		logicstate: make(map[string][]byte),
	}
}

type DebugStateDriver struct {
	address identifiers.Address
	logicID identifiers.LogicID

	logicstate map[string][]byte
}

func (state DebugStateDriver) Address() identifiers.Address  { return state.address }
func (state DebugStateDriver) LogicID() identifiers.LogicID  { return state.logicID }
func (state DebugStateDriver) LogicState() map[string][]byte { return state.logicstate }

func (state DebugStateDriver) GetStorageEntry(key []byte) ([]byte, bool) {
	val, ok := state.logicstate[hex.EncodeToString(key)]

	return val, ok
}

func (state DebugStateDriver) SetStorageEntry(key, val []byte) bool {
	state.logicstate[hex.EncodeToString(key)] = val

	return true
}

func (state DebugStateDriver) Copy() StateDriver {
	clone := &DebugStateDriver{
		address:    state.address,
		logicID:    state.logicID,
		logicstate: make(map[string][]byte),
	}

	for key, val := range state.logicstate {
		clone.logicstate[key] = val
	}

	return clone
}
