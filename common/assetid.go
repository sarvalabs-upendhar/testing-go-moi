package common

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
)

// AssetID is a unique identifier for Assets and is a hex-encoded string.
// It contains information about the nature of the logic and deployed address.
type AssetID string

// NewAssetIDv0 generates a new AssetID with the v0 form. The NewAssetIDv0 v0 Form is defined as follows:
// [version(4bits)|logical(1bit)|stateful(1bit)|reserved(2bits)][dimension(8bits)][standard(16bits)][address(256bits)]
func NewAssetIDv0(logical, stateful bool, dimension uint8, standard AssetStandard, addr Address) AssetID {
	// The 4 MSB bits of the head are set the
	// version of the Asset ID Form (v0)
	var head uint8 = 0x00 << 4

	// If logical flag is on, the 5th MSB is set
	if logical {
		head |= 0x8
	}

	// If stateful flag is on, the 6th MSB is set
	if stateful {
		head |= 0x4
	}

	// Encode the 16-bit standard into its BigEndian bytes
	standardBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(standardBuf, uint16(standard))

	// Order the asset ID buffer [head][dimension][standard][address]
	buf := make([]byte, 0, 36)
	buf = append(buf, head)
	buf = append(buf, dimension)
	buf = append(buf, standardBuf...)
	buf = append(buf, addr[:]...)

	return AssetID(hex.EncodeToString(buf))
}

// Bytes returns the byte form of the AssetID.
// The AssetID is hex-decoded and returned.
// Panics if the asset ID is not a valid hex string
func (asset AssetID) Bytes() []byte {
	return Hex2Bytes(string(asset))
}

// String returns the string form of the AssetID.
func (asset AssetID) String() string {
	return string(asset)
}

// MarshalJSON implements the json.Marshaler interface for AssetID
func (asset AssetID) MarshalJSON() ([]byte, error) {
	return json.Marshal("0x" + string(asset))
}

// UnmarshalJSON implements the json.Unmarshaler interface for AssetID
func (asset *AssetID) UnmarshalJSON(data []byte) error {
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

	// Generate an identifier for the AssetID
	if _, err := AssetID(cleaned).Identifier(); err != nil {
		return err
	}

	*asset = AssetID(cleaned)

	return nil
}

func (asset AssetID) MarshalText() ([]byte, error) {
	return []byte("0x" + string(asset)), nil
}

// Address returns the Asset Address of the AssetID.
// Returns NilAddress if the AssetID is invalid.
func (asset AssetID) Address() Address {
	// Error if length is too short
	if len(asset) < 64 {
		return NilAddress
	}

	// Trim the last 64 characters (32 bytes) and decode to address
	return HexToAddress(string(asset[len(asset)-64:]))
}

// Identifier returns a AssetIdentifier for the AssetID
func (asset AssetID) Identifier() (AssetIdentifier, error) {
	idbytes := Hex2Bytes(string(asset))
	if len(idbytes) == 0 || len(idbytes) < 1 {
		return nil, errors.New("invalid asset ID: insufficient length")
	}

	// Determine the version of the AssetID and check if there are enough bytes
	switch version := int(idbytes[0] & 0xF0); version {
	case 0:
		if len(idbytes) != AssetIDV0Length {
			return nil, errors.New("invalid asset ID: insufficient length for v0")
		}

		// Create an AssetIdentifierV0 and copy the idbytes into it
		identifier := AssetIdentifierV0{}
		copy(identifier[:], idbytes)

		return identifier, nil

	default:
		return nil, errors.Errorf("invalid asset ID: unsupported version: %v", version)
	}
}

// AssetIdentifier is an extension of AssetID which can access
// the encoded properties of the AssetID such as it flags,
// dimension, standard and the address of the logic.
// todo: Standard() must return a type AssetStandard when it exists
type AssetIdentifier interface {
	Version() int
	AssetID() AssetID
	Address() Address
	Standard() AssetStandard
	Dimension() AssetDimension

	Logical() bool
	Stateful() bool
}

type AssetStandard uint16

// MAS is moi asset standard
const (
	MAS0 AssetStandard = iota
	MAS1
)

const AssetIDV0Length = 36

// AssetIdentifierV0 is an implementation of AssetIdentifier for the v0 specification
type AssetIdentifierV0 [AssetIDV0Length]byte

// AssetID returns the AssetIdentifierV0 as an AssetID
func (asset AssetIdentifierV0) AssetID() AssetID {
	return AssetID(hex.EncodeToString(asset[:]))
}

// Version returns the version of the AssetIdentifierV0.
// Returns -1, if the AssetIdentifierV0 is not valid
func (asset AssetIdentifierV0) Version() int { return 0 }

// Logical returns whether the logical flag is set for the AssetIdentifierV0.
func (asset AssetIdentifierV0) Logical() bool {
	// Determine the 5th LSB of the first byte (v0)
	bit := (asset[0] >> 3) & 0x1
	// Return true if bit is set
	return bit != 0
}

// Stateful returns whether the stateful flag is set for the AssetIdentifierV0.
func (asset AssetIdentifierV0) Stateful() bool {
	// Determine the 6th LSB of the first byte (v0)
	bit := (asset[0] >> 2) & 0x1
	// Return true if bit is set
	return bit != 0
}

// Dimension returns the dimension of the AssetIdentifierV0.
func (asset AssetIdentifierV0) Dimension() AssetDimension {
	// Dimension data is in the second byte of the AssetID (v0)
	return AssetDimension(asset[1])
}

// Standard returns the standard of the AssetIdentifierV0.
func (asset AssetIdentifierV0) Standard() AssetStandard {
	// Standard data is in the third and fourth byte of the LogicID (v0)
	standard := asset[2:4]
	// Decode into 16-bit number
	return AssetStandard(binary.BigEndian.Uint16(standard))
}

// Address returns the Asset Address of the AssetIdentifier.
func (asset AssetIdentifierV0) Address() Address {
	// Address data is everything after the fourth byte (v0)
	// We know it will be 32 bytes, because of the validity check
	address := asset[4:]
	// Convert address data into an Address and return
	return BytesToAddress(address)
}
