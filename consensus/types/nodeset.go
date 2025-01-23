package types

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
)

type NodeInfo struct {
	ID          kramaid.KramaID
	PublicKey   []byte
	Msg         *Prepared
	VotingPower int64
}

type NodeSet struct {
	mtx                 sync.RWMutex
	Infos               []*NodeInfo
	Responses           *common.ArrayOfBits
	ExcludedFromICS     bool
	RespCount           atomic.Int32
	SetSizeWithOutDelta uint32
}

// NewNodeSet creates and returns a new instance of NodeSet
func NewNodeSet(ids []kramaid.KramaID, keys [][]byte, required uint32) *NodeSet {
	infos := make([]*NodeInfo, len(ids))

	for index, id := range ids {
		infos[index] = &NodeInfo{
			ID:        id,
			PublicKey: keys[index],
		}
	}

	return &NodeSet{
		mtx:                 sync.RWMutex{},
		Infos:               infos,
		Responses:           common.NewArrayOfBits(len(ids)),
		SetSizeWithOutDelta: required,
	}
}

func (ns *NodeSet) GetRespCount() int {
	return int(ns.RespCount.Load())
}

func (ns *NodeSet) UpdateResponse(index int, v bool) {
	ns.Responses.SetIndex(index, v)
	ns.RespCount.Add(1)
}

func (ns *NodeSet) UpdateViewInfo(index int, msg *Prepared) {
	ns.Infos[index].Msg = msg
}

func (ns *NodeSet) KramaIDs() []kramaid.KramaID {
	ids := make([]kramaid.KramaID, len(ns.Infos))

	for k, v := range ns.Infos {
		ids[k] = v.ID
	}

	return ids
}

// ICSCommittee manages the NodeSets of participants in the ICS, including RandomNodes and ObserverNodes.
// Each participant possesses 1 NodeSets, which are arranged according to their addresses.
// For instance, in the scenario of two participants, ICSCommittee will contain 1 * Participants + 1 sets.
// [P0.Set, P1.Set, RandomSet]
type ICSCommittee struct {
	Sets       []*NodeSet
	totalNodes int
	size       int
}

// NewICSCommittee creates and returns a new instance of NodeSet
func NewICSCommittee(size int) *ICSCommittee {
	ics := &ICSCommittee{
		Sets:       make([]*NodeSet, size),
		totalNodes: 0,
		size:       size,
	}

	return ics
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

func (i *ICSCommittee) UpdateNodePreparedMsg(id kramaid.KramaID, msg *Prepared) {
	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for index, info := range set.Infos {
			if info.ID == id {
				set.UpdateResponse(index, true)
				set.UpdateViewInfo(index, msg)
			}
		}
	}
}

func (i *ICSCommittee) ViewInfosAndSignatures() ([]common.Views, [][]byte) {
	views := make([]common.Views, i.totalNodes)
	signs := make([][]byte, 0, i.totalNodes)
	offset := 0

	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for j, info := range set.Infos {
			if !set.Responses.GetIndex(j) {
				continue
			}

			views[offset+j] = info.Msg.Infos
			signs = append(signs, info.Msg.Signature)
		}

		offset += len(set.Infos)
	}

	return views, signs
}

func (i *ICSCommittee) UpdateSetResponses(position int, responses *common.ArrayOfBits) {
	if responses == nil {
		return
	}

	i.Sets[position].Responses = responses
	i.Sets[position].RespCount.Store(int32(responses.TrueIndicesSize()))
}

func (i *ICSCommittee) UpdateValidatorResponse(indices []int) ([][]byte, error) {
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
			set.UpdateResponse(index, true)

			break
		}
	}

	return publicKeys, nil
}

// GetKramaID returns the slot id, slot index, krama id and bls public key of the validator node based on the index
func (i *ICSCommittee) GetKramaID(index int32) (
	slots []int32,
	slotIndex int,
	kramaID kramaid.KramaID,
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
				if kID.ID == set.Infos[index].ID {
					slots = append(slots, int32(j))
				}
			}
		}

		return slots, int(index), set.Infos[index].ID, set.Infos[index].PublicKey
	}

	return nil, -1, "", nil
}

func (i *ICSCommittee) GetPublicKey(index int32) (kramaID kramaid.KramaID, publicKey []byte) {
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

		return set.Infos[index].ID, set.Infos[index].PublicKey
	}

	return "", nil
}

// HasKramaID returns the index,public key and vote status of the validator node from ICSNodes based on the krama id
func (i *ICSCommittee) HasKramaID(peerID kramaid.KramaID) (int32, []byte, bool) {
	offset := 0

	for _, set := range i.Sets {
		if set == nil {
			continue
		}

		for j, info := range set.Infos {
			if info.ID == peerID {
				return int32(offset + j), info.PublicKey, set.Responses.GetIndex(j)
			}
		}

		offset += len(set.Infos)
	}

	return -1, nil, false
}

// GetIndex returns the index of the validator node from ICSNodes based on the krama id
func (i *ICSCommittee) GetIndex(peerID kramaid.KramaID) (int, int) {
	for i, set := range i.Sets {
		if set == nil || set.ExcludedFromICS {
			continue
		}

		for j, info := range set.Infos {
			if info.ID == peerID {
				return i, j
			}
		}
	}

	return -1, -1
}

// UpdateNodeSet updates the specific node set of the ICSNodes based on the node set type
func (i *ICSCommittee) UpdateNodeSet(setType int, data *NodeSet) {
	if data == nil {
		return
	}

	i.Sets[setType] = data
	i.totalNodes += len(data.Infos)
}

// GetNodes returns krama id's of all the nodes from the ICSNodes nodeset
func (i *ICSCommittee) GetNodes(respondedOnly bool) []kramaid.KramaID {
	nodes := make(map[kramaid.KramaID]struct{})
	distinctNodes := make([]kramaid.KramaID, 0)

	for _, nodeSet := range i.Sets {
		if nodeSet == nil || nodeSet.ExcludedFromICS {
			continue
		}

		for index, info := range nodeSet.Infos {
			if respondedOnly && !nodeSet.Responses.GetIndex(index) {
				continue
			}

			if _, ok := nodes[info.ID]; ok {
				continue
			}

			nodes[info.ID] = struct{}{}

			distinctNodes = append(distinctNodes, info.ID)
		}
	}

	return distinctNodes
}

func (i *ICSCommittee) GetInactiveNodes() []kramaid.KramaID {
	nodes := make(map[kramaid.KramaID]struct{})
	distinctNodes := make([]kramaid.KramaID, 0)

	for _, nodeSet := range i.Sets {
		if nodeSet == nil || nodeSet.ExcludedFromICS {
			continue
		}

		for index, info := range nodeSet.Infos {
			if nodeSet.Responses.GetIndex(index) {
				continue
			}

			if _, ok := nodes[info.ID]; ok {
				continue
			}

			nodes[info.ID] = struct{}{}

			distinctNodes = append(distinctNodes, info.ID)
		}
	}

	return distinctNodes
}

// GetVoteset returns combined voteset of all the nodes from the ICSCommittee
func (i *ICSCommittee) GetVoteset() *common.ArrayOfBits {
	voteSet := common.NewArrayOfBits(i.totalNodes)

	index := 0

	for _, nodeSet := range i.Sets {
		if nodeSet != nil {
			for j := 0; j < len(nodeSet.Infos); j++ {
				voteSet.SetIndex(index+j, nodeSet.Responses.GetIndex(j))
			}

			index += len(nodeSet.Infos)
		}
	}

	return voteSet
}

// GetRespondedNodeCount returns count of nodes that responded from selected ICSNodes
// between start and end indexes (inclusive)
func (i *ICSCommittee) GetRespondedNodeCount(start, end int) (count int) {
	for j := start; j <= end; j++ {
		if i.Sets[j] != nil {
			count += i.Sets[j].GetRespCount()
		}
	}

	return
}

// IsContextQuorum check's whether context quorum condition is satisfied or not
func (i *ICSCommittee) IsContextQuorum() bool {
	for j := 0; j < i.Size()-1; j++ {
		responses := 0
		setSize := 0

		if i.Sets[j] == nil {
			continue
		}

		if i.Sets[j] != nil {
			responses += i.Sets[j].GetRespCount()
			setSize += int(i.Sets[j].SetSizeWithOutDelta)
		}

		if setSize > 0 && responses < setSize*2/3+1 {
			return false
		}
	}

	return true
}

// IsRandomQuorum check's whether random quorum condition is satisfied or not
func (i *ICSCommittee) IsRandomQuorum(requiredRandomNodes int) bool {
	return i.RandomSet().GetRespCount() >= requiredRandomNodes
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

func (i *ICSCommittee) Responses() []*common.ArrayOfBits {
	responses := make([]*common.ArrayOfBits, i.Size())

	for j := 0; j < i.size; j++ {
		if i.Sets[j] != nil && !i.Sets[j].ExcludedFromICS {
			responses[j] = i.Sets[j].Responses.Copy()
		}
	}

	return responses
}

func (i *ICSCommittee) ExcludeParticipantsFromICS(position int) {
	if set := i.Sets[position]; set != nil {
		set.ExcludedFromICS = true
	}
}

func DistinctNodes(operator kramaid.KramaID, nodeSets []*NodeSet) ([]kramaid.KramaID, int, bool) {
	nodes := make(map[kramaid.KramaID]struct{})
	isOperatorIncluded := false

	for _, nodeSet := range nodeSets {
		if nodeSet == nil {
			continue
		}

		for _, info := range nodeSet.Infos {
			if _, hasKramaID := nodes[info.ID]; hasKramaID {
				continue
			}

			if info.ID == operator {
				isOperatorIncluded = true
			}

			nodes[info.ID] = struct{}{}
		}
	}

	distinct := make([]kramaid.KramaID, 0, len(nodes))

	for kramaID := range nodes {
		distinct = append(distinct, kramaID)
	}

	return distinct, len(distinct), isOperatorIncluded
}
