package hexutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/pkg/errors"
)

type decError struct{ msg string }

func (err decError) Error() string { return err.msg }

// Errors
var (
	ErrEmptyString   = &decError{"empty hex string"}
	ErrSyntax        = &decError{"invalid hex string"}
	ErrMissingPrefix = &decError{"hex string without 0x prefix"}
	ErrOddLength     = &decError{"hex string of odd length"}
	ErrEmptyNumber   = &decError{"hex string \"0x\""}
	ErrLeadingZero   = &decError{"hex number with leading zero digits"}
	ErrUint64Range   = &decError{"hex number > 64 bits"}
	ErrUintRange     = &decError{fmt.Sprintf("hex number > %d bits", uintBits)}
	ErrBig256Range   = &decError{"hex number > 256 bits"}
)

func mapError(err error) error {
	var numError *strconv.NumError
	if errors.As(err, &numError) {
		if errors.Is(numError.Err, strconv.ErrRange) {
			return ErrUint64Range
		} else if errors.Is(numError.Err, strconv.ErrSyntax) {
			return ErrSyntax
		}
	}

	var invalidByteError hex.InvalidByteError
	if errors.As(err, &invalidByteError) {
		return ErrSyntax
	}

	if errors.Is(err, hex.ErrLength) {
		return ErrOddLength
	}

	return err
}

func wrapTypeError(err error, typ reflect.Type) error {
	var decError *decError
	if errors.As(err, &decError) {
		return &json.UnmarshalTypeError{Value: err.Error(), Type: typ}
	}

	return err
}

func errNonString(typ reflect.Type) error {
	return &json.UnmarshalTypeError{Value: "non-string", Type: typ}
}
