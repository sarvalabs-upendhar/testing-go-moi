package ixpool

import (
	"math/big"
	"time"

	"github.com/sarvalabs/moichain/types"
)

// GetNonce returns the next nonce from the IxPool if the account is initialized in-memory.
// Otherwise, returns the nonce of the latest state object.
func (i *IxPool) GetNonce(addr types.Address) (uint64, error) {
	if acc := i.accounts.get(addr); acc != nil {
		return acc.getNonce(), nil
	}

	return i.sm.GetNonce(addr, types.NilHash)
}

// GetIxs returns the pending and queued interactions of the given address.
func (i *IxPool) GetIxs(addr types.Address, inclQueued bool) (
	promoted, enqueued []*types.Interaction,
) {
	return i.accounts.getIxs(addr, inclQueued)
}

// GetAllIxs returns the pending and queued interactions of all the accounts.
func (i *IxPool) GetAllIxs(inclQueued bool) (
	allPromoted, allEnqueued map[types.Address][]*types.Interaction,
) {
	return i.accounts.allIxs(inclQueued)
}

// GetAccountWaitTime returns the wait time for an account based on the queried address.
func (i *IxPool) GetAccountWaitTime(addr types.Address) (*big.Int, error) {
	if acc := i.accounts.get(addr); acc != nil {
		return big.NewInt(time.Until(acc.getWaitTime()).Milliseconds()), nil
	}

	return nil, types.ErrAccountNotFound
}

// GetAllAccountsWaitTime returns the wait times for all the accounts that are present in IxPool.
func (i *IxPool) GetAllAccountsWaitTime() map[types.Address]*big.Int {
	waitTime := make(map[types.Address]*big.Int)

	i.accounts.Range(func(key, value interface{}) bool {
		addr, _ := key.(types.Address)
		account := i.accounts.get(addr)

		waitTime[addr] = big.NewInt(time.Until(account.getWaitTime()).Milliseconds())

		return true
	})

	return waitTime
}
