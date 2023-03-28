package types

import (
	"sync"
	"time"

	"github.com/pkg/errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type ClusterState struct {
	mtx                      sync.Mutex
	NodeSet                  *ICSNodeSet
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
	RequestMsg               *ptypes.ICSRequest
	SuccessMsg               *ICSMSG
}

// TODO: Check on locks

func NewICS(
	size int,
	icsReqMsg *ptypes.ICSRequest,
	ixs types.Interactions,
	clusterID types.ClusterID,
	operator id.KramaID,
	reqTime time.Time,
) *ClusterState {
	return &ClusterState{
		NodeSet:          NewICSNodeSet(size),
		Ixs:              ixs,
		ID:               clusterID,
		Operator:         operator,
		operatorIncluded: false,
		AccountInfos:     make(AccountInfos, 0),
		contextDelta:     make(types.ContextDelta),
		Receipts:         make(types.Receipts, 1), // This should be changed base on the transactions
		dirty:            make(map[types.Hash][]byte),
		ICSReqTime:       reqTime,
		RequestMsg:       icsReqMsg,
	}
}

func (cs *ClusterState) Size() int {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Size
}

func (cs *ClusterState) UpdateNodeSet(setType IcsSetType, data *NodeSet) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.NodeSet.UpdateNodeSet(setType, data)
}

func (cs *ClusterState) GetContextDelta() types.ContextDelta {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.contextDelta
}

func (cs *ClusterState) IncludeOperator() {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.operatorIncluded = true
}

func (cs *ClusterState) IsOperatorIncluded() bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.operatorIncluded
}

func (cs *ClusterState) NewHeights() []uint64 {
	heights := make([]uint64, len(cs.AccountInfos))

	if !cs.Ixs[0].Sender().IsNil() {
		heights[0] = cs.AccountInfos.GetHeight(cs.Ixs[0].Sender()) + 1
	}

	if cs.Ixs[0].Receiver().IsNil() {
		return heights
	}

	if !cs.AccountInfos[cs.Ixs[0].Receiver()].IsGenesis {
		heights[1] = cs.AccountInfos.GetHeight(cs.Ixs[0].Receiver()) + 1

		return heights
	}

	heights[2] = cs.AccountInfos.GetHeight(types.SargaAddress) + 1
	heights[1] = cs.AccountInfos.GetHeight(cs.Ixs[0].Receiver())

	return heights
}

func (cs *ClusterState) NewHeight(addr types.Address) uint64 {
	if cs.AccountInfos.IsGenesis(addr) {
		return cs.AccountInfos.GetHeight(addr)
	}

	return cs.AccountInfos.GetHeight(addr) + 1
}

// GetMetaData returns the cluster metadata including the vote messages
func (cs *ClusterState) GetMetaData(msgs []*ICSMSG) (*ICSMetaInfo, error) {
	receiptHash, err := cs.Receipts.Hash()
	if err != nil {
		return nil, err
	}

	m := &ICSMetaInfo{
		ClusterID:    string(cs.ID),
		IxHash:       cs.Ixs[0].Hash(), // Need to be improved
		Operator:     string(cs.Operator),
		ClusterSize:  cs.NodeSet.Size,
		BinaryHash:   cs.BinaryHash,
		IdentityHash: cs.IdentityHash,
		IcsHash:      cs.ICSHash,
		ReceiptHash:  receiptHash,
	}

	rawData, err := polo.Polorize(cs.SuccessMsg)
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

func (cs *ClusterState) IncrementClusterSize(delta int) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.NodeSet.Size += delta
}

func (cs *ClusterState) RespondedEligibleSet() (count int, nodes []id.KramaID) {
	count = cs.NodeSet.GetRespondedNodeCount(0, 3)
	nodes = make([]id.KramaID, 0, count)

	for i := 0; i < 4; i++ {
		if cs.NodeSet.Nodes[i] != nil {
			for _, respIndex := range cs.NodeSet.Nodes[i].Responses.GetTrueIndices() {
				nodes = append(nodes, cs.NodeSet.Nodes[i].Ids[respIndex])
			}
		}
	}

	return
}

func (cs *ClusterState) GetBehaviouralContextDelta(setType IcsSetType) (addedPeer, replacedPeer id.KramaID) {
	for _, peerID := range cs.NodeSet.Nodes[setType].Ids {
		if cs.Operator == peerID { // cs.ICS.Nodes[setType].Responses.GetIndex(index)
			return
		}
	}

	if len(cs.NodeSet.Nodes[setType].Ids) >= gtypes.MaxBehaviourContextSize {
		replacedPeer = cs.NodeSet.Nodes[setType].Ids[0]
	}

	return cs.Operator, replacedPeer
}

func (cs *ClusterState) GetRandomContextDelta(
	setType IcsSetType,
	requiredCount int,
	skipPeers ...id.KramaID,
) (addedPeers, replacedPeers []id.KramaID) {
	addedPeers = make([]id.KramaID, 0, requiredCount)

	if cs.NodeSet.Nodes[setType] != nil {
		if count := len(cs.NodeSet.Nodes[setType].Ids) + requiredCount - gtypes.MaxRandomContextSize; count > 0 {
			replacedPeers = cs.NodeSet.Nodes[setType].Ids[0:count]
		}
	}

	set := cs.NodeSet.Nodes[RandomSet]
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

func (cs *ClusterState) UpdateContextDelta(delta types.ContextDelta) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.contextDelta = delta
}

func (cs *ClusterState) IsContextQuorum() bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.IsContextQuorum()
}

func (cs *ClusterState) IsRandomQuorum(requiredRandomNodes, requiredObserverNodes int) bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Nodes[RandomSet].RespCount >= requiredRandomNodes &&
		cs.NodeSet.Nodes[ObserverSet].RespCount >= requiredObserverNodes
}

func (cs *ClusterState) HasKramaID(kramaID id.KramaID) (int32, bool) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.GetIndex(kramaID)
}

// GetByIndex returns the krama id and bls public key of the validator based on the index
func (cs *ClusterState) GetByIndex(index int32) (id.KramaID, []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	slots, slotIndex, kramaID, publicKey := cs.NodeSet.GetKramaID(index)
	if slots == nil || !cs.NodeSet.Nodes[slots[0]].Responses.GetIndex(slotIndex) {
		return "", nil
	}

	return kramaID, publicKey
}

func (cs *ClusterState) GetICSNodes() []id.KramaID {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.GetNodes()
}

func (cs *ClusterState) GetObservers() []string {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return utils.KramaIDToString(cs.NodeSet.Nodes[ObserverSet].Ids)
}

func (cs *ClusterState) GetRandomNodes() []string {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return utils.KramaIDToString(cs.NodeSet.Nodes[RandomSet].Ids)
}

func (cs *ClusterState) GetTotalVotingPower() []int32 {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	quorum := make([]int32, 3)
	quorum[0] = int32(cs.NodeSet.SenderSetSize())
	quorum[1] = int32(cs.NodeSet.ReceiverSetSize())
	quorum[2] = int32(cs.NodeSet.RandomSetSize())

	return quorum
}

func (cs *ClusterState) GetQuorum() []int32 {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	quorum := make([]int32, 3)
	quorum[0] = int32(cs.NodeSet.SenderQuorumSize())
	quorum[1] = int32(cs.NodeSet.ReceiverQuorumSize())
	quorum[2] = int32(cs.NodeSet.RandomQuorumSize())

	return quorum
}

func (cs *ClusterState) GetContextHash(ixHash types.Hash, addr types.Address) types.Hash {
	receipt, err := cs.Receipts.GetReceipt(ixHash)
	if err != nil {
		return types.NilHash
	}

	return receipt.ContextHashes[addr]
}

func (cs *ClusterState) GetStateHash(ixHash types.Hash, addr types.Address) types.Hash {
	receipt, err := cs.Receipts.GetReceipt(ixHash)
	if err != nil {
		return types.NilHash
	}

	return receipt.StateHashes[addr]
}

func (cs *ClusterState) GetFuelUsed() (fuelUsed uint64) {
	for _, receipt := range cs.Receipts {
		fuelUsed += receipt.FuelUsed
	}

	return
}

func (cs *ClusterState) SetReceipts(r types.Receipts) {
	cs.Receipts = r
}

func (cs *ClusterState) SetGrid(grid []*types.Tesseract) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.Grid = grid
}

func (cs *ClusterState) GetTesseractGrid() []*types.Tesseract {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.Grid
}

func (cs *ClusterState) AddDirty(key types.Hash, data []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.dirty[key] = data
}

func (cs *ClusterState) ComputeICSHash() (types.Hash, error) {
	msg := &ptypes.ICSClusterInfo{
		RandomSet:   cs.GetRandomNodes(),
		ObserverSet: cs.GetObservers(),
		Responses:   make([]*types.ArrayOfBits, 6),
	}

	for j := 0; j < len(cs.NodeSet.Nodes); j++ {
		if cs.NodeSet.Nodes[j] != nil && cs.NodeSet.Nodes[j].Responses != nil && cs.NodeSet.Nodes[j].Responses.Size > 0 {
			msg.Responses[j] = cs.NodeSet.Nodes[j].Responses
		} else {
			msg.Responses[j] = nil
		}
	}

	rawData, err := polo.Polorize(msg)
	if err != nil {
		return types.NilHash, err
	}

	hash := blake2b.Sum256(rawData)
	cs.AddDirty(hash, rawData)
	cs.ICSHash = hash

	return hash, nil
}

func (cs *ClusterState) CreateICSSuccessMsg() *ptypes.ICSSuccessMsg {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	msg := &ptypes.ICSSuccessMsg{
		ClusterID:   string(cs.ID),
		RandomSet:   cs.NodeSet.Nodes[RandomSet].Ids,
		ObserverSet: cs.NodeSet.Nodes[ObserverSet].Ids,
		Responses:   make([]*types.ArrayOfBits, 6),
		Signature:   make([]byte, 0),
		QuorumSizes: make([]int, 6),
	}

	for j := 0; j < len(cs.NodeSet.Nodes); j++ {
		if cs.NodeSet.Nodes[j] != nil {
			msg.Responses[j] = cs.NodeSet.Nodes[j].Responses
			msg.QuorumSizes[j] = cs.NodeSet.Nodes[j].QuorumSize
		}
	}

	return msg
}

func (cs *ClusterState) GetRandomDelta(requiredCount int) []id.KramaID {
	nodes := make([]id.KramaID, 0, requiredCount)
	set := cs.NodeSet.Nodes[RandomSet]

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

func (cs *ClusterState) UpdateClusterSize() {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.NodeSet.Size = 0

	for _, idSet := range cs.NodeSet.Nodes {
		if idSet != nil {
			cs.NodeSet.Size += len(idSet.Ids)
		}
	}
}

func (cs *ClusterState) GetDirty() map[types.Hash][]byte {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.dirty
}

func (cs *ClusterState) ContextLock() map[types.Address]types.ContextLockInfo {
	lockInfo := make(map[types.Address]types.ContextLockInfo)
	for addr, accInfo := range cs.AccountInfos {
		lockInfo[addr] = types.ContextLockInfo{
			ContextHash:   accInfo.ContextHash,
			Height:        accInfo.Height,
			TesseractHash: accInfo.TesseractHash,
		}
	}

	return lockInfo
}

type AccountInfo struct {
	AccType       types.AccountType
	Address       types.Address
	IsGenesis     bool
	ContextHash   types.Hash
	TesseractHash types.Hash
	Height        uint64
	Mode          string
}

func AccountInfoFromAccMetaInfo(metaInfo *types.AccountMetaInfo, isGenesis bool) *AccountInfo {
	return &AccountInfo{
		AccType:       metaInfo.Type,
		Address:       metaInfo.Address,
		IsGenesis:     isGenesis,
		Height:        metaInfo.Height,
		TesseractHash: metaInfo.TesseractHash,
	}
}

type AccountInfos map[types.Address]*AccountInfo

func (a AccountInfos) GetLatestHash(addr types.Address) types.Hash {
	if v, ok := a[addr]; ok {
		return v.TesseractHash
	}

	return types.NilHash
}

func (a AccountInfos) GetHeight(addr types.Address) uint64 {
	if v, ok := a[addr]; ok {
		return v.Height
	}

	return 0
}

func (a AccountInfos) IsGenesis(addr types.Address) bool {
	return a[addr].IsGenesis
}

type ICSMSG struct {
	MsgType   ptypes.MsgType
	Msg       []byte
	Sender    id.KramaID
	ClusterID string
}

func NewICSMsg(msgType ptypes.MsgType, clusterID string, msg []byte) *ICSMSG {
	return &ICSMSG{
		MsgType:   msgType,
		Msg:       msg,
		ClusterID: clusterID,
	}
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
