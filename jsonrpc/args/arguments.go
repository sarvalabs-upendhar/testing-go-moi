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
	ID               identifiers.Identifier `json:"id"` // ID for which to retrieve the latest ts
	WithInteractions bool                   `json:"with_interactions"`
	WithCommitInfo   bool                   `json:"with_commit_info"`
	Options          TesseractNumberOrHash  `json:"options"`
}

type GetAssetInfoArgs struct {
	AssetID identifiers.AssetID   `json:"asset_id"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetAssetMandateOrLockupArgs struct {
	ID      identifiers.Identifier `json:"id"`
	Options TesseractNumberOrHash  `json:"options"`
}

type QueryArgs struct {
	ID      identifiers.Identifier `json:"id"` // ID for which to retrieve the latest ts
	Options TesseractNumberOrHash  `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	ID      identifiers.Identifier `json:"id"`       // ID for which to retrieve the balance
	AssetID identifiers.AssetID    `json:"asset_id"` // AssetID for which to retrieve balance
	Options TesseractNumberOrHash  `json:"options"`
}

type ContextInfoArgs struct {
	ID      identifiers.Identifier `json:"id"` // ID for which to retrieve the latest ts
	Options TesseractNumberOrHash  `json:"options"`
}

type InteractionByTesseract struct {
	ID      identifiers.Identifier `json:"id"`
	Options TesseractNumberOrHash  `json:"options"`
	IxIndex *hexutil.Uint64        `json:"ix_index"`
}

type InteractionByHashArgs struct {
	Hash common.Hash `json:"hash"`
}

// ReceiptArgs is an argument wrapper for retrieving the receipt of an interaction
type ReceiptArgs struct {
	Hash common.Hash `json:"hash"`
}

type GetAccountArgs struct {
	ID      identifiers.Identifier `json:"id"`
	Options TesseractNumberOrHash  `json:"options"`
}

type GetAccountKeysArgs struct {
	ID      identifiers.Identifier `json:"id"`
	Options TesseractNumberOrHash  `json:"options"`
}

type LogicEnlistedArgs struct {
	ID      identifiers.Identifier `json:"id"`
	LogicID identifiers.LogicID    `json:"logic_id"`
}

type InteractionCountArgs struct {
	ID      identifiers.Identifier `json:"id"`
	KeyID   uint64                 `json:"key_id"`
	Options TesseractNumberOrHash  `json:"options"`
}

type GetLogicStorageArgs struct {
	ID         identifiers.Identifier `json:"id"`
	LogicID    identifiers.LogicID    `json:"logic_id"`
	StorageKey hexutil.Bytes          `json:"storage_key"`
	Options    TesseractNumberOrHash  `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  identifiers.LogicID   `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

type CallArgs struct {
	IxArgs  *IxArgs                                           `json:"ix_args"`
	Options map[identifiers.Identifier]*TesseractNumberOrHash `json:"options"`
}

type SyncStatusRequest struct {
	ID              identifiers.Identifier `json:"id"`
	PendingAccounts bool                   `json:"pending_accounts"`
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
	ID identifiers.Identifier `json:"id"`
}

type PeerScoreRequest struct{}

// Public ix args

type SendIX struct {
	IXArgs     string `json:"ix_args"`
	Signatures string `json:"signatures"`
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
	ID       identifiers.Identifier `json:"id"`
	LockType common.LockType        `json:"lock_type"`
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
	Sender common.Sender          `json:"sender"`
	Payer  identifiers.Identifier `json:"payer"`

	SequenceID hexutil.Uint64 `json:"sequence_id"`

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
	ID identifiers.Identifier `json:"id"`
}

type StatusArgs struct{}

type InspectArgs struct{}

// Public net args

type NetArgs struct{}

// Other args

type GetLogicIDArgs struct {
	ID      identifiers.Identifier `json:"id"`
	Options TesseractNumberOrHash  `json:"options"`
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
	ID     identifiers.Identifier `json:"id"`
	Amount *hexutil.Big           `json:"amount"`
}

type KeyAddPayload struct {
	PublicKey          hexutil.Bytes  `json:"public_key"`
	Weight             hexutil.Uint64 `json:"weight"`
	SignatureAlgorithm hexutil.Uint64 `json:"signature_algorithm"`
}

type KeyRevokePayload struct {
	KeyID hexutil.Uint64 `json:"key_id"`
}

type RPCAccountConfigurePayload struct {
	Add    []KeyAddPayload
	Revoke []KeyRevokePayload
}

func GetRPCAccountConfigurePayload(payload *common.AccountConfigurePayload) *RPCAccountConfigurePayload {
	rpcPayload := &RPCAccountConfigurePayload{}

	if payload.Add != nil {
		rpcPayload.Add = make([]KeyAddPayload, len(payload.Add))
		for i, add := range payload.Add {
			rpcPayload.Add[i] = KeyAddPayload{
				PublicKey:          add.PublicKey,
				Weight:             hexutil.Uint64(add.Weight),
				SignatureAlgorithm: hexutil.Uint64(add.SignatureAlgorithm),
			}
		}
	}

	if payload.Revoke != nil {
		rpcPayload.Revoke = make([]KeyRevokePayload, len(payload.Revoke))
		for i, revoke := range payload.Revoke {
			rpcPayload.Revoke[i] = KeyRevokePayload{
				KeyID: hexutil.Uint64(revoke.KeyID),
			}
		}
	}

	return rpcPayload
}

type RPCAssetAction struct {
	Benefactor  identifiers.Identifier `json:"benefactor"`
	Beneficiary identifiers.Identifier `json:"beneficiary"`
	AssetID     identifiers.AssetID    `json:"asset_id"`
	Amount      *hexutil.Big           `json:"amount"`
	Timestamp   *hexutil.Uint64        `json:"timestamp"`
}

type RPCLogicPayload struct {
	Manifest hexutil.Bytes `json:"manifest"`
	LogicID  string        `json:"logic_id"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
}

func (l *RPCLogicPayload) LogicPayload() *common.LogicPayload {
	return &common.LogicPayload{
		Logic:    identifiers.MustLogicIDFromHex(l.LogicID),
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
		LogicID:  payload.Logic.String(),
		Callsite: payload.Callsite,
		Calldata: payload.Calldata,
	}
}
