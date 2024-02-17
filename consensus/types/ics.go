package types

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	gtypes "github.com/sarvalabs/go-moi/state"
)

type ClusterState struct {
	mtx                      sync.Mutex
	selfID                   kramaid.KramaID
	NodeSet                  *common.ICSNodeSet
	Ixs                      common.Interactions
	ClusterID                common.ClusterID
	Operator                 kramaid.KramaID
	AccountInfos             AccountInfos
	contextDelta             common.ContextDelta
	postExecState            common.AccStateHashes
	Receipts                 common.Receipts
	BinaryHash, IdentityHash common.Hash
	ICSHash                  common.Hash
	dirty                    map[common.Hash][]byte
	Tesseract                *common.Tesseract
	ICSReqTime               time.Time
	ICSRespCount             int
	operatorIncluded         bool
	CurrentRole              common.IcsSetType
	RequestMsg               *CanonicalICSRequest
	SuccessMsg               *ICSMSG
}

// TODO: Check on locks

func NewICS(
	size int,
	icsReqMsg *CanonicalICSRequest,
	ixs common.Interactions,
	clusterID common.ClusterID,
	operator kramaid.KramaID,
	reqTime time.Time,
	selfID kramaid.KramaID,
) *ClusterState {
	return &ClusterState{
		NodeSet:          common.NewICSNodeSet(size),
		Ixs:              ixs,
		selfID:           selfID,
		ClusterID:        clusterID,
		Operator:         operator,
		operatorIncluded: false,
		AccountInfos:     make(AccountInfos, 0),
		contextDelta:     make(common.ContextDelta),
		Receipts:         make(common.Receipts, 1), // This should be changed base on the interactions
		dirty:            make(map[common.Hash][]byte),
		ICSReqTime:       reqTime,
		ICSRespCount:     0,
		RequestMsg:       icsReqMsg,
	}
}

func (cs *ClusterState) SelfKramaID() kramaid.KramaID {
	return cs.selfID
}

func (cs *ClusterState) Size() int {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Size
}

func (cs *ClusterState) GetNodeSet(setType common.IcsSetType) *common.NodeSet {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Nodes[setType]
}

func (cs *ClusterState) UpdateNodeSet(setType common.IcsSetType, data *common.NodeSet) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.NodeSet.UpdateNodeSet(setType, data)
}

func (cs *ClusterState) GetContextDelta() common.ContextDelta {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.contextDelta
}

func (cs *ClusterState) ContextDelta(address identifiers.Address) common.DeltaGroup {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	delta, ok := cs.contextDelta[address]
	if !ok {
		return common.DeltaGroup{}
	}

	return *delta
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

func (cs *ClusterState) NewHeights() map[identifiers.Address]uint64 {
	heights := make(map[identifiers.Address]uint64, len(cs.AccountInfos))

	if !cs.Ixs[0].Sender().IsNil() {
		heights[cs.Ixs[0].Sender()] = cs.AccountInfos.GetHeight(cs.Ixs[0].Sender()) + 1
	}

	if cs.Ixs[0].Receiver().IsNil() {
		return heights
	}

	if !cs.AccountInfos[cs.Ixs[0].Receiver()].IsGenesis {
		heights[cs.Ixs[0].Receiver()] = cs.AccountInfos.GetHeight(cs.Ixs[0].Receiver()) + 1

		return heights
	}

	heights[common.SargaAddress] = cs.AccountInfos.GetHeight(common.SargaAddress) + 1
	heights[cs.Ixs[0].Receiver()] = cs.AccountInfos.GetHeight(cs.Ixs[0].Receiver())

	return heights
}

func (cs *ClusterState) NewHeight(addr identifiers.Address) uint64 {
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
		ClusterID:    string(cs.ClusterID),
		IxHash:       cs.Ixs[0].Hash(), // Need to be improved
		Operator:     string(cs.Operator),
		ClusterSize:  cs.NodeSet.Size,
		BinaryHash:   cs.BinaryHash,
		IdentityHash: cs.IdentityHash,
		IcsHash:      cs.ICSHash,
		ReceiptHash:  receiptHash,
	}

	rawData, err := polo.Polorize(cs.GetSuccessMsg())
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

func (cs *ClusterState) RespondedEligibleSet() (count int, nodes []kramaid.KramaID) {
	count = cs.NodeSet.GetRespondedNodeCount(0, 3)
	nodes = make([]kramaid.KramaID, 0, count)

	for i := 0; i < 4; i++ {
		if cs.NodeSet.Nodes[i] != nil {
			for _, respIndex := range cs.NodeSet.Nodes[i].Responses.GetTrueIndices() {
				nodes = append(nodes, cs.NodeSet.Nodes[i].Ids[respIndex])
			}
		}
	}

	return
}

func (cs *ClusterState) GetBehaviouralContextDelta(setType common.IcsSetType) (added, replaced kramaid.KramaID) {
	for _, peerID := range cs.NodeSet.Nodes[setType].Ids {
		if cs.Operator == peerID { // cs.ICS.Nodes[setType].Responses.GetIndex(index)
			return
		}
	}

	if len(cs.NodeSet.Nodes[setType].Ids) >= gtypes.MaxBehaviourContextSize {
		replaced = cs.NodeSet.Nodes[setType].Ids[0]
	}

	return cs.Operator, replaced
}

func (cs *ClusterState) GetRandomContextDelta(
	setType common.IcsSetType,
	requiredCount int,
	skipPeers ...kramaid.KramaID,
) (addedPeers, replacedPeers []kramaid.KramaID) {
	addedPeers = make([]kramaid.KramaID, 0, requiredCount)

	if cs.NodeSet.Nodes[setType] != nil {
		if count := len(cs.NodeSet.Nodes[setType].Ids) + requiredCount - gtypes.MaxRandomContextSize; count > 0 {
			replacedPeers = cs.NodeSet.Nodes[setType].Ids[0:count]
		}
	}

	set := cs.NodeSet.Nodes[common.RandomSet]
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

func (cs *ClusterState) UpdateContextDelta(delta common.ContextDelta) {
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

	return cs.NodeSet.Nodes[common.RandomSet].GetRespCount() >= requiredRandomNodes &&
		cs.NodeSet.Nodes[common.ObserverSet].GetRespCount() >= requiredObserverNodes
}

func (cs *ClusterState) HasKramaID(kramaID kramaid.KramaID) (int32, bool) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.HasKramaID(kramaID)
}

// GetByIndex returns the krama id and bls public key of the validator based on the index
func (cs *ClusterState) GetByIndex(index int32) (kramaid.KramaID, []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	slots, slotIndex, kramaID, publicKey := cs.NodeSet.GetKramaID(index)
	if slots == nil || !cs.NodeSet.Nodes[slots[0]].Responses.GetIndex(slotIndex) {
		return "", nil
	}

	return kramaID, publicKey
}

func (cs *ClusterState) GetICSNodeIndex(kramaid kramaid.KramaID) (common.IcsSetType, int) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.GetIndex(kramaid)
}

func (cs *ClusterState) GetICSVoteset() *common.ArrayOfBits {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.GetVoteset()
}

func (cs *ClusterState) GetObservers() []kramaid.KramaID {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Nodes[common.ObserverSet].Ids
}

func (cs *ClusterState) GetRandomNodes() []kramaid.KramaID {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Nodes[common.RandomSet].Ids
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

func (cs *ClusterState) GetPreviousContextHash(addr identifiers.Address) common.Hash {
	accInfo, ok := cs.AccountInfos[addr]
	if !ok {
		return common.NilHash
	}

	return accInfo.ContextHash
}

func (cs *ClusterState) GetContextHash(addr identifiers.Address) common.Hash {
	return cs.postExecState.ContextHash(addr)
}

func (cs *ClusterState) GetStateHash(addr identifiers.Address) common.Hash {
	return cs.postExecState.StateHash(addr)
}

func (cs *ClusterState) GetFuelUsed() (fuelUsed uint64) {
	for _, receipt := range cs.Receipts {
		fuelUsed += receipt.FuelUsed
	}

	return fuelUsed
}

func (cs *ClusterState) GetSuccessMsg() *ICSMSG {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.SuccessMsg
}

func (cs *ClusterState) SetReceipts(r common.Receipts) {
	cs.Receipts = r
}

func (cs *ClusterState) SetPostExecState(s common.AccStateHashes) {
	cs.postExecState = s
}

func (cs *ClusterState) SetSuccessMsg(msg *ICSMSG) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.SuccessMsg = msg
}

func (cs *ClusterState) SetTesseract(ts *common.Tesseract) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.Tesseract = ts
}

func (cs *ClusterState) GetTesseract() *common.Tesseract {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.Tesseract
}

func (cs *ClusterState) AddDirty(key common.Hash, data []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.dirty[key] = data
}

func (cs *ClusterState) ComputeICSHash() (common.Hash, error) {
	msg := &common.ICSClusterInfo{
		RandomSet:   cs.GetRandomNodes(),
		ObserverSet: cs.GetObservers(),
		Responses:   make([]*common.ArrayOfBits, 6),
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
		return common.NilHash, err
	}

	hash := blake2b.Sum256(rawData)
	cs.AddDirty(hash, rawData)
	cs.ICSHash = hash

	return hash, nil
}

func (cs *ClusterState) CreateICSSuccessMsg() *ICSSuccess {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	msg := &ICSSuccess{
		ClusterID:   cs.ClusterID,
		Responses:   make([]*common.ArrayOfBits, 6),
		Signature:   make([]byte, 0),
		QuorumSizes: make([]int, 6),
	}

	for j := 0; j < len(cs.NodeSet.Nodes); j++ {
		if cs.NodeSet.Nodes[j] != nil {
			msg.Responses[j] = cs.NodeSet.Nodes[j].Responses.Copy()
			msg.QuorumSizes[j] = cs.NodeSet.Nodes[j].QuorumSize
		}
	}

	return msg
}

func (cs *ClusterState) GetRandomDelta(requiredCount int) []kramaid.KramaID {
	nodes := make([]kramaid.KramaID, 0, requiredCount)
	set := cs.NodeSet.Nodes[common.RandomSet]

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

func (cs *ClusterState) ExecutionContext() *common.ExecutionContext {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return &common.ExecutionContext{
		CtxDelta: cs.contextDelta,
		Cluster:  cs.ClusterID,
		Time:     cs.ICSReqTime.Unix(),
	}
}

func (cs *ClusterState) GetDirty() map[common.Hash][]byte {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.dirty
}

func (cs *ClusterState) ContextLock() map[identifiers.Address]common.ContextLockInfo {
	lockInfo := make(map[identifiers.Address]common.ContextLockInfo)
	for addr, accInfo := range cs.AccountInfos {
		lockInfo[addr] = common.ContextLockInfo{
			ContextHash:   accInfo.ContextHash,
			Height:        accInfo.Height,
			TesseractHash: accInfo.TesseractHash,
		}
	}

	return lockInfo
}

func (cs *ClusterState) GetICSRespCount() int {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.ICSRespCount
}

func (cs *ClusterState) IncrementICSRespCount(count int) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.ICSRespCount += count
}

type AccountInfo struct {
	AccType       common.AccountType
	Address       identifiers.Address
	IsGenesis     bool
	ContextHash   common.Hash
	TesseractHash common.Hash
	Height        uint64
	Mode          string
}

func AccountInfoFromAccMetaInfo(metaInfo *common.AccountMetaInfo, isGenesis bool) *AccountInfo {
	return &AccountInfo{
		AccType:       metaInfo.Type,
		Address:       metaInfo.Address,
		IsGenesis:     isGenesis,
		Height:        metaInfo.Height,
		TesseractHash: metaInfo.TesseractHash,
	}
}

type AccountInfos map[identifiers.Address]*AccountInfo

func (a AccountInfos) GetLatestHash(addr identifiers.Address) common.Hash {
	if v, ok := a[addr]; ok {
		return v.TesseractHash
	}

	return common.NilHash
}

func (a AccountInfos) GetHeight(addr identifiers.Address) uint64 {
	if v, ok := a[addr]; ok {
		return v.Height
	}

	return 0
}

func (a AccountInfos) IsGenesis(addr identifiers.Address) bool {
	return a[addr].IsGenesis
}

func (a AccountInfos) Address() []identifiers.Address {
	addrs := make([]identifiers.Address, 0, len(a))

	for addr := range a {
		addrs = append(addrs, addr)
	}

	return addrs
}

type Request struct {
	Ixs          common.Interactions
	Msg          *CanonicalICSRequest
	Operator     kramaid.KramaID
	SlotType     SlotType
	ReqTime      time.Time
	ResponseChan chan error
}

func (r *Request) IxHash() common.Hash {
	return r.Ixs[0].Hash()
}

func (r *Request) GetClusterID() (common.ClusterID, error) {
	switch r.SlotType {
	case OperatorSlot:
		return generateClusterID()
	case ValidatorSlot:
		return r.Msg.ClusterID, nil
	default:
		return "", errors.New("invalid request type")
	}
}

func generateClusterID() (common.ClusterID, error) {
	randHash := make([]byte, 32)

	if _, err := rand.Read(randHash); err != nil {
		return "", err
	}

	return common.ClusterID(base58.Encode(randHash)), nil
}
