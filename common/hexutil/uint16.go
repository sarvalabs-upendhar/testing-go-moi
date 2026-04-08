//nolint:dupl
package hexutil

import (
	"math"

	"github.com/pkg/errors"
)

// Uint16 marshals/unmarshals as a JSON string with 0x prefix.
// The zero value marshals as "0x0".
type Uint16 uint16

// MarshalText implements encoding.TextMarshaler.
func (b Uint16) MarshalText() ([]byte, error) {
	return Uint64(b).MarshalText()
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *Uint16) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(uint16T)
	}

	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), uint16T)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (b *Uint16) UnmarshalText(input []byte) error {
	var u64 Uint64

	err := u64.UnmarshalText(input)
	if u64 > math.MaxUint16 || u64 > Uint64(^uint(0)) || errors.Is(err, ErrUint64Range) {
		return ErrUintRange
	} else if err != nil {
		return err
	}

	*b = Uint16(u64)

	return nil
}

// String returns the hex encoding of b.
func (b Uint16) String() string {
	return EncodeUint64(uint64(b))
}

// ToInt converts b to uint16.
func (b Uint16) ToInt() uint16 {
	return uint16(b)
}
