package core

import (
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
)

// ContextDriver is the context state accessor for
// Logics with access bounded to a specific LogicID.
// Implements the engineio.CtxDriver interface
type ContextDriver struct {
	env string
	src db.Database

	addr  identifiers.Address
	logic identifiers.LogicID
}

func NewContextDriver(env string, src db.Database, addr identifiers.Address, logic identifiers.LogicID) *ContextDriver {
	return &ContextDriver{env: env, src: src, addr: addr, logic: logic}
}

func (ctx ContextDriver) Address() identifiers.Address { return ctx.addr }
func (ctx ContextDriver) LogicID() identifiers.LogicID { return ctx.logic }

func (ctx ContextDriver) GetStorageEntry(key []byte) ([]byte, bool) {
	key = db.StorageKey(ctx.env, ctx.addr, ctx.logic, key)

	val, err := ctx.src.Get(key)
	if err != nil {
		return nil, false
	}

	return val, true
}

func (ctx ContextDriver) SetStorageEntry(key, val []byte) bool {
	key = db.StorageKey(ctx.env, ctx.addr, ctx.logic, key)

	if err := ctx.src.Set(key, val); err != nil {
		return false
	}

	return true
}
