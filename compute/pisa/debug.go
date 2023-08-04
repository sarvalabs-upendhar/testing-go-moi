package pisa

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-polo"
)

// DebugContextDriver is a debug-only implementation of CtxDriver.
// It is only meant for in-memory state management and offers no persistence
// though it can be encoded and stored at the debugging routine's discretion.
// For production workloads, generate a CtxDriver from the state.StateObject.
type DebugContextDriver struct {
	address common.Address
	logicID common.LogicID

	spendable common.AssetMap
	approvals map[common.Address]common.AssetMap
	lockups   map[common.Address]common.AssetMap

	datastore  map[string][]byte
	logicstate map[string]map[string][]byte
}

// NewDebugContextDriver generates a new DebugContextDriver for a given Address
func NewDebugContextDriver(addr common.Address, logic common.LogicID) *DebugContextDriver {
	return &DebugContextDriver{
		address: addr,
		logicID: logic,

		spendable: make(common.AssetMap),
		approvals: make(map[common.Address]common.AssetMap),
		lockups:   make(map[common.Address]common.AssetMap),

		datastore:  make(map[string][]byte),
		logicstate: make(map[string]map[string][]byte),
	}
}

// Polorize implements the polo.Polorizable interface for DebugContextDriver
func (ctx DebugContextDriver) Polorize() (*polo.Polorizer, error) {
	// Create a new Polorizer
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(ctx.address); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.spendable); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.approvals); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.lockups); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.datastore); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(ctx.logicstate); err != nil {
		return nil, err
	}

	return polorizer, nil
}

// Depolorize implements the polo.Depolorizable interface for DebugContextDriver
func (ctx *DebugContextDriver) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	if err = depolorizer.Depolorize(&ctx.address); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.spendable); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.approvals); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.lockups); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.datastore); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&ctx.logicstate); err != nil {
		return err
	}

	return nil
}

// Address returns the address of the DebugContextDriver account
// Implements the CtxDriver interface for DebugContextDriver.
func (ctx DebugContextDriver) Address() common.Address {
	return ctx.address
}

// LogicID returns the logic ID of the DebugContextDriver scope.
// Implements the CtxDriver interface for DebugContextDriver.
func (ctx DebugContextDriver) LogicID() common.LogicID {
	return ctx.logicID
}

// GetStorageEntry retrieves some []byte data for a given key from the DebugContextDriver.
// Returns error if there is no data for the key. Implements the CtxDriver interface for DebugContextDriver.
func (ctx DebugContextDriver) GetStorageEntry(key []byte) ([]byte, bool) {
	tree, ok := ctx.logicstate[string(ctx.logicID)]
	if !ok {
		return nil, false
	}

	val, ok := tree[string(key)]
	if !ok {
		return nil, false
	}

	return val, true
}

// SetStorageEntry inserts a key-value pair of data into the DebugContextDriver.
// If data already exists for the key, it is overwritten. Implements the CtxDriver interface for DebugContextDriver.
func (ctx *DebugContextDriver) SetStorageEntry(key, val []byte) bool {
	tree, ok := ctx.logicstate[string(ctx.logicID)]
	if !ok {
		tree = make(map[string][]byte)
	}

	tree[string(key)] = val
	ctx.logicstate[string(ctx.logicID)] = tree

	return true
}

// Copy generates a copy of the DebugContextDriver
func (ctx *DebugContextDriver) Copy() *DebugContextDriver {
	copied := &DebugContextDriver{
		address: ctx.address,
		logicID: ctx.logicID,

		spendable: ctx.spendable.Copy(),
		approvals: make(map[common.Address]common.AssetMap, len(ctx.approvals)),
		lockups:   make(map[common.Address]common.AssetMap, len(ctx.lockups)),

		datastore:  make(map[string][]byte, len(ctx.datastore)),
		logicstate: make(map[string]map[string][]byte, len(ctx.logicstate)),
	}

	for owner, assets := range ctx.approvals {
		copied.approvals[owner] = assets.Copy()
	}

	for owner, assets := range ctx.lockups {
		copied.lockups[owner] = assets.Copy()
	}

	for key, value := range ctx.datastore {
		v := make([]byte, len(value))
		copy(v, value)
		copied.datastore[key] = v
	}

	for logicID, logicState := range ctx.logicstate {
		copied.logicstate[logicID] = make(map[string][]byte, len(logicState))

		for key, value := range logicState {
			v := make([]byte, len(value))
			copy(v, value)
			copied.logicstate[logicID][key] = v
		}
	}

	return copied
}

type DebugIxnDriver struct {
	ixType    common.IxType
	fuelPrice *big.Int
	fuelLimit *big.Int
	callsite  string
	calldata  []byte
}

func NewDebugIxnDriver(kind common.IxType, price, limit *big.Int, callsite string, calldata []byte) DebugIxnDriver {
	return DebugIxnDriver{
		ixType:    kind,
		fuelPrice: price,
		fuelLimit: limit,
		callsite:  callsite,
		calldata:  calldata,
	}
}

func (ixn DebugIxnDriver) Type() common.IxType { return ixn.ixType }
func (ixn DebugIxnDriver) FuelPrice() *big.Int { return ixn.fuelPrice }
func (ixn DebugIxnDriver) FuelLimit() *big.Int { return ixn.fuelLimit }
func (ixn DebugIxnDriver) Callsite() string    { return ixn.callsite }
func (ixn DebugIxnDriver) Calldata() []byte    { return ixn.calldata }

type DebugEnvDriver struct {
	clusterID string
	timestamp int64
}

func NewDebugEnvDriver(timestamp int64, clusterID string) DebugEnvDriver {
	return DebugEnvDriver{
		clusterID: clusterID,
		timestamp: timestamp,
	}
}

func (env DebugEnvDriver) Timestamp() int64  { return env.timestamp }
func (env DebugEnvDriver) ClusterID() string { return env.clusterID }
