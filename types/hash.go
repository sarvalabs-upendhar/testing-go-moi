package types

/*
Most of the types in this file are yet to be finalized.All the below structs are temporary type definitions
*/
import (
	"encoding/hex"
	"fmt"

	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"
)

const (
	// HashLength is the length of a hash
	HashLength = 32
)

var NilHash Hash

type DBEntry struct {
	Key   []byte
	Value []byte
}

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
	result := make([]byte, len(h)*2)
	hex.Encode(result, h.Bytes())

	return result, nil
}

func (h *Hash) UnmarshalText(text []byte) error {
	if len(text) != HashLength*2 {
		return fmt.Errorf("invalid address length: %d", len(text)/2)
	}

	_, err := hex.Decode(h[:], text)

	return err
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
