package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/compute/engineio"
	pisastate "github.com/sarvalabs/go-pisa/state"
)

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
	// Create an access receipt with the read count
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
