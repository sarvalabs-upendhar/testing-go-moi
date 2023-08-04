package common

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/pkg/errors"
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

// SendIXArgs is an argument wrapper for sending Interactions to the pool
type SendIXArgs struct {
	Type  IxType `json:"type"`
	Nonce uint64 `json:"nonce"`

	Sender   Address `json:"sender"`
	Receiver Address `json:"receiver"`
	Payer    Address `json:"payer"`

	TransferValues  map[AssetID]*big.Int `json:"transfer_values"`
	PerceivedValues map[AssetID]*big.Int `json:"perceived_values"`

	FuelPrice *big.Int `json:"fuel_price"`
	FuelLimit *big.Int `json:"fuel_limit"`

	Payload []byte `json:"payload"`
}

func (args *SendIXArgs) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize send ix args")
	}

	return rawData, nil
}

func (args *SendIXArgs) FromBytes(bytes []byte) error {
	err := polo.Depolorize(args, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize send ix args")
	}

	return nil
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

	Payload []byte `json:"payload"`
}

func SendIxArgsFromIxData(ixData IxData) SendIXArgs {
	return SendIXArgs{
		Type:            ixData.Input.Type,
		Nonce:           ixData.Input.Nonce,
		Sender:          ixData.Input.Sender,
		Receiver:        ixData.Input.Receiver,
		Payer:           ixData.Input.Payer,
		TransferValues:  ixData.Input.TransferValues,
		PerceivedValues: ixData.Input.PerceivedValues,
		FuelLimit:       ixData.Input.FuelLimit,
		FuelPrice:       ixData.Input.FuelPrice,
		Payload:         ixData.Input.Payload,
	}
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
		input.Payload = make([]byte, len(ixInput.Payload))
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

// TODO: We need to generalise the ixHash logic

func NewInteraction(ixData IxData, signature []byte) (*Interaction, error) {
	cpyIxData := ixData.Copy()
	ix := &Interaction{inner: cpyIxData}
	ix.signature.Store(signature)

	data, err := polo.Polorize(cpyIxData)
	if err != nil {
		return nil, err
	}

	switch ixData.Input.Type {
	case IxValueTransfer:
		break
	case IxAssetCreate:
		assetCreatePayload := new(AssetCreatePayload)
		if err = assetCreatePayload.FromBytes(ixData.Input.Payload); err != nil {
			return nil, err
		}

		ix.payload = &IxPayload{
			asset: &AssetPayload{
				Create: assetCreatePayload,
			},
		}

	case IxAssetMint, IxAssetBurn:
		assetMintOrBurnPayload := new(AssetMintOrBurnPayload)
		if err = assetMintOrBurnPayload.FromBytes(ixData.Input.Payload); err != nil {
			return nil, err
		}

		ix.payload = &IxPayload{
			asset: &AssetPayload{
				Mint: assetMintOrBurnPayload,
			},
		}

	case IxLogicDeploy, IxLogicInvoke:
		logicPayload := new(LogicPayload)
		if err = logicPayload.FromBytes(ixData.Input.Payload); err != nil {
			return nil, err
		}

		ix.payload = &IxPayload{
			logic: logicPayload,
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	ix.hash.Store(GetHash(data))
	ix.size.Store(uint64(len(data) + len(signature)))

	return ix, nil
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

func (ix Interaction) IXData() IxData {
	return ix.inner.Copy()
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

// Payer returns the Address of the Interaction sender
func (ix Interaction) Payer() Address {
	return ix.inner.Input.Payer
}

// Receiver returns the Address of the Interaction receiver.
func (ix Interaction) Receiver() Address {
	// Based on the interaction type return the address
	switch ix.Type() {
	case IxAssetCreate:
		return NewAccountAddress(ix.Nonce(), ix.Sender())
	case IxAssetMint, IxAssetBurn:
		payload, err := ix.GetAssetPayload()
		if err != nil {
			panic(err)
		}

		return payload.Mint.Asset.Address()
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

// PerceivedValues returns the map of AssetID to transfer values
func (ix Interaction) PerceivedValues() map[AssetID]*big.Int {
	return ix.inner.Input.PerceivedValues
}

// TransferValues returns the map of AssetID to transfer values
func (ix Interaction) TransferValues() map[AssetID]*big.Int {
	transferValues := make(map[AssetID]*big.Int)
	for assetID, amount := range ix.inner.Input.TransferValues {
		transferValues[assetID] = new(big.Int).Set(amount)
	}

	return transferValues
}

func (ix Interaction) MOITokenValue() *big.Int {
	// Retrieve the transfer values
	values := ix.TransferValues()
	// Return the value for the MOI Token if it exists in the transfer values
	if value, ok := values[KMOITokenAssetID]; ok {
		return value
	}
	// Return a 0 value for no MOI Token in transfer values
	return big.NewInt(0)
}

// Payload returns the interaction payload
func (ix Interaction) Payload() []byte {
	return ix.inner.Input.Payload
}

func (ix *Interaction) GetAssetPayload() (*AssetPayload, error) {
	// If payload has been decoded, return the asset form
	if ix.payload != nil && ix.payload.asset != nil {
		return ix.payload.asset, nil
	}

	return nil, errors.New("payload not found")
}

func (ix *Interaction) GetLogicPayload() (*LogicPayload, error) {
	// If payload has been decoded, return the logic form
	if ix.payload != nil && ix.payload.logic != nil {
		return ix.payload.logic, nil
	}

	return nil, errors.New("payload not found")
}

func (ix Interaction) FuelPrice() *big.Int {
	return new(big.Int).Set(ix.inner.Input.FuelPrice)
}

func (ix Interaction) FuelLimit() *big.Int {
	return new(big.Int).Set(ix.inner.Input.FuelLimit)
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
	return ix.FuelPrice().Cmp(priceLimit) != 0
}

func (ix *Interaction) Hash() Hash {
	if hash := ix.hash.Load(); hash != nil {
		return hash.(Hash) //nolint:forcetypeassert
	}

	hash, err := PoloHash(ix.inner)
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

	ixn, err := NewInteraction(*data, sig)
	if err != nil {
		return err
	}

	*ix = *ixn

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

func (ix *Interaction) PayloadForSignature() ([]byte, error) {
	return polo.Polorize(SendIxArgsFromIxData(ix.inner))
}

func (ix *Interaction) Callsite() string {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return ""
	}

	return payload.Callsite
}

func (ix *Interaction) Calldata() []byte {
	payload, err := ix.GetLogicPayload()
	if err != nil {
		return nil
	}

	return payload.Calldata
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

func (ixs Interactions) FuelLimit() *big.Int {
	limit := new(big.Int)

	// Aggregate the fuel limit for all interactions
	for _, ix := range ixs {
		limit = new(big.Int).Add(limit, ix.inner.Input.FuelLimit)
	}

	return limit
}

type IxByNonce Interactions

func (s IxByNonce) Len() int           { return len(s) }
func (s IxByNonce) Less(i, j int) bool { return s[i].Nonce() < s[j].Nonce() }
func (s IxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
