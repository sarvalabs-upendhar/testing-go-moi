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
}
type Slots struct {
	slots          map[ktypes.ClusterID]*slotInfo
	availableSlots int
	activeAccounts map[ktypes.Address]ktypes.ClusterID
	mtx            sync.RWMutex
}

func (s *Slots) areAccountsActive(addrs ...ktypes.Address) bool {
	for _, v := range addrs {
		if _, ok := s.activeAccounts[v]; ok {
			return true
		}
	}

	return false
}
func (s *Slots) addSlot(id ktypes.ClusterID, slot *slotInfo) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	fromAddr := slot.clusterState.Ixs[0].FromAddress()
	toAddr := slot.clusterState.Ixs[0].ToAddress()

	if !s.areAccountsActive(fromAddr, toAddr) && s.availableSlots > 0 {
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
func (s *Slots) areSlotsAvailable() bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.availableSlots > 0
}

func (s *Slots) cleanupSlot(id ktypes.ClusterID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if slot, ok := s.slots[id]; ok {
		delete(s.activeAccounts, slot.clusterState.Ixs[0].FromAddress())
		delete(s.activeAccounts, slot.clusterState.Ixs[0].ToAddress())
		delete(s.slots, id)
		s.availableSlots++
	}
}
