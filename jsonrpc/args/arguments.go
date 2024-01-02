package args

import (
	"encoding/json"
	"time"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
)

// RPC args

type SubscriptionType string

const (
	NewTesseract           SubscriptionType = "newTesseracts"
	NewTesseractsByAccount SubscriptionType = "newTesseractsByAccount"
	NewLogsByFilter        SubscriptionType = "newLogs"
	PendingIxns            SubscriptionType = "newPendingInteractions"
)

// TesseractArgs is an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	Address          identifiers.Address   `json:"address"` // Address for which to retrieve the latest Tesseract
	WithInteractions bool                  `json:"with_interactions"`
	Options          TesseractNumberOrHash `json:"options"`
}

type QueryArgs struct {
	Address identifiers.Address   `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	Address identifiers.Address   `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type InteractionCountArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type IxPoolArgs struct {
	Address identifiers.Address `json:"address"`
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

type NodeMetaInfoArgs struct {
	KramaID kramaid.KramaID `json:"krama_id"`
	PeerID  string          `json:"peer_id"`
}

type GetLogicStorageArgs struct {
	LogicID    identifiers.LogicID   `json:"logic_id"`
	StorageKey hexutil.Bytes         `json:"storage_key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type GetAssetInfoArgs struct {
	AssetID identifiers.AssetID   `json:"asset_id"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetAccountArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetLogicIDArgs struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  identifiers.LogicID   `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	Address identifiers.Address   `json:"address"`  // Address for which to retrieve the balance
	AssetID identifiers.AssetID   `json:"asset_id"` // Asset for which to retrieve balance
	Options TesseractNumberOrHash `json:"options"`
}

type CallArgs struct {
	IxArgs  *IxArgs                                        `json:"ix_args"`
	Options map[identifiers.Address]*TesseractNumberOrHash `json:"options"`
}

type SendIX struct {
	IXArgs    string `json:"ix_args"`
	Signature string `json:"signature"`
}

type IxArgs struct {
	Type  common.IxType  `json:"type"`
	Nonce hexutil.Uint64 `json:"nonce"`

	Sender   identifiers.Address `json:"sender"`
	Receiver identifiers.Address `json:"receiver"`
	Payer    identifiers.Address `json:"payer"`

	TransferValues  map[identifiers.AssetID]*hexutil.Big `json:"transfer_values"`
	PerceivedValues map[identifiers.AssetID]*hexutil.Big `json:"perceived_values"`

	FuelPrice *hexutil.Big   `json:"fuel_price"`
	FuelLimit hexutil.Uint64 `json:"fuel_limit"`

	Payload hexutil.Bytes `json:"payload"`
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
	AssetID identifiers.AssetID `json:"asset_id"`
	Amount  *hexutil.Big        `json:"amount"`
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
		Logic:    identifiers.LogicID(l.LogicID),
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
		LogicID:  string(payload.Logic),
		Callsite: payload.Callsite,
		Calldata: (hexutil.Bytes)(payload.Calldata),
	}
}

type InteractionByHashArgs struct {
	Hash common.Hash `json:"hash"`
}

type InteractionByTesseract struct {
	Address identifiers.Address   `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
	IxIndex *hexutil.Uint64       `json:"ix_index"`
}

type FilterQueryArgs struct {
	StartHeight *int64              `json:"start_height"`
	EndHeight   *int64              `json:"end_height"`
	Address     identifiers.Address `json:"address"`
	Topics      [][]common.Hash     `json:"topics"`
}

// UnmarshalJSON decodes a Filter Query json object
func (q *FilterQueryArgs) UnmarshalJSON(data []byte) error {
	var obj struct {
		StartHeight *int64              `json:"start_height"`
		EndHeight   *int64              `json:"end_height"`
		Address     identifiers.Address `json:"address"`
		Topics      []interface{}       `json:"topics"`
	}

	err := json.Unmarshal(data, &obj)
	if err != nil {
		return err
	}

	if obj.StartHeight == nil {
		q.StartHeight = &LatestTesseractHeight
	} else {
		q.StartHeight = obj.StartHeight
	}

	if obj.EndHeight == nil {
		q.EndHeight = &LatestTesseractHeight
	} else {
		q.EndHeight = obj.EndHeight
	}

	if obj.Address == identifiers.NilAddress {
		return common.ErrInvalidAddress
	}

	q.Address = obj.Address

	if obj.Topics != nil {
		topics, err := UnmarshalTopic(obj.Topics)
		if err != nil {
			return err
		}

		q.Topics = topics
	}

	// decode topics
	return nil
}

type SyncStatusRequest struct {
	Address         identifiers.Address `json:"address"`
	PendingAccounts bool                `json:"pending_accounts"`
}

type AccSyncStatus struct {
	CurrentHeight     hexutil.Uint64 `json:"current_height"`
	ExpectedHeight    hexutil.Uint64 `json:"expected_height"`
	IsPrimarySyncDone bool           `json:"is_primary_sync_done"`
}

type NodeSyncStatus struct {
	TotalPendingAccounts  hexutil.Uint64        `json:"total_pending_accounts"`
	PendingAccounts       []identifiers.Address `json:"pending_accounts"`
	IsPrincipalSyncDone   bool                  `json:"is_principal_sync_done"`
	PrincipalSyncDoneTime hexutil.Uint64        `json:"principal_sync_done_time"`
	IsInitialSyncDone     bool                  `json:"is_initial_sync_done"`
}

type SyncStatusResponse struct {
	AccSyncResp  *AccSyncStatus  `json:"acc_sync_status"`
	NodeSyncResp *NodeSyncStatus `json:"node_sync_status"`
}

type FilterArgs struct {
	FilterID string `json:"id"`
}

type FilterResponse struct {
	FilterID string `json:"id"`
}

type FilterUninstallResponse struct {
	Status bool `json:"status"`
}

type TesseractFilterArgs struct{}

type TesseractByAccountFilterArgs struct {
	Addr identifiers.Address `json:"address"`
}

type PendingIxnsFilterArgs struct{}

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

type SyncJobInfo struct {
	SyncMode              string            `json:"sync_mode"`
	SnapDownloaded        bool              `json:"snap_downloaded"`
	ExpectedHeight        uint64            `json:"expected_height"`
	CurrentHeight         uint64            `json:"current_height"`
	JobState              string            `json:"job_state"`
	LastModifiedAt        time.Time         `json:"last_modified_at"`
	TesseractQueueLen     uint64            `json:"tesseract_queue_length"`
	BestPeers             []kramaid.KramaID `json:"best_peers"`
	LatticeSyncInProgress bool              `json:"lattice_sync_in_progress"`
}
