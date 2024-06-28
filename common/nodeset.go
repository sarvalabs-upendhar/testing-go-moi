package common

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/sarvalabs/go-legacy-kramaid"
)

type NodeSet struct {
	mtx                 sync.RWMutex
	Ids                 []kramaid.KramaID
	PublicKeys          [][]byte
	Responses           *ArrayOfBits
	VotingPower         []int64
	RespCount           int
	SetSizeWithOutDelta uint32
}

// NewNodeSet creates and returns a new instance of NodeSet
func NewNodeSet(ids []kramaid.KramaID, keys [][]byte, required uint32) *NodeSet {
	return &NodeSet{
		mtx:                 sync.RWMutex{},
		Ids:                 ids,
		PublicKeys:          keys,
		Responses:           NewArrayOfBits(len(ids)),
		VotingPower:         make([]int64, len(ids)),
		RespCount:           0,
		SetSizeWithOutDelta: required,
	}
}

func (ns *NodeSet) GetRespCount() int {
	ns.mtx.RLock()
	defer ns.mtx.RUnlock()

	return ns.RespCount
}

func (ns *NodeSet) UpdateResponse(index int, v bool) {
	ns.mtx.Lock()
	defer ns.mtx.Unlock()

	ns.Responses.SetIndex(index, v)
	ns.RespCount++
}

// ICSNodeSet manages the NodeSets of participants in the ICS, including RandomNodes and ObserverNodes.
// Each participant possesses 2 NodeSets, which are arranged according to their addresses.
// For instance, in the scenario of two participants, ICSNodeSet will contain 2 * Participants + 2 sets.
// [P0.BehaviourSet, P0.RandomSet, P1.BehaviourSet, P1.RandomSet, RandomSet, ObserverSet]
type ICSNodeSet struct {
	Sets       []*NodeSet
	totalNodes int
	size       int
}

// NewICSNodeSet creates and returns a new instance of NodeSet
func NewICSNodeSet(size int) *ICSNodeSet {
	ics := &ICSNodeSet{
		Sets:       make([]*NodeSet, size),
		totalNodes: 0,
		size:       size,
	}

	return ics
}

func (i *ICSNodeSet) TotalNodes() int {
	return i.totalNodes
}

func (i *ICSNodeSet) ObserverSetPosition() int {
	return i.size - 1
}

func (i *ICSNodeSet) RandomSetPosition() int {
	return i.size - 2
}

func (i *ICSNodeSet) ParticipantQuorum(position int) uint32 {
	var count uint32
	if i.Sets[position] != nil {
		count += i.Sets[position].SetSizeWithOutDelta
	}

	if i.Sets[position+1] != nil {
		count += i.Sets[position+1].SetSizeWithOutDelta
	}

	if count == 0 {
		return count
	}

	return count*2/3 + 1
}

func (i *ICSNodeSet) UpdateNodeSetResponses(position int, responses *ArrayOfBits) {
	if responses == nil {
		return
	}

	i.Sets[position].Responses = responses
	i.Sets[position].RespCount = responses.TrueIndicesSize()
}

// GetKramaID returns the slot id, slot index, krama id and bls public key of the validator node based on the index
func (i *ICSNodeSet) GetKramaID(index int32) (slots []int, slotIndex int, kramaID kramaid.KramaID, publicKey []byte) {
	if index < 0 || int(index) >= i.totalNodes {
		return nil, -1, "", nil
	}

	slots = make([]int, 0, i.Size()-1)

	for v, set := range i.Sets {
		if set == nil {
			continue
		}

		if v == len(i.Sets)-1 {
			return nil, -1, "", nil
		}

		if int(index) >= len(set.Ids) {
			index -= int32(len(set.Ids))

			continue
		}

		slots = append(slots, v)

		for j := v + 1; j < len(i.Sets)-1; j++ {
			// check for empty set
			if i.Sets[j] == nil {
				continue
			}
			// check for krama ID on not empty set
			for _, kID := range i.Sets[j].Ids {
				if kID == set.Ids[index] {
					slots = append(slots, j)
				}
			}
		}

		return slots, int(index), set.Ids[index], set.PublicKeys[index]
	}

	return nil, -1, "", nil
}

// HasKramaID returns the index and existence status of the validator node from ICSNodes based on the krama id
func (i *ICSNodeSet) HasKramaID(peerID kramaid.KramaID) (int32, bool) {
	offset := 0

	for index, set := range i.Sets {
		if index == len(i.Sets)-1 {
			break
		}

		if set == nil {
			continue
		}

		for j, kramaID := range set.Ids {
			if kramaID == peerID && set.Responses.GetIndex(j) {
				return int32(offset + j), set.Responses.GetIndex(j)
			}
		}

		offset += len(set.Ids)
	}

	return -1, false
}

// GetIndex returns the index of the validator node from ICSNodes based on the krama id
func (i *ICSNodeSet) GetIndex(peerID kramaid.KramaID) (int, int) {
	for i, set := range i.Sets {
		if set == nil {
			continue
		}

		for j, kramaID := range set.Ids {
			if kramaID == peerID {
				return i, j
			}
		}
	}

	return -1, -1
}

// UpdateNodeSet updates the specific node set of the ICSNodes based on the node set type
func (i *ICSNodeSet) UpdateNodeSet(setType int, data *NodeSet) {
	if data == nil {
		return
	}

	i.Sets[setType] = data
	i.totalNodes += len(data.Ids)
}

// GetNodes returns krama id's of all the nodes from the ICSNodes nodeset
func (i *ICSNodeSet) GetNodes(respondedOnly bool) []kramaid.KramaID {
	nodes := make(map[kramaid.KramaID]struct{})
	distinctNodes := make([]kramaid.KramaID, 0)

	for _, nodeSet := range i.Sets {
		if nodeSet == nil {
			continue
		}

		for index, kramaID := range nodeSet.Ids {
			if respondedOnly && !nodeSet.Responses.GetIndex(index) {
				continue
			}

			if _, ok := nodes[kramaID]; ok {
				continue
			}

			nodes[kramaID] = struct{}{}

			distinctNodes = append(distinctNodes, kramaID)
		}
	}

	return distinctNodes
}

func (i *ICSNodeSet) GetInactiveNodes() []kramaid.KramaID {
	nodes := make(map[kramaid.KramaID]struct{})
	distinctNodes := make([]kramaid.KramaID, 0)

	for _, nodeSet := range i.Sets {
		if nodeSet == nil {
			continue
		}

		for index, kramaID := range nodeSet.Ids {
			if nodeSet.Responses.GetIndex(index) {
				continue
			}

			if _, ok := nodes[kramaID]; ok {
				continue
			}

			nodes[kramaID] = struct{}{}

			distinctNodes = append(distinctNodes, kramaID)
		}
	}

	return distinctNodes
}

// GetVoteset returns combined voteset of all the nodes from the ICSNodeSet
func (i *ICSNodeSet) GetVoteset() *ArrayOfBits {
	voteSet := NewArrayOfBits(i.totalNodes)

	index := 0

	for _, nodeSet := range i.Sets {
		if nodeSet != nil {
			for j := 0; j < len(nodeSet.Ids); j++ {
				voteSet.setIndex(index+j, nodeSet.Responses.GetIndex(j))
			}

			index += len(nodeSet.Ids)
		}
	}

	return voteSet
}

// GetRespondedNodeCount returns count of nodes that responded from selected ICSNodes
// between start and end indexes (inclusive)
func (i *ICSNodeSet) GetRespondedNodeCount(start, end int) (count int) {
	for j := start; j <= end; j++ {
		if i.Sets[j] != nil {
			count += i.Sets[j].GetRespCount()
		}
	}

	return
}

// IsContextQuorum check's whether context quorum condition is satisfied or not
func (i *ICSNodeSet) IsContextQuorum() bool {
	for j := 0; j < i.Size()-2; j += 2 {
		responses := 0
		setSize := 0

		if i.Sets[j] != nil {
			responses += i.Sets[j].GetRespCount()
			setSize += int(i.Sets[j].SetSizeWithOutDelta)
		}

		if i.Sets[j+1] != nil {
			responses += i.Sets[j+1].GetRespCount()
			setSize += int(i.Sets[j+1].SetSizeWithOutDelta)
		}

		if setSize > 0 && responses < setSize*2/3+1 {
			log.Println("Quorum conditions failed", responses, setSize, setSize*2/3+1)

			return false
		}
	}

	return true
}

// IsRandomQuorum check's whether random quorum condition is satisfied or not
func (i *ICSNodeSet) IsRandomQuorum(requiredRandomNodes int) bool {
	return i.RandomSet().GetRespCount() >= requiredRandomNodes
}

func (i *ICSNodeSet) RandomSet() *NodeSet {
	return i.Sets[i.RandomSetPosition()]
}

func (i *ICSNodeSet) ObserverSet() *NodeSet {
	return i.Sets[i.ObserverSetPosition()]
}

// RandomSetSize returns the random node set size
func (i *ICSNodeSet) RandomSetSize() int {
	return len(i.RandomSet().Ids)
}

// RandomQuorumSize returns the random quorum size
func (i *ICSNodeSet) RandomQuorumSize() uint32 {
	return i.RandomSet().SetSizeWithOutDelta*2/3 + 1
}

// String returns the ICSNodes in string
func (i *ICSNodeSet) String() string {
	rawBytes, err := json.Marshal(i)
	if err != nil {
		return "failed to print ics nodes"
	}

	return string(rawBytes)
}

func (i *ICSNodeSet) Size() int {
	return i.size
}

func (i *ICSNodeSet) Responses() []*ArrayOfBits {
	responses := make([]*ArrayOfBits, i.Size())

	for j := 0; j < i.size; j++ {
		if i.Sets[j] != nil {
			responses[j] = i.Sets[j].Responses.Copy()
		}
	}

	return responses
}
