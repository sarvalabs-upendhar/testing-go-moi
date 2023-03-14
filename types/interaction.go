package types

import (
	"crypto/rand"
	"encoding/json"
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

func NewInteraction(ixData IxData, signature []byte) *Interaction {
	ix := &Interaction{inner: ixData}
	ix.signature.Store(signature)

	data, err := polo.Polorize(ixData)
	if err != nil {
		log.Fatalln(err, "failed to generate bytes of interaction message")

		return nil
	}

	ix.hash.Store(GetHash(data))
	ix.size.Store(uint64(len(data) + len(signature)))

	return ix
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

	Payload json.RawMessage
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

func (ix Interaction) Input() IxInput {
	return ix.inner.Input
}

func (ix Interaction) Compute() IxCompute {
	return ix.inner.Compute
}

func (ix Interaction) Trust() IxTrust {
	return ix.inner.Trust
}

func (ix Interaction) Signature() []byte {
	signature, ok := ix.signature.Load().([]byte)
	if !ok {
		panic("invalid data stored into interaction signature")
	}

	return signature
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
	// Based on the interaction type return the address
	switch ix.Type() {
	case IxLogicExecute, IxLogicDeploy:
		payload, err := ix.GetLogicPayload()
		if err != nil {
			panic(err)
		}

		return payload.Logic.Address()
	default:
		return ix.inner.Input.Receiver
	}
}

// Nonce returns the account nonce of the Interaction sender
func (ix Interaction) Nonce() uint64 {
	return ix.inner.Input.Nonce
}

// TransferValues returns the map of AssetID to transfer values
func (ix Interaction) TransferValues() map[AssetID]*big.Int {
	return ix.inner.Input.TransferValues
}

// Payload returns the interaction payload
func (ix Interaction) Payload() []byte {
	return ix.inner.Input.Payload
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

func (ix *Interaction) GetLogicPayload() (*LogicPayload, error) {
	// If payload has been decoded, return the logic form
	if ix.payload != nil {
		return ix.payload.logic, nil
	}

	// Decode the payload bytes from IxInput into an LogicPayload
	logicPayload := new(LogicPayload)
	if err := logicPayload.FromBytes(ix.inner.Input.Payload); err != nil {
		return nil, errors.Wrap(err, "invalid payload")
	}

	// Create a new IxPayload with an asset form
	ix.payload = &IxPayload{logic: logicPayload}
	// Return the AssetPayload
	return logicPayload, nil
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

	hash, err := PoloHash(ix)
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

	data, err := ix.Bytes()
	if err != nil {
		return 0, errors.Wrap(err, "failed to polorize interaction")
	}

	size := uint64(len(data))
	ix.size.Store(size)

	return size, err
}

func (ix Interaction) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	if err := polorizer.Polorize(ix.inner); err != nil {
		return nil, errors.Wrap(err, "failed to polorize interaction data")
	}

	sig, ok := ix.signature.Load().([]byte)
	if !ok {
		panic("invalid data stored into interaction signature")
	}

	polorizer.PolorizeBytes(sig)

	return polorizer, nil
}

func (ix *Interaction) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if err != nil {
		return errors.Wrap(err, "failed to depolorize interaction")
	}

	data := new(IxData)
	if err = depolorizer.Depolorize(data); err != nil {
		return errors.Wrap(err, "failed to depolorize interaction data")
	}

	sig, err := depolorizer.DepolorizeBytes()
	if err != nil {
		return errors.Wrap(err, "failed to depolorize interaction signature")
	}

	*ix = *NewInteraction(*data, sig)

	return nil
}

func (ix Interaction) Bytes() ([]byte, error) {
	polorizer, err := ix.Polorize()
	if err != nil {
		return nil, err
	}

	return polorizer.Bytes(), nil
}

func (ix *Interaction) FromBytes(data []byte) error {
	depolorizer, err := polo.NewDepolorizer(data)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize interaction")
	}

	if err = ix.Depolorize(depolorizer); err != nil {
		return err
	}

	return nil
}

// Interactions are array of Transactions
type Interactions []*Interaction

// Bytes returns the POLO serialized bytes of all Interactions
func (ixs Interactions) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ixs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize interactions")
	}

	return rawData, nil
}

// FromBytes decodes the POLO serialized bytes into Interactions
func (ixs *Interactions) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ixs, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize interactions")
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
