package ixpool

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
)

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
// nonce to ix map is helpful in replacement of interaction
type account struct {
	enqueued, promoted *accountQueue
	nonceToIX          *nonceToIXMap
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

// "enqueue" tries to add the Interaction to the enqueued queue unless it is a replacement.
// In the case of a replacement, it first attempts to replace in the enqueued queue,
// and if that fails, it replaces it in the promoted queue.
func (a *account) enqueue(ix *common.Interaction, replace bool) {
	// check the counter and reset if required
	if a.getDelayCounter() >= MaxWaitCounter && time.Now().After(a.getWaitTime()) {
		a.resetWaitTimeAndCounter()
	}

	replaceInQueue := func(queue minNonceQueue) bool {
		for i, x := range queue {
			if x.Nonce() == ix.Nonce() {
				queue[i] = ix // replace

				return true
			}
		}

		return false
	}

	a.nonceToIX.set(ix)

	if !replace {
		// enqueue ix
		a.enqueued.push(ix)

		return
	}

	if !replaceInQueue(a.enqueued.queue) { // first, try to replace in enqueued
		// then try to replace in promoted
		replaceInQueue(a.promoted.queue)
	}
}

// Promote moves eligible Interactions from enqueued to promoted queue.
//
// Eligible Interactions are all sequential in order of nonce
// and the first one has to have nonce less (or equal) to the account's
// nextNonce.
func (a *account) promote() (uint64, common.Interactions) {
	currentNonce := a.getNonce()
	if a.enqueued.length() == 0 ||
		a.enqueued.peek().Nonce() > currentNonce {
		// nothing to promote
		return 0, nil
	}

	promoted := uint64(0)
	promotedIxns := make(common.Interactions, 0)
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
		promotedIxns = append(promotedIxns, ix)

		// update counters
		nextNonce += 1
		promoted += 1
	}

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	if nextNonce > currentNonce {
		a.setNonce(nextNonce)
	}

	return promoted, promotedIxns
}

// Thread safe map of all accounts registered by the pool.
// Each account (value) is bound to one address (key).
type accountsMap struct {
	sync.Map
}

// Initializes an account for the given address.
func (m *accountsMap) initOnce(addr identifiers.Address, nonce uint64) *account {
	a, _ := m.LoadOrStore(addr, &account{
		enqueued:    newAccountQueue(),
		promoted:    newAccountQueue(),
		nonceToIX:   newNonceToIXMap(),
		nextNonce:   nonce,
		waitTime:    time.Now(),
		requestTime: time.Now(),
	})

	return a.(*account) //nolint:forcetypeassert
}

// exists checks if an account exists within the map.
func (m *accountsMap) exists(addr identifiers.Address) bool {
	_, ok := m.Load(addr)

	return ok
}

// getPrimaries returns the interactions sorted based on the waitTime of the account
func (m *accountsMap) getWaitPrimaries() *waitQueue {
	waitQueue := newWaitQueue()

	m.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(identifiers.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

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
		addressKey, ok := key.(identifiers.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

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
func (m *accountsMap) get(addr identifiers.Address) *account {
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

// promoted returns the number of all promoted interactions.
func (m *accountsMap) promoted() (total uint64) { //nolint:unused
	m.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(identifiers.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

		total += account.promoted.length()

		return true
	})

	return total
}

// getIxs returns the promoted and enqueued Interactions of the given address, depending on the flag.
func (m *accountsMap) getIxs(addr identifiers.Address, includeEnqueued bool) (
	promoted, enqueued []*common.Interaction,
) {
	account := m.get(addr)

	if account != nil {
		if account.promoted.length() != 0 {
			promoted = account.promoted.queue
		}

		if includeEnqueued {
			if account.enqueued.length() != 0 {
				enqueued = account.enqueued.queue
			}
		}
	}

	return promoted, enqueued
}

// allIxs returns all promoted and all enqueued Interactions, depending on the flag.
func (m *accountsMap) allIxs(includeEnqueued bool) (
	allPromoted, allEnqueued map[identifiers.Address][]*common.Interaction,
) {
	allPromoted = make(map[identifiers.Address][]*common.Interaction)
	allEnqueued = make(map[identifiers.Address][]*common.Interaction)

	m.Range(func(key, value interface{}) bool {
		addr, _ := key.(identifiers.Address)
		account := m.get(addr)

		if account.promoted.length() != 0 {
			allPromoted[addr] = account.promoted.queue
		}

		if includeEnqueued {
			if account.enqueued.length() != 0 {
				allEnqueued[addr] = account.enqueued.queue
			}
		}

		return true
	})

	return allPromoted, allEnqueued
}

// nonceToIXMap stores nonce to ix key value pairs
type nonceToIXMap struct {
	mapping map[uint64]*common.Interaction
}

func newNonceToIXMap() *nonceToIXMap {
	return &nonceToIXMap{
		mapping: make(map[uint64]*common.Interaction),
	}
}

func (m *nonceToIXMap) get(nonce uint64) *common.Interaction {
	return m.mapping[nonce]
}

func (m *nonceToIXMap) set(ix *common.Interaction) {
	m.mapping[ix.Nonce()] = ix
}

func (m *nonceToIXMap) reset() {
	m.mapping = make(map[uint64]*common.Interaction)
}

func (m *nonceToIXMap) remove(ixns ...*common.Interaction) {
	for _, ix := range ixns {
		delete(m.mapping, ix.Nonce())
	}
}
