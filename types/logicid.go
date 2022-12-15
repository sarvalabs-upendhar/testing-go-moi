package types

import (
	"encoding/binary"
	"encoding/hex"
)

// LogicID is a unique identifier for a callable logic at a specific logic address.
// It encodes information such as whether the logic is stateful and/or interactive
// along with its kind, edition (upgrade nonce) and the logic address
type LogicID []byte

// NewLogicIDv0 generates a new LogicID with the v0 form.
// Returns an error if the LogicKind is greater than 3.
// LogicID v0 Form is defined as follows:
// [version(4bits)|kind(2bits)|stateful(1bit)|interactive(1bit)][edition(16bits)][address(256bits)]
func NewLogicIDv0(kind LogicKind, stateful bool, interactive bool, edition uint16, address Address) (LogicID, error) {
	// The 4 MSB bits of the head are set the
	// version of the Logic ID Form (v0)
	var head uint8 = 0x00 << 4

	// Error if logic kind value is greater than 3 (2 bit space)
	if kind > 0x3 {
		return nil, ErrInvalidLogicID
	}

	// Set the 5th and 6th MSB to the logic kind.
	// This is guaranteed to be 2 bits wide as we have sized the value
	head |= uint8(kind) << 2

	// If stateful flag is on, the 7th MSB is set
	if stateful {
		head |= 0x2
	}

	// If interactive flag is on, the 8th MSB is set
	if interactive {
		head |= 0x1
	}

	// Encode the 16-bit edition into its BigEndian bytes
	editionBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(editionBuf, edition)

	// Order the logic ID buffer [head][edition][address]
	buf := make([]byte, 0, 35)
	buf = append(buf, head)
	buf = append(buf, editionBuf...)
	buf = append(buf, address[:]...)

	return buf, nil
}

// Hex returns the LogicID as a hex encoded string
func (logic LogicID) Hex() string {
	return hex.EncodeToString(logic)
}

// Valid returns whether the LogicID is valid.
// It must be non nil and have sufficient bytes for its version.
// Only v0 is supported, all other forms are invalid.
func (logic LogicID) Valid() bool {
	if len(logic) == 0 {
		return false
	}

	// Calculate version of the LogicID
	// and check if there are enough bytes
	switch int(logic[0] & 0xF0) {
	case 0:
		return len(logic) == 35
	default:
		return false
	}
}

// Version returns the version of the LogicID.
// Returns -1, if the LogicID is not valid
func (logic LogicID) Version() int {
	// Check validity
	if !logic.Valid() {
		return -1
	}

	// Determine the highest 4 bits of the first byte (v0)
	return int(logic[0] & 0xF0)
}

// Kind returns the LogicKind of the LogicID.
// Returns LogicKindInvalid if LogicID is invalid.
func (logic LogicID) Kind() LogicKind {
	// Check logic version, internally checks validity
	if logic.Version() != 0 {
		return LogicKindInvalid
	}

	// Determine the 5th and 6th bits of the first byte (v0)
	return LogicKind(logic[0] & 0xC)
}

// Stateful returns whether the stateful flag is set for the LogicID.
// Returns false if the LogicID is invalid.
func (logic LogicID) Stateful() bool {
	// Check logic version, internally checks validity
	if logic.Version() != 0 {
		return false
	}

	// Determine the 7th LSB of the first byte (v0)
	bit := (logic[0] >> 1) & 0x1
	// Return true if bit is set
	return bit != 0
}

// Interactive returns whether the interactive flag is set for the LogicID.
// Returns false if the LogicID is invalid.
func (logic LogicID) Interactive() bool {
	// Check logic version, internally checks validity
	if logic.Version() != 0 {
		return false
	}

	// Determine the 8th LSB of the first byte (v0)
	bit := logic[0] & 0x1
	// Return true if bit is set
	return bit != 0
}

// Edition returns the edition number of the LogicID.
// Returns 0 if the LogicID is invalid.
func (logic LogicID) Edition() uint16 {
	// Check logic version, internally checks validity
	if logic.Version() != 0 {
		return 0
	}

	// Edition data is in the second and third byte of the LogicID (v0)
	editionBuf := logic[1:3]
	// Decode into 16-bit number
	edition := binary.BigEndian.Uint16(editionBuf)

	return edition
}

// Address returns the Logic Address of the LogicID.
// Returns NilAddress if the LogicID is invalid.
func (logic LogicID) Address() Address {
	// Check logic version, internally checks validity
	if logic.Version() != 0 {
		return NilAddress
	}

	// Address data is everything after the third byte (v0)
	// We know it will be 32 bytes, because of the validity check
	address := logic[4:]
	// Convert address data into an Address and return
	return BytesToAddress(address)
}
