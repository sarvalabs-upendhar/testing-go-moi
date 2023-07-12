package pisa

import (
	"encoding/binary"
	"math/bits"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/constraints"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

func SlotHash(slot uint8) []byte {
	hash := blake2b.Sum256([]byte{slot})

	return hash[:]
}

func isExportedName(str string) bool {
	return unicode.IsUpper(rune(str[0]))
}

func isMutableName(str string) bool {
	return strings.HasSuffix(str, "!")
}

func isPayableName(str string) bool {
	return strings.HasSuffix(str, "$")
}

// ptrdecode interprets a slice of bytes into a 64-bit unsigned integer
// Returns an overflow error if the data is greater than 8 bytes long.
func ptrdecode(ptr []byte) (uint64, error) {
	tmp := make([]byte, 8)

	switch size := len(ptr); {
	case size == 0:
		return 0, nil
	case size > 8:
		return 0, errors.New("overflow")
	case size < 8:
		copy(tmp[8-len(ptr):], ptr)
	case size == 8:
		copy(tmp, ptr)
	}

	return binary.BigEndian.Uint64(tmp), nil
}

// hasGaps returns if the keys of a map of unsigned numbers has gaps.
// They must also start from 0 and go up to len-1
func hasGaps[U constraints.Unsigned](indices map[U]struct{}) bool {
	for i := U(0); i < U(len(indices)); i++ {
		if _, exists := indices[i]; !exists {
			return true
		}
	}

	return false
}

// contains returns if the elementptr is present in the array of pointers
func contains(array []engineio.ElementPtr, check engineio.ElementPtr) bool {
	for _, ptr := range array {
		if ptr == check {
			return true
		}
	}

	return false
}

// trimBytes returns trimmed []byte as variable sized []byte for the size required by the uint64
func trimBytes(bytearr []byte, n uint64) []byte {
	size := (bits.Len64(n) + 8 - 1) / 8

	return bytearr[8-size:]
}
