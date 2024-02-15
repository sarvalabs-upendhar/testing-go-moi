package common

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/sarvalabs/go-legacy-kramaid"
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

type NodeSet struct {
	mtx         sync.RWMutex
	Ids         []kramaid.KramaID
	PublicKeys  [][]byte
	Responses   *ArrayOfBits
	VotingPower []int64
	RespCount   int
	QuorumSize  int
}

// NewNodeSet creates and returns a new instance of NodeSet
func NewNodeSet(ids []kramaid.KramaID, keys [][]byte, quorumSize int) *NodeSet {
	return &NodeSet{
		mtx:         sync.RWMutex{},
		Ids:         ids,
		PublicKeys:  keys,
		Responses:   NewArrayOfBits(len(ids)),
		VotingPower: make([]int64, len(ids)),
		RespCount:   0,
		QuorumSize:  quorumSize,
	}
}

func (ns *NodeSet) GetRespCount() int {
	ns.mtx.RLock()
	defer ns.mtx.RUnlock()

	return ns.RespCount
}

func (ns *NodeSet) UpdateRespCount() {
	ns.mtx.Lock()
	defer ns.mtx.Unlock()

	ns.RespCount++
}

type ICSNodeSet struct {
	Nodes []*NodeSet
	Size  int
}

// NewICSNodeSet creates and returns a new instance of NodeSet
func NewICSNodeSet(size int) *ICSNodeSet {
	ics := &ICSNodeSet{
		Nodes: make([]*NodeSet, size),
		Size:  0,
	}

	return ics
}

// GetKramaID returns the slot id, slot index, krama id and bls public key of the validator node based on the index
func (i *ICSNodeSet) GetKramaID(index int32) (slots []int, slotIndex int, kramaID kramaid.KramaID, publicKey []byte) {
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

// HasKramaID returns the index and existence status of the validator node from ICSNodes based on the krama id
func (i *ICSNodeSet) HasKramaID(peerID kramaid.KramaID) (int32, bool) {
	offset := 0

	for index, set := range i.Nodes {
		if index == len(i.Nodes)-1 {
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
func (i *ICSNodeSet) GetIndex(peerID kramaid.KramaID) (IcsSetType, int) {
	for i, set := range i.Nodes {
		if set == nil {
			continue
		}

		for j, kramaID := range set.Ids {
			if kramaID == peerID {
				return IcsSetType(i), j
			}
		}
	}

	return -1, -1
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
func (i *ICSNodeSet) GetNodes(respondedOnly bool) []kramaid.KramaID {
	nodes := make(map[kramaid.KramaID]struct{})
	distinctNodes := make([]kramaid.KramaID, 0)

	for _, nodeSet := range i.Nodes {
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

// GetVoteset returns combined voteset of all the nodes from the ICSNodeSet
func (i *ICSNodeSet) GetVoteset() *ArrayOfBits {
	voteSet := NewArrayOfBits(i.Size)

	index := 0

	for _, nodeSet := range i.Nodes {
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
		if i.Nodes[j] != nil {
			count += i.Nodes[j].GetRespCount()
		}
	}

	return
}

// IsContextQuorum check's whether context quorum condition is satisfied or not
func (i *ICSNodeSet) IsContextQuorum() bool {
	for j := 0; j < 4; j += 2 {
		count := 0
		quorum := 0

		if i.Nodes[j] != nil {
			count += i.Nodes[j].GetRespCount()
			quorum += len(i.Nodes[j].Ids)
		}

		if i.Nodes[j+1] != nil {
			count += i.Nodes[j+1].GetRespCount()
			quorum += len(i.Nodes[j+1].Ids)
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
	return i.Nodes[RandomSet].GetRespCount() >= requiredRandomNodes
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

func (i *ICSNodeSet) ICSClusterInfo() *ICSClusterInfo {
	clusterInfo := &ICSClusterInfo{
		Responses: make([]*ArrayOfBits, len(i.Nodes)),
	}

	for index, set := range i.Nodes {
		if set != nil {
			clusterInfo.Responses[index] = set.Responses
		}
	}

	clusterInfo.RandomSet = i.Nodes[RandomSet].Ids
	clusterInfo.ObserverSet = i.Nodes[ObserverSet].Ids

	return clusterInfo
}
