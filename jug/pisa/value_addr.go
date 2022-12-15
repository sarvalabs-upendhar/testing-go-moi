package pisa

import "github.com/sarvalabs/go-polo"

// AddressValue represents a Value that operates like a types.Address
type AddressValue [32]byte

// NewAddressValue generates a new AddressValue for a given [32]byte value.
func NewAddressValue(addr [32]byte) AddressValue { return addr }

// DefaultAddressValue generates a new AddressValue with a nil address.
func DefaultAddressValue() AddressValue { return [32]byte{} }

// Type returns the Datatype of AddressValue, which is TypeAddress.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Type() *Datatype { return TypeAddress }

// Copy returns a copy of AddressValue as a Value.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Copy() Value { return addr }

// Data returns the POLO encoded bytes of AddressValue.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Data() []byte {
	data, _ := polo.Polorize(addr)

	return data
}
