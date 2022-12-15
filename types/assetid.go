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
// AssetID v0 Form is defined as follows: [version(4bits)|kind(2bits)|mintable(1bit)|fungible(1bit)]
// [transferable(1bit)|reserved(1bit)|dimension(6bits)][assetCID(256bits)]
func NewAssetIDv0(
	kind AssetKind, dimension AssetDimension, cid Hash,
	mintable, fungible, transferable bool,
) (AssetID, error) {
	// The 4 MSB bits of the head are set to the version of the AssetID form (v0)
	var head [2]byte
	head[0] = 0x0 << 4

	// Error if the asset kind is greater than 3 (2-bit space)
	if kind > 0x3 {
		return "", errors.New("cannot construct asset ID for invalid asset kind")
	}

	// Set the 5th and 6th MSB to the asset kind.
	// This is guaranteed to be 2 bits wide as we have sized the value
	head[0] |= uint8(kind) << 2

	// If mintable flag is on the 7th MSB is set
	if mintable {
		head[0] |= 0x2
	}

	// If fungible flag is on the 8th MSB is set
	if fungible {
		head[0] |= 0x1
	}

	// If transferable flag is on the 9th MSB is set
	if transferable {
		head[1] |= 0x80
	}

	// Error if the dimension is greater than 63 (6-bit space)
	if dimension > 0x3F {
		return "", errors.New("cannot construct asset ID for invalid dimension")
	}

	// Set the lowest 6 bits to the dimension.
	// This is guaranteed to be 6 bits wide as we have sized the value
	head[1] |= uint8(dimension)

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
