package types

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/mudra/kramaid"
)

type IxPayload struct {
	asset *AssetPayload
	file  *FilePayload  //nolint:unused
	logic *LogicPayload //nolint:unused
}

type AssetPayload struct {
	// Create contains the payload for IxAssetCreate
	Create *AssetCreatePayload
	// Approve contains the payload for IxAssetApprove and IxAssetRevoke
	Approve *AssetApprovePayload
	// Mint contains the payload for IxAssetMint and IxAssetBurn
	Mint *AssetMintPayload
}

type AssetCreatePayload struct {
	Type   AssetKind
	Symbol string
	Supply *big.Int

	Dimension uint8
	Decimals  uint8

	IsFungible     bool
	IsMintable     bool
	IsTransferable bool

	LogicID LogicID
	// LogicCode []byte
}

type AssetMintPayload struct {
	// AssetID is used to specify the Asset ID for which to mint/burn
	Asset AssetID
	// Amount is used for mint/burn
	Amount *big.Int
}

type AssetApprovePayload struct {
	// Spender is used to specify the spender address for approve
	Spender Address
	// Approvals are used to specify the amount of approval for each asset.
	// This is set to 0 for an asset to revoke and 2^256 for infinite allowance.
	Approvals map[AssetID]*big.Int
}

func (asset AssetPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(asset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset payload")
	}

	return data, nil
}

func (asset *AssetPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(asset, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset payload")
	}

	return nil
}

type FilePayload struct {
	Name  string
	Hash  string
	File  []byte
	Nodes []kramaid.KramaID
}

type LogicPayload struct {
	// Logic specifies the Logic ID to execute a method on.
	// Required for IxLogicInvoke, IxLogicInteract, IxLogicEnlist, IxLogicUpgrade
	Logic LogicID

	// Callsite specifies the method name to deploy and invoke.
	// Required for IxLogicDeploy, IxLogicInvoke, IxLogicInteract, IxLogicEnlist
	Callsite string
	// Calldata specifies the input call data.
	// Required for IxLogicDeploy, IxLogicInvoke, IxLogicInteract, IxLogicEnlist
	Calldata []byte

	// Manifest specifies some Logic manifest artifact.
	// Required for IxLogicDeploy, IxLogicUpgrade
	Manifest []byte
}

func (payload *LogicPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize payload payload")
	}

	return data, nil
}

func (payload *LogicPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(payload, data); err != nil {
		return errors.Wrap(err, "failed to depolorize payload payload")
	}

	return nil
}

func (payload *LogicPayload) UnmarshalJSON(data []byte) error {
	type Alias LogicPayload

	aux := &struct {
		Manifest string `json:"manifest"`
		Calldata string `json:"calldata"`
		*Alias
	}{
		Alias: (*Alias)(payload),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.Calldata != "" {
		callDataBytes, err := hex.DecodeString(aux.Calldata)
		if err != nil {
			return err
		}

		payload.Calldata = callDataBytes
	}

	if aux.Manifest != "" {
		manifestBytes, err := hex.DecodeString(aux.Manifest)
		if err != nil {
			return err
		}

		payload.Manifest = manifestBytes
	}

	return nil
}
