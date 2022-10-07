package guna

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

type changeEntry interface {
	revert(*StateManager)
	modifiedAddress() *ktypes.Address
	cID() ktypes.Hash
}

type Journal struct {
	entries []changeEntry
	count   int
}

func (j *Journal) GetIDs() []ktypes.Hash {
	cids := make([]ktypes.Hash, 0)
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

type AssetCreation struct {
	addr *ktypes.Address
	id   ktypes.Hash
}

func (a AssetCreation) modifiedAddress() *ktypes.Address {
	return a.addr
}

func (a AssetCreation) revert(sm *StateManager) {
	// cid, err := a.id.getCID()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	sm.db.DeleteEntry(a.id.Bytes()) // nolint
}
func (a AssetCreation) cID() ktypes.Hash {
	return a.id
}

type ContextUpdation struct {
	addr *ktypes.Address
	id   ktypes.Hash
}

func (c ContextUpdation) modifiedAddress() *ktypes.Address {
	return c.addr
}

func (c ContextUpdation) cID() ktypes.Hash {
	return c.id
}
func (c ContextUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(c.id.Bytes()) // nolint
}

type BalanceUpdation struct {
	addr *ktypes.Address
	id   ktypes.Hash
}

func (b BalanceUpdation) modifiedAddress() *ktypes.Address {
	return b.addr
}

func (b BalanceUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(b.id.Bytes()) // nolint
}
func (b BalanceUpdation) cID() ktypes.Hash {
	return b.id
}

type AccountUpdation struct {
	addr *ktypes.Address
	id   ktypes.Hash
}

func (acc AccountUpdation) modifiedAddress() *ktypes.Address {
	return acc.addr
}

func (acc AccountUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(acc.id.Bytes()) // nolint
}

func (acc AccountUpdation) cID() ktypes.Hash {
	return acc.id
}

type StorageUpdation struct {
	addr *ktypes.Address
	id   ktypes.Hash
}

func (s StorageUpdation) modifiedAddress() *ktypes.Address {
	return s.addr
}

func (s StorageUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(s.id.Bytes()) // nolint
}

func (s StorageUpdation) cID() ktypes.Hash {
	return s.id
}
