package logiclab

import (
	"encoding/hex"

	"github.com/sarvalabs/moichain/types"
)

// StateObject is a state object implementation that
// contains the entire context state of a participant
type StateObject struct {
	Address types.Address

	Datastore  map[string][]byte
	LogicState map[string]map[string][]byte

	Spendable types.AssetMap
	Approvals map[types.Address]types.AssetMap
	Lockups   map[types.Address]types.AssetMap
}

// NewStateObject generate a new StateObject for a given types.Address.
// All balances and key-value stores are initialized with empty values.
func NewStateObject(addr types.Address) *StateObject {
	return &StateObject{
		Address: addr,

		Datastore:  make(map[string][]byte),
		LogicState: make(map[string]map[string][]byte),

		Spendable: make(types.AssetMap),
		Approvals: make(map[types.Address]types.AssetMap),
		Lockups:   make(map[types.Address]types.AssetMap),
	}
}

// GenerateLogicContextObject generates a LogicContextObject from the StateObject for a given types.LogicID
func (state *StateObject) GenerateLogicContextObject(logicID types.LogicID) *LogicContextObject {
	return &LogicContextObject{State: state, Logic: logicID}
}

// LogicContextObject is the context state accessor for
// Logics with access bounded to a specific LogicID
type LogicContextObject struct {
	State *StateObject
	Logic types.LogicID
}

func (ctx LogicContextObject) Address() types.Address { return ctx.State.Address }

func (ctx LogicContextObject) LogicID() types.LogicID { return ctx.Logic }

func (ctx LogicContextObject) GetStorageEntry(key []byte) ([]byte, bool) {
	tree, ok := ctx.State.LogicState[ctx.Logic.Hex()]
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
	tree, ok := ctx.State.LogicState[ctx.Logic.Hex()]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[hex.EncodeToString(key)] = val
	ctx.State.LogicState[ctx.Logic.Hex()] = tree

	return true
}
