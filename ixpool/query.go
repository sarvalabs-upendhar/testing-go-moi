package ixpool

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common"
)

// GetNonce returns the next nonce from the IxPool if the account is initialized in-memory.
// Otherwise, returns the nonce of the latest state object.
func (i *IxPool) GetNonce(addr common.Address) (uint64, error) {
	if acc := i.accounts.get(addr); acc != nil {
		return acc.getNonce(), nil
	}

	return i.sm.GetNonce(addr, common.NilHash)
}

// GetIxs returns the pending and queued interactions of the given address.
func (i *IxPool) GetIxs(addr common.Address, inclQueued bool) (
	promoted, enqueued []*common.Interaction,
) {
	return i.accounts.getIxs(addr, inclQueued)
}

// GetAllIxs returns the pending and queued interactions of all the accounts.
func (i *IxPool) GetAllIxs(inclQueued bool) (
	allPromoted, allEnqueued map[common.Address][]*common.Interaction,
) {
	return i.accounts.allIxs(inclQueued)
}

// GetAccountWaitTime returns the wait time for an account based on the queried address.
func (i *IxPool) GetAccountWaitTime(addr common.Address) (*big.Int, error) {
	if acc := i.accounts.get(addr); acc != nil {
		return big.NewInt(time.Until(acc.getWaitTime()).Milliseconds()), nil
	}

	return nil, common.ErrAccountNotFound
}

// GetAllAccountsWaitTime returns the wait times for all the accounts that are present in IxPool.
func (i *IxPool) GetAllAccountsWaitTime() map[common.Address]*big.Int {
	waitTime := make(map[common.Address]*big.Int)

	i.accounts.Range(func(key, value interface{}) bool {
		addr, _ := key.(common.Address)
		account := i.accounts.get(addr)

		waitTime[addr] = big.NewInt(time.Until(account.getWaitTime()).Milliseconds())

		return true
	})

	return waitTime
}
