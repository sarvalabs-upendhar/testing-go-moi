package args

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
)

// RPC args

// TesseractArgs is an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	Address          common.Address        `json:"address"` // Address for which to retrieve the latest Tesseract
	WithInteractions bool                  `json:"with_interactions"`
	Options          TesseractNumberOrHash `json:"options"`
}

type QueryArgs struct {
	Address common.Address        `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	Address common.Address        `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type InteractionCountArgs struct {
	Address common.Address        `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type IxPoolArgs struct {
	Address common.Address `json:"address"`
}

type LogicCallResult struct {
	Consumed hexutil.Big   `json:"consumed"`
	Outputs  hexutil.Bytes `json:"outputs"`
	Error    hexutil.Bytes `json:"error"`
}

type LogicCallArgs struct {
	Invoker  common.Address `json:"invoker"`
	LogicID  common.LogicID `json:"logic_id"`
	Callsite string         `json:"callsite"`
	Calldata hexutil.Bytes  `json:"calldata"`
}

type InspectArgs struct{}

type StatusArgs struct{}

type ContentArgs struct{}

type NetArgs struct{}

type AccountArgs struct{}

type ConnArgs struct{}

type DebugArgs struct {
	Key string `json:"storage_key"`
}

type GetLogicStorageArgs struct {
	LogicID    common.LogicID        `json:"logic_id"`
	StorageKey hexutil.Bytes         `json:"storage_key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type GetAssetInfoArgs struct {
	AssetID common.AssetID        `json:"asset_id"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetAccountArgs struct {
	Address common.Address        `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetLogicIDArgs struct {
	Address common.Address        `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  common.LogicID        `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	Address common.Address        `json:"address"`  // Address for which to retrieve the balance
	AssetID common.AssetID        `json:"asset_id"` // Asset for which to retrieve balance
	Options TesseractNumberOrHash `json:"options"`
}

type SendIX struct {
	IXArgs    string `json:"ix_args"`
	Signature string `json:"signature"`
}

type RPCAssetCreation struct {
	Symbol string       `json:"symbol"`
	Supply *hexutil.Big `json:"supply"`

	Dimension *hexutil.Uint8  `json:"dimension"`
	Standard  *hexutil.Uint16 `json:"standard"`

	IsLogical  bool `json:"is_logical"`
	IsStateful bool `json:"is_stateful"`

	Logic *RPCLogicPayload `json:"logic_code,omitempty"`
}

type RPCAssetMintOrBurn struct {
	AssetID common.AssetID `json:"asset_id"`
	Amount  *hexutil.Big   `json:"amount"`
}

type RPCLogicPayload struct {
	Manifest hexutil.Bytes `json:"manifest"`
	LogicID  string        `json:"logic_id"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
}

func (l *RPCLogicPayload) LogicPayload() *common.LogicPayload {
	return &common.LogicPayload{
		Manifest: l.Manifest.Bytes(),
		Logic:    common.LogicID(l.LogicID),
		Calldata: l.Calldata,
		Callsite: l.Callsite,
	}
}

func RPClogicPayloadFromLogicPayload(payload *common.LogicPayload) *RPCLogicPayload {
	if payload == nil {
		return nil
	}

	return &RPCLogicPayload{
		Manifest: (hexutil.Bytes)(payload.Manifest),
		LogicID:  payload.Logic.String(),
		Callsite: payload.Callsite,
		Calldata: (hexutil.Bytes)(payload.Calldata),
	}
}

type InteractionByHashArgs struct {
	Hash common.Hash `json:"hash"`
}

type InteractionByTesseract struct {
	Address common.Address        `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
	IxIndex *hexutil.Uint64       `json:"ix_index"`
}
