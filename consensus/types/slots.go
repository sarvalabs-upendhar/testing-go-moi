package types

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
)

type ExecutionResponse struct {
	Err       error
	Tesseract *common.Tesseract
}

const (
	OperatorSlot SlotType = iota
	ValidatorSlot
)

type SlotType int

type AccConsensusLockInfo struct {
	LockType  common.LockType
	ClusterID common.ClusterID
}

const (
	PrepareStage = iota
	PreparedStage
	ProposalStage
)

type Slot struct {
	// TODO: explore using sync pool for slots
	SlotType                        SlotType
	cs                              *ClusterState
	ps                              map[identifiers.Identifier]common.LockType
	Stage                           atomic.Uint32
	Msgs                            chan ConsensusMessage
	BftOutboundChan, BftInboundChan chan ConsensusMessage
	ExecutionResp                   chan ExecutionResponse
	NewICSChan                      chan common.ClusterID
	BftStopChan                     chan error
	InitTime                        time.Time
}

func NewSlot(slotType SlotType, ps map[identifiers.Identifier]common.LockType) *Slot {
	return &Slot{
		SlotType:        slotType,
		ps:              ps,
		InitTime:        time.Now(),
		Msgs:            make(chan ConsensusMessage, 100),
		ExecutionResp:   make(chan ExecutionResponse),
		NewICSChan:      make(chan common.ClusterID),
		BftOutboundChan: make(chan ConsensusMessage, 1000),
		BftInboundChan:  make(chan ConsensusMessage, 1000),
		BftStopChan:     make(chan error),
	}
}

func (info *Slot) UpdateStage(oldSlot, newSlot uint32) bool {
	return info.Stage.CompareAndSwap(oldSlot, newSlot)
}

func (info *Slot) GetStage() uint32 {
	return info.Stage.Load()
}

func (info *Slot) ForwardMsgToKBFTHandler(msg ConsensusMessage) {
	if info == nil {
		return
	}

	select {
	case info.BftInboundChan <- msg:
	default:
		go func() {
			info.BftInboundChan <- msg
		}()
	}
}

func (info *Slot) ForwardMsgToICSHandler(msg ConsensusMessage) {
	if info == nil {
		return
	}

	select {
	case info.Msgs <- msg:
	default:
	}
}

func (info *Slot) ClusterID() common.ClusterID {
	return info.cs.ClusterID
}

func (info *Slot) ClusterState() *ClusterState {
	return info.cs
}

func (info *Slot) UpdateClusterState(cs *ClusterState) {
	info.cs = cs
}

type LockInfo struct {
	lockType  common.LockType
	clusterID common.ClusterID
}

func (l *LockInfo) String() string {
	return fmt.Sprintf("LockType: %v, ClusterID: %v", l.lockType, l.clusterID)
}

type Slots struct {
	slots                   map[common.ClusterID]*Slot
	availableOperatorSlots  int
	availableValidatorSlots int
	activeAccounts          map[identifiers.Identifier][]*LockInfo
	mtx                     sync.RWMutex
}

func NewSlots(operatorSlots, validatorSlots int) *Slots {
	return &Slots{
		slots:                   make(map[common.ClusterID]*Slot),
		availableOperatorSlots:  operatorSlots,
		availableValidatorSlots: validatorSlots,
		activeAccounts:          make(map[identifiers.Identifier][]*LockInfo, 0),
	}
}

func (s *Slots) areAccountsActive(ids map[identifiers.Identifier]common.LockType) bool {
	for id, requiredLock := range ids {
		activeLock, ok := s.activeAccounts[id]
		if !ok {
			continue
		}

		if activeLock[0].lockType == common.MutateLock {
			return true
		}

		if activeLock[0].lockType == common.ReadLock && (requiredLock == common.MutateLock) {
			return true
		}
	}

	return false
}

func (s *Slots) addActiveAccount(id identifiers.Identifier, lockInfo *LockInfo) {
	_, ok := s.activeAccounts[id]
	if !ok {
		s.activeAccounts[id] = make([]*LockInfo, 0)
	}

	s.activeAccounts[id] = append(s.activeAccounts[id], lockInfo)
}

func (s *Slots) AddActiveAccount(id identifiers.Identifier, lockType common.LockType, clusterID common.ClusterID) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	m := map[identifiers.Identifier]common.LockType{id: lockType}

	if s.areAccountsActive(m) {
		return false
	}

	s.addActiveAccount(id, &LockInfo{lockType: lockType, clusterID: clusterID})

	return true
}

func (s *Slots) ClearActiveAccount(id identifiers.Identifier, clusterID common.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if len(s.activeAccounts[id]) == 0 {
		return
	}

	if len(s.activeAccounts[id]) == 1 {
		delete(s.activeAccounts, id)
	}

	infos := s.activeAccounts[id]

	for i := 0; i < len(infos); i++ {
		if infos[i].clusterID != clusterID {
			continue
		}

		infos[i] = infos[len(infos)-1]
		infos = infos[:len(infos)-1]
	}
}

func (s *Slots) CreateSlotAndLockAccounts(
	clusterID common.ClusterID,
	slotType SlotType,
	locks map[identifiers.Identifier]common.LockType,
) (*Slot, common.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.areAccountsActive(locks) {
		return nil, ""
	}

	if !s.areSlotsAvailable(slotType) {
		return nil, ""
	}

	s.slots[clusterID] = NewSlot(slotType, locks)
	s.decrementSlots(slotType)

	for id, lockType := range locks {
		s.addActiveAccount(id, &LockInfo{lockType: lockType, clusterID: clusterID})
	}

	return s.slots[clusterID], ""
}

func (s *Slots) GetSlot(id common.ClusterID) *Slot {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.slots[id]
}

func (s *Slots) AreSlotsAvailable(slotType SlotType) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.areSlotsAvailable(slotType)
}

func (s *Slots) areSlotsAvailable(slotType SlotType) bool {
	if slotType == OperatorSlot {
		return s.availableOperatorSlots > 0
	}

	return true
}

func (s *Slots) CleanupSlot(id common.ClusterID) {
	s.mtx.Lock()
	defer func() {
		s.mtx.Unlock()
	}()

	if slot, ok := s.slots[id]; ok {
		for id := range slot.ps {
			delete(s.activeAccounts, id)
		}

		close(slot.BftInboundChan)
		delete(s.slots, id)

		s.incrementSlots(slot.SlotType)
	}
}

func (s *Slots) decrementSlots(slotType SlotType) {
	if slotType == OperatorSlot {
		s.availableOperatorSlots--

		return
	}

	s.availableValidatorSlots--
}

func (s *Slots) incrementSlots(slotType SlotType) {
	if slotType == OperatorSlot {
		s.availableOperatorSlots++

		return
	}

	s.availableValidatorSlots++
}

func (s *Slots) AvailableOperatorSlots() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.availableOperatorSlots
}

func (s *Slots) ActiveAccounts() map[identifiers.Identifier][]*LockInfo {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.activeAccounts
}
