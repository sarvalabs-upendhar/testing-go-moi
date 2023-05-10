package types

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/mudra/kramaid"
)

type IxType int

const (
	IxInvalid IxType = iota
	IxValueTransfer
	IxFuelSupply

	IxAssetCreate
	IxAssetApprove
	IxAssetRevoke
	IxAssetMint
	IxAssetBurn

	IxLogicDeploy
	IxLogicInvoke
	IxLogicEnlist
	IxLogicInteract
	IxLogicUpgrade
)

var ixTypeToString = map[IxType]string{
	IxInvalid:       "IxInvalid",
	IxValueTransfer: "IxValueTransfer",
	IxFuelSupply:    "IxFuelSupply",
	IxAssetCreate:   "IxAssetCreate",
	IxAssetApprove:  "IxAssetApprove",
	IxAssetRevoke:   "IxAssetRevoke",
	IxAssetMint:     "IxAssetMint",
	IxAssetBurn:     "IxAssetBurn",
	IxLogicDeploy:   "IxLogicDeploy",
	IxLogicInvoke:   "IxLogicInvoke",
}

func (ixtype IxType) String() string {
	str, ok := ixTypeToString[ixtype]
	if !ok {
		return fmt.Sprintf("unknown ixn: %d", ixtype)
	}

	return str
}

type IxData struct {
	Input   IxInput
	Compute IxCompute
	Trust   IxTrust
}

func (ixData *IxData) Copy() IxData {
	return IxData{
		Input:   ixData.Input.Copy(),
		Compute: ixData.Compute.Copy(),
		Trust:   ixData.Trust.Copy(),
	}
}

type IxInput struct {
	Type  IxType `json:"type"`
	Nonce uint64 `json:"nonce"`

	Sender   Address `json:"sender"`
	Receiver Address `json:"receiver"`
	Payer    Address `json:"payer"`

	TransferValues  map[AssetID]*big.Int `json:"transfer_values"`
	PerceivedValues map[AssetID]*big.Int `json:"perceived_values"`
	PerceivedProofs []byte               `json:"perceived_proofs"`

	FuelLimit *big.Int `json:"fuel_limit"`
	FuelPrice *big.Int `json:"fuel_price"`

	Payload json.RawMessage `json:"payload"`
}

func (ixInput *IxInput) Copy() IxInput {
	input := *ixInput

	if ixInput.FuelLimit != nil {
		input.FuelLimit = new(big.Int).Set(ixInput.FuelLimit)
	}

	if ixInput.FuelPrice != nil {
		input.FuelPrice = new(big.Int).Set(ixInput.FuelPrice)
	}

	if len(ixInput.TransferValues) > 0 {
		input.TransferValues = make(map[AssetID]*big.Int)

		for k, v := range ixInput.TransferValues {
			input.TransferValues[k] = new(big.Int).Set(v)
		}
	}

	if len(ixInput.PerceivedValues) > 0 {
		input.PerceivedValues = make(map[AssetID]*big.Int)

		for k, v := range ixInput.PerceivedValues {
			input.PerceivedValues[k] = new(big.Int).Set(v)
		}
	}

	if len(ixInput.PerceivedProofs) > 0 {
		input.PerceivedProofs = make([]byte, len(ixInput.PerceivedProofs))
		copy(input.PerceivedProofs, ixInput.PerceivedProofs)
	}

	if len(ixInput.Payload) > 0 {
		input.Payload = make(json.RawMessage, len(ixInput.Payload))
		copy(input.Payload, ixInput.Payload)
	}

	return input
}

type IxCompute struct {
	Mode         uint64            `json:"mode"`
	Hash         Hash              `json:"hash"`
	ComputeNodes []kramaid.KramaID `json:"compute_nodes"`
}

func (ixCompute *IxCompute) Copy() IxCompute {
	compute := *ixCompute

	if len(ixCompute.ComputeNodes) > 0 {
		compute.ComputeNodes = make([]kramaid.KramaID, len(ixCompute.ComputeNodes))
		copy(compute.ComputeNodes, ixCompute.ComputeNodes)
	}

	return compute
}

type IxTrust struct {
	MTQ        uint              `json:"mtq"`
	TrustNodes []kramaid.KramaID `json:"trust_nodes"`
}

func (ixTrust *IxTrust) Copy() IxTrust {
	trust := IxTrust{
		MTQ: ixTrust.MTQ,
	}

	if len(ixTrust.TrustNodes) > 0 {
		trust.TrustNodes = make([]kramaid.KramaID, len(ixTrust.TrustNodes))
		copy(trust.TrustNodes, ixTrust.TrustNodes)
	}

	return trust
}

type Interaction struct {
	inner   IxData
	payload *IxPayload

	hash      atomic.Value
	size      atomic.Value
	signature atomic.Value
}

func NewInteraction(ixData IxData, signature []byte) *Interaction {
	cpyIxData := ixData.Copy()
	ix := &Interaction{inner: cpyIxData}
	ix.signature.Store(signature)

	data, err := polo.Polorize(cpyIxData)
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

func (ix Interaction) Input() IxInput {
	return ix.inner.Input.Copy()
}

func (ix Interaction) Compute() IxCompute {
	return ix.inner.Compute.Copy()
}

func (ix Interaction) Trust() IxTrust {
	return ix.inner.Trust.Copy()
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
	case IxLogicDeploy:
		return NewAccountAddress(ix.Nonce(), ix.Sender())

	case IxLogicInvoke:
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
	return ix.inner.Input.Copy().FuelPrice
}

func (ix Interaction) FuelLimit() *big.Int {
	return ix.inner.Input.Copy().FuelLimit
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
