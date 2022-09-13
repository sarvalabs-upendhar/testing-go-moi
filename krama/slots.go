package krama

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/krama/ics"
	"gitlab.com/sarvalabs/moichain/krama/kbft"
	"sync"
)

type slotInfo struct {
	//TODO: explore using sync pool for slots
	bft                     *kbft.KBFT
	clusterState            *ics.ClusterInfo
	icsSuccess              chan bool
	outboundMsg, inboundMsg chan *ktypes.ICSMSG
	executionResp           chan ExecutionResponse
	closeCh                 chan struct{}
}

func (info *slotInfo) forwardMsg(msg *ktypes.ICSMSG) {
	if info == nil {
		return
	}

	select {
	case <-info.closeCh:
		return
	case info.inboundMsg <- msg:
	}
}

type Slots struct {
	slots          map[ktypes.ClusterID]*slotInfo
	availableSlots int
	activeAccounts map[ktypes.Address]ktypes.ClusterID
	mtx            sync.RWMutex
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

func (s *Slots) addSlot(id ktypes.ClusterID, slot *slotInfo) bool {
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

func (s *Slots) getSlot(id ktypes.ClusterID) *slotInfo {
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

func (s *Slots) cleanupSlot(id ktypes.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if slot, ok := s.slots[id]; ok {
		delete(s.activeAccounts, slot.clusterState.Ixs[0].FromAddress())
		delete(s.activeAccounts, slot.clusterState.Ixs[0].ToAddress())
		close(slot.closeCh)
		delete(s.slots, id)
		s.availableSlots++
	}
}
