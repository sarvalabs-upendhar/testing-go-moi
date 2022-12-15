package pisa

// Register is the unit of manipulation for the PISA Runtime.
// It represents some arbitrary typed data in the form of a Value.
type Register struct {
	Value
}

// NewRegister generates a new Register from a Value
func NewRegister(value Value) Register {
	return Register{value}
}

// copy returns a deep copy of Register
func (reg Register) copy() Register {
	return Register{reg.Value.Copy()}
}

// empty returns whether the Register has a nil Value
func (reg Register) empty() bool { //nolint:unused
	return reg.Value == nil
}

// RegisterSet is a collection of byte indexed Register objects.
type RegisterSet map[byte]Register

// get retrieves a Register for a given address.
// Returns a blank Register if there is none for the address.
func (regset RegisterSet) get(id byte) (Register, bool) {
	if reg, ok := regset[id]; ok {
		return reg, true
	}

	return Register{}, false
}

// set inserts a Register to a given address.
// Overwrites any existing Register at the address.
func (regset RegisterSet) set(id byte, reg Register) {
	regset[id] = reg
}

// unset clears a Register at a given address
func (regset RegisterSet) unset(id byte) {
	delete(regset, id)
}
