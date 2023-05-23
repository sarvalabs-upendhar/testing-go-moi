package types

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
)

// LogicID is a unique identifier for Logics and is a hex-encoded string.
// It contains information about the nature of the logic and its deployed address.
type LogicID string

// NewLogicIDv0 generates a new LogicID with the v0 form. The LogicID v0 Form is defined as follows:
// [version(4bits)|persistent(1bit)|ephemeral(1bit)|interactive(1bit)|asset(1bit)][edition(16bits)][address(256bits)]
func NewLogicIDv0(persistent, ephemeral, interactive, assetlogic bool, edition uint16, addr Address) LogicID {
	// The 4 MSB bits of the head are set the
	// version of the Logic ID Form (v0)
	var head uint8 = 0x00 << 4

	// If persistent stateful flag is on, the 5th MSB is set
	if persistent {
		head |= 0x8
	}

	// If ephemeral stateful flag is on, the 6th MSB is set
	if ephemeral {
		head |= 0x4
	}

	// If interactive flag is on, the 7th MSB is set
	if interactive {
		head |= 0x2
	}

	// If asset logic flag is on, the 8th MSB is set
	if assetlogic {
		head |= 0x1
	}

	// Encode the 16-bit edition into its BigEndian bytes
	editionBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(editionBuf, edition)

	// Order the logic ID buffer [head][edition][address]
	buf := make([]byte, 0, 35)
	buf = append(buf, head)
	buf = append(buf, editionBuf...)
	buf = append(buf, addr[:]...)

	return LogicID(hex.EncodeToString(buf))
}

// Bytes returns the byte form of the LogicID.
// The LogicID is hex-decoded and returned.
// Panics if the logic ID is not a valid hex string
func (logic LogicID) Bytes() []byte {
	return Hex2Bytes(string(logic))
}

// String returns the string form of the LogicID.
func (logic LogicID) String() string {
	return string(logic)
}

// MarshalJSON implements the json.Marshaler interface for LogicID
func (logic LogicID) MarshalJSON() ([]byte, error) {
	return json.Marshal("0x" + string(logic))
}

// UnmarshalJSON implements the json.Unmarshaler interface for LogicID
func (logic *LogicID) UnmarshalJSON(data []byte) error {
	var decoded string

	// Decode the JSON data into a string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	// Data MUST contain 0x prefix, attempt the trim and check
	// if the size has changed (can save a call to HasPrefix)
	cleaned := strings.TrimPrefix(decoded, "0x")
	if cleaned == decoded {
		return errors.New("missing 0x prefix")
	}

	// Generate an identifier for the LogicID
	if _, err := LogicID(cleaned).Identifier(); err != nil {
		return err
	}

	*logic = LogicID(cleaned)

	return nil
}

// Address returns the Logic Address of the LogicID.
// Returns NilAddress if the LogicID is invalid.
func (logic LogicID) Address() Address {
	// Error if length is too short
	if len(logic) < 64 {
		return NilAddress
	}

	// Trim the last 64 characters (32 bytes) and decode to address
	return HexToAddress(string(logic[len(logic)-64:]))
}

// Identifier returns a LogicIdentifier for the LogicID
func (logic LogicID) Identifier() (LogicIdentifier, error) {
	idbytes := Hex2Bytes(string(logic))
	if len(idbytes) == 0 || len(idbytes) < 1 {
		return nil, errors.New("invalid logic ID: insufficient length")
	}

	// Determine the version of the LogicID and check if there are enough bytes
	switch version := int(idbytes[0] & 0xF0); version {
	case 0:
		if len(idbytes) != LogicIDV0Length {
			return nil, errors.New("invalid logic ID: insufficient length for v0")
		}

		// Create an LogicIdentifierV0 and copy the idbytes into it
		identifier := LogicIdentifierV0{}
		copy(identifier[:], idbytes)

		return identifier, nil

	default:
		return nil, errors.Errorf("invalid logic ID: unsupported version: %v", version)
	}
}

// LogicIdentifier is an extension of LogicID which can access
// the encoded properties of the LogicID such as it state flags,
// edition (upgrade nonce) and the address of the logic
type LogicIdentifier interface {
	Version() int
	LogicID() LogicID
	Address() Address
	Edition() uint16

	PersistentState() bool
	EphemeralState() bool
	Interactive() bool
	AssetLogic() bool
}

const LogicIDV0Length = 35

// LogicIdentifierV0 is an implementation of LogicIdentifier for the v0 specification
type LogicIdentifierV0 [LogicIDV0Length]byte

// LogicID returns the LogicIdentifierV0 as a LogicID
func (logic LogicIdentifierV0) LogicID() LogicID {
	return LogicID(hex.EncodeToString(logic[:]))
}

// Version returns the version of the LogicIdentifierV0.
// Returns -1, if the LogicIdentifierV0 is not valid
func (logic LogicIdentifierV0) Version() int { return 0 }

// PersistentState returns whether the persistent state flag is set for the LogicIdentifierV0.
func (logic LogicIdentifierV0) PersistentState() bool {
	// Determine the 5th LSB of the first byte (v0)
	bit := (logic[0] >> 3) & 0x1
	// Return true if bit is set
	return bit != 0
}

// EphemeralState returns whether the ephemeral state flag is set for the LogicIdentifierV0.
func (logic LogicIdentifierV0) EphemeralState() bool {
	// Determine the 6th LSB of the first byte (v0)
	bit := (logic[0] >> 2) & 0x1
	// Return true if bit is set
	return bit != 0
}

// Interactive returns whether the interactive flag is set for the LogicIdentifierV0.
func (logic LogicIdentifierV0) Interactive() bool {
	// Determine the 7th LSB of the first byte (v0)
	bit := (logic[0] >> 1) & 0x1
	// Return true if bit is set
	return bit != 0
}

// AssetLogic returns whether the asset logic flag is set for the LogicIdentifierV0.
func (logic LogicIdentifierV0) AssetLogic() bool {
	// Determine the 8th LSB of the first byte (v0)
	bit := logic[0] & 0x1
	// Return true if bit is set
	return bit != 0
}

// Edition returns the edition number of the LogicIdentifierV0.
func (logic LogicIdentifierV0) Edition() uint16 {
	// Edition data is in the second and third byte of the LogicID (v0)
	edition := logic[1:3]
	// Decode into 16-bit number
	return binary.BigEndian.Uint16(edition)
}

// Address returns the Logic Address of the LogicIdentifierV0.
func (logic LogicIdentifierV0) Address() Address {
	// Address data is everything after the third byte (v0)
	// We know it will be 32 bytes, because of the validity check
	address := logic[3:]
	// Convert address data into an Address and return
	return BytesToAddress(address)
}
