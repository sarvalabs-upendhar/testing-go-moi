package types

import (
	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/crypto"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-polo"
)

type ICSResponseCode int32

const (
	SlotsFull ICSResponseCode = iota + 1
	InvalidHash
	InvalidInteractions
	InternalError
	NotEligible
	Success
)

func (rc ICSResponseCode) String() string {
	switch rc {
	case SlotsFull:
		return "SlotsFull"
	case InvalidHash:
		return "InvalidHash"
	case InvalidInteractions:
		return "InvalidInteractions"
	case InternalError:
		return "InternalError"
	case NotEligible:
		return "NotEligible"
	case Success:
		return "Success"
	}

	return "Invalid Status Code"
}

type signer func(data []byte, sigType mudraCommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)

type ICSPayload interface {
	Bytes() ([]byte, error)
	FromBytes(bytes []byte) error
}

type ICSMSG struct {
	Sender           id.KramaID
	ClusterID        common.ClusterID
	MsgType          message.MsgType
	UnCompressedSize int // UnCompressedSize will be zero, if payload is not compressed
	Payload          []byte
	DecodedMsg       interface{} `polo:"-"`
	ReceivedFrom     id.KramaID  `polo:"-"`
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

// CompressPayload compresses the ICS message payload if it exceeds the compression threshold.
func (im *ICSMSG) CompressPayload(compressor common.Compressor) error {
	size := len(im.Payload)

	if size >= common.CompressionThreshold {
		data, err := compressor.Compress(im.Payload)
		if err != nil {
			return errors.Wrap(err, "failed to compress ics payload")
		}

		im.Payload = data
		im.UnCompressedSize = size
	}

	return nil
}

// DeCompressPayload decompresses the ICS message payload if it was previously compressed.
func (im *ICSMSG) DeCompressPayload(compressor common.Compressor) error {
	if im.UnCompressedSize == 0 {
		return nil
	}

	dst, err := compressor.Decompress(im.Payload, im.UnCompressedSize)
	if err != nil {
		return errors.Wrap(err, "failed to decompress ics payload")
	}

	im.Payload = dst

	return nil
}

type Prepare struct {
	View uint64
	Ixns []common.Hash
	Ps   []identifiers.Identifier
}

func (pm *Prepare) Bytes() ([]byte, error) {
	raw, err := polo.Polorize(pm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize prepare msg")
	}

	return raw, nil
}

func (pm *Prepare) FromBytes(data []byte) error {
	if err := polo.Depolorize(pm, data); err != nil {
		return errors.Wrap(err, "failed to de polorize prepare msg")
	}

	return nil
}

type Prepared struct {
	View      uint64
	Infos     common.Views
	Signature []byte
}

func (pdMsg *Prepared) Validate() error {
	return nil
}

func (pdMsg *Prepared) Sign(sign signer) error {
	if len(pdMsg.Signature) != 0 {
		return errors.New("non nil signature")
	}

	rawMsg, err := pdMsg.SignBytes()
	if err != nil {
		return err
	}

	pdMsg.Signature, err = sign(rawMsg, mudraCommon.BlsBLST)
	if err != nil {
		return err
	}

	return nil
}

func (pdMsg *Prepared) SignBytes() ([]byte, error) {
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(pdMsg.View); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(pdMsg.Infos); err != nil {
		return nil, err
	}

	return polorizer.Bytes(), nil
}

func (pdMsg *Prepared) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(pdMsg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ics message")
	}

	return rawData, nil
}

func (pdMsg *Prepared) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(pdMsg, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize ics message")
	}

	return nil
}

type ICSHave struct {
	Proposal        *Proposal
	Votes           []*Vote
	ViewVoteBitSets map[uint64]*VoteBitSet
}

func NewICSHave(response map[uint64]*VoteBitSet, proposal *Proposal, votes ...*Vote) *ICSHave {
	return &ICSHave{
		Proposal:        proposal,
		Votes:           votes,
		ViewVoteBitSets: response,
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
	ViewVoteBitSets map[uint64]*VoteBitSet // Array index is the round number
}

func NewICSWant(set map[uint64]*VoteBitSet) *ICSWant {
	return &ICSWant{
		ViewVoteBitSets: set,
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
