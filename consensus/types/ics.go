package types

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
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
	TrustedPeers             []kramaid.KramaID
	ixns                     common.Interactions
	ClusterID                common.ClusterID
	Operator                 kramaid.KramaID
	BinaryHash, IdentityHash common.Hash
	ICSHash                  common.Hash
	dirty                    map[common.Hash][]byte
	Tesseract                *common.Tesseract
	ICSReqTime               time.Time
	ICSRespCount             int
	operatorIncluded         bool
	Participants             common.Participants
	RequestMsg               *CanonicalICSRequest
	SuccessMsg               *ICSMSG
	Transition               *gtypes.Transition
	IsObserver               bool
	quorum                   []uint32
	VRFOutput                [32]byte
	VRFProof                 []byte
	LotteryKey               common.LotteryKey
}

// TODO: Check on locks

func NewICS(
	icsReqMsg *CanonicalICSRequest,
	ixs common.Interactions,
	clusterID common.ClusterID,
	operator kramaid.KramaID,
	reqTime time.Time,
	selfID kramaid.KramaID,
	participants map[identifiers.Address]*common.Participant,
	nodeSet *common.ICSNodeSet,
	lotteryKey common.LotteryKey,
) *ClusterState {
	return &ClusterState{
		NodeSet:          nodeSet,
		ixns:             ixs,
		selfID:           selfID,
		ClusterID:        clusterID,
		Operator:         operator,
		operatorIncluded: false,
		dirty:            make(map[common.Hash][]byte),
		ICSReqTime:       reqTime,
		ICSRespCount:     0,
		RequestMsg:       icsReqMsg,
		Participants:     participants,
		Transition:       gtypes.NewTransition(nil),
		LotteryKey:       lotteryKey,
	}
}

func (cs *ClusterState) IxnHash() common.Hash {
	return cs.ixns[0].Hash()
}

func (cs *ClusterState) SelfKramaID() kramaid.KramaID {
	return cs.selfID
}

func (cs *ClusterState) ParticipantHeight(addr identifiers.Address) uint64 {
	ps, ok := cs.Participants[addr]
	if ok {
		return ps.Height
	}

	return 0
}

func (cs *ClusterState) ParticipantTSHash(addr identifiers.Address) common.Hash {
	ps, ok := cs.Participants[addr]
	if ok {
		return ps.TSHash()
	}

	return common.NilHash
}

func (cs *ClusterState) Size() int {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.TotalNodes()
}

func (cs *ClusterState) GetNodeSet(nodeSetPosition int) *common.NodeSet {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.Sets[nodeSetPosition]
}

func (cs *ClusterState) UpdateNodeSet(nodeSetPosition int, data *common.NodeSet) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.NodeSet.UpdateNodeSet(nodeSetPosition, data)
}

func (cs *ClusterState) UpdateNodeSetResponses(nodeSetPosition int, responses *common.ArrayOfBits) {
	cs.NodeSet.UpdateNodeSetResponses(nodeSetPosition, responses)
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
	heights := make(map[identifiers.Address]uint64, len(cs.Participants))

	for addr, ps := range cs.Participants {
		heights[addr] = ps.NewHeight()
	}

	return heights
}

// GetMetaData returns the cluster metadata including the vote messages
func (cs *ClusterState) GetMetaData(msgs []*ICSMSG) (*ICSMetaInfo, error) {
	m := &ICSMetaInfo{
		ClusterID:    string(cs.ClusterID),
		IxHash:       cs.ixns[0].Hash(), // Need to be improved
		Operator:     string(cs.Operator),
		ClusterSize:  cs.NodeSet.TotalNodes(),
		BinaryHash:   cs.BinaryHash,
		IdentityHash: cs.IdentityHash,
		IcsHash:      cs.ICSHash,
		ReceiptHash:  common.NilHash, // FIXME: observer nodes should execute ixns
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

func (cs *ClusterState) RespondedEligibleSet() (count int, nodes []kramaid.KramaID) {
	count = cs.NodeSet.GetRespondedNodeCount(0, 3)
	nodes = make([]kramaid.KramaID, 0, count)

	for i := 0; i < 4; i++ {
		if cs.NodeSet.Sets[i] != nil {
			for _, respIndex := range cs.NodeSet.Sets[i].Responses.GetTrueIndices() {
				nodes = append(nodes, cs.NodeSet.Sets[i].Ids[respIndex])
			}
		}
	}

	return
}

func (cs *ClusterState) GetBehaviouralContextDelta(
	nodeSetPosition int,
	newPeer kramaid.KramaID,
) (added, replaced kramaid.KramaID) {
	for _, peerID := range cs.NodeSet.Sets[nodeSetPosition].Ids {
		if newPeer == peerID { // cs.ICS.Nodes[setType].Responses.GetIndex(index)
			return
		}
	}

	if len(cs.NodeSet.Sets[nodeSetPosition].Ids) >= gtypes.MaxBehaviourContextSize {
		replaced = cs.NodeSet.Sets[nodeSetPosition].Ids[0]
	}

	return newPeer, replaced
}

func (cs *ClusterState) GetRandomContextDelta(
	nodeSetPosition int,
	requiredCount int,
	skipPeers ...kramaid.KramaID,
) (addedPeers, replacedPeers []kramaid.KramaID) {
	addedPeers = make([]kramaid.KramaID, 0, requiredCount)

	if cs.NodeSet.Sets[nodeSetPosition] != nil {
		if count := len(cs.NodeSet.Sets[nodeSetPosition].Ids) + requiredCount - gtypes.MaxRandomContextSize; count > 0 {
			replacedPeers = cs.NodeSet.Sets[nodeSetPosition].Ids[0:count]
		}
	}

	if len(cs.TrustedPeers) > 0 {
		for _, trustedPeer := range cs.TrustedPeers {
			if !utils.ContainsKramaID(skipPeers, trustedPeer) {
				addedPeers = append(addedPeers, trustedPeer)
			}

			if len(addedPeers) == requiredCount {
				break
			}
		}

		return addedPeers, replacedPeers
	}

	set := cs.NodeSet.RandomSet()
	for index, v := range set.Ids {
		if set.Responses.GetIndex(index) && !utils.ContainsKramaID(skipPeers, v) {
			addedPeers = append(addedPeers, v)
		}

		if len(addedPeers) == requiredCount {
			break
		}
	}

	return addedPeers, replacedPeers
}

func (cs *ClusterState) IsContextQuorum() bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.IsContextQuorum()
}

func (cs *ClusterState) IsRandomQuorum(requiredRandomNodes int) bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.RandomSet().GetRespCount() >= requiredRandomNodes
}

func (cs *ClusterState) IsObserverQuorum(requiredObserverNodes int) bool {
	return cs.NodeSet.ObserverSet().GetRespCount() >= requiredObserverNodes
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
	if slots == nil || !cs.NodeSet.Sets[slots[0]].Responses.GetIndex(slotIndex) {
		return "", nil
	}

	return kramaID, publicKey
}

func (cs *ClusterState) GetICSNodeIndex(kramaid kramaid.KramaID) (int, int) {
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

	return cs.NodeSet.ObserverSet().Ids
}

func (cs *ClusterState) GetRandomNodes() []kramaid.KramaID {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.NodeSet.RandomSet().Ids
}

func (cs *ClusterState) GetQuorum() []uint32 {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	if cs.quorum != nil {
		return cs.quorum
	}

	quorum := make([]uint32, len(cs.Participants)+1) // We add one here for random Set

	for _, ps := range cs.Participants {
		quorum[ps.NodeSetPosition/2] = ps.ConsensusQuorum
	}

	quorum[len(quorum)-1] = cs.NodeSet.RandomQuorumSize()

	cs.quorum = quorum

	return quorum
}

func (cs *ClusterState) GetSuccessMsg() *ICSMSG {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.SuccessMsg
}

func (cs *ClusterState) SetStateTransition(st *gtypes.Transition) {
	cs.Transition = st
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
		RandomSet:                 cs.GetRandomNodes(),
		RandomSetSizeWithoutDelta: cs.NodeSet.RandomSet().SetSizeWithOutDelta,
		ObserverSet:               cs.GetObservers(),
		Responses:                 cs.NodeSet.Responses(),
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
		ClusterID: cs.ClusterID,
		Responses: cs.NodeSet.Responses(),
		Signature: make([]byte, 0),
	}

	return msg
}

func (cs *ClusterState) ExecutionContext() *common.ExecutionContext {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return &common.ExecutionContext{
		Participants: cs.Participants.IxnParticipants(),
		CtxDelta:     cs.ContextDelta(),
		Cluster:      cs.ClusterID,
		Time:         uint64(cs.ICSReqTime.Unix()),
	}
}

func (cs *ClusterState) GetDirty() map[common.Hash][]byte {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.dirty
}

func (cs *ClusterState) ContextLock() map[identifiers.Address]common.ContextLockInfo {
	lockInfo := make(map[identifiers.Address]common.ContextLockInfo)
	for addr, ps := range cs.Participants {
		lockInfo[addr] = common.ContextLockInfo{
			ContextHash:   ps.ContextHash,
			Height:        ps.Height,
			TesseractHash: ps.TesseractHash,
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

func (cs *ClusterState) ContextDelta() common.ContextDelta {
	contextDelta := make(common.ContextDelta)

	for addr, ps := range cs.Participants {
		if ps.ContextDelta != nil {
			contextDelta[addr] = ps.ContextDelta

			continue
		}
	}

	return contextDelta
}

func (cs *ClusterState) Ixns() common.Interactions {
	return cs.ixns
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
	Ctx          context.Context
	Ixs          common.Interactions
	Msg          *CanonicalICSRequest
	Operator     kramaid.KramaID
	SlotType     SlotType
	ReqTime      time.Time
	ResponseChan chan Response
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

type Response struct {
	Err  error
	Data any
}

func generateClusterID() (common.ClusterID, error) {
	randHash := make([]byte, 32)

	if _, err := rand.Read(randHash); err != nil {
		return "", err
	}

	return common.ClusterID(base58.Encode(randHash)), nil
}

type ICSOperatorInfo struct {
	KramaID  kramaid.KramaID
	Priority uint64
	Attempts uint8
}
