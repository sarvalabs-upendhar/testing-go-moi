package types

import (
	"bytes"
	"encoding/json"
	"log"

	"github.com/pkg/errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
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

type IcsSetType int

const (
	SenderBehaviourSet IcsSetType = iota
	SenderRandomSet
	ReceiverBehaviourSet
	ReceiverRandomSet
	RandomSet
	ObserverSet
)

type Vote struct {
	Type           ConsensusMsgType
	Round          int32
	GridID         *types.TesseractGridID
	Timestamp      int64
	ValidatorIndex int32
	Signature      []byte
}

type CanonicalVote struct {
	Type   ConsensusMsgType
	Round  int32
	GridID *types.TesseractGridID
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
		GridID: v.GridID,
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
	PeerID  id.KramaID
	MsgType ptypes.MsgType
	Msg     []byte
}

type TimedWALMessage struct {
	ClusterID types.ClusterID
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
	Height    []uint64
	Round     int32
	POLRound  int32
	Grid      *TesseractGrid
	GridID    *types.TesseractGridID
	Timestamp int64
	Signature []byte
}

// NewProposal is a constructor function that generates and returns a new Proposal.
// Accepts the heights, round, POL round and a tesseract grid id.
// Timestamp of the proposal is set to Now()
func NewProposal(
	heights []uint64,
	round int32,
	polround int32,
	grid *TesseractGrid,
	gridID *types.TesseractGridID,
) *Proposal {
	return &Proposal{
		Type:     PROPOSAL,
		Height:   heights,
		Round:    round,
		POLRound: polround,
		GridID:   gridID,
		Grid:     grid,
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
	PeerID id.KramaID

	// Represents the wrapped message
	Message Cmessage
}

// ICSMsg returns a new instance of ICSMSG
func (c *ConsensusMessage) ICSMsg(clusterID types.ClusterID) (*ICSMSG, error) {
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
func getRawMessage(message Cmessage) (ptypes.MsgType, []byte, error) {
	switch msg := message.(type) {
	case *VoteMessage:
		rawData, err := msg.Vote.Bytes()
		if err != nil {
			return -1, nil, err
		}

		return ptypes.VOTEMSG, rawData, nil
	case *ProposalMessage:
		rawData, err := msg.Proposal.Bytes()
		if err != nil {
			return -1, nil, err
		}

		return ptypes.PROPOSALMSG, rawData, nil
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

type TesseractGrid struct {
	Hash       types.Hash
	Total      int32
	Tesseracts []*types.Tesseract
}

// GetTesseractGridID creates and returns a new instance of TesseractGridID
func (t *TesseractGrid) GetTesseractGridID() (*types.TesseractGridID, error) {
	gridID := &types.TesseractGridID{
		Hash: t.Hash,
		Parts: &types.TesseractParts{
			Total:   t.Total,
			Hashes:  make([]types.Hash, 0, len(t.Tesseracts)),
			Heights: make([]uint64, 0, len(t.Tesseracts)),
		},
	}

	for _, tesseract := range t.Tesseracts {
		tsHash, err := tesseract.Hash()
		if err != nil {
			return nil, err
		}

		gridID.Parts.Hashes = append(gridID.Parts.Hashes, tsHash)
		gridID.Parts.Heights = append(gridID.Parts.Heights, tesseract.Header.Height)
	}

	return gridID, nil
}

// CompareHash checks whether the given grid hash argument matches with the tesseract grid hash
func (t *TesseractGrid) CompareHash(gridHash types.Hash) bool {
	if len(gridHash.Bytes()) == 0 {
		return false
	}

	if t == nil {
		return false
	}

	return bytes.Equal(t.Hash.Bytes(), gridHash.Bytes())
}

type NodeSet struct {
	Ids         []id.KramaID
	PublicKeys  [][]byte
	Responses   *types.ArrayOfBits
	VotingPower []int64
	Count       int
	QuorumSize  int
}

// NewNodeSet creates and returns a new instance of NodeSet
func NewNodeSet(ids []id.KramaID, keys [][]byte) *NodeSet {
	return &NodeSet{
		Ids:         ids,
		PublicKeys:  keys,
		Responses:   types.NewArrayOfBits(len(ids)),
		VotingPower: make([]int64, len(ids)),
		Count:       0,
	}
}

type ICSNodeSet struct {
	Nodes []*NodeSet
	Size  int
}

// NewICSNodeSet creates and returns a new instance of ICSNodes
func NewICSNodeSet(size int) *ICSNodeSet {
	ics := &ICSNodeSet{
		Nodes: make([]*NodeSet, size),
		Size:  0,
	}

	return ics
}

// GetKramaID returns the slot id, slot index, krama id and bls public key of the validator node based on the index
func (i *ICSNodeSet) GetKramaID(index int32) (slots []int, slotIndex int, kramaID id.KramaID, publicKey []byte) {
	if index < 0 || int(index) >= i.Size {
		return nil, -1, "", nil
	}

	slots = make([]int, 0, 5)

	for v, set := range i.Nodes {
		if set == nil {
			continue
		}

		if v == len(i.Nodes)-1 {
			return nil, -1, "", nil
		}

		if int(index) >= len(set.Ids) {
			index -= int32(len(set.Ids))

			continue
		}

		slots = append(slots, v)

		for j := v + 1; j < len(i.Nodes)-1; j++ {
			// check for empty set
			if i.Nodes[j] == nil {
				continue
			}
			// check for krama ID on not empty set
			for _, kID := range i.Nodes[j].Ids {
				if kID == set.Ids[index] {
					slots = append(slots, j)
				}
			}
		}

		return slots, int(index), set.Ids[index], set.PublicKeys[index]
	}

	return nil, -1, "", nil
}

// GetIndex returns the index and existence status of the validator node from ICSNodes based on the krama id
func (i *ICSNodeSet) GetIndex(peerID id.KramaID) (int32, bool) {
	offset := 0

	for index, set := range i.Nodes {
		if index == len(i.Nodes)-1 {
			break
		}

		if set == nil {
			continue
		}

		for j, kramaID := range set.Ids {
			if kramaID == peerID {
				return int32(offset + j), set.Responses.GetIndex(j)
			}
		}

		offset += len(set.Ids)
	}

	return -1, false
}

// UpdateNodeSet updates the specific node set of the ICSNodes based on the node set type
func (i *ICSNodeSet) UpdateNodeSet(setType IcsSetType, data *NodeSet) {
	if data == nil {
		return
	}

	i.Nodes[setType] = data
	i.Size += len(data.Ids)
}

// GetNodes returns krama id's of all the nodes from the ICSNodes nodeset
func (i *ICSNodeSet) GetNodes() []id.KramaID {
	var nodes []id.KramaID

	for _, nodeSet := range i.Nodes {
		if nodeSet != nil {
			nodes = append(nodes, nodeSet.Ids...)
		}
	}

	return nodes
}

// IsContextQuorum check's whether context quorum condition is satisfied or not
func (i *ICSNodeSet) IsContextQuorum() bool {
	for j := 0; j < 4; j += 2 {
		count := 0
		quorum := 0

		if i.Nodes[j] != nil {
			count += i.Nodes[j].Count
			quorum += len(i.Nodes[j].Ids)
		}

		if i.Nodes[j+1] != nil {
			count += i.Nodes[j+1].Count
			quorum += len(i.Nodes[j].Ids)
		}

		if quorum > 0 && count < quorum*2/3+1 {
			log.Println("Quorum conditions failed", count, quorum, quorum*2/3+1)

			return false
		}
	}

	return true
}

// IsRandomQuorum check's whether random quorum condition is satisfied or not
func (i *ICSNodeSet) IsRandomQuorum(requiredRandomNodes int) bool {
	return i.Nodes[RandomSet].Count >= requiredRandomNodes
}

// SenderSetSize returns the sum of number of nodes in the sender's behaviour node set and random node set
func (i *ICSNodeSet) SenderSetSize() int {
	count := 0

	if i.Nodes[SenderBehaviourSet] != nil {
		count += len(i.Nodes[SenderBehaviourSet].Ids)
	}

	if i.Nodes[SenderRandomSet] != nil {
		count += len(i.Nodes[SenderRandomSet].Ids)
	}

	if count <= 0 {
		return 0
	}

	return count
}

// ReceiverSetSize returns the sum of number of nodes in the receiver's behaviour node set and random node set
func (i *ICSNodeSet) ReceiverSetSize() int {
	count := 0

	if i.Nodes[ReceiverBehaviourSet] != nil {
		count += len(i.Nodes[ReceiverBehaviourSet].Ids)
	}

	if i.Nodes[ReceiverRandomSet] != nil {
		count += len(i.Nodes[ReceiverRandomSet].Ids)
	}

	if count <= 0 {
		return 0
	}

	return count
}

// RandomSetSize returns the random node set size
func (i *ICSNodeSet) RandomSetSize() int {
	count := len(i.Nodes[RandomSet].Ids)
	if count <= 0 {
		return 0
	}

	return count
}

// SenderQuorumSize returns the sender's quorum size
func (i *ICSNodeSet) SenderQuorumSize() int {
	count := i.SenderSetSize()
	if count <= 0 {
		return 0
	}

	return count*2/3 + 1
}

// ReceiverQuorumSize returns the receiver's quorum size
func (i *ICSNodeSet) ReceiverQuorumSize() int {
	count := i.ReceiverSetSize()
	if count <= 0 {
		return 0
	}

	return count*2/3 + 1
}

// RandomQuorumSize returns the random quorum size
func (i *ICSNodeSet) RandomQuorumSize() int {
	return i.Nodes[RandomSet].QuorumSize*2/3 + 1
}

// String returns the ICSNodes in string
func (i *ICSNodeSet) String() string {
	rawBytes, err := json.Marshal(i)
	if err != nil {
		return "failed to print ics nodes"
	}

	return string(rawBytes)
}
