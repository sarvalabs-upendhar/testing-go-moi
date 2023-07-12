package pisa

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func must[T any](object T, err error) T {
	if err != nil {
		panic(err)
	}

	return object
}

func TestSlotHash(t *testing.T) {
	hash := SlotHash(33)
	assert.Equal(t, []byte{
		0x2, 0x68, 0xbe, 0x9d, 0xbd, 0x4, 0x46, 0xea, 0xa2, 0x17, 0xe1, 0xde, 0xc8, 0xf3, 0x99, 0x24,
		0x93, 0x5, 0xe5, 0x51, 0xd7, 0xfc, 0x14, 0x37, 0xdd, 0x84, 0x52, 0x1f, 0x74, 0xaa, 0x62, 0x1c,
	}, hash)
}

func TestIsExportedName(t *testing.T) {
	assert.True(t, isExportedName("MyName"))
	assert.False(t, isExportedName("myName"))
}

func TestIsMutableName(t *testing.T) {
	assert.True(t, isMutableName("myVar!"))
	assert.False(t, isMutableName("myVar"))
}

func TestIsPayableName(t *testing.T) {
	assert.True(t, isPayableName("myVar$"))
	assert.False(t, isPayableName("myVar"))
}

func TestPtrdecode(t *testing.T) {
	testCases := []struct {
		name  string
		input []byte
		value uint64
		err   error
	}{
		{
			name:  "empty slice",
			input: []byte{},
			value: 0,
			err:   nil,
		},
		{
			name:  "8 bytes",
			input: []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
			value: 1311768467463790320,
			err:   nil,
		},
		{
			name:  "less than 8 bytes",
			input: []byte{0x12, 0x34, 0x56},
			value: 0x123456,
			err:   nil,
		},
		{
			name:  "more than 8 bytes",
			input: []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11},
			value: 0,
			err:   errors.New("overflow"),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			value, err := ptrdecode(test.input)

			assert.Equal(t, test.value, value)
			if test.err == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.err.Error())
			}
		})
	}
}

func TestHasGaps(t *testing.T) {
	indices := make(map[uint8]struct{})

	for i := uint8(0); i < 10; i++ {
		indices[i] = struct{}{}
	}

	assert.False(t, hasGaps(indices))
	delete(indices, 3)
	assert.True(t, hasGaps(indices))
}
