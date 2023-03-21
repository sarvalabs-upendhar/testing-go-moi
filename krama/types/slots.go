package types

import (
	"sync"

	"github.com/sarvalabs/moichain/types"
)

type ExecutionResponse struct {
	Err  error
	Grid []*types.Tesseract
}

const (
	OperatorSlot SlotType = iota
	ValidatorSlot
)

type SlotType int

type Slot struct {
	// TODO: explore using sync pool for slots
	SlotType                        SlotType
	clusterState                    *ClusterState
	ICSSuccessChan                  chan bool
	OutboundChan, InboundChan       chan *ICSMSG
	BftOutboundChan, BftInboundChan chan ConsensusMessage
	ExecutionResp                   chan ExecutionResponse
	CloseCh                         chan struct{}
}

func NewSlot(slotType SlotType, clusterState *ClusterState) *Slot {
	return &Slot{
		SlotType:        slotType,
		clusterState:    clusterState,
		ICSSuccessChan:  make(chan bool),
		OutboundChan:    make(chan *ICSMSG),
		InboundChan:     make(chan *ICSMSG),
		ExecutionResp:   make(chan ExecutionResponse),
		BftOutboundChan: make(chan ConsensusMessage, 1000),
		BftInboundChan:  make(chan ConsensusMessage, 1000),
		CloseCh:         make(chan struct{}),
	}
}

func (info *Slot) ForwardMsg(msg ConsensusMessage) {
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

func (info *Slot) ForwardInboundMsg(msg *ICSMSG) {
	if info == nil {
		return
	}

	select {
	case <-info.CloseCh:
		return
	case info.InboundChan <- msg:
	}
}

func (info *Slot) ClusterID() types.ClusterID {
	return info.clusterState.ID
}

func (info *Slot) ClusterState() *ClusterState {
	return info.clusterState
}

type Slots struct {
	slots                   map[types.ClusterID]*Slot
	availableOperatorSlots  int
	availableValidatorSlots int
	activeAccounts          map[types.Address]types.ClusterID
	mtx                     sync.RWMutex
}

func NewSlots(operatorSlots, validatorSlots int) *Slots {
	return &Slots{
		slots:                   make(map[types.ClusterID]*Slot),
		availableOperatorSlots:  operatorSlots,
		availableValidatorSlots: validatorSlots,
		activeAccounts:          make(map[types.Address]types.ClusterID, (operatorSlots+validatorSlots)*2),
	}
}

func (s *Slots) areAccountsActive(addrs ...types.Address) bool {
	for _, v := range addrs {
		if !v.IsNil() {
			if _, ok := s.activeAccounts[v]; ok {
				return true
			}
		}
	}

	return false
}

func (s *Slots) AreAccountsActive(addrs ...types.Address) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.areAccountsActive(addrs...)
}

func (s *Slots) AddSlot(slot *Slot) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// if any one of the accounts are active return false
	for addr := range slot.clusterState.AccountInfos {
		if s.areAccountsActive(addr) {
			return false
		}
	}

	if !s.areSlotsAvailable(slot.SlotType) {
		return false
	}

	s.slots[slot.clusterState.ID] = slot
	s.decrementSlots(slot.SlotType)

	for addr := range slot.clusterState.AccountInfos {
		s.activeAccounts[addr] = slot.clusterState.ID
	}

	return true
}

func (s *Slots) GetSlot(id types.ClusterID) *Slot {
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

	return s.availableValidatorSlots > 0
}

func (s *Slots) CleanupSlot(id types.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if slot, ok := s.slots[id]; ok {
		for addr := range slot.clusterState.AccountInfos {
			delete(s.activeAccounts, addr)
		}

		close(slot.CloseCh)
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
