package types

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
)

type CounterType int

const (
	// VoteCounter is used to count the votes for prepared messages
	VoteCounter CounterType = iota
	// MajorityCounter is used to count the first 2/3 majority votes.
	MajorityCounter
)

type NodeInfo struct {
	ID          identifiers.KramaID
	PublicKey   []byte
	Msg         *Prepared
	VotingPower int64
}

type Response struct {
	Bits  *common.ArrayOfBits
	Count atomic.Int32
}

type NodeSet struct {
	mtx                 sync.RWMutex
	Infos               []*common.ValidatorInfo
	Responses           map[CounterType]*Response
	ExcludedFromICS     bool
	SetSizeWithOutDelta uint32
}

// NewNodeSet creates and returns a new instance of NodeSet
func NewNodeSet(vals []*common.ValidatorInfo, required uint32) *NodeSet {
	ns := &NodeSet{
		mtx:                 sync.RWMutex{},
		Infos:               vals,
		Responses:           make(map[CounterType]*Response),
		SetSizeWithOutDelta: required,
	}

	ns.Responses[VoteCounter] = &Response{
		Bits: common.NewArrayOfBits(len(vals)),
	}

	ns.Responses[MajorityCounter] = &Response{
		Bits: common.NewArrayOfBits(len(vals)),
	}

	return ns
}

func (ns *NodeSet) GetRespCount(ct CounterType) int {
	return int(ns.Responses[ct].Count.Load())
}

func (ns *NodeSet) UpdateResponse(ct CounterType, index int, v bool) {
	if ns.Responses[ct].Bits.GetIndex(index) {
		return
	}

	ns.Responses[ct].Bits.SetIndex(index, v)
	ns.Responses[ct].Count.Add(1)
}

func (ns *NodeSet) UpdateViewInfo(index int, msg *Prepared) {
	ns.Infos[index].Msg = msg
}

func (ns *NodeSet) ValidatorIndices() []common.ValidatorIndex {
	indices := make([]common.ValidatorIndex, len(ns.Infos))

	for k, info := range ns.Infos {
		indices[k] = info.ID
	}

	return indices
}

func (ns *NodeSet) KramaIDs() []identifiers.KramaID {
	ids := make([]identifiers.KramaID, len(ns.Infos))

	for k, v := range ns.Infos {
		ids[k] = v.KramaID
	}

	return ids
}

// NodesetData keeps track of node set position of consensus nodes hash
// If PSCount and ExcludedPSCount are equal, it means the nodeset can be excluded from consensus.
type NodesetData struct {
	Position        int
	PSCount         int // PSCount represents the number of unique participants having same consensus nodes hash
	ExcludedPSCount int // ExcludedPSCount represents the number of unique participants excluded from consensus
}

// ICSCommittee manages the NodeSets for unique consensus nodes hash of participants in the ICS,
// including RandomNodes and ObserverNodes.
// ConsensusNodesHash helps in keeping track of index of consensus nodes hash in the node set.
// For instance, in the scenario of 4 participants where c1, c2, c1, c2 are consensus nodes hash
// of participants respectively,
// ICSCommittee will contain 2 nodes set (due to 2 unique consensus nodes hash) + 1 random sets.
// [c1, c2, RandomSet]
// ConsensusNodesHash represents mapping between ConsensusNodesHash and it's nodeset position
type ICSCommittee struct {
	Sets               []*NodeSet
	ConsensusNodesHash map[common.Hash]*NodesetData
	totalNodes         int
	size               int // size represent number of node sets
}

// NewICSCommittee creates and returns a new instance of NodeSet
func NewICSCommittee() *ICSCommittee {
	ics := &ICSCommittee{
		Sets:               make([]*NodeSet, 0),
		ConsensusNodesHash: make(map[common.Hash]*NodesetData),
		totalNodes:         0,
	}

	return ics
}

func (i *ICSCommittee) ContextSet() []*NodeSet {
	return i.Sets[:len(i.Sets)-1]
}

func (i *ICSCommittee) IncrementPSCount(consensusNodesHash common.Hash) {
	nodesetData := i.ConsensusNodesHash[consensusNodesHash]
	nodesetData.PSCount++
}

func (i *ICSCommittee) IncrementExcludedPSCount(consensusNodesHash common.Hash) {
	nodesetData := i.ConsensusNodesHash[consensusNodesHash]
	nodesetData.ExcludedPSCount++

	if nodesetData.ExcludedPSCount == nodesetData.PSCount {
		i.Sets[nodesetData.Position].ExcludedFromICS = true
	}
}

// AppendNodeSet appends a node set and tracks its position using the consensus nodes hash.
// When appending a random set, provide nilHash as the consensus nodes hash,
// since the random nodes hash differs from the consensus nodes hash, it doesn't need to be stored.
func (i *ICSCommittee) AppendNodeSet(consensusNodesHash common.Hash, data *NodeSet) {
	if data == nil {
		return
	}

	i.Sets = append(i.Sets, data)
	i.totalNodes += len(data.Infos)
	i.size++

	if !consensusNodesHash.IsNil() {
		i.ConsensusNodesHash[consensusNodesHash] = &NodesetData{
			Position: len(i.Sets) - 1,
			PSCount:  1,
		}
	}
}

func (i *ICSCommittee) UpdateNodeset(consensusNodesHash common.Hash, consensusSet *NodeSet, ps common.State) {
	if _, ok := i.GetNodesetPosition(consensusNodesHash); !ok {
		i.AppendNodeSet(consensusNodesHash, consensusSet)
	} else {
		i.IncrementPSCount(consensusNodesHash)
	}

	if ps.StateHash == common.NilHash {
		i.IncrementExcludedPSCount(consensusNodesHash)
	}
}

func (i *ICSCommittee) HasConsensusNodesHash(hash common.Hash) bool {
	_, ok := i.ConsensusNodesHash[hash]

	return ok
}

func (i *ICSCommittee) GetNodesetPosition(hash common.Hash) (int, bool) {
	data, ok := i.ConsensusNodesHash[hash]
	if !ok {
		return 0, false
	}

	return data.Position, ok
}

func (i *ICSCommittee) TotalNodes() int {
	return i.totalNodes
}

func (i *ICSCommittee) StochasticSetPosition() int {
	return i.size - 1
}

func (i *ICSCommittee) ParticipantQuorum(position int) uint32 {
	var count uint32

	if i.Sets[position] == nil {
		return 0
	}

	count = i.Sets[position].SetSizeWithOutDelta
	if count == 0 {
		return count
	}

	return count*2/3 + 1
}

func (i *ICSCommittee) UpdateNodePreparedMsg(id identifiers.KramaID, msg *Prepared) {
	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for index, info := range set.Infos {
			if info.KramaID == id {
				set.UpdateResponse(VoteCounter, index, true)
				set.UpdateViewInfo(index, msg)
			}
		}
	}
}

func (i *ICSCommittee) UpdateResponse(ct CounterType, id identifiers.KramaID) {
	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for index, info := range set.Infos {
			if info.KramaID == id {
				set.UpdateResponse(ct, index, true)
			}
		}
	}
}

func (i *ICSCommittee) ViewInfos() ([]common.Views, error) {
	views := make([]common.Views, i.totalNodes)
	offset := 0

	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for j, info := range set.Infos {
			if !set.Responses[VoteCounter].Bits.GetIndex(j) {
				continue
			}

			prepareMsg, err := getPrepareMsg(info)
			if err != nil {
				return nil, err
			}

			views[offset+j] = prepareMsg.Infos
		}

		offset += len(set.Infos)
	}

	return views, nil
}

func (i *ICSCommittee) ViewInfosAndSignatures() ([]common.Views, [][]byte, error) {
	views := make([]common.Views, i.totalNodes)
	signs := make([][]byte, 0, i.totalNodes)
	offset := 0

	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for j, info := range set.Infos {
			if !set.Responses[VoteCounter].Bits.GetIndex(j) {
				continue
			}

			prepareMsg, err := getPrepareMsg(info)
			if err != nil {
				return nil, nil, err
			}

			views[offset+j] = prepareMsg.Infos
			signs = append(signs, prepareMsg.Signature)
		}

		offset += len(set.Infos)
	}

	return views, signs, nil
}

func (i *ICSCommittee) UpdateSetResponses(ct CounterType, position int, responses *common.ArrayOfBits) {
	if responses == nil {
		return
	}

	i.Sets[position].Responses[ct] = &Response{
		Bits: responses,
	}

	i.Sets[position].Responses[ct].Count.Store(int32(responses.TrueIndicesSize()))
}

func (i *ICSCommittee) UpdateValidatorResponse(ct CounterType, indices []int) ([][]byte, error) {
	publicKeys := make([][]byte, 0, len(indices))

	for _, index := range indices {
		if index < 0 || index >= i.totalNodes {
			return nil, errors.New("invalid validator index")
		}

		for _, set := range i.Sets {
			if set == nil {
				continue
			}

			if index >= len(set.Infos) {
				index -= len(set.Infos)

				continue
			}

			publicKeys = append(publicKeys, set.Infos[index].PublicKey)
			set.UpdateResponse(ct, index, true)

			break
		}
	}

	return publicKeys, nil
}

func (i *ICSCommittee) GetSlots(id identifiers.KramaID) []int32 {
	slots := make([]int32, 0, i.Size())

	for j := 0; j < i.Size(); j++ {
		for _, info := range i.Sets[j].Infos {
			if info.KramaID == id {
				slots = append(slots, int32(j))

				break
			}
		}
	}

	return slots
}

// GetKramaID returns the slot id, slot index, krama id and bls public key of the validator node based on the index
func (i *ICSCommittee) GetKramaID(index int32) (
	slots []int32,
	slotIndex int,
	kramaID identifiers.KramaID,
	publicKey []byte,
) {
	if index < 0 || int(index) >= i.totalNodes {
		return nil, -1, "", nil
	}

	slots = make([]int32, 0, i.Size()-1)

	for v, set := range i.Sets {
		if set == nil {
			continue
		}

		if int(index) >= len(set.Infos) {
			index -= int32(len(set.Infos))

			continue
		}

		slots = append(slots, int32(v))

		for j := v + 1; j < len(i.Sets)-1; j++ {
			// check for empty set
			if i.Sets[j] == nil {
				continue
			}
			// check for krama ID on not empty set
			for _, kID := range i.Sets[j].Infos {
				if kID.KramaID == set.Infos[index].KramaID {
					slots = append(slots, int32(j))
				}
			}
		}

		return slots, int(index), set.Infos[index].KramaID, set.Infos[index].PublicKey
	}

	return nil, -1, "", nil
}

func (i *ICSCommittee) GetPublicKey(index int32) (kramaID identifiers.KramaID, publicKey []byte) {
	if index < 0 || int(index) >= i.totalNodes {
		return "", nil
	}

	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		if int(index) >= len(set.Infos) {
			index -= int32(len(set.Infos))

			continue
		}

		return set.Infos[index].KramaID, set.Infos[index].PublicKey
	}

	return "", nil
}

// HasKramaID returns the index,public key and vote status of the validator node from ICSNodes based on the krama id
func (i *ICSCommittee) HasKramaID(peerID identifiers.KramaID) (int32, []byte, bool) {
	offset := 0

	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for j, info := range set.Infos {
			if info.KramaID == peerID {
				return int32(offset + j), info.PublicKey, set.Responses[VoteCounter].Bits.GetIndex(j)
			}
		}

		offset += len(set.Infos)
	}

	return -1, nil, false
}

// GetIndex returns the index of the validator node from ICSNodes based on the krama id
func (i *ICSCommittee) GetIndex(peerID identifiers.KramaID) (int, int) {
	for i, set := range i.Sets {
		if set == nil || set.ExcludedFromICS {
			continue
		}

		for j, info := range set.Infos {
			if info.KramaID == peerID {
				return i, j
			}
		}
	}

	return -1, -1
}

// GetNodes returns krama id's of all the nodes from the ICSNodes nodeset
func (i *ICSCommittee) GetNodes() []identifiers.KramaID {
	nodes := make(map[identifiers.KramaID]struct{})
	distinctNodes := make([]identifiers.KramaID, 0)

	for _, nodeSet := range i.Sets {
		if nodeSet == nil || nodeSet.ExcludedFromICS {
			continue
		}

		for _, info := range nodeSet.Infos {
			if _, ok := nodes[info.KramaID]; ok {
				continue
			}

			nodes[info.KramaID] = struct{}{}

			distinctNodes = append(distinctNodes, info.KramaID)
		}
	}

	return distinctNodes
}

func (i *ICSCommittee) GetInactiveNodes(ct CounterType) []identifiers.KramaID {
	nodes := make(map[identifiers.KramaID]struct{})
	distinctNodes := make([]identifiers.KramaID, 0)

	for _, nodeSet := range i.Sets {
		if nodeSet == nil || nodeSet.ExcludedFromICS {
			continue
		}

		for index, info := range nodeSet.Infos {
			if nodeSet.Responses[ct].Bits.GetIndex(index) {
				continue
			}

			if _, ok := nodes[info.KramaID]; ok {
				continue
			}

			nodes[info.KramaID] = struct{}{}

			distinctNodes = append(distinctNodes, info.KramaID)
		}
	}

	return distinctNodes
}

// GetVoteset returns combined voteset of all the nodes from the ICSCommittee
func (i *ICSCommittee) GetVoteset(ct CounterType) *common.ArrayOfBits {
	voteSet := common.NewArrayOfBits(i.totalNodes)

	index := 0

	for _, nodeSet := range i.Sets {
		if nodeSet != nil {
			for j := 0; j < len(nodeSet.Infos); j++ {
				voteSet.SetIndex(index+j, nodeSet.Responses[ct].Bits.GetIndex(j))
			}

			index += len(nodeSet.Infos)
		}
	}

	return voteSet
}

// GetRespondedNodeCount returns count of nodes that responded from selected ICSNodes
// between start and end indexes (inclusive)
func (i *ICSCommittee) GetRespondedNodeCount(ct CounterType, start, end int) (count int) {
	for j := start; j <= end; j++ {
		if i.Sets[j] != nil {
			count += i.Sets[j].GetRespCount(ct)
		}
	}

	return
}

// IsSlotsQuorum check's whether slots quorum condition are satisfied, slot here refers to the participant's nodeset
func (i *ICSCommittee) IsSlotsQuorum(slots []int32) bool {
	for _, j := range slots {
		responses := 0
		setSize := 0

		if i.Sets[j] == nil {
			continue
		}

		responses += i.Sets[j].GetRespCount(MajorityCounter)
		setSize += int(i.Sets[j].SetSizeWithOutDelta)

		if setSize > 0 && responses < setSize*2/3+1 {
			return false
		}
	}

	return true
}

// IsContextQuorum check's whether context quorum condition is satisfied or not
func (i *ICSCommittee) IsContextQuorum(ct CounterType) bool {
	for j := 0; j < i.Size()-1; j++ {
		responses := 0
		setSize := 0

		if i.Sets[j] == nil {
			continue
		}

		if i.Sets[j] != nil {
			responses += i.Sets[j].GetRespCount(ct)
			setSize += int(i.Sets[j].SetSizeWithOutDelta)
		}

		if setSize > 0 && responses < setSize*2/3+1 {
			return false
		}
	}

	return true
}

// IsRandomQuorum check's whether random quorum condition is satisfied or not
func (i *ICSCommittee) IsRandomQuorum(ct CounterType, requiredRandomNodes int) bool {
	return i.RandomSet().GetRespCount(ct) >= requiredRandomNodes
}

func (i *ICSCommittee) RandomSet() *NodeSet {
	return i.Sets[i.StochasticSetPosition()]
}

// RandomSetSize returns the random node set size
func (i *ICSCommittee) RandomSetSize() int {
	return len(i.RandomSet().Infos)
}

// RandomQuorumSize returns the random quorum size
func (i *ICSCommittee) RandomQuorumSize() uint32 {
	return i.RandomSet().SetSizeWithOutDelta*2/3 + 1
}

func (i *ICSCommittee) RandomSetSizeWithOutDelta() uint32 {
	return i.RandomSet().SetSizeWithOutDelta
}

// String returns the ICSNodes in string
func (i *ICSCommittee) String() string {
	rawBytes, err := json.Marshal(i)
	if err != nil {
		return "failed to print ics nodes"
	}

	return string(rawBytes)
}

func (i *ICSCommittee) Size() int {
	return i.size
}

func (i *ICSCommittee) Responses(ct CounterType) []*common.ArrayOfBits {
	responses := make([]*common.ArrayOfBits, i.Size())

	for j := 0; j < i.size; j++ {
		if i.Sets[j] != nil && !i.Sets[j].ExcludedFromICS {
			responses[j] = i.Sets[j].Responses[ct].Bits.Copy()
		}
	}

	return responses
}

func getPrepareMsg(valInfo *common.ValidatorInfo) (*Prepared, error) {
	if prepared, ok := valInfo.Msg.(*Prepared); ok {
		return prepared, nil
	}

	return nil, errors.New("invalid prepare message type")
}

func DistinctNodes(operator identifiers.KramaID, nodeSets []*NodeSet) (map[common.ValidatorIndex]struct{}, int, bool) {
	nodes := make(map[common.ValidatorIndex]struct{})
	isOperatorIncluded := false

	for _, nodeSet := range nodeSets {
		if nodeSet == nil {
			continue
		}

		for _, info := range nodeSet.Infos {
			if _, hasKramaID := nodes[info.ID]; hasKramaID {
				continue
			}

			if info.KramaID == operator {
				isOperatorIncluded = true
			}

			nodes[info.ID] = struct{}{}
		}
	}

	return nodes, len(nodes), isOperatorIncluded
}
