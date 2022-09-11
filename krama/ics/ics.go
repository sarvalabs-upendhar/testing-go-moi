package ics

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"golang.org/x/crypto/blake2b"
	"sync"
	"time"
)

type ClusterInfo struct {
	mtx                      sync.Mutex
	ICS                      *ktypes.ICSNodes
	Ixs                      ktypes.Interactions
	ID                       ktypes.ClusterID
	Operator                 id.KramaID
	AccountInfos             AccountInfos
	contextDelta             ktypes.ContextDelta
	Receipts                 ktypes.Receipts
	BinaryHash, IdentityHash ktypes.Hash
	ICSHash                  ktypes.Hash
	dirty                    map[ktypes.Hash][]byte
	Grid                     []*ktypes.Tesseract
	ICSReqTime               time.Time
	operatorIncluded         bool
	CurrentRole              ktypes.IcsSetType
	SuccessMsg               *ktypes.ICSMSG
	ContextLock              map[ktypes.Address]ktypes.ContextLockInfo
}

//TODO: Check on locks

func NewICS(
	size int,
	ixs ktypes.Interactions,
	clusterID ktypes.ClusterID,
	operator id.KramaID,
	reqTime time.Time,
) *ClusterInfo {
	return &ClusterInfo{
		ICS:              ktypes.NewICSNodes(size),
		Ixs:              ixs,
		ID:               clusterID,
		Operator:         operator,
		operatorIncluded: false,
		AccountInfos:     make(AccountInfos, 0),
		contextDelta:     make(ktypes.ContextDelta),
		Receipts:         make(ktypes.Receipts, 1), //This should be changed base on the transactions
		dirty:            make(map[ktypes.Hash][]byte),
		ICSReqTime:       reqTime,
		ContextLock:      make(map[ktypes.Address]ktypes.ContextLockInfo),
	}
}

func (i *ClusterInfo) Size() int {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.ICS.Size
}

func (i *ClusterInfo) UpdateNodeSet(setType ktypes.IcsSetType, data *ktypes.NodeSet) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	i.ICS.UpdateNodeSet(setType, data)
}
func (i *ClusterInfo) GetContextDelta() ktypes.ContextDelta {
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
func (i *ClusterInfo) GetMetaData(msgs []*ktypes.ICSMSG) *ktypes.ICSMetaInfo {
	m := &ktypes.ICSMetaInfo{
		ClusterID:    string(i.ID),
		IxHash:       i.Ixs[0].Hash, //Need to be improved
		Operator:     string(i.Operator),
		ClusterSize:  i.ICS.Size,
		BinaryHash:   i.BinaryHash,
		IdentityHash: i.IdentityHash,
		IcsHash:      i.ICSHash,
		ReceiptHash:  i.Receipts.Hash(),
	}

	m.Msgs = append(m.Msgs, polo.Polorize(i.SuccessMsg))
	for _, v := range msgs {
		m.Msgs = append(m.Msgs, polo.Polorize(v))
	}

	return m
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

func (i *ClusterInfo) GetBehaviouralContextDelta(setType ktypes.IcsSetType) (addedPeer, replacedPeer id.KramaID) {
	for _, peerID := range i.ICS.Nodes[setType].Ids {
		if i.Operator == peerID { //i.ICS.Nodes[setType].Responses.GetIndex(index)

			return
		}
	}

	if len(i.ICS.Nodes[setType].Ids) >= ktypes.MaxBehaviourContextSize {
		replacedPeer = i.ICS.Nodes[setType].Ids[0]
	}

	return i.Operator, replacedPeer
}

func (i *ClusterInfo) GetRandomContextDelta(
	setType ktypes.IcsSetType,
	requiredCount int,
	skipPeers []id.KramaID) (addedPeers, replacedPeers []id.KramaID) {
	addedPeers = make([]id.KramaID, 0, requiredCount)

	if i.ICS.Nodes[setType] == nil {
		return
	}

	if i.ICS.Nodes[setType] != nil {
		if x := len(i.ICS.Nodes[setType].Ids) + requiredCount - ktypes.MaxRandomContextSize; x > 0 {
			replacedPeers = i.ICS.Nodes[setType].Ids[0:x]
		}
	}

	set := i.ICS.Nodes[ktypes.RandomSet]
	for index, v := range set.Ids {
		if set.Responses.GetIndex(index) && !kutils.ContainsKramaID(skipPeers, v) {
			addedPeers = append(addedPeers, v)
		}

		if len(addedPeers) == requiredCount {
			break
		}
	}

	return
}

func (i *ClusterInfo) UpdateContextDelta(delta ktypes.ContextDelta) {
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

	return i.ICS.Nodes[ktypes.RandomSet].Count >= requiredRandomNodes &&
		i.ICS.Nodes[ktypes.ObserverSet].Count >= requiredObserverNodes
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

func (i *ClusterInfo) GetObservers() []string {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return ktypes.KIPPeerIDToString(i.ICS.Nodes[ktypes.ObserverSet].Ids)
}
func (i *ClusterInfo) GetRandomNodes() []string {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return ktypes.KIPPeerIDToString(i.ICS.Nodes[ktypes.RandomSet].Ids)
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

func (i *ClusterInfo) GetContextHash(ixHash ktypes.Hash, addr ktypes.Address) ktypes.Hash {
	receipt, err := i.Receipts.GetReceipt(ixHash)
	if err != nil {
		return ktypes.NilHash
	}

	return receipt.ContextHashes[addr]
}
func (i *ClusterInfo) GetStateHash(ixHash ktypes.Hash, addr ktypes.Address) ktypes.Hash {
	receipt, err := i.Receipts.GetReceipt(ixHash)
	if err != nil {
		return ktypes.NilHash
	}

	return receipt.StateHashes[addr]
}

func (i *ClusterInfo) GetGasUsed() (gasUsed uint64) {
	for _, receipt := range i.Receipts {
		gasUsed += receipt.GasUsed
	}

	return
}

func (i *ClusterInfo) SetReceipts(r ktypes.Receipts) {
	i.Receipts = r
}

func (i *ClusterInfo) SetGrid(grid []*ktypes.Tesseract) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.Grid = grid
}
func (i *ClusterInfo) GetTesseractGrid() []*ktypes.Tesseract {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.Grid
}

func (i *ClusterInfo) AddDirty(key ktypes.Hash, data []byte) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.dirty[key] = data
}

func (i *ClusterInfo) ComputeICSHash() (hash ktypes.Hash) {
	msg := &ktypes.ICSClusterInfo{
		RandomSet:   i.GetRandomNodes(),
		ObserverSet: i.GetObservers(),
		Responses:   make([]*ktypes.ArrayOfBits, 6),
	}

	for j := 0; j < len(i.ICS.Nodes); j++ {
		if i.ICS.Nodes[j] != nil && i.ICS.Nodes[j].Responses != nil && i.ICS.Nodes[j].Responses.Size > 0 {
			msg.Responses[j] = i.ICS.Nodes[j].Responses
		} else {
			msg.Responses[j] = nil
		}
	}

	rawData := polo.Polorize(msg)
	hash = blake2b.Sum256(rawData)
	i.AddDirty(hash, rawData)
	i.ICSHash = hash

	return
}
func (i *ClusterInfo) CreateICSSuccessMsg() *ktypes.ICSSuccess {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	msg := &ktypes.ICSSuccess{
		ClusterID:   string(i.ID),
		RandomSet:   i.ICS.Nodes[ktypes.RandomSet].Ids,
		ObserverSet: i.ICS.Nodes[ktypes.ObserverSet].Ids,
		Responses:   make([]*ktypes.ArrayOfBits, 6),
		Signature:   make([]byte, 0),
		QuorumSizes: make([]int, 6),
	}

	for j := 0; j < len(i.ICS.Nodes); j++ {
		if i.ICS.Nodes[j] != nil {
			msg.Responses[j] = i.ICS.Nodes[j].Responses
			msg.QuorumSizes[j] = i.ICS.Nodes[j].QuorumSize
		}
	}

	//i.Nodes[SenderBehaviourSet].Responses
	//msg.Responses[SenderBehaviourSet] = i.Nodes[SenderBehaviourSet].Responses.ToProto()
	//msg.Responses[SenderRandomSet] = i.Nodes[SenderRandomSet].Responses.ToProto()
	//msg.Responses[ReceiverBehaviourSet] = i.Nodes[ReceiverBehaviourSet].Responses.ToProto()
	//msg.Responses[ReceiverRandomSet] = i.Nodes[ReceiverRandomSet].Responses.ToProto()
	//msg.Responses[RandomSet] = i.Nodes[RandomSet].Responses.ToProto()
	//msg.Responses[ObserverSet] = i.Nodes[ObserverSet].Responses.ToProto()

	return msg
}

func (i *ClusterInfo) GetRandomDelta(requiredCount int) []id.KramaID {
	nodes := make([]id.KramaID, 0, requiredCount)
	set := i.ICS.Nodes[ktypes.RandomSet]

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

func (i *ClusterInfo) GetDirty() map[ktypes.Hash][]byte {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return i.dirty
}

type AccountInfos map[ktypes.Address]*ktypes.AccountMetaInfo

func (a AccountInfos) GetLatestHash(addr ktypes.Address) ktypes.Hash {
	if v, ok := a[addr]; ok {
		return v.TesseractHash
	}

	return ktypes.NilHash
}

func (a AccountInfos) GetHeight(addr ktypes.Address) int64 {
	if v, ok := a[addr]; ok {
		return v.Height.Int64()
	}

	return 0
}
func (a AccountInfos) IsGenesis(addr ktypes.Address) bool {
	return a[addr].Height.Int64() == -1
}
