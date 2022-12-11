package types

import (
	"crypto/rand"
	"log"
	"math/big"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/mudra/kramaid"
)

type IxType int

const (
	IxValueTransfer IxType = iota
	IxFuelSupply

	IxAssetCreate
	IxAssetApprove
	IxAssetRevoke
	IxAssetMint
	IxAssetBurn

	IxLogicDeploy
	IxLogicExecute
	IxLogicUpgrade

	IxFileCreate
	IxFileUpdate

	IxParticipantRegister
	IxValidatorRegister
	IxValidatorUnregister

	IxStakeBond
	IxStakeUnbond
	IxStakeTransfer
)

type Interaction struct {
	inner   IxData
	payload *IxPayload

	hash      atomic.Value
	size      atomic.Value
	signature atomic.Value
}

func NewInteraction(data IxData, signature []byte) *Interaction {
	return InteractionMessage{Data: data, Signature: signature}.ToInteraction()
}

func NewRandomHashInteraction() *Interaction {
	hash := make([]byte, 32)
	if _, err := rand.Read(hash); err != nil {
		return nil
	}

	v := atomic.Value{}
	v.Store(BytesToHash(hash))

	return &Interaction{hash: v}
}

type IxData struct {
	Input   IxInput
	Compute IxCompute
	Trust   IxTrust
}

type IxInput struct {
	Type  IxType
	Nonce uint64

	Sender   Address
	Receiver Address
	Payer    Address

	TransferValues  map[AssetID]*big.Int
	PerceivedValues map[AssetID]*big.Int
	PerceivedProofs []byte

	FuelLimit *big.Int
	FuelPrice *big.Int

	Payload []byte
}

type IxCompute struct {
	Mode  int
	Hash  []byte
	Nodes []kramaid.KramaID
}

type IxTrust struct {
	MTQ   uint
	Nodes []kramaid.KramaID
}

// Type returns the type of Interaction as an IxType
func (ix Interaction) Type() IxType {
	return ix.inner.Input.Type
}

// Sender returns the Address of the Interaction sender
func (ix Interaction) Sender() Address {
	return ix.inner.Input.Sender
}

// Receiver returns the Address of the Interaction receiver.
func (ix Interaction) Receiver() Address {
	return ix.inner.Input.Receiver
}

// Nonce returns the account nonce of the Interaction sender
func (ix Interaction) Nonce() uint64 {
	return ix.inner.Input.Nonce
}

// TransferValues returns the map of AssetID to transfer values
func (ix Interaction) TransferValues() map[AssetID]*big.Int {
	return ix.inner.Input.TransferValues
}

func (ix *Interaction) GetAssetPayload() (*AssetPayload, error) {
	// If payload has been decoded, return the asset form
	if ix.payload != nil {
		return ix.payload.asset, nil
	}

	// Decode the payload bytes from IxInput into an AssetPayload
	assetPayload := new(AssetPayload)
	if err := assetPayload.FromBytes(ix.inner.Input.Payload); err != nil {
		return nil, errors.Wrap(err, "invalid payload")
	}

	// Create a new IxPayload with an asset form
	ix.payload = &IxPayload{asset: assetPayload}
	// Return the AssetPayload
	return assetPayload, nil
}

func (ix Interaction) FuelPrice() *big.Int {
	return ix.inner.Input.FuelPrice
}

func (ix Interaction) FuelLimit() *big.Int {
	return ix.inner.Input.FuelLimit
}

func (ix Interaction) FuelPriceCmp(other *Interaction) int {
	return ix.FuelPrice().Cmp(other.FuelPrice())
}

func (ix Interaction) FuelPriceIntCmp(other *big.Int) int {
	return ix.FuelPrice().Cmp(other)
}

func (ix Interaction) Cost() *big.Int {
	return new(big.Int).Mul(ix.FuelPrice(), ix.FuelLimit())
}

func (ix Interaction) IsUnderpriced(priceLimit *big.Int) bool {
	return ix.FuelPrice().Cmp(priceLimit) < 0
}

func (ix *Interaction) Hash() Hash {
	if hash := ix.hash.Load(); hash != nil {
		return hash.(Hash) //nolint:forcetypeassert
	}

	hash, err := PoloHash(ix.ToMessage())
	if err != nil {
		return NilHash
	}

	ix.hash.Store(hash)

	return hash
}

func (ix *Interaction) Size() (uint64, error) {
	if size := ix.size.Load(); size != nil {
		return size.(uint64), nil //nolint:forcetypeassert
	}

	data, err := polo.Polorize(ix.ToMessage())
	if err != nil {
		return 0, errors.Wrap(err, "failed to polorize interaction")
	}

	size := uint64(len(data))
	ix.size.Store(size)

	return size, err
}

func (ix Interaction) ToMessage() InteractionMessage {
	if sig, ok := ix.signature.Load().([]byte); ok {
		return InteractionMessage{Data: ix.inner, Signature: sig}
	}

	return InteractionMessage{Data: ix.inner}
}

type InteractionMessage struct {
	Data      IxData
	Signature []byte
}

func (ixmsg InteractionMessage) Bytes() ([]byte, error) {
	data, err := polo.Polorize(ixmsg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize interaction message")
	}

	return data, nil
}

func (ixmsg *InteractionMessage) FromBytes(data []byte) error {
	if err := polo.Depolorize(ixmsg, data); err != nil {
		return errors.Wrap(err, "failed to depolorize interaction message")
	}

	return nil
}

func (ixmsg InteractionMessage) ToInteraction() *Interaction {
	ix := &Interaction{inner: ixmsg.Data}
	ix.signature.Store(ixmsg.Signature)

	data, err := polo.Polorize(ixmsg.Data)
	if err != nil {
		log.Fatalln(err, "failed to generate bytes of interaction message")

		return nil
	}

	ix.hash.Store(GetHash(data))
	ix.size.Store(uint64(len(data) + len(ixmsg.Signature)))

	return ix
}

// Interactions are array of Transactions
type Interactions []*Interaction

// Bytes returns the POLO serialized bytes of all Interactions
func (ixs Interactions) Bytes() ([]byte, error) {
	packer := polo.NewPacker()
	for _, ix := range ixs {
		if err := packer.Pack(ix.ToMessage()); err != nil {
			return nil, errors.Wrap(err, "failed to pack interactions")
		}
	}

	return packer.Bytes(), nil
}

// FromBytes decodes the POLO serialized bytes into Interactions
func (ixs *Interactions) FromBytes(bytes []byte) error {
	unpacker, err := polo.NewUnpacker(bytes)
	if err != nil {
		return err
	}

	for !unpacker.Done() {
		ixMsg := new(InteractionMessage)
		if err = unpacker.Unpack(ixMsg); err != nil {
			return errors.Wrap(err, "failed to unpack interactions")
		}

		*ixs = append(*ixs, ixMsg.ToInteraction())
	}

	return nil
}

// Hash returns the hash of all Interactions
func (ixs Interactions) Hash() (Hash, error) {
	data, err := ixs.Bytes()
	if err != nil {
		return NilHash, err
	}

	return GetHash(data), nil
}

type IxByNonce Interactions

func (s IxByNonce) Len() int           { return len(s) }
func (s IxByNonce) Less(i, j int) bool { return s[i].Nonce() < s[j].Nonce() }
func (s IxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
