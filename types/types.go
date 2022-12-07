package types

/*
Most of the types in this file are yet to be finalized.All the below structs are temporary type definitions
*/
import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"
)

const (
	// AddressLength is the length of account address
	AddressLength = 32

	HashLength = 32
)

const (
	RegularAccount AccType = iota
	SargaAccount
	ContractAccount
)

var (
	NilAddress Address
	NilHash    Hash
)

type ParticipantRole int

// AccType ...
type AccType int

// Accounts ...
type Accounts []*AccountMetaInfo

// Hash represents the 32 byte hash of arbitrary data.
type Hash [HashLength]byte

// Address represents the 32 byte address of an MOI account.
type Address [AddressLength]byte

func (a Address) IsNil() bool {
	return a == NilAddress
}

func (a Address) String() string {
	if a == NilAddress {
		return ""
	}

	return a.Hex()
}
func (a Address) Bytes() []byte { return a[:] }

// SetBytes sets the address to the value of b.
func (a *Address) SetBytes(b []byte) {
	if len(b) > len(a) {
		b = b[len(b)-AddressLength:]
	}

	copy(a[AddressLength-len(b):], b)
}

func (a Address) MarshalText() ([]byte, error) {
	result := make([]byte, len(a)*2)
	hex.Encode(result, a.Bytes())

	return result, nil
}

// Hex return the Hex representation of the Address
func (a Address) Hex() string {
	return "0x" + hex.EncodeToString(a[:])
}

// BytesToAddress returns the address from b
func BytesToAddress(b []byte) Address {
	var a Address

	a.SetBytes(b)

	return a
}

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

// HexToAddress converts string to Address
func HexToAddress(s string) Address {
	return BytesToAddress(FromHex(s))
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

// BitArray is a thread-safe implementation of a bit array.
type ArrayOfBits struct {
	mtx      sync.Mutex `polo:"-"`
	Size     int        `json:"size"`
	Elements []uint64   `json:"elements"`
}

// NewBitArray returns a new bit array.
func NewArrayOfBits(size int) *ArrayOfBits {
	if size <= 0 {
		return nil
	}

	return &ArrayOfBits{
		Size:     size,
		Elements: make([]uint64, numElements(size)),
	}
}

// Size returns the number of bits in the bitarray
func (b *ArrayOfBits) SizeOf() int {
	if b == nil {
		return 0
	}

	return b.Size
}

// GetIndex returns the bit at index i within the bit array.
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
	c := make([]uint64, len(b.Elements))

	copy(c, b.Elements)

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

	b.mtx.Lock()
	trueIndices := b.GetTrueIndices()
	b.mtx.Unlock()

	if len(trueIndices) == 0 { // no bits set to true
		return 0, false
	}

	return trueIndices[rand.Intn(len(trueIndices))], true
}

func (b *ArrayOfBits) TrueIndicesSize() int {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	return len(b.GetTrueIndices())
}

func (b *ArrayOfBits) GetTrueIndices() []int {
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

type Account struct {
	Nonce          uint64
	AccType        AccType
	Balance        Hash
	AssetApprovals Hash
	ContextHash    Hash
	StorageRoot    Hash
	LogicRoot      Hash
	FileRoot       Hash
}

func (a *Account) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(a)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account")
	}

	return rawData, nil
}

func (a *Account) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(a, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account")
	}

	return nil
}

type DBEntry struct {
	Key   []byte
	Value []byte
}

type AccountGenesisInfo struct {
	MoiID  string
	IxHash Hash
}

func (agi *AccountGenesisInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(agi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize genesis account info")
	}

	return rawData, nil
}

func (agi *AccountGenesisInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(agi, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize genesis account info")
	}

	return nil
}

type Receipt struct {
	IxType        int
	IxHash        Hash
	FuelUsed      uint64
	StateHashes   map[Address]Hash
	ContextHashes map[Address]Hash
	ExtraData     json.RawMessage
}

type AssetCreationReceipt struct {
	AssetID string `json:"asset_id"`
}

func (r *Receipt) SetExtraData(data interface{}) error {
	rawData, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(errors.New("Receipt generation failed"), err.Error())
	}

	r.ExtraData = rawData

	return nil
}

type Receipts map[Hash]*Receipt

func (rs Receipts) Hash() (Hash, error) {
	hash, err := PoloHash(rs)
	if err != nil {
		return NilHash, errors.Wrap(err, "failed to polorize receipts")
	}

	return hash, nil
}

func (rs Receipts) GetReceipt(ixHash Hash) (*Receipt, error) {
	if receipt, ok := rs[ixHash]; ok {
		return receipt, nil
	}

	return nil, ErrReceiptNotFound
}

func (rs Receipts) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(rs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize receipts")
	}

	return rawData, nil
}

func (rs *Receipts) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rs, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize receipts")
	}

	return nil
}

func AccTypeFromIxType(ixType IxType) AccType {
	switch ixType {
	case 2:
		return ContractAccount
	default:
		return RegularAccount
	}
}

func (acc *Accounts) Bytes() ([]byte, error) {
	return polo.Polorize(acc)
}
