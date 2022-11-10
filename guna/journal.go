package guna

import (
	"gitlab.com/sarvalabs/moichain/types"
)

type changeEntry interface {
	revert(*StateManager)
	modifiedAddress() *types.Address
	cID() types.Hash
}

type Journal struct {
	entries []changeEntry
	count   int
}

func (j *Journal) GetIDs() []types.Hash {
	cids := make([]types.Hash, 0)
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
	addr *types.Address
	id   types.Hash
}

func (a AssetCreation) modifiedAddress() *types.Address {
	return a.addr
}

func (a AssetCreation) revert(sm *StateManager) {
	// cid, err := a.id.getCID()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	sm.db.DeleteEntry(a.id.Bytes())
}

func (a AssetCreation) cID() types.Hash {
	return a.id
}

type ContextUpdation struct {
	addr *types.Address
	id   types.Hash
}

func (c ContextUpdation) modifiedAddress() *types.Address {
	return c.addr
}

func (c ContextUpdation) cID() types.Hash {
	return c.id
}

func (c ContextUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(c.id.Bytes())
}

type BalanceUpdation struct {
	addr *types.Address
	id   types.Hash
}

func (b BalanceUpdation) modifiedAddress() *types.Address {
	return b.addr
}

func (b BalanceUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(b.id.Bytes())
}

func (b BalanceUpdation) cID() types.Hash {
	return b.id
}

type AccountUpdation struct {
	addr *types.Address
	id   types.Hash
}

func (acc AccountUpdation) modifiedAddress() *types.Address {
	return acc.addr
}

func (acc AccountUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(acc.id.Bytes())
}

func (acc AccountUpdation) cID() types.Hash {
	return acc.id
}

type StorageUpdation struct {
	addr *types.Address
	id   types.Hash
}

func (s StorageUpdation) modifiedAddress() *types.Address {
	return s.addr
}

func (s StorageUpdation) revert(sm *StateManager) {
	sm.db.DeleteEntry(s.id.Bytes())
}

func (s StorageUpdation) cID() types.Hash {
	return s.id
}
