package types

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/mudra/kramaid"
)

const (
	ValueTransfer IxType = iota
	AssetCreation
)

// IxType ...
type IxType int

// Interaction ...
type Interaction struct {
	Data IxData
	Hash Hash
	Size int64 `polo:"-"`
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

	ComputationalNodes []kramaid.KramaID `json:"interaction_data_compute_value_nodes"`

	ComputationalHash []byte `json:"interaction_data_compute_value_hash"`
}

type InteractionTrust struct {
	ConsensusNodes []kramaid.KramaID `json:"interaction_data_compute_value_consensus_nodes"`

	MTQ uint `json:"interaction_data_compute_value_mtq"`
}

// IxData has the complete information of a interaction which is signed by user
type IxData struct {
	Input     InteractionInput
	Compute   InteractionCompute
	Trust     InteractionTrust
	Signature []byte
}

// Interactions are array of Transactions
type Interactions []*Interaction

func (is Interactions) Bytes() ([]byte, error) {
	return polo.Polorize(is)
}

func (is Interactions) Hash() (Hash, error) {
	return PoloHash(is)
}

func (ix *Interaction) GetSize() (int64, error) {
	// FIXME: size should calculated after signature integration
	bz, err := polo.Polorize(ix)
	if err != nil {
		return 0, errors.Wrap(err, "failed to polorize interaction")
	}
	return int64(len(bz)), nil
}

func (ix *Interaction) GetAssetCreationPayload() *AssetDataInput {
	return &ix.Data.Input.Payload.AssetData
}

func (ix *Interaction) IxType() IxType {
	return ix.Data.Input.Type
}

func (ix *Interaction) GetIxHash() (Hash, error) {
	if ix.Hash.IsNil() {
		h, err := PoloHash(ix)
		if err != nil {
			return Hash{}, errors.Wrap(err, "failed to polorize interaction")
		}
		ix.Hash = h

		return h, nil
	}

	return ix.Hash, nil
}

func (ix *Interaction) Sign(prv *ecdsa.PrivateKey) error {
	// h := ix.GetIxHash()
	// sig, err := kcrypto.Sign(h[:], prv)
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
