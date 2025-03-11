package identifiers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"hash"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"golang.org/x/crypto/sha3"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

const HardenedStartIndex = 2147483648 // 2^31

// Nil is a nil [32]byte value.
// Can be used to represent any nil identifier.
var Nil [32]byte

// RandomFingerprint generates a random 24-byte fingerprint
func RandomFingerprint() (fingerprint [24]byte) {
	_, _ = rand.Read(fingerprint[:])

	return fingerprint
}

func GetRandomPrivateKeys(signingKey [32]byte) ([]byte, error) {
	// Let's derive 'm' in the path
	masterKey, err := hdkeychain.NewMaster(signingKey[:], &chaincfg.MainNetParams) // here key is master key
	if err != nil {
		return nil, err
	}

	// Hardened keys index starts from 2147483648 (2^31)
	// So.,
	// 44 = 2147483648 + 44 = 2147483692
	// 6174 = 2147483648 + 6174 = 2147489822
	igcParams := [2]uint32{2147483692, 2147489822}

	tempKey := masterKey
	for _, n := range igcParams {
		tempKey, err = tempKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}

	// to persist consensus and network private keys
	var aggPrivKey []byte
	// Let's derive PrivateKey for signing, so load keyPair at path: m/44'/6174'/5020'/0/n
	validatorPrivKey := tempKey

	var validatorPath [3]uint32
	validatorPath[0] = HardenedStartIndex + 5020 // hardened
	validatorPath[1] = 0
	validatorPath[2] = 0

	for _, n := range validatorPath {
		validatorPrivKey, err = validatorPrivKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}
	// Now validatorPrivKey points to extended private key at path: m/44'/6174'/5020'/0/n

	// Casting to Elliptic curve Private key
	privKeyInEC, err := validatorPrivKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	signingPrivKeyInBytes := privKeyInEC.Serialize()

	aggPrivKey = append(aggPrivKey, signingPrivKeyInBytes...)

	// Let's derive PrivateKey for communication, so load keyPair at path: m/44'/6174'/6020'/0/n
	ntwPrivKey := tempKey

	var networkPath [3]uint32
	networkPath[0] = HardenedStartIndex + 6020 // hardened
	networkPath[1] = 0
	networkPath[2] = 0

	for _, n := range networkPath {
		ntwPrivKey, err = ntwPrivKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}

	// Now ntwPrivKey points to extended private key at path: m/44'/6174'/6020'/0/n

	// Casting to Elliptic curve Private key
	nPrivKeyInEC, err := ntwPrivKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	ntwPrivKeyInBytes := nPrivKeyInEC.Serialize()

	aggPrivKey = append(aggPrivKey, ntwPrivKeyInBytes...)

	return aggPrivKey, nil
}

func RandomNetworkKey() ([]byte, error) {
	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	if err != nil {
		return nil, err
	}

	privateKeys, err := GetRandomPrivateKeys(signKey)
	if err != nil {
		return nil, err
	}

	return privateKeys[32:], nil
}

var (
	prefix0xString = "0x"
	prefix0xBytes  = []byte(prefix0xString)
)

var (
	ErrMissingHexPrefix = errors.New("missing '0x' prefix")
	ErrInvalidLength    = errors.New("invalid length")

	ErrUnsupportedTag     = errors.New("unsupported tag")
	ErrUnsupportedFlag    = errors.New("unsupported flag")
	ErrUnsupportedVersion = errors.New("unsupported tag version")
	ErrUnsupportedKind    = errors.New("unsupported tag kind")
)

// trim0xPrefixString trims the 0x prefix from the given string (if it exists).
func trim0xPrefixString(value string) string {
	return strings.TrimPrefix(value, prefix0xString)
}

// trim0xPrefixBytes trims the 0x prefix from the given byte slice (if it exists).
func trim0xPrefixBytes(value []byte) []byte {
	return bytes.TrimPrefix(value, prefix0xBytes)
}

// has0xPrefixBytes checks if the given byte slice has a 0x prefix.
func has0xPrefixBytes(value []byte) bool {
	return bytes.HasPrefix(value, prefix0xBytes)
}

// decodeHexString decodes the given hex string into a byte slice.
// It trims the 0x prefix (if found) from the string before decoding.
func decodeHexString(str string) ([]byte, error) {
	// Trim the 0x prefix from the string (if it exists)
	str = trim0xPrefixString(str)

	decoded, err := hex.DecodeString(str)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

// trimFingerprint returns the 24 bytes in the middle of the given 32-byte array.
func trimFingerprint(bytes [32]byte) [24]byte {
	return [24]byte(bytes[4:28])
}

// trimVariant returns the 4 least-significant bytes of the given 32-byte array.
func trimVariant(bytes [32]byte) [4]byte {
	return [4]byte(bytes[28:])
}

// marshal32 is a generic marshal function for 32-byte identifiers.
// To be used in conjunction with MarshalText
func marshal32(data [32]byte) ([]byte, error) {
	buffer := make([]byte, 32*2+2)

	// Copy the 0x prefix into the buffer
	copy(buffer[:2], prefix0xString)
	// Hex-encode the copied value into the buffer
	hex.Encode(buffer[2:], data[:])

	return buffer, nil
}

// unmarshal32 is generic unmarshal function for 32-byte identifiers.
// To be used in conjunction with UnmarshalText
func unmarshal32(data []byte) ([32]byte, error) {
	// Assert that the 0x prefix exists
	if !has0xPrefixBytes(data) {
		return Nil, ErrMissingHexPrefix
	}

	// Trim the 0x prefix
	data = trim0xPrefixBytes(data)

	// Check that the data has enough length for the identifier data
	if len(data) != 32*2 {
		return Nil, ErrInvalidLength
	}

	// Decode the hex-encoded data
	decoded, err := decodeHexString(string(data))
	if err != nil {
		return Nil, err
	}

	return [32]byte(decoded), nil
}

// must is correctness enforcer for error handling.
// For use in functions that should never return an error.
// Panics if an error is encountered.
func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}

	return t
}

func GetAddressFromPublicBytes(pubKey []byte) string {
	addr := keccak256(pubKey[1:])
	addr = addr[12:]

	return hex.EncodeToString(addr)
}

func GetPublicKeyFromPrivateBytes(validatorPrvKey []byte, compressed bool) (pubkey []byte) {
	_, publicKeyInEC := btcec.PrivKeyFromBytes(validatorPrvKey)

	if compressed {
		pubkey = publicKeyInEC.SerializeCompressed()
	} else {
		pubkey = publicKeyInEC.SerializeUncompressed()
	}

	return pubkey
}

func keccak256(data ...[]byte) []byte {
	b := make([]byte, 32)
	d, ok := sha3.NewLegacyKeccak256().(interface {
		hash.Hash
		Read([]byte) (int, error)
	})

	if !ok {
		return b
	}

	for _, b := range data {
		d.Write(b)
	}

	if _, err := d.Read(b); err != nil {
		return nil
	}

	return b
}
