package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	networkmsgs "github.com/sarvalabs/go-moi/network/message"
)

type Vote struct {
	Type          common.ConsensusMsgType
	IsQC          bool
	View          uint64
	SignerIndex   int32
	Timestamp     int64
	TSHash        common.Hash
	SignerIndices *common.ArrayOfBits
	Signature     []byte
}

type CanonicalVote struct {
	Type   common.ConsensusMsgType
	View   uint64
	TSHash common.Hash
}

// Bytes polorizes and returns either the CanonicalVote in bytes or an error if failed to polorize.
func (cv *CanonicalVote) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(cv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize canonical vote")
	}

	return rawData, nil
}

// SignBytes creates a CanonicalVote from Vote.
// Polorizes and returns either the CanonicalVote in bytes or an error if failed to polorize.
func (v *Vote) SignBytes() ([]byte, error) {
	canonicalVote := CanonicalVote{
		Type:   v.Type,
		TSHash: v.TSHash,
		View:   v.View,
	}

	rawData, err := polo.Polorize(canonicalVote)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize vote")
	}

	return rawData, nil
}

// Bytes polorizes and returns either the vote in bytes or an error if failed to polorize.
func (v *Vote) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize vote")
	}

	return rawData, nil
}

// FromBytes deplorizes and updates the vote or returns an error if failed to depolorize.
func (v *Vote) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(v, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize vote")
	}

	return nil
}

func (v *Vote) Hash() (common.Hash, error) {
	return common.PoloHash(*v)
}

func (v *Vote) Validate() error {
	// TODO: Validate the vote
	return nil
}

type WALMsg struct {
	PeerID  identifiers.KramaID
	MsgType networkmsgs.MsgType
	Msg     []byte
}

type TimedWALMessage struct {
	ClusterID common.ClusterID
	Timestamp int64
	Message   *WALMsg
}

func (twm *TimedWALMessage) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(twm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize timed wal message")
	}

	return rawData, nil
}

func (twm *TimedWALMessage) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(twm, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize timed wal message")
	}

	return nil
}

type PreparedInfo struct {
	View          uint64
	PeerViews     []common.Views
	SignerIndices *common.ArrayOfBits
	Signature     []byte
}

type Proposal struct {
	Type      common.ConsensusMsgType
	PrepareQc *PreparedInfo
	Tesseract *common.Tesseract
}

func (p *Proposal) Copy() *Proposal {
	proposal := *p

	proposal.Tesseract = p.Tesseract.CopyWithCommitInfo()

	return &proposal
}

func (p *Proposal) Ixs() common.Interactions {
	return p.Tesseract.Interactions()
}

func (p Proposal) ProposalMsg() *ProposalMsg {
	ixn := p.Tesseract.Interactions()

	return &ProposalMsg{
		Type:         p.Type,
		PrepareQc:    p.PrepareQc,
		Tesseract:    p.Tesseract,
		Interactions: ixn.IxList(),
		CommitInfo:   p.Tesseract.CommitInfo(),
	}
}

type ProposalMsg struct {
	Type         common.ConsensusMsgType
	PrepareQc    *PreparedInfo
	Tesseract    *common.Tesseract
	Interactions []*common.Interaction
	CommitInfo   *common.CommitInfo
}

func (pmsg *ProposalMsg) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(pmsg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize proposal message")
	}

	return rawData, nil
}

func (pmsg *ProposalMsg) FromBytes(rawData []byte) error {
	if err := polo.Depolorize(pmsg, rawData); err != nil {
		return errors.Wrap(err, "failed to depolorize proposal message")
	}

	return nil
}

func (pmsg *ProposalMsg) Proposal() *Proposal {
	// TODO: Check if we need to deep copy on Tesseract
	ixns := common.NewInteractionsWithLeaderCheck(true, pmsg.Interactions...)
	pmsg.Tesseract.WithIxnAndReceipts(ixns, nil, pmsg.CommitInfo)

	return &Proposal{
		Type:      pmsg.Type,
		PrepareQc: pmsg.PrepareQc,
		Tesseract: pmsg.Tesseract,
	}
}

func (p *Proposal) Validate() error {
	return nil
}

func (p *Proposal) Heights() map[identifiers.Identifier]uint64 {
	return p.Tesseract.Heights()
}

func (p *Proposal) View() uint64 {
	return p.PrepareQc.View
}

func (p *Proposal) ClusterID() common.ClusterID {
	return p.Tesseract.ClusterID()
}

func (p *Proposal) Locks() map[identifiers.Identifier]common.LockType {
	return p.Tesseract.ConsensusInfo().AccountLocks
}

func NewProposal(prepareQc *PreparedInfo, ts *common.Tesseract) *Proposal {
	return &Proposal{
		Type:      common.PROPOSAL,
		PrepareQc: prepareQc,
		Tesseract: ts,
	}
}

// Bytes polorizes and returns either the Proposal in bytes or an error if failed to polorize.
func (p *Proposal) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(p)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize proposal")
	}

	return rawData, nil
}

// ConsensusPayload is an interface that all consensus message types should implement
type ConsensusPayload interface {
	Validate() error
	Bytes() ([]byte, error)
}

// ConsensusMessage is a struct that represents an envelope for a consensus message.
// Implements the ConsensusPayload interface and wraps another message satisfying this interface.
type ConsensusMessage struct {
	PeerID    identifiers.KramaID
	Recipient identifiers.KramaID
	// Represents the wrapped message
	Payload ConsensusPayload
}

// ICSMsg returns a new instance of ICSMSG
func (c *ConsensusMessage) ICSMsg(clusterID common.ClusterID) (*ICSMSG, error) {
	msgType, msg, err := getRawMessage(c.Payload)
	if err != nil {
		return nil, err
	}

	return &ICSMSG{
		Sender:    c.PeerID,
		ClusterID: clusterID,
		MsgType:   msgType,
		Payload:   msg,
	}, nil
}

// WALMsg returns a new instance of WALMsg
func (c *ConsensusMessage) WALMsg() (*WALMsg, error) {
	msgType, msg, err := getRawMessage(c.Payload)
	if err != nil {
		return nil, err
	}

	return &WALMsg{
		c.PeerID,
		msgType,
		msg,
	}, nil
}

// getRawMessage returns the message type and raw message.
func getRawMessage(message ConsensusPayload) (networkmsgs.MsgType, []byte, error) {
	switch msg := message.(type) {
	case *Vote:
		rawData, err := msg.Bytes()
		if err != nil {
			return -1, nil, err
		}

		return networkmsgs.VOTEMSG, rawData, nil
	case *Proposal:
		// we create a proposal message from the proposal
		if len(msg.Tesseract.Interactions().IxList()) == 0 {
			panic("interactions length is 0")
		}

		rawData, err := msg.ProposalMsg().Bytes()
		if err != nil {
			return -1, nil, err
		}

		return networkmsgs.PROPOSAL, rawData, nil
	default:
		return -1, nil, errors.New("invalid message type")
	}
}

// Validate is a method of ConsensusMessage to implement the ConsensusPayload interface.
// Returns an error if message is not valid or could not be validated.
func (c *ConsensusMessage) Validate() error {
	return nil
}

type WantMessage struct {
	PeerID   identifiers.KramaID
	MsgType  common.ConsensusMsgType
	MsgIdxs  []int32
	RespChan chan []*Vote
}

type VoteBitSet struct {
	Prevotes   *common.ArrayOfBits
	Precommits *common.ArrayOfBits
}

type SafetyData struct {
	Qc             []*common.Qc
	TSHashes       []common.Hash
	ProposalTSHash common.Hash
}

func (sd *SafetyData) UpdateQc(qc *common.Qc) {
	if len(sd.Qc) > 0 {
		// TODO: Make sure we save the qc for read lock accounts
		if sd.Qc[len(sd.Qc)-1].View < qc.View {
			sd.Qc = []*common.Qc{qc}
			sd.TSHashes = []common.Hash{qc.TSHash}

			return
		}

		if sd.Qc[len(sd.Qc)-1].View == qc.View && sd.Qc[len(sd.Qc)-1].Type == common.PREVOTE {
			sd.Qc[len(sd.Qc)-1] = qc

			return
		}
	}

	sd.Qc = append(sd.Qc, qc)
	sd.TSHashes = append(sd.TSHashes, qc.TSHash)
}

func (sd *SafetyData) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(sd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize safety data")
	}

	return rawData, nil
}

func (sd *SafetyData) FromBytes(raw []byte) error {
	if err := polo.Depolorize(sd, raw); err != nil {
		return errors.Wrap(err, "failed to depolorize safety data")
	}

	return nil
}

func (sd *SafetyData) LastView() uint64 {
	return sd.Qc[len(sd.Qc)-1].View
}
