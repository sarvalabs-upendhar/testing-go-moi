package engineio

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-moi/common"
)

// CtxDriver represents an interface for accessing and manipulating
// context information of an account state. It is bounded to the context
// of particular account and can only mutate within applicable portions
// of the context state within the bounds of the logic's namespace
type CtxDriver interface {
	Address() common.Address
	LogicID() common.LogicID

	GetStorageEntry([]byte) ([]byte, bool)
	SetStorageEntry([]byte, []byte) bool

	// Height() -> U256
	// Hash() -> U256
}

// ContextStateMatrix is matrix indicating the use of different
// types of context state and the element pointers for them
type ContextStateMatrix map[ContextStateKind]ElementPtr

// Persistent indicates if the ContextStateMatrix has an entry for PersistentState
func (matrix ContextStateMatrix) Persistent() bool {
	_, exists := matrix[PersistentState]

	return exists
}

// Ephemeral indicates if the ContextStateMatrix has an entry for EphemeralState
func (matrix ContextStateMatrix) Ephemeral() bool {
	_, exists := matrix[EphemeralState]

	return exists
}

// ContextStateKind represents the scope of stateful data in a context
type ContextStateKind int

const (
	PersistentState ContextStateKind = iota
	EphemeralState
)

var contextStateKindToString = map[ContextStateKind]string{
	PersistentState: "persistent",
	EphemeralState:  "ephemeral",
}

var contextStateKindFromString = map[string]ContextStateKind{
	"persistent": PersistentState,
	"ephemeral":  EphemeralState,
}

// String implements the Stringer interface for ContextStateKind
func (state ContextStateKind) String() string {
	str, ok := contextStateKindToString[state]
	if !ok {
		panic("unknown ContextStateKind variant")
	}

	return str
}

// Polorize implements the polo.Polorizable interface for ContextStateKind
func (state ContextStateKind) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeString(state.String())

	return polorizer, nil
}

// Depolorize implements the polo.Depolorizable interface for ContextStateKind
func (state *ContextStateKind) Depolorize(depolorizer *polo.Depolorizer) error {
	raw, err := depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	kind, ok := contextStateKindFromString[raw]
	if !ok {
		return errors.New("invalid ContextStateKind value")
	}

	*state = kind

	return nil
}

// MarshalJSON implements the json.Marshaller interface for ContextStateKind
func (state ContextStateKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(state.String())
}

// UnmarshalJSON implements the json.Unmarshaller interface for ContextStateKind
func (state *ContextStateKind) UnmarshalJSON(data []byte) error {
	raw := new(string)
	if err := json.Unmarshal(data, raw); err != nil {
		return err
	}

	kind, ok := contextStateKindFromString[*raw]
	if !ok {
		return errors.New("invalid ContextStateKind value")
	}

	*state = kind

	return nil
}

// MarshalYAML implements the yaml.Marshaller interface for ContextStateKind
func (state ContextStateKind) MarshalYAML() (interface{}, error) {
	return state.String(), nil
}

// UnmarshalYAML implements the yaml.Unmarshaller interface for ContextStateKind
func (state *ContextStateKind) UnmarshalYAML(node *yaml.Node) error {
	raw := new(string)
	if err := node.Decode(raw); err != nil {
		return err
	}

	kind, ok := contextStateKindFromString[*raw]
	if !ok {
		return errors.New("invalid ContextStateKind value")
	}

	*state = kind

	return nil
}

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
