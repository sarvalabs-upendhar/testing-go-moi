package types

import (
	"encoding/hex"
	"log"

	"github.com/pkg/errors"
)

// AssetID ...
type AssetID string

// NewAssetIDv0 generates a new Asset ID from the given asset parameters.
// NOTE: this is currently unused and the asset IDs are generated within the guna package.
// TODO: update asset creation logic and the methods on AssetID for this new standard.
func NewAssetIDv0(
	kind AssetKind, dimension AssetDimension, cid Hash,
	mintable, fungible, transferable bool,
) (AssetID, error) {
	// The head is initialized as 2 byte array. The highest 4 bits need to
	// be set to version the AssetID form (v0). But is not required for "0".
	var head [2]byte

	// Error if the dimension is greater than 63 (6-bit space)
	if dimension > 0x3F {
		return "", errors.New("cannot construct asset ID for invalid dimension")
	}

	// Set the lowest 4 bits of the first byte to the highest 4 bits of the dimension (out of 6 bits)
	head[0] |= (uint8(dimension) >> 2) & 0xF
	// Set the highest 2 bits of the second byte to the lowest 2 bits of the dimension
	head[1] = (uint8(dimension) & 0x3) << 6

	// Error if the dimension is greater than 3 (2-bit space)
	if kind > 0x3 {
		return "", errors.New("cannot construct asset ID for invalid asset kind")
	}

	// Set the 3rd and 4th bit of the second byte to kind.
	// This is safe because we have already sized the asset kind
	head[1] |= uint8(kind) << 4

	// If mintable flag is on the 5th MSB is set
	if mintable {
		head[1] |= 0x8
	}

	// If fungible flag is on the 6th MSB is set
	if fungible {
		head[1] |= 0x4
	}

	// If transferable flag is on the 7th MSB is set
	if transferable {
		head[1] |= 0x2
	}

	// Order the asset ID buffer [head][cid]
	buf := make([]byte, 0, 34)
	buf = append(buf, head[:]...)
	buf = append(buf, cid[:]...)

	// Encode the buffer into its hex string and return as an AssetID
	return AssetID(hex.EncodeToString(buf)), nil
}

func (asset AssetID) GetCID() []byte {
	data, err := hex.DecodeString(string(asset))
	if err != nil {
		return nil
	}

	return data[2:]
}

func (asset AssetID) GetDimension() (AssetDimension, error) {
	data, err := hex.DecodeString(string(asset))
	if err != nil {
		return 0, err
	}

	return AssetDimension(data[0]), nil
}

func (asset AssetID) IsFungible() bool {
	data, err := hex.DecodeString(string(asset))
	if err != nil {
		log.Fatal(err)
	}

	if data[1]&(0x01<<7) == 0x80 {
		return true
	}

	return false
}

func (asset AssetID) IsMintable() bool {
	data, err := hex.DecodeString(string(asset))
	if err != nil {
		log.Fatal(err)
	}

	if 0x01&data[1] == 1 {
		return true
	}

	return false
}
