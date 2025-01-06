package ixpool

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
)

// GetSequenceID returns the next sequenceID from the IxPool if the account is initialized in-memory.
// Otherwise, returns the sequenceID of the latest state object.
func (i *IxPool) GetSequenceID(addr identifiers.Address, keyID uint64) (uint64, error) {
	if acc := i.accounts.getAccountQueue(addr, keyID); acc != nil {
		return acc.getSequenceID(), nil
	}

	return i.sm.GetSequenceID(addr, keyID, common.NilHash)
}

// GetIxs returns the pending and queued interactions of the given address.
func (i *IxPool) GetIxs(addr identifiers.Address, inclQueued bool) (
	promoted, enqueued []*common.Interaction,
) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.accounts.getIxs(addr, inclQueued)
}

// GetAllIxs returns the pending and queued interactions of all the accounts.
func (i *IxPool) GetAllIxs(inclQueued bool) (
	allPromoted, allEnqueued map[identifiers.Address][]*common.Interaction,
) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.accounts.allIxs(inclQueued)
}

// GetAccountWaitTime returns the wait time for an account based on the queried address.
func (i *IxPool) GetAccountWaitTime(addr identifiers.Address) (*big.Int, error) {
	if acc := i.accounts.getAccount(addr); acc != nil {
		return big.NewInt(time.Until(acc.getWaitTime()).Milliseconds()), nil
	}

	return nil, common.ErrAccountNotFound
}

// GetAllAccountsWaitTime returns the wait times for all the accounts that are present in IxPool.
func (i *IxPool) GetAllAccountsWaitTime() map[identifiers.Address]*big.Int {
	waitTime := make(map[identifiers.Address]*big.Int)

	for addr, acc := range i.accounts.accounts {
		waitTime[addr] = big.NewInt(time.Until(acc.getWaitTime()).Milliseconds())
	}

	return waitTime
}
