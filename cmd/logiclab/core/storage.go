package core

import (
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
)

// StorageDriver is the storage state accessor for
// Logics with access bounded to a specific LogicID.
// Implements the engineio.StateDriver interface
type StorageDriver struct {
	env string
	src db.Database

	addr  identifiers.Address
	logic identifiers.LogicID
}

func NewStorageDriver(env string, src db.Database, addr identifiers.Address, logic identifiers.LogicID) *StorageDriver {
	return &StorageDriver{env: env, src: src, addr: addr, logic: logic}
}

func (ctx StorageDriver) Address() identifiers.Address { return ctx.addr }
func (ctx StorageDriver) LogicID() identifiers.LogicID { return ctx.logic }

func (ctx StorageDriver) GetStorageEntry(key []byte) ([]byte, error) {
	key = db.StorageKey(ctx.env, ctx.addr, ctx.logic, key)

	val, err := ctx.src.Get(key)
	if err != nil {
		return nil, err
	}

	return val, nil
}

func (ctx StorageDriver) SetStorageEntry(key, val []byte) error {
	key = db.StorageKey(ctx.env, ctx.addr, ctx.logic, key)

	if err := ctx.src.Set(key, val); err != nil {
		return err
	}

	return nil
}
