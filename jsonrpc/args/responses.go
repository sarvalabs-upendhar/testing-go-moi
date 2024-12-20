package args

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/libp2p/go-libp2p/core/protocol"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
)

// core and ix RPC Responses

type RPCDeeds struct {
	AssetID   string             `json:"asset_id"`
	AssetInfo RPCAssetDescriptor `json:"asset_info"`
}

type RPCInteractions []*RPCInteraction

type RPCTesseract struct {
	Participants     RPCParticipants `json:"participants"`
	InteractionsHash common.Hash     `json:"interactions_hash"`
	ReceiptsHash     common.Hash     `json:"receipts_hash"`
	Epoch            *hexutil.Big    `json:"epoch"`
	TimeStamp        hexutil.Uint64  `json:"time_stamp"`
	Operator         string          `json:"operator"`
	FuelUsed         hexutil.Uint64  `json:"fuel_used"`
	FuelLimit        hexutil.Uint64  `json:"fuel_limit"`
	ConsensusInfo    RPCPoXtData     `json:"consensus_info"`

	Seal hexutil.Bytes `json:"seal"`

	Hash       common.Hash     `json:"hash"`
	Ixns       RPCInteractions `json:"ixns"`
	CommitInfo RPCCommitInfo   `json:"commit_info"`
}

// ContextResponse is response object for fetching context info
type ContextResponse struct {
	BehaviourNodes []string `json:"behaviour_nodes"`
	RandomNodes    []string `json:"random_nodes"`
	StorageNodes   []string `json:"storage_nodes"`
}

type RPCAssetDescriptor struct {
	Symbol   string              `json:"symbol"`
	Operator identifiers.Address `json:"operator"`
	Supply   hexutil.Big         `json:"supply"`

	Dimension hexutil.Uint8  `json:"dimension"`
	Standard  hexutil.Uint16 `json:"standard"`

	IsLogical  bool `json:"is_logical"`
	IsStateFul bool `json:"is_stateful"`

	LogicID identifiers.LogicID `json:"logic_id,omitempty"`
}

func GetRPCAssetDescriptor(ad *common.AssetDescriptor) RPCAssetDescriptor {
	return RPCAssetDescriptor{
		Symbol:     ad.Symbol,
		Operator:   ad.Operator,
		Dimension:  hexutil.Uint8(ad.Dimension),
		Standard:   hexutil.Uint16(ad.Standard),
		Supply:     (hexutil.Big)(*ad.Supply),
		IsLogical:  ad.IsLogical,
		IsStateFul: ad.IsStateFul,
		LogicID:    ad.LogicID,
	}
}

type TDU struct {
	AssetID identifiers.AssetID `json:"asset_id"`
	Amount  *hexutil.Big        `json:"amount"`
}

type RPCIxOp struct {
	Type    common.IxOpType `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type RPCInteraction struct {
	Nonce hexutil.Uint64 `json:"nonce"`

	Sender identifiers.Address `json:"sender"`
	Payer  identifiers.Address `json:"payer"`

	FuelPrice *hexutil.Big   `json:"fuel_price"`
	FuelLimit hexutil.Uint64 `json:"fuel_limit"`

	IxOps []RPCIxOp `json:"ix_operations"`

	Hash         common.Hash     `json:"hash"`
	Signature    hexutil.Bytes   `json:"signature"`
	TSHash       common.Hash     `json:"ts_hash"`
	Participants RPCParticipants `json:"participants"`
	IxIndex      hexutil.Uint64  `json:"ix_index"`
}

type RPCIxOpResult struct {
	TxType hexutil.Uint64    `json:"tx_type"`
	Status common.IxOpStatus `json:"status"`
	Data   json.RawMessage   `json:"data"`
}

type RPCReceipt struct {
	IxHash       common.Hash          `json:"ix_hash"`
	Status       common.ReceiptStatus `json:"status"`
	FuelUsed     hexutil.Uint64       `json:"fuel_used"`
	IxOps        []*RPCIxOpResult     `json:"ix_operations"`
	From         identifiers.Address  `json:"from"`
	IXIndex      hexutil.Uint64       `json:"ix_index,omitempty"`
	TSHash       common.Hash          `json:"ts_hash,omitempty"`
	Participants RPCParticipants      `json:"participants,omitempty"`
}

type RPCAccount struct {
	Nonce   hexutil.Uint64     `json:"nonce"`
	AccType common.AccountType `json:"acc_type"`

	AssetDeeds  common.Hash `json:"asset_deeds"`
	ContextHash common.Hash `json:"context_hash"`
	StorageRoot common.Hash `json:"storage_root"`
	AssetRoot   common.Hash `json:"asset_root"`
	LogicRoot   common.Hash `json:"logic_root"`
	FileRoot    common.Hash `json:"file_root"`
}

type RPCAccountMetaInfo struct {
	Type          common.AccountType  `json:"type"`
	Address       identifiers.Address `json:"address"`
	Height        hexutil.Uint64      `json:"height"`
	TesseractHash common.Hash         `json:"tesseract_hash"`
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

type RPCLog struct {
	Address identifiers.Address `json:"address"`
	LogicID identifiers.LogicID `json:"logic_id,omitempty"`
	Topics  []common.Hash       `json:"topics"`
	Data    hexutil.Bytes       `json:"data"`

	// Derived fields, avoid serializing these fields while storing to DB
	IxHash       common.Hash     `json:"ix_hash"`
	TSHash       common.Hash     `json:"ts_hash"`
	Participants RPCParticipants `json:"participants"`
}

// Ixpool RPC Responses

// InteractionResponse is a struct that represents a single interaction
type InteractionResponse struct {
	Nonce     hexutil.Uint64      `json:"nonce"`
	Sender    identifiers.Address `json:"sender"`
	Cost      *hexutil.Big        `json:"cost"`
	FuelPrice *hexutil.Big        `json:"fuel_price"`
	FuelLimit hexutil.Uint64      `json:"fuel_limit"`
	IxOps     []IxOp              `json:"ix_operations"`
	Input     string              `json:"input"`
	Hash      common.Hash         `json:"hash"`
}

// NewInteractionResponse is a contructor function that generates
// and returns a new InteractionResponse for a given Interaction
func NewInteractionResponse(ix *common.Interaction) *InteractionResponse {
	ops := make([]IxOp, len(ix.IXData().IxOps))

	for idx, op := range ix.IXData().IxOps {
		ops[idx] = IxOp{
			Type:    op.Type,
			Payload: op.Payload,
		}
	}

	return &InteractionResponse{
		Nonce:     hexutil.Uint64(ix.Nonce()),
		Sender:    ix.Sender(),
		Cost:      (*hexutil.Big)(ix.Cost()),
		FuelPrice: (*hexutil.Big)(ix.FuelPrice()),
		FuelLimit: hexutil.Uint64(ix.FuelLimit()),
		IxOps:     ops,
		Hash:      ix.Hash(),
	}
}

type ContentResponse struct {
	Pending map[identifiers.Address]map[hexutil.Uint64]*InteractionResponse `json:"pending"`
	Queued  map[identifiers.Address]map[hexutil.Uint64]*InteractionResponse `json:"queued"`
}

type ContentFromResponse struct {
	Pending map[hexutil.Uint64]*InteractionResponse `json:"pending"`
	Queued  map[hexutil.Uint64]*InteractionResponse `json:"queued"`
}

type StatusResponse struct {
	Pending hexutil.Uint64 `json:"pending"`
	Queued  hexutil.Uint64 `json:"queued"`
}

type InspectResponse struct {
	Pending  map[string]map[string]string `json:"pending"`
	Queued   map[string]map[string]string `json:"queued"`
	WaitTime map[string]*WaitTimeResponse `json:"wait_time"`
}

type WaitTimeResponse struct {
	Expired bool         `json:"expired"`
	Time    *hexutil.Big `json:"time"`
}

// Net RPC Responses

type NodeInfoResponse struct {
	KramaID kramaid.KramaID `json:"krama_id"`
}

// Debug RPC Responses

type Stream struct {
	Protocol  protocol.ID `json:"protocol"`
	Direction int         `json:"direction"`
}

type Connection struct {
	PeerID  string   `json:"peer_id"`
	Streams []Stream `json:"streams"`
}

type NodeMetaInfoResponse struct {
	Addrs       []string        `json:"addrs"`
	KramaID     kramaid.KramaID `json:"krama_id"`
	RTT         hexutil.Uint64  `json:"rtt"`
	WalletCount hexutil.Uint    `json:"wallet_count"`
}

type ConnectionsResponse struct {
	Conns              []Connection   `json:"connections"`
	InboundConnCount   int64          `json:"inbound_conn_count"`
	OutboundConnCount  int64          `json:"outbound_conn_count"`
	ActivePubSubTopics map[string]int `json:"active_pub_sub_topics"`
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

type RPCTopicScore struct {
	Name                     string  `json:"topic_name"`
	TimeInMesh               uint64  `json:"time_in_mesh"`
	FirstMessageDeliveries   float64 `json:"first_message_deliveries"`
	MeshMessageDeliveries    float64 `json:"mesh_message_deliveries"`
	InvalidMessageDeliveries float64 `json:"invalid_message_deliveries"`
}

type RPCTopicScores []RPCTopicScore

func (scores RPCTopicScores) Sort() {
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Name < scores[j].Name
	})
}

type RPCPeerScore struct {
	ID                 peer.ID         `json:"peer_id"`
	TopicScores        []RPCTopicScore `json:"topic_scores"`
	AppSpecificScore   float64         `json:"app_specific_score"`
	GossipScore        float64         `json:"gossip_score"`
	IPColocationFactor float64         `json:"ip_colocation_factor"`
	BehaviourPenalty   float64         `json:"behaviour_penalty"`
}

type RPCPeersScore []RPCPeerScore

func (peers RPCPeersScore) Sort() {
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].ID < peers[j].ID
	})
}

type DiagnosisResponse struct{}

// Other RPC Responses

type RPCState struct {
	Address        identifiers.Address `json:"address"`
	Height         hexutil.Uint64      `json:"height"`
	TransitiveLink common.Hash         `json:"transitive_link"`
	PrevContext    common.Hash         `json:"prev_context"`
	LatestContext  common.Hash         `json:"latest_context"`
	ContextDelta   *common.DeltaGroup  `json:"context_delta"`
	StateHash      common.Hash         `json:"state_hash"`
}

type RPCParticipants []RPCState

func (participants RPCParticipants) Sort() {
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].Address.Hex() < participants[j].Address.Hex()
	})
}

type RPCPoXtData struct {
	Proposer     kramaid.KramaID                         `json:"operator"`
	BinaryHash   common.Hash                             `json:"binary_hash"`
	IdentityHash common.Hash                             `json:"identity_hash"`
	View         hexutil.Uint64                          `json:"view"`
	LastCommit   map[identifiers.Address]common.Hash     `json:"last_commit"`
	AccountLocks map[identifiers.Address]common.LockType `json:"account_locks"`
	ICSSeed      [32]byte                                `json:"ics_seed"`
	ICSProof     hexutil.Bytes                           `json:"ics_proof"`
	EvidenceHash map[identifiers.Address]common.Hash     `json:"evidence_hash"`
}

func (t *RPCTesseract) HasParticipant(addr identifiers.Address) bool {
	for _, s := range t.Participants {
		if s.Address == addr {
			return true
		}
	}

	return false
}

func (t *RPCTesseract) Height(addr identifiers.Address) uint64 {
	for _, p := range t.Participants {
		if p.Address == addr {
			return p.Height.ToUint64()
		}
	}

	// return 1000 as we will not use 1000 tesseracts in tests
	return 1000
}

type RPCQc struct {
	Type          common.ConsensusMsgType `json:"type"`
	Address       identifiers.Address     `json:"address"`
	LockType      common.LockType         `json:"lock_type"`
	View          uint64                  `json:"view"`
	TSHash        common.Hash             `json:"ts_hash"`
	SignerIndices string                  `json:"signer_indices"`
	Signature     []byte                  `json:"signature"`
}

type RPCCommitInfo struct {
	QC                        *RPCQc            `json:"commit_qc"`
	Operator                  kramaid.KramaID   `json:"operator"`
	ClusterID                 common.ClusterID  `json:"cluster_id"`
	View                      uint64            `json:"commit_view"`
	RandomSet                 []kramaid.KramaID `json:"random_set"`
	RandomSetSizeWithoutDelta uint32            `json:"random_set_size"`
}

func CreateRPCCommitInfo(info *common.CommitInfo) RPCCommitInfo {
	if info == nil {
		return RPCCommitInfo{}
	}

	rpcCommitInfo := RPCCommitInfo{
		Operator:                  info.Operator,
		ClusterID:                 info.ClusterID,
		View:                      info.View,
		RandomSet:                 info.RandomSet,
		RandomSetSizeWithoutDelta: info.RandomSetSizeWithoutDelta,
	}

	if info.QC != nil {
		rpcCommitInfo.QC = &RPCQc{
			Type:      info.QC.Type,
			Address:   info.QC.Address,
			LockType:  info.QC.LockType,
			View:      info.QC.View,
			TSHash:    info.QC.TSHash,
			Signature: info.QC.Signature,
		}

		if info.QC.SignerIndices != nil {
			rpcCommitInfo.QC.SignerIndices = info.QC.SignerIndices.String()
		}
	}

	return rpcCommitInfo
}

// Not used

type Hashes struct {
	Address     identifiers.Address `json:"address"`
	StateHash   common.Hash         `json:"state_hash"`
	ContextHash common.Hash         `json:"context_hash"`
}

type RPCHashes []Hashes

func (hashes RPCHashes) Sort() {
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].Address.Hex() < hashes[j].Address.Hex()
	})
}
