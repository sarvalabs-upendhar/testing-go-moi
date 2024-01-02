package core

import (
	"encoding/hex"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
)

// AccountState is a state object implementation that
// contains the entire context state of a participant
type AccountState struct {
	Address  identifiers.Address
	Storage  Storage
	Balances *Balances
}

// Balances is a container for all the different asset balances of an account.
// It can maintain the spendable, approval and lockup based asset balances.
type Balances struct {
	Spendable common.AssetMap
	Approvals map[identifiers.Address]common.AssetMap
	Lockups   map[identifiers.Address]common.AssetMap
}

// Storage is a simple two-dimensional binary key-value store
type Storage map[string]map[string][]byte

// NewAccountState generate a new StateObject for a given types.Address.
// All balances and key-value stores are initialized with empty values.
func NewAccountState(addr identifiers.Address) *AccountState {
	return &AccountState{
		Address: addr,
		Storage: make(Storage),
		Balances: &Balances{
			Spendable: make(common.AssetMap),
			Approvals: make(map[identifiers.Address]common.AssetMap),
			Lockups:   make(map[identifiers.Address]common.AssetMap),
		},
	}
}

// ContextDriver generates a ContextDriver from the AccountState for a given LogicID
func (state *AccountState) ContextDriver(logic identifiers.LogicID) *ContextDriver {
	return &ContextDriver{Logic: logic, State: state}
}

// ContextDriver is the context state accessor for
// Logics with access bounded to a specific LogicID.
// Implements the engineio.CtxDriver interface
type ContextDriver struct {
	Logic identifiers.LogicID
	State *AccountState
}

func (ctx ContextDriver) Address() identifiers.Address { return ctx.State.Address }

func (ctx ContextDriver) LogicID() identifiers.LogicID { return ctx.Logic }

func (ctx ContextDriver) GetStorageEntry(key []byte) ([]byte, bool) {
	tree, ok := ctx.State.Storage[string(ctx.Logic)]
	if !ok {
		return nil, false
	}

	val, ok := tree[hex.EncodeToString(key)]
	if !ok {
		return nil, false
	}

	return val, true
}

func (ctx ContextDriver) SetStorageEntry(key, val []byte) bool {
	tree, ok := ctx.State.Storage[string(ctx.Logic)]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[hex.EncodeToString(key)] = val
	ctx.State.Storage[ctx.Logic.String()] = tree

	return true
}
