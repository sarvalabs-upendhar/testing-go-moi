package types

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/mr-tron/base58"
	"github.com/sarvalabs/go-moi/common"
	gtypes "github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-polo"
)

type ClusterState struct {
	mtx                      sync.Mutex
	selfID                   identifiers.KramaID
	committee                *ICSCommittee
	voteSet                  *HeightVoteSet
	ixns                     common.Interactions
	ClusterID                common.ClusterID
	Proposer                 identifiers.KramaID
	operator                 identifiers.KramaID
	BinaryHash, IdentityHash common.Hash
	ICSHash                  common.Hash
	dirty                    map[common.Hash][]byte
	ts                       *common.Tesseract
	ICSRespCount             int
	operatorIncluded         bool
	Participants             common.Participants
	SuccessMsg               *ICSMSG
	Transition               *gtypes.Transition
	IsObserver               bool
	quorum                   []uint32
	SystemObject             *gtypes.SystemObject
	// TODO: Load following view infos appropriately
	localViewInfo   common.Views
	highestViewInfo common.Views
	preparedQc      *PreparedInfo
	currentView     *View
	TrustedPeers    []identifiers.KramaID
	isTSStored      bool // indicates if the proposed tesseract is already committed
}

func (cs *ClusterState) IsTSStored() bool {
	return cs.isTSStored
}

func (cs *ClusterState) GetSeed() [32]byte {
	// TODO: Implement this function
	return [32]byte{}
}

func (cs *ClusterState) SetPreparedQc(prepareQc *PreparedInfo) {
	cs.preparedQc = prepareQc
}

func (cs *ClusterState) Committee() *ICSCommittee {
	return cs.committee
}

// TODO: Check on locks

func NewICS(
	ixs common.Interactions,
	clusterID common.ClusterID,
	operator identifiers.KramaID,
	reqTime time.Time,
	selfID identifiers.KramaID,
	committee *ICSCommittee,
	systemObject *gtypes.SystemObject,
	participants map[identifiers.Identifier]*common.Participant,
	viewInfos common.Views,
	view *View,
	isTSStored bool,
) *ClusterState {
	return &ClusterState{
		ixns:             ixs,
		selfID:           selfID,
		ClusterID:        clusterID,
		operator:         operator,
		operatorIncluded: false,
		dirty:            make(map[common.Hash][]byte),
		ICSRespCount:     0,
		Participants:     participants,
		committee:        committee,
		SystemObject:     systemObject,
		Transition:       gtypes.NewTransition(nil, nil, nil),
		localViewInfo:    viewInfos.Copy(),
		highestViewInfo:  make([]*common.ViewInfo, len(viewInfos)),
		currentView:      view,
		isTSStored:       isTSStored,
	}
}

func (cs *ClusterState) IxnsHash() common.Hash {
	hash, _ := cs.ixns.Hash()

	return hash
}

func (cs *ClusterState) SelfKramaID() identifiers.KramaID {
	return cs.selfID
}

func (cs *ClusterState) ParticipantHeight(id identifiers.Identifier) uint64 {
	ps, ok := cs.Participants[id]
	if ok {
		return ps.Height
	}

	return 0
}

func (cs *ClusterState) ParticipantTSHash(id identifiers.Identifier) common.Hash {
	ps, ok := cs.Participants[id]
	if ok {
		return ps.TSHash()
	}

	return common.NilHash
}

func (cs *ClusterState) CurrentView() *View {
	return cs.currentView
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

func (cs *ClusterState) AppendNodeSet(data *NodeSet) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.committee.AppendNodeSet(common.NilHash, data)
}

func (cs *ClusterState) UpdateNodeSetResponses(ct CounterType, nodeSetPosition int, responses *common.ArrayOfBits) {
	cs.committee.UpdateSetResponses(ct, nodeSetPosition, responses)
}

func (cs *ClusterState) Operator() identifiers.KramaID {
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

func (cs *ClusterState) ExcludeParticipantsFromICS(ids common.IdentifierList) {
	cs.Participants.ExcludeFromICS(ids)

	for _, info := range cs.Participants {
		if info.ExcludedFromICS() {
			cs.committee.IncrementExcludedPSCount(info.ConsensusNodesHash)
		}
	}
}

func (cs *ClusterState) NewHeights() map[identifiers.Identifier]uint64 {
	heights := make(map[identifiers.Identifier]uint64, len(cs.Participants))
	for id, ps := range cs.Participants {
		heights[id] = ps.NewHeight()
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
		rawData, err = polo.Polorize(v)
		if err != nil {
			return nil, err
		}

		m.Msgs = append(m.Msgs, rawData)
	}

	return m, nil
}

func (cs *ClusterState) GetConsensusNodesDelta(
	nodeSetPosition int,
	newPeer identifiers.KramaID,
) (added, replaced identifiers.KramaID) {
	for _, info := range cs.committee.Sets[nodeSetPosition].Infos {
		if newPeer == info.KramaID { // cs.ICS.Nodes[setType].Responses.GetIndex(index)
			return
		}
	}

	if len(cs.committee.Sets[nodeSetPosition].Infos) >= common.ConsensusNodesSize {
		replaced = cs.committee.Sets[nodeSetPosition].Infos[0].KramaID
	}

	return newPeer, replaced
}

func (cs *ClusterState) LocalViewInfo() []*common.ViewInfo {
	return cs.localViewInfo
}

func (cs *ClusterState) HighestViewInfo() common.Views {
	return cs.highestViewInfo
}

func (cs *ClusterState) IsContextQuorum(ct CounterType) bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.IsContextQuorum(ct)
}

func (cs *ClusterState) IsRandomQuorum(ct CounterType) bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.RandomSet().GetRespCount(ct) >= int(cs.committee.RandomQuorumSize())
}

func (cs *ClusterState) HasKramaID(kramaID identifiers.KramaID) (int32, []byte, bool) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.HasKramaID(kramaID)
}

func (cs *ClusterState) IsICSMember(kramaID identifiers.KramaID) bool {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	pos, _ := cs.committee.GetIndex(kramaID)

	return pos >= 0
}

// GetByIndex returns the krama id and bls public key of the validator based on the index
func (cs *ClusterState) GetByIndex(index int32) (identifiers.KramaID, []byte) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	slots, _, kramaID, publicKey := cs.committee.GetKramaID(index)
	if slots == nil {
		return "", nil
	}

	return kramaID, publicKey
}

func (cs *ClusterState) GetICSVoteset(ct CounterType) *common.ArrayOfBits {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.GetVoteset(ct)
}

func (cs *ClusterState) GetRandomNodes() []common.ValidatorIndex {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	return cs.committee.RandomSet().ValidatorIndices()
}

func (cs *ClusterState) GetQuorum() []uint32 {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	if cs.quorum != nil {
		return cs.quorum
	}

	quorum := make([]uint32, cs.committee.Size())

	for i := 0; i < cs.committee.Size()-1; i++ {
		if cs.committee.Sets[i].ExcludedFromICS {
			continue
		}

		quorum[i] = cs.committee.ParticipantQuorum(i)
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
		Time:     uint64(cs.currentView.StartTime().Unix()),
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

	for id, ps := range cs.Participants {
		if !ps.ExcludeFromICS && ps.ContextDelta != nil {
			contextDelta[id] = ps.ContextDelta
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
	ID            identifiers.Identifier
	IsGenesis     bool
	ContextHash   common.Hash
	TesseractHash common.Hash
	Height        uint64
	Mode          string
}

func AccountInfoFromAccMetaInfo(metaInfo *common.AccountMetaInfo, isGenesis bool) *AccountInfo {
	return &AccountInfo{
		AccType:       metaInfo.Type,
		ID:            metaInfo.ID,
		IsGenesis:     isGenesis,
		Height:        metaInfo.Height,
		TesseractHash: metaInfo.TesseractHash,
	}
}

type AccountInfos map[identifiers.Identifier]*AccountInfo

func (a AccountInfos) GetLatestHash(id identifiers.Identifier) common.Hash {
	if v, ok := a[id]; ok {
		return v.TesseractHash
	}

	return common.NilHash
}

func (a AccountInfos) GetHeight(id identifiers.Identifier) uint64 {
	if v, ok := a[id]; ok {
		return v.Height
	}

	return 0
}

func (a AccountInfos) IsGenesis(id identifiers.Identifier) bool {
	return a[id].IsGenesis
}

func (a AccountInfos) Identifiers() []identifiers.Identifier {
	ids := make([]identifiers.Identifier, 0, len(a))

	for id := range a {
		ids = append(ids, id)
	}

	return ids
}

func GenerateClusterID() (common.ClusterID, error) {
	randHash := make([]byte, 32)

	if _, err := rand.Read(randHash); err != nil {
		return "", err
	}

	return common.ClusterID(base58.Encode(randHash)), nil
}

type ICSOperatorInfo struct {
	KramaID  identifiers.KramaID
	Priority uint64
	Attempts uint8
}
