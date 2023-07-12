package internal

import (
	"encoding/hex"

	"github.com/sarvalabs/go-moi/common"
)

// StateObject is a state object implementation that
// contains the entire context state of a participant
type StateObject struct {
	Address common.Address

	Datastore  map[string][]byte
	LogicState map[string]map[string][]byte

	Spendable common.AssetMap
	Approvals map[common.Address]common.AssetMap
	Lockups   map[common.Address]common.AssetMap
}

// NewStateObject generate a new StateObject for a given types.Address.
// All balances and key-value stores are initialized with empty values.
func NewStateObject(addr common.Address) *StateObject {
	return &StateObject{
		Address: addr,

		Datastore:  make(map[string][]byte),
		LogicState: make(map[string]map[string][]byte),

		Spendable: make(common.AssetMap),
		Approvals: make(map[common.Address]common.AssetMap),
		Lockups:   make(map[common.Address]common.AssetMap),
	}
}

// GenerateLogicContextObject generates a LogicContextObject from the StateObject for a given types.LogicID
func (state *StateObject) GenerateLogicContextObject(logicID common.LogicID) *LogicContextObject {
	return &LogicContextObject{State: state, Logic: logicID}
}

// LogicContextObject is the context state accessor for
// Logics with access bounded to a specific LogicID
type LogicContextObject struct {
	State *StateObject
	Logic common.LogicID
}

func (ctx LogicContextObject) Address() common.Address { return ctx.State.Address }

func (ctx LogicContextObject) LogicID() common.LogicID { return ctx.Logic }

func (ctx LogicContextObject) GetStorageEntry(key []byte) ([]byte, bool) {
	tree, ok := ctx.State.LogicState[ctx.Logic.String()]
	if !ok {
		return nil, false
	}

	val, ok := tree[hex.EncodeToString(key)]
	if !ok {
		return nil, false
	}

	return val, true
}

func (ctx LogicContextObject) SetStorageEntry(key, val []byte) bool {
	tree, ok := ctx.State.LogicState[ctx.Logic.String()]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[hex.EncodeToString(key)] = val
	ctx.State.LogicState[ctx.Logic.String()] = tree

	return true
}
