package hexutil

import (
	"encoding/hex"
	"strconv"
)

// Bytes marshals/unmarshals as a JSON string with 0x prefix.
// The empty slice marshals as "0x".
type Bytes []byte

// MarshalText implements encoding.TextMarshaler
func (b Bytes) MarshalText() ([]byte, error) {
	result := make([]byte, len(b)*2+2)
	copy(result, `0x`)
	hex.Encode(result[2:], b)

	return result, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *Bytes) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(bytesT)
	}

	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), bytesT)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (b *Bytes) UnmarshalText(input []byte) error {
	raw, err := checkText(input, true)
	if err != nil {
		return err
	}

	dec := make([]byte, len(raw)/2)
	if _, err = hex.Decode(dec, raw); err != nil {
		err = mapError(err)
	} else {
		*b = dec
	}

	return err
}

// String returns the hex encoding of b.
func (b Bytes) String() string {
	return Encode(b)
}

// Bytes returns the bytes
func (b Bytes) Bytes() []byte {
	return b
}

// DecodeUint64 decodes a hex string with 0x prefix as a quantity.
func DecodeUint64(input string) (uint64, error) {
	raw, err := checkNumber(input)
	if err != nil {
		return 0, err
	}

	dec, err := strconv.ParseUint(raw, 16, 64)
	if err != nil {
		err = mapError(err)
	}

	return dec, err
}

// MustDecodeUint64 decodes a hex string with 0x prefix as a quantity. It panics for invalid input.
func MustDecodeUint64(input string) uint64 {
	dec, err := DecodeUint64(input)
	if err != nil {
		panic(err)
	}

	return dec
}

// EncodeUint64 encodes i as a hex string with 0x prefix.
func EncodeUint64(i uint64) string {
	enc := make([]byte, 2, 10)
	copy(enc, "0x")

	return string(strconv.AppendUint(enc, i, 16))
}
