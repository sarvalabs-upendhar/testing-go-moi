package common

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
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
	Mint *AssetMintOrBurnPayload
}

type AssetCreatePayload struct {
	Symbol string
	Supply *big.Int

	Standard  AssetStandard
	Dimension uint8

	IsStateFul bool
	IsLogical  bool

	LogicPayload *LogicPayload
}

func (asset AssetCreatePayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(asset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset create payload")
	}

	return data, nil
}

func (asset *AssetCreatePayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(asset, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset create payload")
	}

	return nil
}

type AssetMintOrBurnPayload struct {
	// AssetID is used to specify the Asset ID for which to mint
	Asset identifiers.AssetID
	// Amount is used for mint/burn
	Amount *big.Int
}

func (mint AssetMintOrBurnPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(mint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset mint payload")
	}

	return data, nil
}

func (mint *AssetMintOrBurnPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(mint, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset mint payload")
	}

	return nil
}

type AssetApprovePayload struct {
	// Spender is used to specify the spender address for approve
	Spender identifiers.Address
	// Approvals are used to specify the amount of approval for each asset.
	// This is set to 0 for an asset to revoke and 2^256 for infinite allowance.
	Approvals map[identifiers.AssetID]*big.Int
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
	Logic identifiers.LogicID

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
