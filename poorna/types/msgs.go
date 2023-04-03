package types

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type MsgType int64

const (
	REQUESTMSG MsgType = iota + 1
	RESPONSEMSG
	ICSSUCCESS
	NEWIXSMSG
	// NEWPEER
	RANDOMWALKREQ
	ACCSTATUSMSG
	ACCSYNCREQ
	ACCSYNCRRESP
	PROPOSALMSG
	VOTEMSG
	NTQTABLESYNCREQ
	NTQTABLESYNCRESP
	HANDSHAKEMSG
	AGORAREQ
	AGORARESP
	DISCONNECTREQ
)

const (
	SlotsFull ICSResponseCode = iota + 1
	InvalidHash
	InvalidInteractions
	InternalError
	Success
)

type ICSResponseCode int32

type MessagePayload interface {
	Bytes() ([]byte, error)
	FromBytes(bytes []byte) error
}

type Message struct {
	MsgType MsgType
	Sender  kramaid.KramaID
	Payload []byte
}

var NilMessage Message

func (m *Message) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize message")
	}

	return rawData, nil
}

func (m *Message) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(m, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize message")
	}

	return nil
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
	StatusCode  ICSResponseCode
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

type DisconnectReq struct {
	Reason string
}

func (disconnReq *DisconnectReq) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(disconnReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize disconnect request message")
	}

	return rawData, nil
}

func (disconnReq *DisconnectReq) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(disconnReq, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize disconnect request message")
	}

	return nil
}

func (ism *ICSSuccessMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ism)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics success message")
	}

	return rawData, nil
}

func (ism *ICSSuccessMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ism, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics success message")
	}

	return nil
}

type HandshakeMSG struct {
	Address []string
	NTQ     float32
	Degree  int32
	Error   string
}

var NilHandshakeMSG HandshakeMSG

func ConstructHandshakeMSG(
	address []string,
	ntq float32,
	degree int32,
	err string,
) HandshakeMSG {
	return HandshakeMSG{
		Address: address,
		NTQ:     ntq,
		Degree:  degree,
		Error:   err,
	}
}

func (m *Message) IsHandShakeMessage() bool {
	var hsMsg HandshakeMSG

	if err := hsMsg.FromBytes(m.Payload); err != nil {
		return false
	}

	return true
}

func (hs *HandshakeMSG) Bytes() ([]byte, error) {
	rawBytes, err := polo.Polorize(hs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize handshake message")
	}

	return rawBytes, nil
}

func (hs *HandshakeMSG) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(hs, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize handshake message")
	}

	return nil
}

type ICSClusterInfo struct {
	RandomSet   []string
	ObserverSet []string
	Responses   []*types.ArrayOfBits
}

func (ci *ICSClusterInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ci)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics cluster info")
	}

	return rawData, nil
}

func (ci *ICSClusterInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ci, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics cluster info")
	}

	return nil
}

type RandomWalkReq struct {
	ReqID  int64
	Count  int32
	Topic  string
	PeerID kramaid.KramaID
}

func (rwr *RandomWalkReq) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rwr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize random walk request")
	}

	return nil
}

type RandomWalkResp struct {
	ReqID    int64
	ID       kramaid.KramaID
	PeerAddr []string
}

func (rwr *RandomWalkResp) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(rwr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize random walk response")
	}

	return rawData, nil
}

func (rwr *RandomWalkResp) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rwr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize random walk response")
	}

	return nil
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

func (asr *AccountSyncRequest) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(asr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account sync request")
	}

	return rawData, nil
}

func (asr *AccountSyncRequest) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(asr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account sync request")
	}

	return nil
}

type AccountSyncResponse struct {
	Slot     int32
	Bucket   int32
	Accounts []*types.AccountMetaInfo
}

func (asr *AccountSyncResponse) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(asr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account sync request")
	}

	return nil
}

type TesseractReq struct {
	Hash             types.Hash
	Number           uint64
	WithInteractions bool
}

type PeerInfo struct {
	ID   peer.ID
	Data []byte
}

func (pi *PeerInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(pi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize peer info")
	}

	return rawData, nil
}

type SyncReputationInfo struct {
	Msg []PeerInfo
}

type TesseractMessage struct {
	Sender             kramaid.KramaID
	CanonicalTesseract *types.CanonicalTesseract
	Ixns               []byte
	Delta              map[types.Hash][]byte
}

func (tm *TesseractMessage) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(tm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract message")
	}

	return rawData, nil
}

func (tm *TesseractMessage) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(tm, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract message")
	}

	return nil
}

func (tm *TesseractMessage) Tesseract() (*types.Tesseract, error) {
	var ixns types.Interactions

	if err := ixns.FromBytes(tm.Ixns); err != nil {
		if !errors.Is(err, polo.ErrNullPack) {
			return nil, err
		}
	}

	return types.NewTesseract(
		tm.CanonicalTesseract.Header,
		tm.CanonicalTesseract.Body,
		ixns,
		nil,
		tm.CanonicalTesseract.Seal,
	), nil
}

type HelloMsg struct {
	KramaID   kramaid.KramaID
	Address   []string
	Signature []byte
}

func (hm *HelloMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(hm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize hello message")
	}

	return rawData, nil
}

func (hm *HelloMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(hm, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize hello message")
	}

	return nil
}
