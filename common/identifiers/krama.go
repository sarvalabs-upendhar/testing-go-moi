package identifiers

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
)

// KramaIDKind represents the kinds of recognized krama identifiers.
type KramaIDKind byte

const (
	KindGuardian KramaIDKind = iota
)

const (
	// KramaIDV0 represents version 0 (0000 in binary)
	KramaIDV0 = 0
)

var supportedVersions = map[KramaIDKind]uint8{
	KindGuardian: 0,
}

// KramaIDTag represents the tag of a krama id.
// The first 4-bit nibble represents the kind of the identifier (KramaIDKind),
// and the second 4-bit nibble represents the version for that identifier kind.
//
// While the version is currently set to 0 for all kinds, it allows for future
// changes to the identifier format while maintaining backward compatibility.
//
// This format allows for up to 16 different kinds of krama id's and 16 different
// versions for each kind. While this headroom is excessive for current requirements,
// and could be optimized further, using the nibble as the smallest unit, allows for
// easily recognizing the kind and version of an identifier in its hexadecimal format.
type KramaIDTag byte

// TagKramaV0 combines kind and version using bitwise operations
// First 4 bits: Kind (0000 for Kind)
// Last 4 bits: Version (0000 for v0)
const (
	TagKramaV0 = KramaIDTag((KindGuardian << 4) | KramaIDV0)
)

// Kind returns the kind from the KramaIDTag
func (tag KramaIDTag) Kind() KramaIDKind {
	// Determine the kind from the upper 4 bits
	return KramaIDKind(tag >> 4)
}

// Version returns the version from the KramaIDTag
func (tag KramaIDTag) Version() uint8 {
	// Determine the version from the lower 4 bits
	return uint8(tag & 0x0F)
}

// Validate checks if the KramaIDTag is valid and returns an error if not.
// An error is returned if the version is not supported or the kind is invalid
func (tag KramaIDTag) Validate() error {
	// Check if the kind is under the maximum supported kind
	if tag.Kind() > KindGuardian {
		return ErrUnsupportedKind
	}

	// Check if the version is supported for the kind
	if tag.Version() > supportedVersions[tag.Kind()] {
		return ErrUnsupportedVersion
	}

	return nil
}

// NetworkZone represents different zones in the network
// Used in metadata to identify the node's network zone
type NetworkZone uint8

const (
	NetworkZone0 NetworkZone = iota // 0000
	NetworkZone1                    // 0001
	NetworkZone2                    // 0010
	NetworkZone3                    // 0011
)

// KramaMetadata represents the second byte of the KramaID
// Format: [network zone (4 bits)][reserved (4 bits)]
type KramaMetadata byte

// NetworkZone extracts the network zone from the metadata
// Returns NetworkZone by shifting right by 4 bits to get the zone value
func (km KramaMetadata) NetworkZone() NetworkZone {
	return NetworkZone(km >> 4)
}

// Validate checks if the KramaMetadata is valid and returns an error if not.
func (km KramaMetadata) Validate() error {
	if km.NetworkZone() > NetworkZone3 {
		return errors.New("invalid network zone")
	}

	return nil
}

// KramaID represents a unique identifier for network nodes in the MOI Protocol
// Format: [TAG (1 byte)][Metadata (1 byte)][Peer ID (variable length)]
// All components are Base58 encoded for the final string representation
type KramaID string

// NewKramaID generates a KramaID using the given kind, version, network zone and private key
// The resulting ID follows the format:
// - First byte (TAG): [Kind (4 bits)][Version (4 bits)]
// - Second byte (Metadata): [Network Zone (4 bits)][Reserved (4 bits)]
// - Remaining bytes: Peer ID derived from the private key
func NewKramaID(kind KramaIDKind, version uint8, networkZone NetworkZone, privateKey []byte) (KramaID, error) {
	peerID, err := GeneratePeerID(privateKey)
	if err != nil {
		return "", err
	}

	tag := KramaIDTag((uint8(kind) << 4) | version)
	// Create metadata with network zone in upper 4 bits
	// Lower 4 bits are reserved for future use
	metadata := networkZone << 4
	b58encoded := base58.Encode([]byte{byte(tag), byte(metadata)})

	// Construct raw KramaID: [TAG][Metadata][Peer ID]
	kramaID := strings.Join([]string{b58encoded, peerID.String()}, "")

	// Encode the complete ID in Base58
	return KramaID(kramaID), nil
}

// NewKramaIDFromPeerID generates a KramaID using an existing kind, version, network zone and peer id
// Similar to NewKramaID but accepts a peer.ID instead of generating one
func NewKramaIDFromPeerID(kind KramaIDKind, version uint8, networkZone NetworkZone, peerID peer.ID) KramaID {
	tag := KramaIDTag((uint8(kind) << 4) | version)
	metadata := networkZone << 4
	b58encoded := base58.Encode([]byte{byte(tag), byte(metadata)})

	// Construct raw KramaID: [TAG][Metadata][Peer ID]
	kramaID := strings.Join([]string{b58encoded, peerID.String()}, "")

	return KramaID(kramaID)
}

// MustKramaID is an enforced version of NewKramaID.
// Panics if an error occurs. Use with caution.
func MustKramaID(kind KramaIDKind, version uint8, networkZone NetworkZone, data []byte) KramaID {
	return must(NewKramaID(kind, version, networkZone, data))
}

// Decompose extracts the individual components of the KramaID
// Returns:
// - tag: first byte indicating type and version
// - metadata: second byte containing network zone and reserved bits
// - peerID: remaining bytes as string
// - error: if the KramaID is invalid
func (kid KramaID) Decompose() (tag KramaIDTag, metadata KramaMetadata, peerID string, err error) {
	if kid == "" {
		return 0, 0, "", errors.New("invalid krama id: empty")
	}

	decoded, err := base58.Decode(string(kid))
	if err != nil || len(decoded) < 3 { // Minimum length: 1 (tag) + 1 (metadata) + variable length (peer ID)
		return 0, 0, "", errors.New("invalid krama id: insufficient length")
	}

	length, err := peerIDLength(KramaIDTag(decoded[0]))
	if err != nil {
		return 0, 0, "", err
	}

	if len(kid) <= length {
		return 0, 0, "", errors.New("invalid krama id: invalid length")
	}

	return KramaIDTag(decoded[0]), KramaMetadata(decoded[1]), string(kid)[len(kid)-length:], nil
}

// Tag extracts the identifier type and version from the KramaID
func (kid KramaID) Tag() (KramaIDTag, error) {
	tag, _, _, err := kid.Decompose()

	return tag, err
}

// Metadata extracts the metadata byte containing network zone and reserved bits
func (kid KramaID) Metadata() (KramaMetadata, error) {
	_, metadata, _, err := kid.Decompose()

	return metadata, err
}

// PeerID extracts the peer ID portion from the KramaID
func (kid KramaID) PeerID() (string, error) {
	_, _, peerID, err := kid.Decompose()

	return peerID, err
}

// DecodedPeerID decodes the encoded PeerID from the KramaID into a peer.ID type.
func (kid KramaID) DecodedPeerID() (peer.ID, error) {
	id, err := kid.PeerID()
	if err != nil {
		return "", err
	}

	// Decode the encoded PeerID
	peerID, err := peer.Decode(id)
	if err != nil {
		return "", err
	}

	return peerID, nil
}

// String returns the KramaID as a string.
func (kid KramaID) String() string {
	return string(kid)
}

// Validate returns an error if the KramaID is invalid.
// An error is returned if the KramaID has an invalid tag or metadata.
func (kid KramaID) Validate() error {
	tag, metadata, peerID, err := kid.Decompose()
	if err != nil {
		return fmt.Errorf("failed to decompose krama id: %w", err)
	}

	// Check basic validity of the identifier tag
	if err = tag.Validate(); err != nil {
		return fmt.Errorf("invalid tag: %w", err)
	}

	if err = metadata.Validate(); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	if _, err = peer.Decode(peerID); err != nil {
		return fmt.Errorf("invalid peer id: %w", err)
	}

	return nil
}

// Helper functions

// GeneratePeerID creates a new peer.ID from a private key
// Uses secp256k1 curve for key operations
func GeneratePeerID(prvKey []byte) (peer.ID, error) {
	key, err := crypto.UnmarshalSecp256k1PrivateKey(prvKey)
	if err != nil {
		return "", errors.Wrap(err, "error decoding secp256k1 key")
	}

	peerID, err := peer.IDFromPublicKey(key.GetPublic())
	if err != nil {
		return "", errors.Wrap(err, "failed to generate peer ID")
	}

	return peerID, nil
}

// GenerateKramaIDv0 creates a new version 0 KramaID
// This function follows the updated specification:
// [TAG (1 byte)][Metadata (1 byte)][Peer ID (variable)]
func GenerateKramaIDv0(networkZone NetworkZone, privateKey []byte) (KramaID, error) {
	peerID, err := GeneratePeerID(privateKey)
	if err != nil {
		return "", err
	}

	// Create metadata with network zone in upper 4 bits
	// Lower 4 bits are reserved for future use
	metadata := networkZone << 4
	b58encoded := base58.Encode([]byte{byte(TagKramaV0), byte(metadata)})

	// Construct raw KramaID: [TAG][Metadata][Peer ID]
	kramaID := strings.Join([]string{b58encoded, peerID.String()}, "")

	// Encode the complete ID in Base58
	return KramaID(kramaID), nil
}

// RandomKramaIDv0 generates a random version 0 KramaID
// Useful for testing and development purposes
func RandomKramaIDv0() (KramaID, error) {
	nPrivKey, err := RandomNetworkKey()
	if err != nil {
		return "", err
	}

	kramaID, err := GenerateKramaIDv0(NetworkZone(rand.UintN(3)), nPrivKey)
	if err != nil {
		return "", err
	}

	return kramaID, nil
}

// peerIDLength returns the expected length of a peer ID for a given KramaIDTag.
// It returns an error if the tag is unsupported.
func peerIDLength(tag KramaIDTag) (int, error) {
	switch tag {
	case TagKramaV0:
		return 53, nil
	default:
		return 0, ErrUnsupportedTag
	}
}
