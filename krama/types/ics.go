package types

import (
	"sync"
	"time"

	"github.com/pkg/errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"golang.org/x/crypto/blake2b"
)

type ClusterInfo struct {
	mtx                      sync.Mutex
	ICS                      *ICSNodes
	Ixs                      types.Interactions
	ID                       types.ClusterID
	Operator                 id.KramaID
	AccountInfos             AccountInfos
	contextDelta             types.ContextDelta
	Receipts                 types.Receipts
	BinaryHash, IdentityHash types.Hash
	ICSHash                  types.Hash
	dirty                    map[types.Hash][]byte
	Grid                     []*types.Tesseract
	ICSReqTime               time.Time
	operatorIncluded         bool
	CurrentRole              IcsSetType
	SuccessMsg               *ICSMSG
	ContextLock              map[types.Address]types.ContextLockInfo
}

// TODO: Check on locks

func NewICS(
	size int,
	ixs types.Interactions,
	clusterID types.ClusterID,
	operator id.KramaID,
	reqTime time.Time,
) *ClusterInfo {
	return &ClusterInfo{
		ICS:              NewICSNodes(size),
		Ixs:              ixs,
		ID:               clusterID,
		Operator:         operator,
		operatorIncluded: false,
		AccountInfos:     make(AccountInfos, 0),
		contextDelta:     make(types.ContextDelta),
		Receipts:         make(types.Receipts, 1), // This should be changed base on the transactions
		dirty:            make(map[types.Hash][]byte),
		ICSReqTime:       reqTime,
		ContextLock:      make(map[types.Address]types.ContextLockInfo),
	}
}

func (i *ClusterInfo) Size() int {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.ICS.Size
}

func (i *ClusterInfo) UpdateNodeSet(setType IcsSetType, data *NodeSet) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	i.ICS.UpdateNodeSet(setType, data)
}

func (i *ClusterInfo) GetContextDelta() types.ContextDelta {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.contextDelta
}

func (i *ClusterInfo) IncludeOperator() {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	i.operatorIncluded = true
}

func (i *ClusterInfo) IsOperatorIncluded() bool {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.operatorIncluded
}

// GetMetaData returns the ClusterInfo metadata including the given ClusterInfo messages
func (i *ClusterInfo) GetMetaData(msgs []*ICSMSG) (*ICSMetaInfo, error) {
	receiptHash, err := i.Receipts.Hash()
	if err != nil {
		return nil, err
	}

	m := &ICSMetaInfo{
		ClusterID:    string(i.ID),
		IxHash:       i.Ixs[0].Hash, // Need to be improved
		Operator:     string(i.Operator),
		ClusterSize:  i.ICS.Size,
		BinaryHash:   i.BinaryHash,
		IdentityHash: i.IdentityHash,
		IcsHash:      i.ICSHash,
		ReceiptHash:  receiptHash,
	}

	rawData, err := polo.Polorize(i.SuccessMsg)
	if err != nil {
		return nil, err
	}

	m.Msgs = append(m.Msgs, rawData)

	for _, v := range msgs {
		rawData, err := polo.Polorize(v)
		if err != nil {
			return nil, err
		}

		m.Msgs = append(m.Msgs, rawData)
	}

	return m, nil
}

func (i *ClusterInfo) IncrementClusterSize(delta int) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.ICS.Size += delta
}

func (i *ClusterInfo) RespondedEligibleSet() (count int, nodes []id.KramaID) {
	for j := 0; j < 4; j++ {
		if i.ICS.Nodes[j] != nil {
			count += i.ICS.Nodes[j].Count
			nodes = append(nodes, i.ICS.Nodes[j].Ids...)
		}
	}

	return
}

func (i *ClusterInfo) GetBehaviouralContextDelta(setType IcsSetType) (addedPeer, replacedPeer id.KramaID) {
	for _, peerID := range i.ICS.Nodes[setType].Ids {
		if i.Operator == peerID { // i.ICS.Nodes[setType].Responses.GetIndex(index)
			return
		}
	}

	if len(i.ICS.Nodes[setType].Ids) >= gtypes.MaxBehaviourContextSize {
		replacedPeer = i.ICS.Nodes[setType].Ids[0]
	}

	return i.Operator, replacedPeer
}

func (i *ClusterInfo) GetRandomContextDelta(
	setType IcsSetType,
	requiredCount int,
	skipPeers ...id.KramaID,
) (addedPeers, replacedPeers []id.KramaID) {
	addedPeers = make([]id.KramaID, 0, requiredCount)

	if i.ICS.Nodes[setType] == nil {
		return
	}

	if i.ICS.Nodes[setType] != nil {
		if x := len(i.ICS.Nodes[setType].Ids) + requiredCount - gtypes.MaxRandomContextSize; x > 0 {
			replacedPeers = i.ICS.Nodes[setType].Ids[0:x]
		}
	}

	set := i.ICS.Nodes[RandomSet]
	for index, v := range set.Ids {
		if set.Responses.GetIndex(index) && !utils.ContainsKramaID(skipPeers, v) {
			addedPeers = append(addedPeers, v)
		}

		if len(addedPeers) == requiredCount {
			break
		}
	}

	return
}

func (i *ClusterInfo) UpdateContextDelta(delta types.ContextDelta) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.contextDelta = delta
}

func (i *ClusterInfo) IsContextQuorum() bool {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.ICS.IsContextQuorum()
}

func (i *ClusterInfo) IsRandomQuorum(requiredRandomNodes, requiredObserverNodes int) bool {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.ICS.Nodes[RandomSet].Count >= requiredRandomNodes &&
		i.ICS.Nodes[ObserverSet].Count >= requiredObserverNodes
}

func (i *ClusterInfo) HasKramaID(kramaID id.KramaID) (int32, bool) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.ICS.GetIndex(kramaID)
}

// GetByIndex returns the krama id and bls public key of the validator based on the index
func (i *ClusterInfo) GetByIndex(index int32) (id.KramaID, []byte) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	slotID, slotIndex, kramaID, publicKey := i.ICS.GetKramaID(index)
	if slotID == -1 || !i.ICS.Nodes[slotID].Responses.GetIndex(slotIndex) {
		return "", nil
	}

	return kramaID, publicKey
}

func (i *ClusterInfo) GetICSNodes() []id.KramaID {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.ICS.GetNodes()
}

func (i *ClusterInfo) GetObservers() []string {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return types.KIPPeerIDToString(i.ICS.Nodes[ObserverSet].Ids)
}

func (i *ClusterInfo) GetRandomNodes() []string {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return types.KIPPeerIDToString(i.ICS.Nodes[RandomSet].Ids)
}

func (i *ClusterInfo) GetTotalVotingPower() []int32 {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	quorum := make([]int32, 3)
	quorum[0] = int32(i.ICS.SenderSetSize())
	quorum[1] = int32(i.ICS.ReceiverSetSize())
	quorum[2] = int32(i.ICS.RandomSetSize())

	return quorum
}

func (i *ClusterInfo) GetQuorum() []int32 {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	quorum := make([]int32, 3)
	quorum[0] = int32(i.ICS.SenderQuorumSize())
	quorum[1] = int32(i.ICS.ReceiverQuorumSize())
	quorum[2] = int32(i.ICS.RandomQuorumSize())

	return quorum
}

func (i *ClusterInfo) GetContextHash(ixHash types.Hash, addr types.Address) types.Hash {
	receipt, err := i.Receipts.GetReceipt(ixHash)
	if err != nil {
		return types.NilHash
	}

	return receipt.ContextHashes[addr]
}

func (i *ClusterInfo) GetStateHash(ixHash types.Hash, addr types.Address) types.Hash {
	receipt, err := i.Receipts.GetReceipt(ixHash)
	if err != nil {
		return types.NilHash
	}

	return receipt.StateHashes[addr]
}

func (i *ClusterInfo) GetGasUsed() (gasUsed uint64) {
	for _, receipt := range i.Receipts {
		gasUsed += receipt.GasUsed
	}

	return
}

func (i *ClusterInfo) SetReceipts(r types.Receipts) {
	i.Receipts = r
}

func (i *ClusterInfo) SetGrid(grid []*types.Tesseract) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.Grid = grid
}

func (i *ClusterInfo) GetTesseractGrid() []*types.Tesseract {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.Grid
}

func (i *ClusterInfo) AddDirty(key types.Hash, data []byte) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.dirty[key] = data
}

func (i *ClusterInfo) ComputeICSHash() (types.Hash, error) {
	msg := &ptypes.ICSClusterInfo{
		RandomSet:   i.GetRandomNodes(),
		ObserverSet: i.GetObservers(),
		Responses:   make([]*types.ArrayOfBits, 6),
	}

	for j := 0; j < len(i.ICS.Nodes); j++ {
		if i.ICS.Nodes[j] != nil && i.ICS.Nodes[j].Responses != nil && i.ICS.Nodes[j].Responses.Size > 0 {
			msg.Responses[j] = i.ICS.Nodes[j].Responses
		} else {
			msg.Responses[j] = nil
		}
	}

	rawData, err := polo.Polorize(msg)
	if err != nil {
		return types.NilHash, err
	}

	hash := blake2b.Sum256(rawData)
	i.AddDirty(hash, rawData)
	i.ICSHash = hash

	return hash, nil
}

func (i *ClusterInfo) CreateICSSuccessMsg() *ptypes.ICSSuccessMsg {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	msg := &ptypes.ICSSuccessMsg{
		ClusterID:   string(i.ID),
		RandomSet:   i.ICS.Nodes[RandomSet].Ids,
		ObserverSet: i.ICS.Nodes[ObserverSet].Ids,
		Responses:   make([]*types.ArrayOfBits, 6),
		Signature:   make([]byte, 0),
		QuorumSizes: make([]int, 6),
	}

	for j := 0; j < len(i.ICS.Nodes); j++ {
		if i.ICS.Nodes[j] != nil {
			msg.Responses[j] = i.ICS.Nodes[j].Responses
			msg.QuorumSizes[j] = i.ICS.Nodes[j].QuorumSize
		}
	}

	// i.Nodes[SenderBehaviourSet].Responses
	// msg.Responses[SenderBehaviourSet] = i.Nodes[SenderBehaviourSet].Responses.ToProto()
	// msg.Responses[SenderRandomSet] = i.Nodes[SenderRandomSet].Responses.ToProto()
	// msg.Responses[ReceiverBehaviourSet] = i.Nodes[ReceiverBehaviourSet].Responses.ToProto()
	// msg.Responses[ReceiverRandomSet] = i.Nodes[ReceiverRandomSet].Responses.ToProto()
	// msg.Responses[RandomSet] = i.Nodes[RandomSet].Responses.ToProto()
	// msg.Responses[ObserverSet] = i.Nodes[ObserverSet].Responses.ToProto()

	return msg
}

func (i *ClusterInfo) GetRandomDelta(requiredCount int) []id.KramaID {
	nodes := make([]id.KramaID, 0, requiredCount)
	set := i.ICS.Nodes[RandomSet]

	for index, v := range set.Ids {
		if set.Responses.GetIndex(index) {
			nodes = append(nodes, v)
		}

		if len(nodes) == requiredCount {
			break
		}
	}

	return nodes
}

func (i *ClusterInfo) UpdateClusterSize() {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	i.ICS.Size = 0

	for _, idSet := range i.ICS.Nodes {
		if idSet != nil {
			i.ICS.Size += len(idSet.Ids)
		}
	}
}

func (i *ClusterInfo) GetDirty() map[types.Hash][]byte {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.dirty
}

type AccountInfos map[types.Address]*types.AccountMetaInfo

func (a AccountInfos) GetLatestHash(addr types.Address) types.Hash {
	if v, ok := a[addr]; ok {
		return v.TesseractHash
	}

	return types.NilHash
}

func (a AccountInfos) GetHeight(addr types.Address) int64 {
	if v, ok := a[addr]; ok {
		return v.Height.Int64()
	}

	return 0
}

func (a AccountInfos) IsGenesis(addr types.Address) bool {
	return a[addr].Height.Int64() == -1
}

type ICSMSG struct {
	MsgType   ptypes.MsgType
	Msg       []byte
	Sender    id.KramaID
	ClusterID string
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
