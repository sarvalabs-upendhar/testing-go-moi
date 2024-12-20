package args

import (
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
)

// Public core args

// TesseractArgs is an argument wrapper for retrieving the latest ts
type TesseractArgs struct {
	Address          identifiers.Address   `json:"address"` // Address for which to retrieve the latest ts
	WithInteractions bool                  `json:"with_interactions"`
	WithCommitInfo   bool                  `json:"with_commit_info"`
	Options          TesseractNumberOrHash `json:"options"`
}

type GetAssetInfoArgs struct {
	AssetID identifiers.AssetID   `json:"asset_id"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetAssetMandateArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type QueryArgs struct {
	Address identifiers.Address   `json:"address"` // Address for which to retrieve the latest ts
	Options TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	Address identifiers.Address   `json:"address"`  // Address for which to retrieve the balance
	AssetID identifiers.AssetID   `json:"asset_id"` // AssetID for which to retrieve balance
	Options TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	Address identifiers.Address   `json:"address"` // Address for which to retrieve the latest ts
	Options TesseractNumberOrHash `json:"options"`
}

type InteractionByTesseract struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
	IxIndex *hexutil.Uint64       `json:"ix_index"`
}

type InteractionByHashArgs struct {
	Hash common.Hash `json:"hash"`
}

// ReceiptArgs is an argument wrapper for retrieving the receipt of an interaction
type ReceiptArgs struct {
	Hash common.Hash `json:"hash"`
}

type GetAccountArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicEnlistedArgs struct {
	Address identifiers.Address `json:"address"`
	LogicID identifiers.LogicID `json:"logic_id"`
}

type InteractionCountArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetLogicStorageArgs struct {
	Address    identifiers.Address   `json:"address"`
	LogicID    identifiers.LogicID   `json:"logic_id"`
	StorageKey hexutil.Bytes         `json:"storage_key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  identifiers.LogicID   `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

type CallArgs struct {
	IxArgs  *IxArgs                                        `json:"ix_args"`
	Options map[identifiers.Address]*TesseractNumberOrHash `json:"options"`
}

type SyncStatusRequest struct {
	Address         identifiers.Address `json:"address"`
	PendingAccounts bool                `json:"pending_accounts"`
}

// Public debug args

type DebugArgs struct {
	Key string `json:"storage_key"`
}

type NodeMetaInfoArgs struct {
	KramaID kramaid.KramaID `json:"krama_id"`
	PeerID  string          `json:"peer_id"`
}

type AccountArgs struct{}

type ConnArgs struct{}

type DiagnosisRequest struct {
	OutputPath           string   `json:"output_path"`
	Collectors           []string `json:"collectors"`
	ProfileTime          string   `json:"profile_time"`
	MutexProfileFraction int      `json:"mutex_profile_fraction"`
	BlockProfileRate     string   `json:"block_profile_rate"`
}

type SyncJobRequest struct {
	Address identifiers.Address `json:"address"`
}

type PeerScoreRequest struct{}

// Public ix args

type SendIX struct {
	IXArgs    string `json:"ix_args"`
	Signature string `json:"signature"`
}

type IxFund struct {
	AssetID identifiers.AssetID `json:"asset_id"`
	Amount  *hexutil.Big        `json:"amount"`
}

type IxOp struct {
	Type    common.IxOpType `json:"type"`
	Payload hexutil.Bytes   `json:"payload"`
}

type IxParticipant struct {
	Address  identifiers.Address `json:"address"`
	LockType common.LockType     `json:"lock_type"`
}

type IxConsensusPreference struct {
	MTQ        hexutil.Uint      `json:"mtq"`
	TrustNodes []kramaid.KramaID `json:"trust_nodes"`
}

type IxPreferences struct {
	Compute   hexutil.Bytes          `json:"compute"`
	Consensus *IxConsensusPreference `json:"consensus"`
}

type IxArgs struct {
	Sender identifiers.Address `json:"sender"`
	Payer  identifiers.Address `json:"payer"`

	Nonce hexutil.Uint64 `json:"nonce"`

	FuelPrice *hexutil.Big   `json:"fuel_price"`
	FuelLimit hexutil.Uint64 `json:"fuel_limit"`

	Funds        []IxFund        `json:"funds"`
	IxOps        []IxOp          `json:"ix_operations"`
	Participants []IxParticipant `json:"participants"`

	Perception hexutil.Bytes `json:"perception"`

	Preferences *IxPreferences `json:"preferences"`
}

// Public ixpool args

type ContentArgs struct{}

type IxPoolArgs struct {
	Address identifiers.Address `json:"address"`
}

type StatusArgs struct{}

type InspectArgs struct{}

// Public net args

type NetArgs struct{}

// Other args

type GetLogicIDArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
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

type RPCAssetSupply struct {
	AssetID identifiers.AssetID `json:"asset_id"`
	Amount  *hexutil.Big        `json:"amount"`
}

type RPCParticipantCreate struct {
	Address identifiers.Address `json:"address"`
	Amount  *hexutil.Big        `json:"amount"`
}

type RPCAssetAction struct {
	Benefactor  identifiers.Address `json:"benefactor"`
	Beneficiary identifiers.Address `json:"beneficiary"`
	AssetID     identifiers.AssetID `json:"asset_id"`
	Amount      *hexutil.Big        `json:"amount"`
}

type RPCLogicPayload struct {
	Manifest hexutil.Bytes `json:"manifest"`
	LogicID  string        `json:"logic_id"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
}

func (l *RPCLogicPayload) LogicPayload() *common.LogicPayload {
	return &common.LogicPayload{
		Logic:    identifiers.LogicID(l.LogicID),
		Calldata: l.Calldata,
		Callsite: l.Callsite,
	}
}

func RPCLogicPayloadFromLogicPayload(payload *common.LogicPayload) *RPCLogicPayload {
	if payload == nil {
		return nil
	}

	return &RPCLogicPayload{
		Manifest: payload.Manifest,
		LogicID:  string(payload.Logic),
		Callsite: payload.Callsite,
		Calldata: payload.Calldata,
	}
}
