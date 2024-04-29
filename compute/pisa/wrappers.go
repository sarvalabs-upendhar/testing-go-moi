package pisa

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/common"
	pisalogic "github.com/sarvalabs/go-pisa/logic"
	pisastate "github.com/sarvalabs/go-pisa/state"
	"github.com/sarvalabs/go-polo"
)

type Logic struct {
	driver engineio.LogicDriver
}

func (logic Logic) Callsite(site string) (uint64, bool) {
	callsite, ok := logic.driver.GetCallsite(site)
	if !ok {
		return 0, false
	}

	return callsite.Ptr, true
}

func (logic Logic) Classdef(class string) (uint64, bool) {
	classdef, ok := logic.driver.GetClassdef(class)
	if !ok {
		return 0, false
	}

	return classdef.Ptr, true
}

func (logic Logic) PersistentState() (*pisalogic.Element, bool) {
	ptr, ok := logic.driver.PersistentState()
	if !ok {
		return nil, false
	}

	element, ok := logic.driver.GetElement(ptr)
	if !ok {
		return nil, false
	}

	if element.Kind != StateElement {
		return nil, false
	}

	return &pisalogic.Element{
		Kind: pisalogic.StateElement,
		Deps: element.Deps,
		Data: element.Data,
	}, true
}

func (logic Logic) EphemeralState() (*pisalogic.Element, bool) {
	ptr, ok := logic.driver.EphemeralState()
	if !ok {
		return nil, false
	}

	element, ok := logic.driver.GetElement(ptr)
	if !ok {
		return nil, false
	}

	if element.Kind != StateElement {
		return nil, false
	}

	return &pisalogic.Element{
		Kind: pisalogic.StateElement,
		Deps: element.Deps,
		Data: element.Data,
	}, true
}

func (logic Logic) Element(ptr uint64) (*pisalogic.Element, bool) {
	element, ok := logic.driver.GetElement(ptr)
	if !ok {
		return nil, false
	}

	metadata, ok := ElementMetadata[element.Kind]
	if !ok {
		return nil, false
	}

	return &pisalogic.Element{
		Kind: metadata.pisakind,
		Deps: element.Deps,
		Data: element.Data,
	}, true
}

func (logic Logic) Dependencies(ptr uint64) []uint64 {
	return logic.driver.GetElementDeps(ptr)
}

type Ixn struct {
	driver   engineio.InteractionDriver
	calldata polo.Document
}

func newIxn(driver engineio.InteractionDriver) Ixn {
	calldata := make(polo.Document)
	_ = polo.Depolorize(&calldata, driver.Calldata())

	return Ixn{driver: driver, calldata: calldata}
}

func (ixn Ixn) Kind() string { return ixn.driver.Type().String() }

func (ixn Ixn) FuelPrice() *big.Int { return ixn.driver.FuelPrice() }
func (ixn Ixn) FuelLimit() uint64   { return ixn.driver.FuelLimit() }

func (ixn Ixn) Callsite() string        { return ixn.driver.Callsite() }
func (ixn Ixn) Calldata() polo.Document { return ixn.calldata }

type Env struct {
	driver engineio.EnvironmentDriver
}

func (env Env) Timestamp() uint64 { return env.driver.Timestamp() }
func (env Env) ClusterID() string { return env.driver.ClusterID() }

type Crypto int

func (Crypto) ValidateSignature(sig []byte) bool {
	return common.CanUnmarshalSignature(sig)
}

func (Crypto) VerifySignature(data, sig, pub []byte) (bool, error) {
	return crypto.Verify(data, sig, pub)
}

type State struct {
	driver  engineio.StateDriver
	borrows map[uint8]struct{}
	slotptr map[uint8]*Slot
}

func newState(driver engineio.StateDriver) *State {
	return &State{
		driver:  driver,
		borrows: make(map[uint8]struct{}),
		slotptr: make(map[uint8]*Slot),
	}
}

func (state State) Address() [32]byte   { return state.driver.Address() }
func (state State) LogicID() string     { return string(state.driver.LogicID()) }
func (state State) LogicAddr() [32]byte { return state.driver.LogicID().Address() }

func (state *State) Free(slot uint8) bool {
	if !state.IsBorrowed(slot) {
		return false
	}

	delete(state.borrows, slot)

	return true
}

func (state *State) Return(slot pisastate.SlotDriver) (pisastate.AccessReceipt, error) {
	if !state.IsBorrowed(slot.Slot()) {
		return pisastate.AccessReceipt{}, errors.New("slot not currently borrowed")
	}

	// Flush the changes and get the access receipt
	receipt, err := slot.Flush()
	if err != nil {
		return receipt, err
	}

	// Free the slot
	state.Free(slot.Slot())

	return receipt, nil
}

func (state *State) Borrow(slot uint8) (pisastate.SlotDriver, error) {
	// Check if the slot is already borrowed
	if state.IsBorrowed(slot) {
		return nil, errors.New("slot is already borrowed")
	}
	// Mark the slot as borrowed
	state.borrows[slot] = struct{}{}

	// Check if a reference to the slot already exists
	if slotptr, ok := state.slotptr[slot]; ok {
		return slotptr, nil
	}

	// Create a new slot driver
	slotptr := &Slot{
		slot: slot, state: state,
		dirty:     make(map[[32]byte][]byte),
		readCache: make(map[[32]byte][]byte),
	}

	// Insert the slot driver into the slotptr map
	state.slotptr[slot] = slotptr

	return slotptr, nil
}

func (state *State) HasBorrows() bool {
	return len(state.borrows) != 0
}

func (state *State) ClearBorrows() {
	state.borrows = make(map[uint8]struct{})
}

func (state *State) IsBorrowed(slot uint8) bool {
	_, ok := state.borrows[slot]

	return ok
}

type Slot struct {
	slot  uint8
	state *State
	dirty map[[32]byte][]byte

	readCount uint64
	readCache map[[32]byte][]byte
}

func (slot Slot) Slot() uint8   { return slot.slot }
func (slot Slot) Count() uint64 { return slot.readCount }

func (slot *Slot) Exists(key pisastate.StorageKey) bool {
	// Read the value and check if it exists
	// This has side effect of caching the value that was read,
	// allowing it to be accessed efficiently in the next access
	_, ok := slot.Read(key)

	return ok
}

func (slot *Slot) Read(key pisastate.StorageKey) ([]byte, bool) {
	// Generate the 32-byte raw key
	rawkey := key.Bytes32()

	// Check the dirty entries for the key
	if entry, ok := slot.dirty[rawkey]; ok {
		return entry, true
	}

	// Check if an entry exists in the read-cache
	if entry, ok := slot.readCache[rawkey]; ok {
		return entry, true
	}

	// If not found in caches, attempt to read from the state tree
	rawval, ok := slot.state.driver.GetStorageEntry(rawkey[:])
	if !ok {
		return nil, false
	}

	// Cache the entry into the read-cache
	slot.readCache[rawkey] = rawval
	slot.readCount += uint64(len(rawval))

	return rawval, true
}

func (slot *Slot) Write(key pisastate.StorageKey, val []byte) error {
	// Generate the 32-byte raw key
	rawkey := key.Bytes32()

	// Set the value to the dirty entries
	slot.dirty[rawkey] = val
	slot.readCache[rawkey] = val

	return nil
}

func (slot *Slot) Flush() (pisastate.AccessReceipt, error) {
	// Create a access receipt with the read count
	receipt := pisastate.AccessReceipt{Read: slot.readCount}

	for key, val := range slot.dirty {
		// This forces the compiler to create new pointers for each key iteration
		key, val := key, val

		if len(val) == 0 {
			// Get the current entry for the key in the storage
			// If there is no value, len(entry) == 0
			entry, _ := slot.state.driver.GetStorageEntry(key[:])
			// Increment the deletion count on the access receipt
			receipt.Delete += uint64(len(entry))

			// Delete the value for the key
			slot.state.driver.SetStorageEntry(key[:], []byte{})

			continue
		}

		slot.state.driver.SetStorageEntry(key[:], val)
		receipt.Write += uint64(len(val))
	}

	// Reset all caches and counters
	slot.readCount = 0
	slot.readCache = make(map[[32]byte][]byte)
	slot.dirty = make(map[[32]byte][]byte)

	return receipt, nil
}
