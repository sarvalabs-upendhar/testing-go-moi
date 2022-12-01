package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
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
	PROPOSALMSG
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
	NTQ     int32
	Degree  int32
	Error   string
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

type InteractionMsg struct {
	Ixs types.Interactions
}

func (im *InteractionMsg) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(im, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize interaction message")
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
	ID      kramaid.KramaID
	Ntq     int32
	Address []string
	Degree  int64
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
	Tesseract *types.Tesseract
	Sender    kramaid.KramaID
	Delta     map[types.Hash][]byte
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

type HelloMsg struct {
	Info      PeerInfo
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
