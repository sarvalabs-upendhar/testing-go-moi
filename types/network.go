package types

import (
	"math/big"

	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type ICSRequest struct {
	ClusterID            string
	Operator             string
	ContextLock          map[Address]ContextLockInfo
	IxData               []byte
	Ntq                  int32
	Timestamp            int64
	StakingContractState Hash
	ContextType          int32
}

type ICSResponse struct {
	ClusterID   string
	Response    int64
	StatusCode  int64
	RandomNodes []string
}

type ICSSuccessMsg struct {
	ClusterID   string
	RandomSet   []kramaid.KramaID
	ObserverSet []kramaid.KramaID
	Responses   []*ArrayOfBits
	Signature   []byte
	QuorumSizes []int
}
type MsgType int64

const (
	REQUESTMSG MsgType = iota
	RESPONSEMSG
	ICSSUCCESS
	NEWIXSMSG
	// NEWPEER
	RANDOMWALKREQ
	ACCSTATUSMSG
	ACCSYNCREQ
	ACCSYNCRRESP
	VOTEMSG
	NTQTABLESYNCREQ
	NTQTABLESYNCRESP
	HANDSHAKEMSG
	AGORAREQ
	AGORARESP
)

type ICSMSG struct {
	MsgType   MsgType
	Msg       []byte
	Sender    kramaid.KramaID
	ClusterID string
}
type HandshakeMSG struct {
	Address []string
	NTQ     int32
	Degree  int32
	Error   string
}
type ICSMetaInfo struct {
	ClusterID    string
	IxHash       Hash
	Operator     string
	ClusterSize  int
	ContextDelta map[string][]string
	GridID       Hash
	BinaryHash   Hash
	IdentityHash Hash
	IcsHash      Hash
	ReceiptHash  Hash
	Msgs         [][]byte
}

type ICSClusterInfo struct {
	RandomSet   []string
	ObserverSet []string
	Responses   []*ArrayOfBits
}

type Message struct {
	MsgType MsgType
	Sender  kramaid.KramaID
	Payload []byte
}

type RandomWalkReq struct {
	ReqID  int64
	Count  int32
	Topic  string
	PeerID kramaid.KramaID
}

type RandomWalkResp struct {
	ReqID    int64
	ID       kramaid.KramaID
	PeerAddr []string
}

type AccountsStatusMsg struct {
	TotalAccounts []byte
	BucketSizes   map[int32][]byte
	NTQ           float32
}

type AccountSyncRequest struct {
	BulkSync bool
	Bucket   int32
	Address  Address
}
type InteractionMsg struct {
	Ixs Interactions
}
type AccountSyncResponse struct {
	Slot     int32
	Bucket   int32
	Accounts []*AccountMetaInfo
}

type AccountMetaInfo struct {
	Address       Address
	Type          AccType
	Mode          string
	Height        *big.Int
	TesseractHash Hash
	LatticeExists bool
	StateExists   bool
}

type TesseractReq struct {
	Hash             Hash
	Number           uint64
	WithInteractions bool
}
type PeerInfo struct {
	ID      kramaid.KramaID
	Ntq     int32
	Address []string
	Degree  int64
}

type SyncReputationInfo struct {
	Msg []PeerInfo
}

type TesseractMessage struct {
	Tesseract *Tesseract
	Sender    kramaid.KramaID
	Delta     map[Hash][]byte
}

type HelloMsg struct {
	Info      PeerInfo
	Signature []byte
}

type AssetInfo struct {
	Owner       string
	Dimension   uint8
	TotalSupply uint64
	Symbol      string
	IsFungible  bool
	IsMintable  bool
}
