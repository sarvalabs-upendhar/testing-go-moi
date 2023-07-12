package common

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

// ArrayOfBits is a thread-safe implementation of a bit array.
type ArrayOfBits struct {
	mtx      sync.Mutex `polo:"-"`
	Size     int        `json:"size"`
	Elements []uint64   `json:"elements"`
}

// NewArrayOfBits returns a new bit array.
func NewArrayOfBits(size int) *ArrayOfBits {
	if size <= 0 {
		return nil
	}

	return &ArrayOfBits{
		Size:     size,
		Elements: make([]uint64, numElements(size)),
	}
}

// SizeOf returns the number of bits in the bitarray
func (b *ArrayOfBits) SizeOf() int {
	if b == nil {
		return 0
	}

	return b.Size
}

// GetIndex returns true if the bit at the given index is set.
func (b *ArrayOfBits) GetIndex(i int) bool {
	if b == nil {
		return false
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	return b.getIndex(i)
}

func (b *ArrayOfBits) getIndex(index int) bool {
	if index >= b.Size {
		return false
	}

	return b.Elements[index/64]&(uint64(1)<<uint(index%64)) > 0
}

// SetIndex sets the bit at index i within the bit array.
func (b *ArrayOfBits) SetIndex(index int, v bool) bool {
	if b == nil {
		return false
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	return b.setIndex(index, v)
}

func (b *ArrayOfBits) setIndex(index int, v bool) bool {
	if index >= b.Size {
		return false
	}

	if v {
		b.Elements[index/64] |= uint64(1) << uint(index%64)
	} else {
		b.Elements[index/64] &= ^(uint64(1) << uint(index%64))
	}

	return true
}

// Copy returns a copy of the provided bit array.
func (b *ArrayOfBits) Copy() *ArrayOfBits {
	if b == nil {
		return nil
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	return b.copy()
}

func (b *ArrayOfBits) copy() *ArrayOfBits {
	var c []uint64

	if len(b.Elements) > 0 {
		c = make([]uint64, len(b.Elements))

		copy(c, b.Elements)
	}

	return &ArrayOfBits{
		Size:     b.Size,
		Elements: c,
	}
}

func (b *ArrayOfBits) copyBits(size int) *ArrayOfBits {
	c := make([]uint64, numElements(size))

	copy(c, b.Elements)

	return &ArrayOfBits{
		Size:     size,
		Elements: c,
	}
}

// Or returns a bit array resulting from a bitwise OR of the two bit arrays.
func (b *ArrayOfBits) Or(input *ArrayOfBits) *ArrayOfBits {
	if b == nil && input == nil {
		return nil
	}

	if b == nil && input != nil {
		return input.Copy()
	}

	if input == nil {
		return b.Copy()
	}

	b.mtx.Lock()
	input.mtx.Lock()

	c := b.copyBits(MaxInteger(b.Size, input.Size))

	smaller := MinInteger(len(b.Elements), len(input.Elements))
	for i := 0; i < smaller; i++ {
		c.Elements[i] |= input.Elements[i]
	}

	b.mtx.Unlock()
	input.mtx.Unlock()

	return c
}

// And returns a bit array resulting from a bitwise AND of the two bit arrays.
func (b *ArrayOfBits) And(input *ArrayOfBits) *ArrayOfBits {
	if b == nil || input == nil {
		return nil
	}

	b.mtx.Lock()
	input.mtx.Lock()

	defer func() {
		b.mtx.Unlock()
		input.mtx.Unlock()
	}()

	return b.and(input)
}

func (b *ArrayOfBits) and(input *ArrayOfBits) *ArrayOfBits {
	c := b.copyBits(MinInteger(b.Size, input.Size))

	for i := 0; i < len(c.Elements); i++ {
		c.Elements[i] &= input.Elements[i]
	}

	return c
}

// Not returns a bit array resulting from a bitwise Not of the provided bit array.
func (b *ArrayOfBits) Not() *ArrayOfBits {
	if b == nil {
		return nil
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	return b.not()
}

func (b *ArrayOfBits) not() *ArrayOfBits {
	c := b.copy()
	for i := 0; i < len(c.Elements); i++ {
		c.Elements[i] = ^c.Elements[i]
	}

	return c
}

// Subtract subtracts the two bit-arrays bitwise, without carrying the bits.
func (b *ArrayOfBits) Subtract(input *ArrayOfBits) *ArrayOfBits {
	if b == nil || input == nil {
		// TODO: Decide if we should do 1's complement here?
		return nil
	}

	b.mtx.Lock()
	input.mtx.Lock()

	c := b.copyBits(b.Size)
	smaller := MinInteger(len(b.Elements), len(input.Elements))

	for i := 0; i < smaller; i++ {
		c.Elements[i] &^= input.Elements[i]
	}

	b.mtx.Unlock()
	input.mtx.Unlock()

	return c
}

// IsEmpty returns true iff all bits in the bit array are 0
func (b *ArrayOfBits) IsEmpty() bool {
	if b == nil {
		return true // should this be opposite?
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	for _, e := range b.Elements {
		if e > 0 {
			return false
		}
	}

	return true
}

// IsFull returns true iff all bits in the bit array are 1.
func (b *ArrayOfBits) IsFull() bool {
	if b == nil {
		return true
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Check all elements except the last
	for _, elem := range b.Elements[:len(b.Elements)-1] {
		if (^elem) != 0 {
			return false
		}
	}

	// Check that the last element has (lastElemBits) 1's
	lastElemBits := (b.Size+63)%64 + 1
	lastElem := b.Elements[len(b.Elements)-1]

	return (lastElem+1)&((uint64(1)<<uint(lastElemBits))-1) == 0
}

// PickRandom returns a random index for a set bit in the bit array.
// If there is no such value, it returns 0, false.
// It uses the global randomness in `random.go` to get this index.
func (b *ArrayOfBits) PickRandom() (int, bool) {
	if b == nil {
		return 0, false
	}

	trueIndices := b.GetTrueIndices()

	if len(trueIndices) == 0 { // no bits set to true
		return 0, false
	}

	return trueIndices[rand.Intn(len(trueIndices))], true
}

func (b *ArrayOfBits) TrueIndicesSize() int {
	return len(b.GetTrueIndices())
}

func (b *ArrayOfBits) GetTrueIndices() []int {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	trueIndices := make([]int, 0, b.Size)
	curBit := 0
	numElements := len(b.Elements)
	// set all true indices
	for i := 0; i < numElements-1; i++ {
		elem := b.Elements[i]
		if elem == 0 {
			curBit += 64

			continue
		}

		for j := 0; j < 64; j++ {
			if (elem & (uint64(1) << uint64(j))) > 0 {
				trueIndices = append(trueIndices, curBit)
			}
			curBit++
		}
	}
	// handle last element
	lastElem := b.Elements[numElements-1]
	numFinalSize := b.Size - curBit

	for i := 0; i < numFinalSize; i++ {
		if (lastElem & (uint64(1) << uint64(i))) > 0 {
			trueIndices = append(trueIndices, curBit)
		}
		curBit++
	}

	return trueIndices
}

// String returns a string representation of ArrayOfBits: BA{<bit-string>},
// where <bit-string> is a sequence of 'x' (1) and '_' (0).
// The <bit-string> includes spaces and newlines to help people.
// For a simple sequence of 'x' and '_' characters with no spaces or newlines,
// see the MarshalJSON() method.
// Example: "BA{_x_}" or "nil-ArrayOfBits" for nil.
func (b *ArrayOfBits) String() string {
	return b.StringIndented("")
}

// StringIndented returns the same thing as String(), but applies the indent
// at every 10th bit, and twice at every 50th bit.
func (b *ArrayOfBits) StringIndented(indent string) string {
	if b == nil {
		return "nil-ArrayOfBits"
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	return b.stringForm(indent)
}

func (b *ArrayOfBits) stringForm(indent string) string {
	lines := []string{}
	size := ""

	for i := 0; i < b.Size; i++ {
		if b.getIndex(i) {
			size += "x"
		} else {
			size += "_"
		}

		if i%100 == 99 {
			lines = append(lines, size)
			size = ""
		}

		if i%10 == 9 {
			size += indent
		}

		if i%50 == 49 {
			size += indent
		}
	}

	if len(size) > 0 {
		lines = append(lines, size)
	}

	return fmt.Sprintf("BA{%v:%v}", b.Size, strings.Join(lines, indent))
}

// Bytes returns the byte representation of the bits within the ArrayOfBits.
func (b *ArrayOfBits) Bytes() []byte {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	numBytes := (b.Size + 7) / 8
	bytes := make([]byte, numBytes)

	for i := 0; i < len(b.Elements); i++ {
		elemBytes := [8]byte{}
		binary.LittleEndian.PutUint64(elemBytes[:], b.Elements[i])
		copy(bytes[i*8:], elemBytes[:])
	}

	return bytes
}

// Update sets the b's bits to be that of the other bit array.
func (b *ArrayOfBits) Update(inputbits *ArrayOfBits) {
	if b == nil || inputbits == nil {
		return
	}

	b.mtx.Lock()
	inputbits.mtx.Lock()
	copy(b.Elements, inputbits.Elements)
	inputbits.mtx.Unlock()
	b.mtx.Unlock()
}

func numElements(bits int) int {
	return (bits + 63) / 64
}

func MaxInteger(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func MinInteger(a, b int) int {
	if a < b {
		return a
	}

	return b
}
