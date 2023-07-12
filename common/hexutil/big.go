package hexutil

import (
	"math/big"
)

// Big marshals/unmarshals as a JSON string with 0x prefix.
// The zero value marshals as "0x0".
//
// Negative integers are not supported at this time. Attempting to marshal them will
// return an error. Values larger than 256bits are rejected by Unmarshal but will be
// marshaled without error.
type Big big.Int

// MarshalText implements encoding.TextMarshaler
func (b Big) MarshalText() ([]byte, error) {
	return []byte(EncodeBig((*big.Int)(&b))), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *Big) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(bigT)
	}

	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), bigT)
}

// UnmarshalText implements encoding.TextUnmarshaler
func (b *Big) UnmarshalText(input []byte) error {
	raw, err := checkNumberText(input)
	if err != nil {
		return err
	}

	if len(raw) > 64 {
		return ErrBig256Range
	}

	words := make([]big.Word, len(raw)/bigWordNibbles+1)
	end := len(raw)

	for i := range words {
		start := end - bigWordNibbles
		if start < 0 {
			start = 0
		}

		for ri := start; ri < end; ri++ {
			nib := decodeNibble(raw[ri])
			if nib == badNibble {
				return ErrSyntax
			}

			words[i] *= 16
			words[i] += big.Word(nib)
		}

		end = start
	}

	var dec big.Int

	dec.SetBits(words)
	*b = (Big)(dec)

	return nil
}

// ToInt converts b to a big.Int.
func (b *Big) ToInt() *big.Int {
	return (*big.Int)(b)
}

// String returns the hex encoding of b.
func (b *Big) String() string {
	return EncodeBig(b.ToInt())
}

// DecodeBig decodes a hex string with 0x prefix as a quantity.
// Numbers larger than 256 bits are not accepted.
func DecodeBig(input string) (*big.Int, error) {
	raw, err := checkNumber(input)
	if err != nil {
		return nil, err
	}

	if len(raw) > 64 {
		return nil, ErrBig256Range
	}

	words := make([]big.Word, len(raw)/bigWordNibbles+1)
	end := len(raw)

	for i := range words {
		start := end - bigWordNibbles
		if start < 0 {
			start = 0
		}

		for ri := start; ri < end; ri++ {
			nib := decodeNibble(raw[ri])
			if nib == badNibble {
				return nil, ErrSyntax
			}

			words[i] *= 16
			words[i] += big.Word(nib)
		}

		end = start
	}

	dec := new(big.Int).SetBits(words)

	return dec, nil
}

// MustDecodeBig decodes a hex string with 0x prefix as a quantity.
// It panics for invalid input.
func MustDecodeBig(input string) *big.Int {
	dec, err := DecodeBig(input)
	if err != nil {
		panic(err)
	}

	return dec
}

// EncodeBig encodes bigint as a hex string with 0x prefix.
func EncodeBig(bigint *big.Int) string {
	switch sign := bigint.Sign(); sign {
	case 0:
		return "0x0"
	case 1:
		return "0x" + bigint.Text(16)
	default:
		return "-0x" + bigint.Text(16)[1:]
	}
}
