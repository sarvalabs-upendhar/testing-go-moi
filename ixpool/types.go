package ixpool

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/moichain/types"
)

const MaxWaitCounter = 20

var (
	ErrNonceTooLow   = errors.New("nonce too low")
	ErrAlreadyKnown  = errors.New("already known")
	ErrOversizedData = errors.New("over sized data")
)

type promoteRequest struct {
	account map[types.Address]interface{}
}

type enqueueRequest struct {
	ixs types.Interactions
}

// Thread safe map of all accounts registered by the pool.
// Each account (value) is bound to one address (key).
type accountsMap struct {
	sync.Map
	count uint64
}

// Initializes an account for the given address.
func (m *accountsMap) initOnce(addr types.Address, nonce uint64) *account {
	a, _ := m.LoadOrStore(addr, &account{})
	newAccount := a.(*account) //nolint:forcetypeassert
	// run only once
	newAccount.init.Do(func() {
		// create queues
		newAccount.enqueued = newAccountQueue()
		newAccount.promoted = newAccountQueue()

		// set the nonce
		newAccount.setNonce(nonce)

		// set the waitTime to current time
		newAccount.waitTime = time.Now()
		newAccount.requestTime = time.Now()

		// update global count
		atomic.AddUint64(&m.count, 1)
	})

	return newAccount
}

// exists checks if an account exists within the map.
func (m *accountsMap) exists(addr types.Address) bool {
	_, ok := m.Load(addr)

	return ok
}

// getPrimaries returns the interactions sorted based on the waitTime of the account
func (m *accountsMap) getWaitPrimaries() *waitQueue {
	waitQueue := newWaitQueue()

	m.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(types.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		if !time.Now().After(account.getWaitTime()) {
			return true
		}
		// add head of the queue
		if ix := account.promoted.peek(); ix != nil {
			waitIX := &WaitInteractions{account.getDelayCounter(), ix}
			waitQueue.Push(waitIX)
		}

		return true
	})

	return waitQueue
}

// getCostPrimaries returns the interactions sorted based on the waitTime of the account
func (m *accountsMap) getCostPrimaries() *pricedQueue {
	priceQueue := newPricedQueue()

	m.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(types.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		if !time.Now().After(account.getWaitTime()) {
			return true
		}
		// add head of the queue
		if ix := account.promoted.peek(); ix != nil {
			priceQueue.Push(ix)
		}

		return true
	})

	return priceQueue
}

// get returns the account associated with the given address.
func (m *accountsMap) get(addr types.Address) *account {
	a, ok := m.Load(addr)
	if !ok {
		return nil
	}

	account, ok := a.(*account)
	if !ok {
		return nil
	}

	return account
}

// promoted returns the number of all promoted transactions.
func (m *accountsMap) promoted() (total uint64) { //nolint
	m.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(types.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		total += account.promoted.length()

		return true
	})

	return
}

// allIxs returns all promoted and all enqueued Interactions, depending on the flag.
func (m *accountsMap) allIxs(includeEnqueued bool) ( //nolint
	allPromoted, allEnqueued map[types.Address][]*types.Interaction,
) {
	allPromoted = make(map[types.Address][]*types.Interaction)
	allEnqueued = make(map[types.Address][]*types.Interaction)

	m.Range(func(key, value interface{}) bool {
		addr, _ := key.(types.Address)
		account := m.get(addr)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		if account.promoted.length() != 0 {
			allPromoted[addr] = account.promoted.queue
		}

		if includeEnqueued {
			account.enqueued.lock(false)
			defer account.enqueued.unlock()

			if account.enqueued.length() != 0 {
				allEnqueued[addr] = account.enqueued.queue
			}
		}

		return true
	})

	return
}

// remove deletes the account from the accountsMap if the enqueue and promoted queue of the account is empty
func (m *accountsMap) remove(address types.Address) {
	account := m.get(address)

	if account.enqueued.length() == 0 && account.promoted.length() == 0 {
		m.Delete(address)
	}
}

// An account is the core structure for processing
// Interactions from a specific address. The nextNonce
// field is what separetes the enqueued from promoted:
//
//  1. enqueued - Interactions higher than the nextNonce
//  2. promoted - Interactions lower than the nextNonce
//
// If an enqueued Interaction matches the nextNonce,
// a promoteRequest is signaled for this account
// indicating the account's enqueued Interaction(s)
// are ready to be moved to the promoted queue.
type account struct {
	init               sync.Once
	enqueued, promoted *accountQueue
	requestTime        time.Time
	waitTime           time.Time
	delayCounter       int32
	nextNonce          uint64
	waitLock           sync.RWMutex // waitLock facilitates safe access to requestTime, waitTime and delayCounter
}

func (a *account) incrementCounter(baseTime time.Duration) {
	a.waitLock.Lock()
	defer a.waitLock.Unlock()
	a.delayCounter++
	a.waitTime = time.Now().Add(utils.ExponentialTimeout(baseTime, a.delayCounter))
}

// getWaitTime returns the wait time associated with the account
func (a *account) getWaitTime() time.Time {
	a.waitLock.RLock()
	defer a.waitLock.RUnlock()

	return a.waitTime
}

func (a *account) getDelayCounter() int32 {
	a.waitLock.RLock()
	defer a.waitLock.RUnlock()

	return a.delayCounter
}

// getNonce returns the next expected nonce for this account.
func (a *account) getNonce() uint64 {
	return atomic.LoadUint64(&a.nextNonce)
}

// setNonce sets the next expected nonce for this account.
func (a *account) setNonce(nonce uint64) {
	atomic.StoreUint64(&a.nextNonce, nonce)
}

// resetWaitTimeAndCounter checks the waitTime,counter and resets if conditions are met
func (a *account) resetWaitTimeAndCounter() {
	a.waitLock.Lock()
	defer a.waitLock.Unlock()

	a.delayCounter = 0
	a.waitTime = time.Now()
}

// enqueue attempts to push the Interaction onto the enqueued queue.
func (a *account) enqueue(ix *types.Interaction) error {
	a.enqueued.lock(true)
	defer a.enqueued.unlock()

	// only accept low nonce if
	// ix was demoted
	if ix.Nonce() < a.getNonce() {
		return ErrNonceTooLow
	}

	// check the counter and reset if required
	if a.getDelayCounter() >= MaxWaitCounter && time.Now().After(a.getWaitTime()) {
		a.resetWaitTimeAndCounter()
	}

	// enqueue ix
	a.enqueued.push(ix)

	return nil
}

// Promote moves eligible Interactions from enqueued to promoted queue.
//
// Eligible Interactions are all sequential in order of nonce
// and the first one has to have nonce less (or equal) to the account's
// nextNonce.
func (a *account) promote() (uint64, []types.Hash) {
	a.promoted.lock(true)
	a.enqueued.lock(true)

	defer func() {
		a.enqueued.unlock()
		a.promoted.unlock()
	}()

	currentNonce := a.getNonce()
	if a.enqueued.length() == 0 ||
		a.enqueued.peek().Nonce() > currentNonce {
		// nothing to promote
		return 0, nil
	}

	promoted := uint64(0)
	promotedIxnHashes := make([]types.Hash, 0)
	nextNonce := a.enqueued.peek().Nonce()

	for {
		ix := a.enqueued.peek()
		if ix == nil ||
			ix.Nonce() != nextNonce {
			break
		}

		// pop from enqueued
		ix = a.enqueued.pop()

		// push to promoted
		a.promoted.push(ix)
		promotedIxnHashes = append(promotedIxnHashes, ix.Hash())

		// update counters
		nextNonce += 1
		promoted += 1
	}

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	if nextNonce > currentNonce {
		a.setNonce(nextNonce)
	}

	return promoted, promotedIxnHashes
}
