package types

import (
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	networkmsgs "github.com/sarvalabs/go-moi/network/message"
)

type (
	ConsensusMsgType int
	WALMsgType       int
)

const (
	PROPOSAL ConsensusMsgType = iota
	PREVOTE
	PRECOMMIT
)

type Vote struct {
	Type           ConsensusMsgType
	Round          int32
	Heights        map[identifiers.Address]uint64
	TSHash         common.Hash
	Timestamp      int64
	ValidatorIndex int32
	Signature      []byte
}

type CanonicalVote struct {
	Type   ConsensusMsgType
	Round  int32
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
		Round:  v.Round,
		TSHash: v.TSHash,
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

func (v *Vote) Validate() error {
	// TODO: Validate the vote
	return nil
}

type WALMsg struct {
	PeerID  kramaid.KramaID
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

type Proposal struct {
	Type      ConsensusMsgType
	Height    map[identifiers.Address]uint64
	Round     int32
	POLRound  int32
	Tesseract *common.Tesseract
	Timestamp int64
	Signature []byte
}

// NewProposal is a constructor function that generates and returns a new Proposal.
// Accepts the heights, round, POL round and a tesseract.
// Timestamp of the proposal is set to Now()
func NewProposal(
	heights map[identifiers.Address]uint64,
	round int32,
	polround int32,
	ts *common.Tesseract,
) *Proposal {
	return &Proposal{
		Type:      PROPOSAL,
		Height:    heights,
		Round:     round,
		POLRound:  polround,
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

// Cmessage is an interface that represents messages used in achieving consensus
type Cmessage interface {
	// Validate is a method that validates the message
	Validate() error
	Bytes() ([]byte, error)
}

// ConsensusMessage is a struct that represents an envelope for a consensus message.
// Implements the Cmessage interface and wraps another message satisfying this interface.
type ConsensusMessage struct {
	// Represents the KipID of the message sender
	PeerID kramaid.KramaID

	// Represents the wrapped message
	Message Cmessage
}

// ICSMsg returns a new instance of ICSMSG
func (c *ConsensusMessage) ICSMsg(clusterID common.ClusterID) (*ICSMSG, error) {
	msgType, msg, err := getRawMessage(c.Message)
	if err != nil {
		return nil, err
	}

	return &ICSMSG{
		msgType,
		msg,
		c.PeerID,
		string(clusterID),
	}, nil
}

// WALMsg returns a new instance of WALMsg
func (c *ConsensusMessage) WALMsg() (*WALMsg, error) {
	msgType, msg, err := getRawMessage(c.Message)
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
func getRawMessage(message Cmessage) (networkmsgs.MsgType, []byte, error) {
	switch msg := message.(type) {
	case *VoteMessage:
		rawData, err := msg.Vote.Bytes()
		if err != nil {
			return -1, nil, err
		}

		return networkmsgs.VOTEMSG, rawData, nil
	case *ProposalMessage:
		rawData, err := msg.Proposal.Bytes()
		if err != nil {
			return -1, nil, err
		}

		return networkmsgs.PROPOSALMSG, rawData, nil
	default:
		return -1, nil, errors.New("invalid message type")
	}
}

// Validate is a method of ConsensusMessage to implement the Cmessage interface.
// Returns an error if message is not valid or could not be validated.
func (c *ConsensusMessage) Validate() error {
	return nil
}

// ProposalMessage is a struct that represents a Proposal consensus message.
// Implements the Cmessage interface.
type ProposalMessage struct {
	// Represents the wrapped proposal message
	Proposal *Proposal
}

// Validate is a method of ProposalMessage to implement the Cmessage interface.
// Returns an error if message is not valid or could not be validated.
func (m *ProposalMessage) Validate() error {
	return nil
}

// Bytes is a method of ProposalMessage to implement the Cmessage interface.
// Returns either the ProposalMessage in bytes or an error if failed to polorize.
func (m *ProposalMessage) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize vote message")
	}

	return rawData, err
}

// VoteMessage is a struct that represents a Vote consensus message.
// Implements the Cmessage interface.
type VoteMessage struct {
	Vote *Vote
}

// Validate is a method of VoteMessage to implement the Cmessage interface.
// Returns an error if message is not valid or could not be validated.
func (m *VoteMessage) Validate() error {
	return m.Vote.Validate()
}

// Bytes is a method of VoteMessage to implement the Cmessage interface.
// Returns either the VoteMessage in bytes or an error if failed to polorize.
func (m *VoteMessage) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize vote message")
	}

	return rawData, err
}
