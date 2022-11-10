package types

import (
	"bytes"
	"encoding/json"
	"log"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
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
	GridID         *TesseractGridID
	Timestamp      int64
	ValidatorIndex int32
	Signature      []byte
}
type CanonicalVote struct {
	Type   ConsensusMsgType
	Round  int32
	GridID *TesseractGridID
}

func (v *Vote) SignBytes() []byte {
	canonicalVote := CanonicalVote{
		Type:   v.Type,
		Round:  v.Round,
		GridID: v.GridID,
	}

	return polo.Polorize(canonicalVote)
}

func (v *Vote) Bytes() []byte {
	return polo.Polorize(v)
}

func (v *Vote) Validate() error {
	// TODO: Validate the vote
	return nil
}

type TesseractGridID struct {
	Hash  Hash
	Parts *TesseractParts
}

func (tid *TesseractGridID) IsNil() bool {
	return tid.Hash == NilHash && len(tid.Parts.Hashes) == 0
}

func (tid *TesseractGridID) String() string {
	if !tid.IsNil() {
		return tid.Hash.Hex()
	}

	return "Nil"
}

type TesseractParts struct {
	Total   int32
	Hashes  []Hash
	Heights []uint64
}

type TimedWALMessage struct {
	ClusterID ClusterID
	Timestamp int64
	Message   ConsensusMessage
}

type Proposal struct {
	Height    []uint64
	Round     int32
	POLRound  int32
	Grid      *TesseractGrid
	GridID    *TesseractGridID
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
	gridID *TesseractGridID,
) *Proposal {
	return &Proposal{
		Height:   heights,
		Round:    round,
		POLRound: polround,
		GridID:   gridID,
		Grid:     grid,
	}
}

// Cmessage is an interface that represents messages used in achieving consensus
type Cmessage interface {
	// Validate is a method that validates the message
	Validate() error
}

// ConsensusMessage is a struct that represents an envelope for a consensus message.
// Implements the Cmessage interface and wraps another message satisfying this interface.
type ConsensusMessage struct {
	// Represents the KipID of the message sender
	PeerID id.KramaID

	// Represents the wrapped message
	Message Cmessage
}

func (c *ConsensusMessage) ICSMsg(clusterID ClusterID) *ICSMSG {
	var (
		msgType MsgType
		rawData []byte
	)

	switch msg := c.Message.(type) {
	case *VoteMessage:
		rawData = msg.Vote.Bytes()
		msgType = VOTEMSG
	default:
		return nil
	}

	return &ICSMSG{
		msgType,
		rawData,
		c.PeerID,
		string(clusterID),
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

type TesseractGrid struct {
	Hash       Hash
	Total      int32
	Tesseracts []*Tesseract
}

func (t *TesseractGrid) GetTesseractGridID() *TesseractGridID {
	gridID := &TesseractGridID{
		Hash: t.Hash,
		Parts: &TesseractParts{
			Total:   t.Total,
			Hashes:  make([]Hash, 0, len(t.Tesseracts)),
			Heights: make([]uint64, 0, len(t.Tesseracts)),
		},
	}

	for _, tesseract := range t.Tesseracts {
		gridID.Parts.Hashes = append(gridID.Parts.Hashes, tesseract.Hash())
		gridID.Parts.Heights = append(gridID.Parts.Heights, tesseract.Header.Height)
	}

	return gridID
}

func (t *TesseractGrid) CompareHash(h Hash) bool {
	if len(h.Bytes()) == 0 {
		return false
	}

	if t == nil {
		return false
	}

	return bytes.Equal(t.Hash.Bytes(), h.Bytes())
}

type NodeSet struct {
	Ids         []id.KramaID
	PublicKeys  [][]byte
	Responses   *ArrayOfBits
	VotingPower []int64
	Count       int
	QuorumSize  int
}

func NewNodeSet(ids []id.KramaID, keys [][]byte) *NodeSet {
	return &NodeSet{
		Ids:         ids,
		PublicKeys:  keys,
		Responses:   NewArrayOfBits(len(ids)),
		VotingPower: make([]int64, len(ids)),
		Count:       0,
	}
}

type ICSNodes struct {
	Nodes []*NodeSet
	Size  int
}

func NewICSNodes(size int) *ICSNodes {
	ics := &ICSNodes{
		Nodes: make([]*NodeSet, size),
		Size:  0,
	}

	return ics
}

func (i *ICSNodes) GetKramaID(index int32) (slotID int, slotIndex int, kramaID id.KramaID, publicKey []byte) {
	if index < 0 || int(index) >= i.Size {
		return -1, -1, "", nil
	}

	for v, set := range i.Nodes {
		if set == nil {
			continue
		}

		if v == len(i.Nodes)-1 {
			return -1, -1, "", nil
		}

		if int(index) >= len(set.Ids) {
			index -= int32(len(set.Ids))

			continue
		}

		// if set.Responses.GetIndex(int(index)) {
		return v, int(index), set.Ids[index], set.PublicKeys[index]
	}

	return -1, -1, "", nil
}

func (i *ICSNodes) GetIndex(peerID id.KramaID) (int32, bool) {
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

func (i *ICSNodes) UpdateNodeSet(setType IcsSetType, data *NodeSet) {
	if data == nil {
		return
	}

	i.Nodes[setType] = data
	i.Size += len(data.Ids)
}

func (i *ICSNodes) GetNodes() []id.KramaID {
	var nodes []id.KramaID

	for _, nodeSet := range i.Nodes {
		if nodeSet != nil {
			nodes = append(nodes, nodeSet.Ids...)
		}
	}

	return nodes
}

func (i *ICSNodes) IsContextQuorum() bool {
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

func (i *ICSNodes) IsRandomQuorum(requiredRandomNodes int) bool {
	return i.Nodes[RandomSet].Count >= requiredRandomNodes
}

func (i *ICSNodes) SenderSetSize() int {
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

func (i *ICSNodes) ReceiverSetSize() int {
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

func (i *ICSNodes) RandomSetSize() int {
	count := len(i.Nodes[RandomSet].Ids)
	if count <= 0 {
		return 0
	}

	return count
}

func (i *ICSNodes) SenderQuorumSize() int {
	count := i.SenderSetSize()
	if count <= 0 {
		return 0
	}

	return count*2/3 + 1
}

func (i *ICSNodes) ReceiverQuorumSize() int {
	count := i.ReceiverSetSize()
	if count <= 0 {
		return 0
	}

	return count*2/3 + 1
}

func (i *ICSNodes) RandomQuorumSize() int {
	return i.Nodes[RandomSet].QuorumSize*2/3 + 1
}

func (i *ICSNodes) String() string {
	rawBytes, err := json.Marshal(i)
	if err != nil {
		return "failed to print ics nodes"
	}

	return string(rawBytes)
}
