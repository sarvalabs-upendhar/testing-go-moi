package common

/*
Most of the types in this file are yet to be finalized.All the below structs are temporary type definitions
*/
import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/minio/highwayhash"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"
)

// DBEntry ...
// todo: find a more appropriate file for this struct
type DBEntry struct {
	Key   []byte
	Value []byte
}

// HashLength is the length of a hash
const HashLength = 32

var NilHash Hash

// Hash represents the 32 byte hash of arbitrary data.
type Hash [HashLength]byte

func BytesToHash(b []byte) Hash {
	var h Hash

	h.SetBytes(b)

	return h
}

func (h Hash) String() string {
	if h.IsNil() {
		return ""
	}

	return h.Hex()
}

func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-32:]
	}

	copy(h[32-len(b):], b)
}

func (h Hash) IsNil() bool {
	return h == NilHash
}

func (h Hash) Bytes() []byte { return h[:] }

func (h Hash) Hex() string { return BytesToHex(h.Bytes()) }

func (h Hash) MarshalText() ([]byte, error) {
	result := make([]byte, len(h)*2+2)
	copy(result[:2], "0x")
	hex.Encode(result[2:], h.Bytes())

	return result, nil
}

func (h *Hash) UnmarshalText(text []byte) error {
	if !(len(text) >= 2 && text[0] == byte('0') && (text[1] == byte('X') || text[1] == byte('x'))) {
		return ErrInvalidHash
	}

	text = text[2:]

	if len(text) != HashLength*2 {
		return fmt.Errorf("invalid address length: %d", len(text)/2)
	}

	_, err := hex.Decode(h[:], text)

	return err
}

// Hashes are array of hashes
type Hashes []Hash

func (h Hashes) Len() int {
	return len(h)
}

func (h Hashes) Less(i, j int) bool {
	if polarity := bytes.Compare(h[i].Bytes(), h[j].Bytes()); polarity < 0 {
		return true
	}

	return false
}

func (h Hashes) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Bytes returns the POLO serialized bytes of all hashes
func (h Hashes) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize interactions")
	}

	return rawData, nil
}

// FromBytes decodes the POLO serialized bytes into hashes
func (h *Hashes) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(h, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize interactions")
	}

	return nil
}

// FromHex returns the bytes represented by the hexadecimal string s
func FromHex(s string) []byte {
	if has0xPrefix(s) {
		s = s[2:]
	}

	if len(s)%2 == 1 {
		s = "0" + s
	}

	return Hex2Bytes(s)
}

func BytesToHex(data []byte) string {
	return hex.EncodeToString(data)
}

func HexToHash(s string) Hash {
	return BytesToHash(Hex2Bytes(s))
}

// has0xPrefix checks wheather the given string has 0x as prefix
func has0xPrefix(str string) bool {
	return len(str) >= 2 && str[0] == '0' && (str[1] == 'x' || str[1] == 'X')
}

// Hex2Bytes decodes string to []byte
func Hex2Bytes(str string) []byte {
	if has0xPrefix(str) {
		str = str[2:]
	}

	h, err := hex.DecodeString(str)
	if err != nil {
		panic(err)
	}

	return h
}

func PoloHash(x interface{}) (Hash, error) {
	bytes, err := polo.Polorize(x)
	if err != nil {
		return Hash{}, err
	}

	sum := blake2b.Sum256(bytes)

	return sum, nil
}

func GetHash(data []byte) Hash {
	return blake2b.Sum256(data)
}

// Key used for FastSum64
var fastSumHashKey = blake2b.Sum256([]byte("hash_fast_sum64_key"))

// FastSum64 returns a hash sum of the input data using highwayhash. This method is not secure, but
// may be used as a quick identifier for objects where collisions are acceptable.
func FastSum64(data []byte) uint64 {
	return highwayhash.Sum64(data, fastSumHashKey[:])
}

// FastSum256 returns a hash sum of the input data using highwayhash. This method is not secure, but
// may be used as a quick identifier for objects where collisions are acceptable.
func FastSum256(data []byte) [32]byte {
	return highwayhash.Sum(data, fastSumHashKey[:])
}
