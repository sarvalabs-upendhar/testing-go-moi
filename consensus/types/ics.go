package types

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	gtypes "github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-polo"
)

type ClusterState struct {
	mtx                      sync.Mutex
	selfID                   kramaid.KramaID
	committee                *ICSCommittee
	voteSet                  *HeightVoteSet
	ixns                     common.Interactions
	ClusterID                common.ClusterID
	Proposer                 kramaid.KramaID
	operator                 kramaid.KramaID
	BinaryHash, IdentityHash common.Hash
	ICSHash                  common.Hash
	dirty                    map[common.Hash][]byte
	ts                       *common.Tesseract
	ICSReqTime               time.Time
	ICSRespCount             int
	operatorIncluded         bool
	Participants             common.Participants
	SuccessMsg               *ICSMSG
	Transition               *gtypes.Transition
	IsObserver               bool
	quorum                   []uint32
	// TODO: Load following view infos appropriately
	localViewInfo   common.Views
	highestViewInfo common.Views
	preparedQc      *PreparedInfo
	view            uint64
	TrustedPeers    []kramaid.KramaID
}

func (cs *ClusterState) SetPrepareQc(prepareQc *PreparedInfo) {
	cs.preparedQc = prepareQc
}

func (cs *ClusterState) Committee() *ICSCommittee {
	return cs.committee
}

// TODO: Check on locks

func NewICS(
	ixs common.Interactions,
	clusterID common.ClusterID,
	operator kramaid.KramaID,
	reqTime time.Time,
	selfID kramaid.KramaID,
	committee *ICSCommittee,
	participants map[identifiers.Address]*common.Participant,
	viewInfos common.Views,
	currentView uint64,
) *ClusterState {
	return &ClusterState{
		ixns:             ixs,
		selfID:           selfID,
		ClusterID:        clusterID,
		operator:         operator,
		operatorIncluded: false,
		dirty:            make(map[common.Hash][]byte),
		ICSReqTime:       reqTime,
		ICSRespCount:     0,
		Participants:     participants,
		committee:        committee,
		Transition:       gtypes.NewTransition(nil, nil),
		localViewInfo:    viewInfos.Copy(),
		highestViewInfo:  viewInfos.Copy(),
		view:             currentView,
	}
}

func (cs *ClusterState) IxnsHash() common.Hash {
	hash, _ := cs.ixns.Hash()

	return hash
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

func (cs *ClusterState) CurrentView() uint64 {
	return cs.view
}

func (cs *ClusterState) Size() int {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.TotalNodes()
}

func (cs *ClusterState) UpdateVoteSet(vs *HeightVoteSet) {
	cs.voteSet = vs
}

func (cs *ClusterState) VoteSet() *HeightVoteSet {
	return cs.voteSet
}

func (cs *ClusterState) GetNodeSet(nodeSetPosition int) *NodeSet {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.Sets[nodeSetPosition]
}

func (cs *ClusterState) UpdateNodeSet(nodeSetPosition int, data *NodeSet) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.committee.UpdateNodeSet(nodeSetPosition, data)
}

func (cs *ClusterState) UpdateNodeSetResponses(nodeSetPosition int, responses *common.ArrayOfBits) {
	cs.committee.UpdateSetResponses(nodeSetPosition, responses)
}

func (cs *ClusterState) Operator() kramaid.KramaID {
	return cs.operator
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

func (cs *ClusterState) ExcludeParticipantsFromICS(addrs common.Addresses) {
	cs.Participants.ExcludeFromICS(addrs)

	for _, info := range cs.Participants {
		if info.ExcludedFromICS() {
			cs.committee.ExcludeParticipantsFromICS(info.NodeSetPosition)
		}
	}
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
		IxHash:       cs.ixns.IxList()[0].Hash(), // Need to be improved
		Operator:     string(cs.operator),
		ClusterSize:  cs.committee.TotalNodes(),
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

func (cs *ClusterState) GetBehaviouralContextDelta(
	nodeSetPosition int,
	newPeer kramaid.KramaID,
) (added, replaced kramaid.KramaID) {
	for _, info := range cs.committee.Sets[nodeSetPosition].Infos {
		if newPeer == info.ID { // cs.ICS.Nodes[setType].Responses.GetIndex(index)
			return
		}
	}

	if len(cs.committee.Sets[nodeSetPosition].Infos) >= common.BehaviouralContextSize {
		replaced = cs.committee.Sets[nodeSetPosition].Infos[0].ID
	}

	return newPeer, replaced
}

func (cs *ClusterState) GetRandomContextDelta(
	nodeSetPosition int,
	requiredCount int,
	skipPeers ...kramaid.KramaID,
) (addedPeers, replacedPeers []kramaid.KramaID) {
	addedPeers = make([]kramaid.KramaID, 0, requiredCount)

	if cs.committee.Sets[nodeSetPosition] != nil {
		if count := len(cs.committee.Sets[nodeSetPosition].Infos) + requiredCount - common.StochasticSetSize; count > 0 {
			replacedPeers = cs.committee.Sets[nodeSetPosition].KramaIDs()[0:count]
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

	set := cs.committee.RandomSet()
	for index, info := range set.Infos {
		if set.Responses.GetIndex(index) && !utils.ContainsKramaID(skipPeers, info.ID) {
			addedPeers = append(addedPeers, info.ID)
		}

		if len(addedPeers) == requiredCount {
			break
		}
	}

	return addedPeers, replacedPeers
}

func (cs *ClusterState) LocalViewInfo() []*common.ViewInfo {
	return cs.localViewInfo
}

func (cs *ClusterState) HighestViewInfo() common.Views {
	return cs.highestViewInfo
}

func (cs *ClusterState) IsContextQuorum() bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.IsContextQuorum()
}

func (cs *ClusterState) IsRandomQuorum() bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.RandomSet().GetRespCount() >= int(cs.committee.RandomQuorumSize())
}

func (cs *ClusterState) HasKramaID(kramaID kramaid.KramaID) (int32, []byte, bool) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.HasKramaID(kramaID)
}

// GetByIndex returns the krama id and bls public key of the validator based on the index
func (cs *ClusterState) GetByIndex(index int32) (kramaid.KramaID, []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	slots, slotIndex, kramaID, publicKey := cs.committee.GetKramaID(index)
	if slots == nil || !cs.committee.Sets[slots[0]].Responses.GetIndex(slotIndex) {
		return "", nil
	}

	return kramaID, publicKey
}

func (cs *ClusterState) GetICSVoteset() *common.ArrayOfBits {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.GetVoteset()
}

func (cs *ClusterState) GetRandomNodes() []kramaid.KramaID {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.RandomSet().KramaIDs()
}

func (cs *ClusterState) GetQuorum() []uint32 {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	if cs.quorum != nil {
		return cs.quorum
	}

	quorum := make([]uint32, len(cs.Participants)+1) // We add one here for random Set

	for _, ps := range cs.Participants {
		if ps.ExcludeFromICS {
			continue
		}

		quorum[ps.NodeSetPosition] = ps.ConsensusQuorum
	}

	quorum[len(quorum)-1] = cs.committee.RandomQuorumSize()

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
	cs.ts = ts
}

func (cs *ClusterState) Tesseract() *common.Tesseract {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.ts
}

func (cs *ClusterState) AddDirty(key common.Hash, data []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.dirty[key] = data
}

func (cs *ClusterState) ExecutionContext() *common.ExecutionContext {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return &common.ExecutionContext{
		CtxDelta: cs.ContextDelta(),
		Cluster:  cs.ClusterID,
		Time:     uint64(cs.ICSReqTime.Unix()),
	}
}

func (cs *ClusterState) GetDirty() map[common.Hash][]byte {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.dirty
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
		if !ps.ExcludeFromICS && ps.ContextDelta != nil {
			contextDelta[addr] = ps.ContextDelta

			continue
		}
	}

	return contextDelta
}

func (cs *ClusterState) Ixns() common.Interactions {
	return cs.ixns
}

func (cs *ClusterState) PrepareQc() *PreparedInfo {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.preparedQc
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

func GenerateClusterID() (common.ClusterID, error) {
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
