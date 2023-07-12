package state

import (
	"github.com/sarvalabs/go-moi/common"
)

type changeEntry interface {
	revert(*StateManager)
	modifiedAddress() *common.Address
	cID() common.Hash
}

type Journal struct {
	entries []changeEntry
	count   int
}

func (j *Journal) GetIDs() []common.Hash {
	cids := make([]common.Hash, 0)
	for _, v := range j.entries {
		cids = append(cids, v.cID())
	}

	return cids
}

func (j *Journal) append(c changeEntry) {
	j.entries = append(j.entries, c)

	j.count++
}

/*
func (j *journal) revert(sm *StateManager) {
	for i := len(j.entries) - 1; i >= 0; i-- {
		j.entries[i].revert(sm)
	}
}
*/

type ContextUpdation struct {
	addr *common.Address
	id   common.Hash
}

func (c ContextUpdation) modifiedAddress() *common.Address {
	return c.addr
}

func (c ContextUpdation) cID() common.Hash {
	return c.id
}

func (c ContextUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(c.id.Bytes())
}

type RegistryUpdation struct {
	addr *common.Address
	id   common.Hash
}

func (r RegistryUpdation) modifiedAddress() *common.Address {
	return r.addr
}

func (r RegistryUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(r.id.Bytes())
}

func (r RegistryUpdation) cID() common.Hash {
	return r.id
}

type BalanceUpdation struct {
	addr *common.Address
	id   common.Hash
}

func (b BalanceUpdation) modifiedAddress() *common.Address {
	return b.addr
}

func (b BalanceUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(b.id.Bytes())
}

func (b BalanceUpdation) cID() common.Hash {
	return b.id
}

type AccountUpdation struct {
	addr *common.Address
	id   common.Hash
}

func (acc AccountUpdation) modifiedAddress() *common.Address {
	return acc.addr
}

func (acc AccountUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(acc.id.Bytes())
}

func (acc AccountUpdation) cID() common.Hash {
	return acc.id
}

type StorageUpdation struct {
	addr *common.Address
	id   common.Hash
}

func (s StorageUpdation) modifiedAddress() *common.Address {
	return s.addr
}

func (s StorageUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(s.id.Bytes())
}

func (s StorageUpdation) cID() common.Hash {
	return s.id
}
