package types

import (
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
	// Type specifies the type of Logic. Only required for Deploy
	Type LogicKind
	// IsStateful specifies if the Logic is stateful. Only required for Deploy
	IsStateful bool
	// Manifest specifies some Logic manifest artifact. Only required for Deploy and Upgrade
	Manifest []byte

	// Logic specifies the Logic ID to execute a method on. Only required for Execute and Upgrade
	Logic LogicID
	// Callpoint specifies the method name to execute. Only required for Execute
	Callpoint string
	// Calldata specifies the input call data. Only required for Execute and Deploy
	Calldata []byte
}
