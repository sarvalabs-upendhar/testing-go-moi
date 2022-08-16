package kbft

import (
	"bytes"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

// AccountSet is a struct that represents a set of Account addresses
type AccountSet struct {
	// Represents the collection of Addresses
	set []ktypes.Address
}

// GetByIndex is a method of AccountSet that retrieves an address from the set based on its index in the set.
// Accepts an int64 index and returns a ktypes.Address.
func (as *AccountSet) GetByIndex(index int64) (addrs ktypes.Address) {
	// Check for index out of bounds
	if index < 0 || int(index) >= len(as.set) {
		return ktypes.Address{}
	}

	// Return the address at the index
	return as.set[index]
}

// GetByAddress is a method of AccountSet that retrieves the index of a given Address in the set.
// Accepts a ktypes.Address and returns an int64 and returns -1 if Address is not part of the set.
func (as *AccountSet) GetByAddress(addr ktypes.Address) int64 {
	// Check if address exists in the set
	for index, value := range as.set {
		if bytes.Equal(addr.Bytes(), value.Bytes()) {
			return int64(index)
		}
	}

	// Return -1 if address does not exist
	return -1
}
