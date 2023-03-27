package runtime

import (
	"encoding/binary"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

// IsExportedName returns whether a given string represents an exported label.
// The first letter of the string must be unicode upper case character for this to be true
func IsExportedName(str string) bool {
	return unicode.IsUpper(rune(str[0]))
}

func IsMutableName(str string) bool {
	return strings.HasSuffix(str, "!")
}

func IsPayableName(str string) bool {
	return strings.HasSuffix(str, "$")
}

// decipher interprets a slice of bytes into a 64-bit unsigned integer
// Returns an overflow error if the data is greater than 8 bytes long.
func ptrdecode(d []byte) (uint64, error) {
	tmp := make([]byte, 8)

	switch size := len(d); {
	case size == 0:
		return 0, nil
	case size > 8:
		return 0, errors.New("overflow")
	case size < 8:
		copy(tmp[8-len(d):], d)
	case size == 8:
		copy(tmp, d)
	}

	return binary.BigEndian.Uint64(tmp), nil
}

func SlotHash(slot uint8) []byte {
	hash := blake2b.Sum256([]byte{slot})

	return hash[:]
}
