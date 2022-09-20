package ktypes

/*
Most of the types in this file are yet to be finalized.All the below structs are temporary type definitions
*/
import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/ipfs/go-cid"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"golang.org/x/crypto/blake2b"
	"math/big"
	"math/rand"
	"strings"
	"sync"
)

const (
	//AddressLength is the length of account address
	AddressLength            = 32
	CIDPrefixVersion  uint64 = 1
	CIDPrefixCodec    uint64 = 0x50
	CIDPrefixMhType   uint64 = 0xb220
	CIDPrefixMhLength int    = -1
)
const (
	SargaAccount AccType = iota
	RegularAccount
)
const (
	Sender ParticipantRole = iota
	Receiver
	Genesis
)
const (
	ValueTransfer IxType = iota
	AssetCreation
)

var (
	NilAddress Address
	NilHash    Hash
)

type ParticipantRole int

// IxType ...
type IxType int

// AccType ...
type AccType int

// Accounts ...
type Accounts []*AccountMetaInfo

// ClusterID ...
type ClusterID string

//AssetID ...
type AssetID string

// Hash represents the 32 byte hash of arbitrary data.
type Hash [32]byte

// Address represents the 32 byte address of an MOI account.
type Address [32]byte

func (c ClusterID) String() string {
	return string(c)
}

func (c ClusterID) Hash() Hash {
	rawHash, err := base58.Decode(c.String())
	if err != nil {
		return NilHash
	}

	return BytesToHash(rawHash)
}
func (a Address) String() string {
	if a == NilAddress {
		return "nil address"
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

type ContextLockInfo struct {
	ContextHash   Hash
	Height        uint64
	TesseractHash Hash
}

type AccDetailsQueue struct {
	queue []*AccountMetaInfo
	lock  sync.RWMutex
}

func (a *AccDetailsQueue) Push(data []*AccountMetaInfo) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.queue = append(a.queue, data...)
}

func (a *AccDetailsQueue) Pop() (*AccountMetaInfo, error) {
	if len(a.queue) > 0 {
		a.lock.Lock()
		defer a.lock.Unlock()

		data := a.queue[0]
		a.queue = a.queue[1:]

		return data, nil
	}

	return nil, errors.New("Queue is empty")
}

func (a *AccDetailsQueue) Len() int {
	a.lock.Lock()
	defer a.lock.Unlock()

	return len(a.queue)
}

func (acc *Accounts) Bytes() []byte {
	return polo.Polorize(acc)
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
	if h == NilHash {
		return "nil hash"
	}

	return h.Hex()
}

func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-32:]
	}

	copy(h[32-len(b):], b)
}
func (h Hash) Bytes() []byte { return h[:] }
func (h Hash) Hex() string   { return BytesToHex(h.Bytes()) }
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

// Interaction ...
type Interaction struct {
	Data IxData
	Hash Hash
	Size int64 `polo_skip:"true"`
}

type InteractionInput struct {
	Type IxType

	Nonce uint64

	From Address

	To Address

	Payer Address

	TransferValue map[AssetID]uint64

	PerceivedValue map[AssetID]uint64

	AnuLimit uint64

	AnuPrice uint64 `json:"interaction_data_input_anu_price"`

	Proof ProofData `json:"interaction_data_input_proof"`

	Payload InteractionInputPayload `json:"interaction_data_input_payload"`
}
type ProofData struct {
	ProtocolID int `json:"interaction_data_input_proof_protocolid"`

	ProofType int `json:"interaction_data_input_proof_type"`

	ProofData []byte `json:"interaction_data_input_proof_data"`
}

type InteractionInputPayload struct {
	Init []byte `json:"init"`

	Data []byte `json:"data"`

	LogicAddress Address

	File FileDataInput

	AssetData AssetDataInput

	ApprovalData ApprovalDataInput
}
type AssetDataInput struct {
	Dimension int `json:"asset_data_input_total_dimension"`

	TotalSupply uint64 `json:"asset_data_input_total_supply"`

	Symbol string `json:"asset_data_input_symbol"`

	Code []byte

	IsFungible bool
	IsMintable bool
}
type ApprovalDataInput struct {
	Operator Address

	Approvals map[AssetID]uint64
}

type FileDataInput struct {
	Name string `json:"file_data_name"`

	Hash string `json:"file_data_hash"`

	Nodes []string `json:"file_data_nodes"`

	File []byte `json:"file_data_file"`
}
type InteractionCompute struct {
	ComputeMode int `json:"interaction_data_compute_mode"`

	ComputationalNodes []id.KramaID `json:"interaction_data_compute_value_nodes"`

	ComputationalHash []byte `json:"interaction_data_compute_value_hash"`
}
type InteractionTrust struct {
	ConsensusNodes []id.KramaID `json:"interaction_data_compute_value_consensus_nodes"`

	MTQ uint `json:"interaction_data_compute_value_mtq"`
}

// IxData has the complete information of a interaction which is signed by user
type IxData struct {
	Input     InteractionInput
	Compute   InteractionCompute
	Trust     InteractionTrust
	Signature []byte
}

//Interactions are array of Transactions
type Interactions []*Interaction

func (is Interactions) Bytes() []byte {
	return polo.Polorize(is)
}

func (is Interactions) Hash() Hash {
	return PoloHash(is)
}

func (ix *Interaction) GetSize() int64 {
	//FIXME: size should calculated after signature integration
	return ix.Size
}
func (ix *Interaction) GetAssetCreationPayload() *AssetDataInput {
	return &ix.Data.Input.Payload.AssetData
}
func (ix *Interaction) IxType() IxType {
	return ix.Data.Input.Type
}
func (ix *Interaction) GetIxHash() Hash {
	if ix.Hash == NilHash {
		h := PoloHash(ix)
		ix.Hash = h

		return h
	}

	return ix.Hash
}
func (ix *Interaction) Sign(prv *ecdsa.PrivateKey) error {
	//h := ix.GetIxHash()
	//sig, err := kcrypto.Sign(h[:], prv)
	sig, err := make([]byte, 0), errors.New("nil")
	if err != nil {
		return err
	}

	return ix.SetSignatureValues(sig)
}

func (ix *Interaction) SetSignatureValues(sig []byte) error {
	ix.Data.Signature = sig

	return nil
}
func (ix *Interaction) FromAddress() Address {
	return ix.Data.Input.From
}
func (ix *Interaction) ToAddress() Address {
	return ix.Data.Input.To
}

// Nonce returns the account nonce of the transaction
func (ix *Interaction) Nonce() uint64 { return ix.Data.Input.Nonce }

func (ix *Interaction) GasPrice() *big.Int { return new(big.Int).SetUint64(ix.Data.Input.AnuPrice) }

func (ix *Interaction) GasPriceCmp(other *Interaction) int {
	return new(big.Int).SetUint64(ix.Data.Input.AnuPrice).Cmp(new(big.Int).SetUint64(other.Data.Input.AnuPrice))
}
func (ix *Interaction) Gas() uint64 { return ix.Data.Input.AnuLimit }

func (ix *Interaction) GasPriceIntCmp(other *big.Int) int {
	return new(big.Int).SetUint64(ix.Data.Input.AnuPrice).Cmp(other)
}

func (ix *Interaction) Cost() *big.Int {
	total := new(big.Int).Mul(
		new(big.Int).SetUint64(ix.Data.Input.AnuPrice),
		new(big.Int).SetUint64(ix.Data.Input.AnuLimit),
	)

	return total
}

func (ix *Interaction) IsUnderpriced(priceLimit uint64) bool {
	return ix.GasPrice().Cmp(big.NewInt(0).SetUint64(priceLimit)) < 0
}

type IxByNonce Interactions

func (s IxByNonce) Len() int           { return len(s) }
func (s IxByNonce) Less(i, j int) bool { return s[i].Data.Input.Nonce < s[j].Data.Input.Nonce }
func (s IxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func ToKIPPeerID(nodes []string) []id.KramaID {
	ids := make([]id.KramaID, 0, len(nodes))

	for _, v := range nodes {
		ids = append(ids, id.KramaID(v))
	}

	return ids
}
func KIPPeerIDToString(peers []id.KramaID) []string {
	ids := make([]string, 0, len(peers))

	for _, v := range peers {
		ids = append(ids, string(v))
	}

	return ids
}

// BitArray is a thread-safe implementation of a bit array.
type ArrayOfBits struct {
	mtx      sync.Mutex `polo_skip:"true"`
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

	return trueIndices[rand.Intn(len(trueIndices))], true //nolint
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
func PoloHash(x interface{}) Hash {
	bytes := polo.Polorize(x)
	sum := blake2b.Sum256(bytes)
	h := BytesToHash(sum[:])
	//h := sha256.Sum256(bytes)

	return h
}

type TesseractResponse struct {
	Data  []byte
	Delta map[Hash][]byte
}

func HashToCid(hash Hash) (cid.Cid, error) {
	multiHash, err := multihash.Encode(hash.Bytes(), CIDPrefixMhType)
	if err != nil {
		return cid.Undef, err
	}

	switch CIDPrefixVersion {
	case 0:
		return cid.NewCidV0(multiHash), nil
	case 1:
		return cid.NewCidV1(CIDPrefixCodec, multiHash), nil
	default:
		return cid.Undef, fmt.Errorf("invalid cid version")
	}
}

func GetHash(data []byte) Hash {
	return blake2b.Sum256(data)
}

// KnownCache is a cache for known hashes.
type KnownCache struct {
	hashes mapset.Set
	max    int
}

// NewKnownCache creates a new knownCache with a max capacity.
func NewKnownCache(max int) *KnownCache {
	return &KnownCache{
		max:    max,
		hashes: mapset.NewSet(),
	}
}

// Add adds a list of elements to the set.
func (k *KnownCache) Add(data ...interface{}) {
	for k.hashes.Cardinality() > max(0, k.max-len(data)) {
		k.hashes.Pop()
	}

	for _, hash := range data {
		k.hashes.Add(hash)
	}
}

// Contains returns whether the given item is in the set.
func (k *KnownCache) Contains(data interface{}) bool {
	return k.hashes.Contains(data)
}

// Cardinality returns the number of elements in the set.
func (k *KnownCache) Cardinality() int {
	return k.hashes.Cardinality()
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

type BitSet struct {
	Size     int
	Elements []uint64
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

type DBEntry struct {
	Key   []byte
	Value []byte
}

type AccountGenesisInfo struct {
	MoiID  string
	IxHash Hash
}

type Receipt struct {
	IxType        int
	IxHash        Hash
	GasUsed       uint64
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

func (rs Receipts) Hash() Hash {
	return PoloHash(rs)
}

func (rs Receipts) GetReceipt(ixHash Hash) (*Receipt, error) {
	if receipt, ok := rs[ixHash]; ok {
		return receipt, nil
	}

	return nil, ErrReceiptNotFound
}

//func ComputeReceiptsHash(rs []*Receipt) Hash {
//
//	var receipts Receipts
//	for _, v := range rs {
//		receipts = append(receipts, v)
//	}
//
//	return PoloHash(receipts)
//}
func AccTypeFromIxType(ixType IxType) AccType {
	switch ixType {
	default:
		return RegularAccount
	}
}
