package types

import (
	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-polo"
)

type ICSResponseCode int32

const (
	SlotsFull ICSResponseCode = iota + 1
	InvalidHash
	InvalidInteractions
	InternalError
	Success
)

type ICSPayload interface {
	Bytes() ([]byte, error)
	FromBytes(bytes []byte) error
}

type ICSMSG struct {
	Sender       id.KramaID
	ClusterID    common.ClusterID
	MsgType      message.MsgType
	Payload      []byte
	DecodedMsg   interface{} `polo:"-"`
	ReceivedFrom id.KramaID  `polo:"-"`
}

func NewICSMsg(sender id.KramaID, clusterID common.ClusterID, msgType message.MsgType, payload []byte) *ICSMSG {
	return &ICSMSG{
		Sender:    sender,
		ClusterID: clusterID,
		MsgType:   msgType,
		Payload:   payload,
	}
}

func (im *ICSMSG) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(im)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics message")
	}

	return rawData, nil
}

func (im *ICSMSG) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(im, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics message")
	}

	return nil
}

type CanonicalICSRequest struct {
	ClusterID               common.ClusterID
	Operator                string
	ContextLock             map[identifiers.Address]common.ContextLockInfo
	IxData                  []byte
	Timestamp               int64
	StakingContractState    common.Hash
	RandomSet               []id.KramaID
	ObserverSet             []id.KramaID
	RequiredRandomSetSize   uint32
	RequiredObserverSetSize uint32
}

func (ics CanonicalICSRequest) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ics)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize canonical ics request")
	}

	return rawData, nil
}

func (ics *CanonicalICSRequest) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ics, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize canonical ics request")
	}

	return nil
}

type ICSRequest struct {
	ReqData   []byte
	Signature []byte
}

func NewICSRequest(rawICSReq []byte, signature []byte) *ICSRequest {
	return &ICSRequest{
		ReqData:   rawICSReq,
		Signature: signature,
	}
}

func (ir ICSRequest) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics request message")
	}

	return rawData, nil
}

func (ir *ICSRequest) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ir, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics request message")
	}

	return nil
}

type ICSResponse struct {
	StatusCode ICSResponseCode
}

func NewICSResponse(statusCode ICSResponseCode) *ICSResponse {
	return &ICSResponse{
		StatusCode: statusCode,
	}
}

func (ir ICSResponse) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics response message")
	}

	return rawData, nil
}

func (ir *ICSResponse) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ir, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics response message")
	}

	return nil
}

type ICSSuccess struct {
	ClusterID common.ClusterID
	Responses []*common.ArrayOfBits
	Signature []byte
}

func NewICSSuccess(responses []*common.ArrayOfBits, signature []byte) *ICSSuccess {
	return &ICSSuccess{
		Responses: responses,
		Signature: signature,
	}
}

func (is ICSSuccess) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(is)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics success message")
	}

	return rawData, nil
}

func (is *ICSSuccess) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(is, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics success message")
	}

	return nil
}

type ICSFailure struct {
	ClusterID common.ClusterID
}

func NewICSFailure(clusterID common.ClusterID) *ICSFailure {
	return &ICSFailure{
		ClusterID: clusterID,
	}
}

func (ifr ICSFailure) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ifr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics failure message")
	}

	return rawData, nil
}

func (ifr *ICSFailure) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ifr, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics failure message")
	}

	return nil
}

type ICSHave struct {
	Votes            []*Vote
	RoundVoteBitSets map[int32]*VoteBitSet
}

func NewICSHave(response map[int32]*VoteBitSet, vote ...*Vote) *ICSHave {
	return &ICSHave{
		Votes:            vote,
		RoundVoteBitSets: response,
	}
}

func (ih ICSHave) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ih)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics have message")
	}

	return rawData, nil
}

func (ih *ICSHave) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ih, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics have message")
	}

	return nil
}

type ICSWant struct {
	RoundVoteBitSets map[int32]*VoteBitSet // Array index is the round number
}

func NewICSWant(set map[int32]*VoteBitSet) *ICSWant {
	return &ICSWant{
		RoundVoteBitSets: set,
	}
}

func (iw ICSWant) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(iw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics want message")
	}

	return rawData, nil
}

func (iw *ICSWant) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(iw, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics want message")
	}

	return nil
}

type ICSGraft struct {
	Data []byte
}

func NewICSGraft(data []byte) *ICSGraft {
	return &ICSGraft{
		Data: data,
	}
}

func (ig ICSGraft) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics graft message")
	}

	return rawData, nil
}

func (ig *ICSGraft) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ig, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics graft message")
	}

	return nil
}

type ICSPrune struct {
	Data []byte
}

func NewICSPrune(data []byte) *ICSPrune {
	return &ICSPrune{
		Data: data,
	}
}

func (ip ICSPrune) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ip)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics prune message")
	}

	return rawData, nil
}

func (ip *ICSPrune) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ip, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics prune message")
	}

	return nil
}
