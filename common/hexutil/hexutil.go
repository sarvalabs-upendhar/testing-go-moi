/*
Package hexutil implements hex encoding with 0x prefix.

# Encoding Rules

All hex data must have prefix "0x".

For byte slices, the hex data must be of even length. An empty byte slice
encodes as "0x".

Integers are encoded using the least amount of digits (no leading zero digits). Their
encoding may be of uneven length. The number zero encodes as "0x0".
*/
package hexutil

import (
	"encoding/hex"
	"reflect"
)

var (
	bytesT  = reflect.TypeOf(Bytes(nil))
	bigT    = reflect.TypeOf((*Big)(nil))
	uintT   = reflect.TypeOf(Uint(0))
	uint8T  = reflect.TypeOf(Uint8(0))
	uint16T = reflect.TypeOf(Uint16(0))
	uint64T = reflect.TypeOf(Uint64(0))
)

// Decode decodes a hex string with 0x prefix.
func Decode(input string) ([]byte, error) {
	if len(input) == 0 {
		return nil, ErrEmptyString
	}

	if !has0xPrefix(input) {
		return nil, ErrMissingPrefix
	}

	b, err := hex.DecodeString(input[2:])
	if err != nil {
		err = mapError(err)
	}

	return b, err
}

// MustDecode decodes a hex string with 0x prefix. It panics for invalid input.
func MustDecode(input string) []byte {
	dec, err := Decode(input)
	if err != nil {
		panic(err)
	}

	return dec
}

// Encode encodes b as a hex string with 0x prefix.
func Encode(b []byte) string {
	enc := make([]byte, len(b)*2+2)
	copy(enc, "0x")
	hex.Encode(enc[2:], b)

	return string(enc)
}

func isString(input []byte) bool {
	return len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"'
}

func bytesHave0xPrefix(input []byte) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

func has0xPrefix(input string) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

func checkNumber(input string) (raw string, err error) {
	if len(input) == 0 {
		return "", ErrEmptyString
	}

	if !has0xPrefix(input) {
		return "", ErrMissingPrefix
	}

	input = input[2:]
	if len(input) == 0 {
		return "", ErrEmptyNumber
	}

	if len(input) > 1 && input[0] == '0' {
		return "", ErrLeadingZero
	}

	return input, nil
}

func checkText(input []byte, wantPrefix bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, nil // empty strings are allowed
	}

	if bytesHave0xPrefix(input) {
		input = input[2:]
	} else if wantPrefix {
		return nil, ErrMissingPrefix
	}

	if len(input)%2 != 0 {
		return nil, ErrOddLength
	}

	return input, nil
}

func checkNumberText(input []byte) (raw []byte, err error) {
	if len(input) == 0 {
		return nil, nil // empty strings are allowed
	}

	if !bytesHave0xPrefix(input) {
		return nil, ErrMissingPrefix
	}

	input = input[2:]
	if len(input) == 0 {
		return nil, ErrEmptyNumber
	}

	if len(input) > 1 && input[0] == '0' {
		return nil, ErrLeadingZero
	}

	return input, nil
}
