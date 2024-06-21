package types

import (
	"sync"

	"github.com/sarvalabs/go-moi-identifiers"

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

type Slot struct {
	// TODO: explore using sync pool for slots
	SlotType                        SlotType
	cs                              *ClusterState
	ps                              map[identifiers.Address]common.IxParticipant
	ICSSuccessChan                  chan bool
	BftOutboundChan, BftInboundChan chan ConsensusMessage
	ExecutionResp                   chan ExecutionResponse
}

func NewSlot(slotType SlotType, ps map[identifiers.Address]common.IxParticipant) *Slot {
	return &Slot{
		SlotType:        slotType,
		ps:              ps,
		ICSSuccessChan:  make(chan bool),
		ExecutionResp:   make(chan ExecutionResponse),
		BftOutboundChan: make(chan ConsensusMessage, 1000),
		BftInboundChan:  make(chan ConsensusMessage, 1000),
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

func (info *Slot) ClusterID() common.ClusterID {
	return info.cs.ClusterID
}

func (info *Slot) ClusterState() *ClusterState {
	return info.cs
}

func (info *Slot) ICSRequestMsg() *CanonicalICSRequest {
	return info.cs.RequestMsg
}

func (info *Slot) UpdateClusterState(cs *ClusterState) {
	info.cs = cs
}

type Slots struct {
	slots                   map[common.ClusterID]*Slot
	activeIxns              map[common.Hash]common.ClusterID
	availableOperatorSlots  int
	availableValidatorSlots int
	activeAccounts          map[identifiers.Address]common.ClusterID
	mtx                     sync.RWMutex
}

func NewSlots(operatorSlots, validatorSlots int) *Slots {
	return &Slots{
		slots:                   make(map[common.ClusterID]*Slot),
		availableOperatorSlots:  operatorSlots,
		availableValidatorSlots: validatorSlots,
		activeAccounts:          make(map[identifiers.Address]common.ClusterID, (operatorSlots+validatorSlots)*2),
		activeIxns:              make(map[common.Hash]common.ClusterID),
	}
}

func (s *Slots) areAccountsActive(addrs ...identifiers.Address) bool {
	for _, v := range addrs {
		if !v.IsNil() {
			if _, ok := s.activeAccounts[v]; ok {
				return true
			}
		}
	}

	return false
}

func (s *Slots) AddActiveAccount(addr identifiers.Address) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.areAccountsActive(addr) {
		return false
	}

	s.activeAccounts[addr] = ""

	return true
}

func (s *Slots) ClearActiveAccount(addr identifiers.Address) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.activeAccounts, addr)
}

func (s *Slots) CreateSlot(
	clusterID common.ClusterID,
	req Request,
	ps map[identifiers.Address]common.IxParticipant,
) (*Slot, common.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// if any one of the accounts are active return false
	for addr := range ps {
		if s.areAccountsActive(addr) {
			return nil, s.activeIxns[req.Ixs[0].Hash()]
		}
	}

	if !s.areSlotsAvailable(req.SlotType) {
		return nil, ""
	}

	s.slots[clusterID] = NewSlot(req.SlotType, ps)
	s.decrementSlots(req.SlotType)

	for addr := range ps {
		s.activeAccounts[addr] = clusterID
	}

	s.activeIxns[req.Ixs[0].Hash()] = clusterID

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

	return s.availableValidatorSlots > 0
}

func (s *Slots) CleanupSlot(id common.ClusterID) {
	s.mtx.Lock()
	defer func() {
		s.mtx.Unlock()
	}()

	if slot, ok := s.slots[id]; ok {
		for addr := range slot.ps {
			delete(s.activeAccounts, addr)
		}

		close(slot.BftInboundChan)
		delete(s.slots, id)

		for ixHash, clusterID := range s.activeIxns {
			if clusterID == id {
				delete(s.activeIxns, ixHash)
			}
		}

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
