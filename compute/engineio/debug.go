package engineio

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/manishmeganathan/depgraph"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
)

type DebugEngineInstance interface {
	EngineInstance

	FuelReset()
	FuelLevel() uint64

	GetEventDriver() EventDriver

	GetLocalDriver() StateDriver
	SetLocalDriver(StateDriver)

	GetSenderDriver() StateDriver
	SetSenderDriver(StateDriver)
}

type debugLogicDriver struct {
	id       identifiers.LogicID
	kind     EngineKind
	manifest [32]byte

	sealed     bool
	persistent *uint64
	ephemeral  *uint64

	elements     map[ElementPtr]*LogicElement
	dependencies *depgraph.DependencyGraph

	callsites map[string]Callsite
	classdefs map[string]Classdef

	eventdefs map[string]Eventdef
}

func newDebugLogicDriver(t *testing.T, address identifiers.Address, descriptor LogicDescriptor) debugLogicDriver {
	t.Helper()

	// Generate the LogicID from the payload
	logicID := identifiers.NewLogicIDv0(
		descriptor.Persistent != nil,
		descriptor.Ephemeral != nil,
		descriptor.Interactable, false,
		0, address,
	)

	return debugLogicDriver{
		id:       logicID,
		kind:     descriptor.Engine,
		manifest: descriptor.ManifestHash,

		sealed:     false,
		persistent: descriptor.Persistent,
		ephemeral:  descriptor.Ephemeral,

		dependencies: descriptor.Depgraph,
		elements:     descriptor.Elements,

		callsites: descriptor.Callsites,
		classdefs: descriptor.Classdefs,

		eventdefs: descriptor.Eventdefs,
	}
}

func (logic debugLogicDriver) LogicID() identifiers.LogicID { return logic.id }
func (logic debugLogicDriver) Engine() EngineKind           { return logic.kind }
func (logic debugLogicDriver) ManifestHash() [32]byte       { return logic.manifest }
func (logic debugLogicDriver) IsSealed() bool               { return logic.sealed }

func (logic debugLogicDriver) IsInteractable() bool {
	identifier, err := logic.id.Identifier()
	if err != nil {
		panic("failed to fetch logic identifier")
	}

	return identifier.HasInteractableSites()
}

func (logic debugLogicDriver) PersistentState() (ElementPtr, bool) {
	if logic.persistent == nil {
		return 0, false
	}

	return *logic.persistent, true
}

func (logic debugLogicDriver) EphemeralState() (ElementPtr, bool) {
	if logic.ephemeral == nil {
		return 0, false
	}

	return *logic.ephemeral, true
}

func (logic debugLogicDriver) GetCallsite(name string) (Callsite, bool) {
	callsite, ok := logic.callsites[name]

	return callsite, ok
}

func (logic debugLogicDriver) GetClassdef(name string) (Classdef, bool) {
	classdef, ok := logic.classdefs[name]

	return classdef, ok
}

func (logic debugLogicDriver) GetEventdef(name string) (Eventdef, bool) {
	eventdef, ok := logic.eventdefs[name]

	return eventdef, ok
}

func (logic debugLogicDriver) GetElement(ptr ElementPtr) (*LogicElement, bool) {
	element, ok := logic.elements[ptr]

	return element, ok
}

func (logic debugLogicDriver) GetElementDeps(ptr ElementPtr) []ElementPtr {
	return logic.dependencies.Dependencies(ptr)
}

type debugStateDriver struct {
	address identifiers.Address
	logicID identifiers.LogicID

	logicstate map[string][]byte
}

func newDebugStateDriver(t *testing.T, address identifiers.Address, logicID identifiers.LogicID) *debugStateDriver {
	t.Helper()

	return &debugStateDriver{
		address:    address,
		logicID:    logicID,
		logicstate: make(map[string][]byte),
	}
}

func (state debugStateDriver) Address() identifiers.Address  { return state.address }
func (state debugStateDriver) LogicID() identifiers.LogicID  { return state.logicID }
func (state debugStateDriver) LogicState() map[string][]byte { return state.logicstate }

func (state debugStateDriver) GetStorageEntry(key []byte) ([]byte, error) {
	val, ok := state.logicstate[hex.EncodeToString(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

func (state *debugStateDriver) SetStorageEntry(key, val []byte) error {
	state.logicstate[hex.EncodeToString(key)] = val

	return nil
}

func (state debugStateDriver) Copy() *debugStateDriver {
	clone := &debugStateDriver{
		address:    state.address,
		logicID:    state.logicID,
		logicstate: make(map[string][]byte),
	}

	for key, val := range state.logicstate {
		clone.logicstate[key] = val
	}

	return clone
}

type debugTxnDriver struct {
	kind     common.IxOpType
	hash     common.Hash
	price    *big.Int
	limit    uint64
	callsite string
	calldata []byte
}

func newDebugTxnDriver(
	t *testing.T, kind common.IxOpType,
	hash common.Hash,
	callsite string, calldata []byte,
	limit uint64, price *big.Int,
) debugTxnDriver {
	t.Helper()

	return debugTxnDriver{
		kind:     kind,
		hash:     hash,
		price:    price,
		limit:    limit,
		callsite: callsite,
		calldata: calldata,
	}
}

func (txn debugTxnDriver) Type() common.IxOpType { return txn.kind }
func (txn debugTxnDriver) Hash() common.Hash     { return txn.hash }
func (txn debugTxnDriver) FuelPrice() *big.Int   { return txn.price }
func (txn debugTxnDriver) FuelLimit() uint64     { return txn.limit }
func (txn debugTxnDriver) Callsite() string      { return txn.callsite }
func (txn debugTxnDriver) Calldata() []byte      { return txn.calldata }

type debugEnvDriver struct {
	timestamp uint64
	clusterID string
}

func newDebugEnvDriver(t *testing.T, timestamp uint64, clusterID string) debugEnvDriver {
	t.Helper()

	return debugEnvDriver{
		timestamp: timestamp,
		clusterID: clusterID,
	}
}

func (env debugEnvDriver) Timestamp() uint64 { return env.timestamp }
func (env debugEnvDriver) ClusterID() string { return env.clusterID }

type debugEventDriver struct {
	logicID identifiers.LogicID
	events  []common.Log
}

func newDebugEventDriver(t *testing.T, logicID identifiers.LogicID) *debugEventDriver {
	t.Helper()

	return &debugEventDriver{
		logicID: logicID,
		events:  make([]common.Log, 0),
	}
}

func (events *debugEventDriver) Logic() identifiers.LogicID { return events.logicID }
func (events *debugEventDriver) Count() uint64              { return uint64(len(events.events)) }
func (events *debugEventDriver) Collect() []common.Log      { return events.events }

func (events *debugEventDriver) Reset() {
	events.events = make([]common.Log, 0)
}

func (events *debugEventDriver) Insert(event common.Log) {
	events.events = append(events.events, event)
}

func (events *debugEventDriver) Fetch(index uint64) (common.Log, bool) {
	if index >= events.Count() {
		return common.Log{}, false
	}

	return events.events[index], true
}

func (events *debugEventDriver) Iterate() <-chan common.Log {
	ch := make(chan common.Log)

	go func() {
		defer close(ch)

		for _, event := range events.events {
			ch <- event
		}
	}()

	return ch
}
