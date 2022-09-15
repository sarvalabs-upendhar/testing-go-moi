package types

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"sync"
)

type ExecutionResponse struct {
	Err  error
	Grid []*ktypes.Tesseract
}

type Slot struct {
	//TODO: explore using sync pool for slots
	clusterState                    *ClusterInfo
	ICSSuccessChan                  chan bool
	OutboundChan, InboundChan       chan *ktypes.ICSMSG
	BftOutboundChan, BftInboundChan chan ktypes.ConsensusMessage
	ExecutionResp                   chan ExecutionResponse
	CloseCh                         chan struct{}
}

func NewSlot(clusterState *ClusterInfo) *Slot {
	return &Slot{
		clusterState:    clusterState,
		ICSSuccessChan:  make(chan bool),
		OutboundChan:    make(chan *ktypes.ICSMSG),
		InboundChan:     make(chan *ktypes.ICSMSG),
		ExecutionResp:   make(chan ExecutionResponse),
		BftOutboundChan: make(chan ktypes.ConsensusMessage, 1000),
		BftInboundChan:  make(chan ktypes.ConsensusMessage, 1000),
		CloseCh:         make(chan struct{}),
	}
}

func (info *Slot) ForwardMsg(msg ktypes.ConsensusMessage) {
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

func (info *Slot) ForwardInboundMsg(msg *ktypes.ICSMSG) {
	if info == nil {
		return
	}

	select {
	case <-info.CloseCh:
		return
	case info.InboundChan <- msg:
	}
}

func (info *Slot) ClusterID() ktypes.ClusterID {
	return info.clusterState.ID
}

func (info *Slot) CLusterInfo() *ClusterInfo {
	return info.clusterState
}

type Slots struct {
	slots          map[ktypes.ClusterID]*Slot
	availableSlots int
	activeAccounts map[ktypes.Address]ktypes.ClusterID
	mtx            sync.RWMutex
}

func NewSlots(size int) *Slots {
	return &Slots{
		slots:          make(map[ktypes.ClusterID]*Slot),
		availableSlots: size,
		activeAccounts: make(map[ktypes.Address]ktypes.ClusterID, size*2),
	}
}

func (s *Slots) areAccountsActive(addrs ...ktypes.Address) bool {
	for _, v := range addrs {
		if v != ktypes.NilAddress {
			if _, ok := s.activeAccounts[v]; ok {
				return true
			}
		}
	}

	return false
}

func (s *Slots) AreAccountsActive(addrs ...ktypes.Address) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.areAccountsActive(addrs...)
}

func (s *Slots) AddSlot(id ktypes.ClusterID, slot *Slot) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	fromAddr := slot.clusterState.Ixs[0].FromAddress()
	toAddr := slot.clusterState.Ixs[0].ToAddress()

	if !s.areAccountsActive(fromAddr, toAddr) && s.areSlotsAvailable() {
		s.slots[id] = slot
		s.availableSlots = s.availableSlots - 1
		s.activeAccounts[slot.clusterState.Ixs[0].FromAddress()] = slot.clusterState.ID
		s.activeAccounts[slot.clusterState.Ixs[0].ToAddress()] = slot.clusterState.ID

		return true
	}

	return false
}

func (s *Slots) GetSlot(id ktypes.ClusterID) *Slot {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.slots[id]
}
func (s *Slots) AreSlotsAvailable() bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.areSlotsAvailable()
}

func (s *Slots) areSlotsAvailable() bool {
	return s.availableSlots > 0
}

func (s *Slots) CleanupSlot(id ktypes.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if slot, ok := s.slots[id]; ok {
		delete(s.activeAccounts, slot.clusterState.Ixs[0].FromAddress())
		delete(s.activeAccounts, slot.clusterState.Ixs[0].ToAddress())
		close(slot.CloseCh)
		delete(s.slots, id)
		s.availableSlots++
	}
}
