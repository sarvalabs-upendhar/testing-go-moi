package types

import (
	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/moichain/mudra/kramaid"
)

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

type Message struct {
	MsgType MsgType
	Sender  kramaid.KramaID
	Payload []byte
}

type ICSRequest struct {
	ClusterID            string
	Operator             string
	ContextLock          map[types.Address]types.ContextLockInfo
	IxData               []byte
	Ntq                  int32
	Timestamp            int64
	StakingContractState types.Hash
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
	Responses   []*types.ArrayOfBits
	Signature   []byte
	QuorumSizes []int
}

type HandshakeMSG struct {
	Address []string
	NTQ     int32
	Degree  int32
	Error   string
}

type ICSClusterInfo struct {
	RandomSet   []string
	ObserverSet []string
	Responses   []*types.ArrayOfBits
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
	Address  types.Address
}

type InteractionMsg struct {
	Ixs types.Interactions
}

type AccountSyncResponse struct {
	Slot     int32
	Bucket   int32
	Accounts []*types.AccountMetaInfo
}

type TesseractReq struct {
	Hash             types.Hash
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
	Tesseract *types.Tesseract
	Sender    kramaid.KramaID
	Delta     map[types.Hash][]byte
}

type HelloMsg struct {
	Info      PeerInfo
	Signature []byte
}
