package args

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
)

// core and ix RPC Responses

type RPCDeeds struct {
	AssetID   string             `json:"asset_id"`
	AssetInfo RPCAssetDescriptor `json:"asset_info"`
}
type SortedRPCMandatesOrLockup []RPCMandateOrLockup

func (a SortedRPCMandatesOrLockup) Len() int {
	return len(a)
}

func (a SortedRPCMandatesOrLockup) Less(i, j int) bool {
	assetI, assetJ := a[i].AssetID.String(), a[j].AssetID.String()
	if assetI != assetJ {
		return assetI < assetJ
	}

	tokenI, tokenJ := a[i].TokenID.String(), a[j].TokenID.String()
	if tokenI != tokenJ {
		return tokenI < tokenJ
	}

	return a[i].Amount.ToInt().Cmp(a[j].Amount.ToInt()) == -1
}

func (a SortedRPCMandatesOrLockup) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

type RPCMandateOrLockup struct {
	ID      identifiers.Identifier `json:"id"`
	AssetID identifiers.AssetID    `json:"asset_id"`
	TokenID hexutil.Uint64         `json:"token_id"`
	Amount  *hexutil.Big           `json:"amount"`
	Expiry  hexutil.Uint64         `json:"expiry"`
}

type RPCInteractions []*RPCInteraction

type RPCTesseract struct {
	Participants     RPCParticipantsStates `json:"participants"`
	Hash             common.Hash           `json:"hash"`
	Epoch            *hexutil.Big          `json:"epoch"`
	TimeStamp        hexutil.Uint64        `json:"time_stamp"`
	FuelUsed         hexutil.Uint64        `json:"fuel_used"`
	FuelLimit        hexutil.Uint64        `json:"fuel_limit"`
	InteractionsHash common.Hash           `json:"interactions_hash"`
	ReceiptsHash     common.Hash           `json:"receipts_hash"`
	ConsensusInfo    RPCPoXtData           `json:"consensus_info"`
	CommitInfo       RPCCommitInfo         `json:"commit_info"`
	Ixns             RPCInteractions       `json:"ixns"`
	Seal             hexutil.Bytes         `json:"seal"`
}

type RPCSubAccounts struct {
	InheritedAccount identifiers.Identifier   `json:"inherited_account"`
	SubAccounts      []identifiers.Identifier `json:"sub_accounts"`
}

// ContextResponse is response object for fetching context info
type ContextResponse struct {
	ConsensusNodes   []string               `json:"consensus_nodes"`
	StorageNodes     []string               `json:"storage_nodes"`
	InheritedAccount identifiers.Identifier `json:"inherited_account"`
	SubAccounts      []RPCSubAccounts       `json:"sub_accounts"`
}

type RPCAssetDescriptor struct {
	AssetID           identifiers.AssetID      `json:"asset_id"`
	Symbol            string                   `json:"symbol"`
	Dimension         hexutil.Uint8            `json:"dimension"`
	Decimals          hexutil.Uint8            `json:"decimals"`
	Creator           identifiers.Identifier   `json:"creator"`
	Manager           identifiers.Identifier   `json:"manager"`
	MaxSupply         *hexutil.Big             `json:"max_supply"`
	CirculatingSupply *hexutil.Big             `json:"circulating_supply"`
	EnableEvents      bool                     `json:"enable_events"`
	StaticMetadata    map[string]hexutil.Bytes `json:"static_metadata,omitempty"`
	DynamicMetadata   map[string]hexutil.Bytes `json:"dynamic_metadata,omitempty"`
	LogicID           string                   `json:"logic_id,omitempty"`
}

func GetRPCAssetDescriptor(ad *common.AssetDescriptor) *RPCAssetDescriptor {
	staticMetadata := make(map[string]hexutil.Bytes)
	dynamicMetadata := make(map[string]hexutil.Bytes)

	for k, v := range ad.StaticMetaData {
		staticMetadata[k] = v
	}

	for k, v := range ad.DynamicMetaData {
		dynamicMetadata[k] = v
	}

	var logicID string
	if ad.LogicID != identifiers.Nil {
		logicID = ad.LogicID.String()
	}

	return &RPCAssetDescriptor{
		AssetID:           ad.AssetID,
		Symbol:            ad.Symbol,
		Decimals:          hexutil.Uint8(ad.Decimals),
		Creator:           ad.Creator,
		Manager:           ad.Manager,
		MaxSupply:         (*hexutil.Big)(ad.MaxSupply),
		CirculatingSupply: (*hexutil.Big)(ad.CirculatingSupply),
		EnableEvents:      ad.EnableEvents,
		StaticMetadata:    staticMetadata,
		DynamicMetadata:   dynamicMetadata,
		LogicID:           logicID,
	}
}

type TDU struct {
	AssetID identifiers.AssetID `json:"asset_id"`
	TokenID hexutil.Uint64      `json:"token_id"`
	Amount  *hexutil.Big        `json:"amount"`
}

type RPCIxOp struct {
	Type    common.IxOpType `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type RPCSender struct {
	ID         identifiers.Identifier `json:"id"`
	SequenceID hexutil.Uint64         `json:"sequence_id"`
	KeyID      hexutil.Uint64         `json:"key_id"`
}

type RPCSignature struct {
	ID        identifiers.Identifier `json:"id"`
	KeyID     hexutil.Uint64         `json:"key_id"`
	Signature hexutil.Bytes          `json:"signature"`
}

type RPCIxFund struct {
	AssetID identifiers.AssetID `json:"asset_id"`
	Amount  *hexutil.Big        `json:"amount"`
}

type RPCIxParticipant struct {
	ID       identifiers.Identifier `json:"id"`
	LockType common.LockType        `json:"lock_type"`
}

type RPCIxParticipants []RPCIxParticipant

type RPCInteraction struct {
	IxIndex   hexutil.Uint64 `json:"ix_index"`
	Hash      common.Hash    `json:"hash"`
	FuelPrice *hexutil.Big   `json:"fuel_price"`
	FuelLimit hexutil.Uint64 `json:"fuel_limit"`

	Sender         RPCSender              `json:"sender"`
	Payer          identifiers.Identifier `json:"payer"`
	IxParticipants RPCIxParticipants      `json:"ix_participants"`
	Funds          []RPCIxFund            `json:"funds"`

	IxOps []RPCIxOp `json:"ix_operations"`

	ParticipantsState RPCParticipantsStates `json:"participants_state"`

	Signatures []RPCSignature `json:"signatures"`
	TSHash     common.Hash    `json:"ts_hash"`
}

type RPCIxOpResult struct {
	TxType hexutil.Uint64    `json:"tx_type"`
	Status common.IxOpStatus `json:"status"`
	Data   json.RawMessage   `json:"data"`
}

type RPCReceipt struct {
	IxHash       common.Hash            `json:"ix_hash"`
	Status       common.ReceiptStatus   `json:"status"`
	FuelUsed     hexutil.Uint64         `json:"fuel_used"`
	IxOps        []*RPCIxOpResult       `json:"ix_operations"`
	From         identifiers.Identifier `json:"from"`
	IXIndex      hexutil.Uint64         `json:"ix_index,omitempty"`
	TSHash       common.Hash            `json:"ts_hash,omitempty"`
	Participants RPCParticipantsStates  `json:"participants,omitempty"`
}

type RPCAccount struct {
	AccType common.AccountType `json:"acc_type"`

	AssetDeeds  common.Hash `json:"asset_deeds"`
	ContextHash common.Hash `json:"context_hash"`
	StorageRoot common.Hash `json:"storage_root"`
	AssetRoot   common.Hash `json:"asset_root"`
	LogicRoot   common.Hash `json:"logic_root"`
	FileRoot    common.Hash `json:"file_root"`
	KeysHash    common.Hash `json:"keys_hash"`
}

type RPCAccountKey struct {
	ID                 hexutil.Uint64 `json:"id"`
	PublicKey          hexutil.Bytes  `json:"publicKey"`
	Weight             hexutil.Uint64 `json:"weight"`
	SignatureAlgorithm hexutil.Uint64 `json:"signature_algorithm"`
	Revoked            bool           `json:"revoked"`
	SequenceID         hexutil.Uint64 `json:"sequence_id"`
}

type RPCAccountMetaInfo struct {
	Type          common.AccountType     `json:"type"`
	ID            identifiers.Identifier `json:"id"`
	Height        hexutil.Uint64         `json:"height"`
	TesseractHash common.Hash            `json:"tesseract_hash"`
}

type RPCValidator struct {
	ID              common.ValidatorIndex  `json:"validator_id"`
	KramaID         identifiers.KramaID    `json:"krama_id"`
	ActiveStake     *hexutil.Big           `json:"active_stake"`
	InactiveStake   *hexutil.Big           `json:"inactive_stake"`
	SocialTokens    *hexutil.Big           `json:"social_tokens"`
	BehaviourTokens *hexutil.Big           `json:"behaviour_tokens"`
	Rewards         *hexutil.Big           `json:"rewards"`
	WalletID        identifiers.Identifier `json:"wallet_id"`
}

type AccSyncStatus struct {
	CurrentHeight     hexutil.Uint64 `json:"current_height"`
	ExpectedHeight    hexutil.Uint64 `json:"expected_height"`
	IsPrimarySyncDone bool           `json:"is_primary_sync_done"`
}

type NodeSyncStatus struct {
	TotalPendingAccounts  hexutil.Uint64           `json:"total_pending_accounts"`
	PendingAccounts       []identifiers.Identifier `json:"pending_accounts"`
	PendingTesseractHash  []common.Hash            `json:"pending_tesseract_hash"`
	IsPrincipalSyncDone   bool                     `json:"is_principal_sync_done"`
	PrincipalSyncDoneTime hexutil.Uint64           `json:"principal_sync_done_time"`
	IsInitialSyncDone     bool                     `json:"is_initial_sync_done"`
}

type SyncStatusResponse struct {
	AccSyncResp  *AccSyncStatus  `json:"acc_sync_status"`
	NodeSyncResp *NodeSyncStatus `json:"node_sync_status"`
}

type RPCLog struct {
	ID      identifiers.Identifier `json:"id"`
	LogicID identifiers.Identifier `json:"logic_id,omitempty"`
	Topics  []common.Hash          `json:"topics"`
	Data    hexutil.Bytes          `json:"data"`

	// Derived fields, avoid serializing these fields while storing to DB
	IxHash       common.Hash           `json:"ix_hash"`
	TSHash       common.Hash           `json:"ts_hash"`
	Participants RPCParticipantsStates `json:"participants"`
}

// Ixpool RPC Responses

// InteractionResponse is a struct that represents a single interaction
type InteractionResponse struct {
	SequenceID hexutil.Uint64         `json:"sequence-id"`
	Sender     identifiers.Identifier `json:"sender"`
	Cost       *hexutil.Big           `json:"cost"`
	FuelPrice  *hexutil.Big           `json:"fuel_price"`
	FuelLimit  hexutil.Uint64         `json:"fuel_limit"`
	IxOps      []IxOp                 `json:"ix_operations"`
	Input      string                 `json:"input"`
	Hash       common.Hash            `json:"hash"`
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
		SequenceID: hexutil.Uint64(ix.SequenceID()),
		Sender:     ix.SenderID(),
		Cost:       (*hexutil.Big)(ix.Cost()),
		FuelPrice:  (*hexutil.Big)(ix.FuelPrice()),
		FuelLimit:  hexutil.Uint64(ix.FuelLimit()),
		IxOps:      ops,
		Hash:       ix.Hash(),
	}
}

type ContentResponse struct {
	Pending map[identifiers.Identifier]map[hexutil.Uint64]*InteractionResponse `json:"pending"`
	Queued  map[identifiers.Identifier]map[hexutil.Uint64]*InteractionResponse `json:"queued"`
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
	KramaID identifiers.KramaID `json:"krama_id"`
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
	Addrs       []string            `json:"addrs"`
	KramaID     identifiers.KramaID `json:"krama_id"`
	RTT         hexutil.Uint64      `json:"rtt"`
	WalletCount hexutil.Uint        `json:"wallet_count"`
}

type ConnectionsResponse struct {
	Conns              []Connection   `json:"connections"`
	InboundConnCount   int64          `json:"inbound_conn_count"`
	OutboundConnCount  int64          `json:"outbound_conn_count"`
	ActivePubSubTopics map[string]int `json:"active_pub_sub_topics"`
}

type SyncJobInfo struct {
	SyncMode              string                `json:"sync_mode"`
	SnapDownloaded        bool                  `json:"snap_downloaded"`
	ExpectedHeight        uint64                `json:"expected_height"`
	CurrentHeight         uint64                `json:"current_height"`
	JobState              string                `json:"job_state"`
	LastModifiedAt        time.Time             `json:"last_modified_at"`
	TesseractQueueLen     uint64                `json:"tesseract_queue_length"`
	BestPeers             []identifiers.KramaID `json:"best_peers"`
	LatticeSyncInProgress bool                  `json:"lattice_sync_in_progress"`
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
	ID             identifiers.Identifier `json:"id"`
	Height         hexutil.Uint64         `json:"height"`
	TransitiveLink common.Hash            `json:"transitive_link"`
	LockedContext  common.Hash            `json:"locked_context"`
	ContextDelta   *common.DeltaGroup     `json:"context_delta"`
	StateHash      common.Hash            `json:"state_hash"`
}

type RPCParticipantsStates []RPCState

func (participants RPCParticipantsStates) Sort() {
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].ID.Hex() < participants[j].ID.Hex()
	})
}

type RPCPoXtData struct {
	Proposer     identifiers.KramaID                        `json:"operator"`
	BinaryHash   common.Hash                                `json:"binary_hash"`
	IdentityHash common.Hash                                `json:"identity_hash"`
	View         hexutil.Uint64                             `json:"view"`
	LastCommit   map[identifiers.Identifier]common.Hash     `json:"last_commit"`
	AccountLocks map[identifiers.Identifier]common.LockType `json:"account_locks"`
	ICSSeed      [32]byte                                   `json:"ics_seed"`
	ICSProof     hexutil.Bytes                              `json:"ics_proof"`
	EvidenceHash map[identifiers.Identifier]common.Hash     `json:"evidence_hash"`
}

func (t *RPCTesseract) HasParticipant(id identifiers.Identifier) bool {
	for _, s := range t.Participants {
		if s.ID == id {
			return true
		}
	}

	return false
}

func (t *RPCTesseract) Height(id identifiers.Identifier) uint64 {
	for _, p := range t.Participants {
		if p.ID == id {
			return p.Height.ToUint64()
		}
	}

	// return 1000 as we will not use 1000 tesseracts in tests
	return 1000
}

type RPCQc struct {
	Type          common.ConsensusMsgType `json:"type"`
	ID            identifiers.Identifier  `json:"id"`
	LockType      common.LockType         `json:"lock_type"`
	View          uint64                  `json:"view"`
	TSHash        common.Hash             `json:"ts_hash"`
	SignerIndices string                  `json:"signer_indices"`
	Signature     []byte                  `json:"signature"`
}

type RPCCommitInfo struct {
	QC                        *RPCQc                  `json:"commit_qc"`
	Operator                  identifiers.KramaID     `json:"operator"`
	ClusterID                 common.ClusterID        `json:"cluster_id"`
	View                      uint64                  `json:"commit_view"`
	RandomSet                 []common.ValidatorIndex `json:"random_set"`
	RandomSetSizeWithoutDelta uint32                  `json:"random_set_size"`
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
			ID:        info.QC.ID,
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
	ID          identifiers.Identifier `json:"id"`
	StateHash   common.Hash            `json:"state_hash"`
	ContextHash common.Hash            `json:"context_hash"`
}

type RPCHashes []Hashes

func (hashes RPCHashes) Sort() {
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].ID.Hex() < hashes[j].ID.Hex()
	})
}
