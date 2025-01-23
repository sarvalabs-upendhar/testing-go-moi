package ixpool

import (
	"sort"
	"sync"
	"time"

	"github.com/petar/GoLLRB/llrb"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
)

type accountQueue struct {
	keyID              uint64
	enqueued, promoted *ixQueue
	sequenceIDToIX     *sequenceIDToIXMap
	nextSequenceID     uint64
}

// getSequenceID returns the next expected sequenceID for this account.
func (a *accountQueue) getSequenceID() uint64 {
	return a.nextSequenceID
}

// setSequenceID sets the next expected sequenceID for this account.
func (a *accountQueue) setSequenceID(sequenceID uint64) {
	a.nextSequenceID = sequenceID
}

// "enqueue" tries to add the Interaction to the enqueued queue unless it is a replacement.
// In the case of a replacement, it first attempts to replace in the enqueued queue,
// and if that fails, it replaces it in the promoted queue and returns true.
func (a *accountQueue) enqueue(ix *common.Interaction, replace bool) bool {
	replaceInQueue := func(queue minSequenceIDQueue) bool {
		for i, x := range queue {
			if x.SequenceID() == ix.SequenceID() {
				queue[i] = ix // replace

				return true
			}
		}

		return false
	}

	a.sequenceIDToIX.set(ix)

	if !replace {
		// enqueue ix
		a.enqueued.push(ix)

		return false
	}

	if !replaceInQueue(a.enqueued.queue) { // first, try to replace in enqueued
		// then try to replace in promoted
		return replaceInQueue(a.promoted.queue)
	}

	return false
}

// Promote moves eligible Interactions from enqueued to promoted queue.
//
// Eligible Interactions are all sequential in order of sequenceID
// and the first one has to have sequenceID less (or equal) to the account's
// nextSequenceID.
func (a *accountQueue) promote() (uint64, []*common.Interaction) {
	currentSequenceID := a.getSequenceID()
	if a.enqueued.length() == 0 ||
		a.enqueued.peek().SequenceID() > currentSequenceID {
		// nothing to promote
		return 0, nil
	}

	promoted := uint64(0)
	promotedIxns := make([]*common.Interaction, 0)
	nextSequenceID := a.enqueued.peek().SequenceID()

	for {
		ix := a.enqueued.peek()
		if ix == nil ||
			ix.SequenceID() != nextSequenceID {
			break
		}

		// pop from enqueued
		ix = a.enqueued.pop()

		// push to promoted
		a.promoted.push(ix)
		promotedIxns = append(promotedIxns, ix)

		// update counters
		nextSequenceID += 1
		promoted += 1
	}

	// only update the sequenceID map if the new sequenceID
	// is higher than the one previously stored.
	if nextSequenceID > currentSequenceID {
		a.setSequenceID(nextSequenceID)
	}

	return promoted, promotedIxns
}

// An account is the core structure for processing
// Interactions from a specific account id and key id. The nextSequenceID
// field is what separates the enqueued from promoted:
//
//  1. enqueued - Interactions higher than the nextSequenceID
//  2. promoted - Interactions lower than the nextSequenceID
//
// If an enqueued Interaction matches the nextSequenceID,
// a promoteRequest is signaled for this account
// indicating the account's enqueued Interaction(s)
// are ready to be moved to the promoted queue.
// sequenceID to ix map is helpful in replacement of interaction
type account struct {
	accountQueues []*accountQueue
	requestTime   time.Time
	waitTime      time.Time
	delayCounter  int32
	waitLock      sync.RWMutex // waitLock facilitates safe access to requestTime, waitTime and delayCounter
}

func (a *account) get(keyID uint64) *accountQueue {
	for _, accQueue := range a.accountQueues {
		if accQueue.keyID == keyID {
			return accQueue
		}
	}

	return nil
}

func (a *account) sortAccountQueues() {
	sort.Slice(a.accountQueues, func(i, j int) bool {
		return a.accountQueues[i].keyID < a.accountQueues[j].keyID
	})
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

// resetWaitTimeAndCounter checks the waitTime,counter and resets if conditions are met
func (a *account) resetWaitTimeAndCounter() {
	a.waitLock.Lock()
	defer a.waitLock.Unlock()

	a.delayCounter = 0
	a.waitTime = time.Now()
}

type ID struct {
	id identifiers.Identifier
}

func (a *ID) Less(other llrb.Item) bool {
	return a.id.String() < other.(*ID).id.String() //nolint: forcetypeassert
}

// Each account (value) is bound to one id (key).
type accountsManager struct {
	accounts           map[identifiers.Identifier]*account
	sortedParticipants *llrb.LLRB
}

func newAccountsMap() *accountsManager {
	return &accountsManager{
		accounts:           make(map[identifiers.Identifier]*account),
		sortedParticipants: llrb.New(),
	}
}

// exists checks if an account exists within the map.
func (m *accountsManager) exists(id identifiers.Identifier) bool {
	_, ok := m.accounts[id]

	return ok
}

// getPrimaries returns the interactions sorted based on the waitTime of the account
func (m *accountsManager) getWaitPrimaries() *waitQueue {
	waitQueue := newWaitQueue()

	for _, acc := range m.accounts {
		for _, accQueue := range acc.accountQueues {
			if !time.Now().After(acc.getWaitTime()) {
				break
			}
			// add head of the queue
			if ix := accQueue.promoted.peek(); ix != nil {
				waitIX := &WaitInteractions{acc.getDelayCounter(), ix}
				waitQueue.Push(waitIX)
			}
		}
	}

	return waitQueue
}

// getCostPrimaries returns the interactions sorted based on the waitTime of the account
func (m *accountsManager) getCostPrimaries() *pricedQueue {
	priceQueue := newPricedQueue()

	for _, acc := range m.accounts {
		for _, accQueue := range acc.accountQueues {
			if !time.Now().After(acc.getWaitTime()) {
				break
			}

			// add head of the queue
			if ix := accQueue.promoted.peek(); ix != nil {
				priceQueue.Push(ix)
			}
		}
	}

	return priceQueue
}

// get returns the account associated with the given id.
func (m *accountsManager) getAccount(id identifiers.Identifier) *account {
	acc, ok := m.accounts[id]
	if !ok {
		return nil
	}

	return acc
}

// get returns the account associated with the given id.
func (m *accountsManager) getAccountQueue(id identifiers.Identifier, keyID uint64) *accountQueue {
	acc := m.getAccount(id)
	if acc == nil {
		return nil
	}

	return acc.get(keyID)
}

func (m *accountsManager) getAccountAndAccountQueue(id identifiers.Identifier, keyID uint64) (*account, *accountQueue) {
	acc := m.getAccount(id)
	if acc == nil {
		return nil, nil
	}

	return acc, acc.get(keyID)
}

// promoted returns the number of all promoted interactions.
func (a *account) promoted() (total uint64) { //nolint:unused
	for _, accQueue := range a.accountQueues {
		total += accQueue.promoted.length()
	}

	return total
}

// getIxs returns the promoted and enqueued Interactions of the given id, depending on the flag.
func (m *accountsManager) getIxs(id identifiers.Identifier, includeEnqueued bool) (
	promoted, enqueued []*common.Interaction,
) {
	account := m.getAccount(id)

	if account != nil {
		for _, accQueue := range account.accountQueues {
			if accQueue.promoted.length() != 0 {
				promoted = accQueue.promoted.queue
			}

			if includeEnqueued {
				if accQueue.enqueued.length() != 0 {
					enqueued = accQueue.enqueued.queue
				}
			}
		}
	}

	return promoted, enqueued
}

// allIxs returns all promoted and all enqueued Interactions, depending on the flag.
func (m *accountsManager) allIxs(includeEnqueued bool) (
	allPromoted, allEnqueued map[identifiers.Identifier][]*common.Interaction,
) {
	allPromoted = make(map[identifiers.Identifier][]*common.Interaction)
	allEnqueued = make(map[identifiers.Identifier][]*common.Interaction)

	for id, acc := range m.accounts {
		for _, accQueue := range acc.accountQueues {
			if accQueue.promoted.length() != 0 {
				allPromoted[id] = accQueue.promoted.queue
			}

			if includeEnqueued {
				if accQueue.enqueued.length() != 0 {
					allEnqueued[id] = accQueue.enqueued.queue
				}
			}
		}
	}

	return allPromoted, allEnqueued
}

func (m *accountsManager) addToSortedAccounts(id identifiers.Identifier) {
	m.sortedParticipants.ReplaceOrInsert(&ID{
		id: id,
	})
}

func (m *accountsManager) deleteInSortedAccounts(id identifiers.Identifier) {
	m.sortedParticipants.Delete(&ID{id: id})
}

// sequenceIDToIXMap stores sequenceID to ix key value pairs
type sequenceIDToIXMap struct {
	mapping map[uint64]*common.Interaction
}

func newSequenceIDToIXMap() *sequenceIDToIXMap {
	return &sequenceIDToIXMap{
		mapping: make(map[uint64]*common.Interaction),
	}
}

func (m *sequenceIDToIXMap) get(sequenceID uint64) *common.Interaction {
	return m.mapping[sequenceID]
}

func (m *sequenceIDToIXMap) set(ix *common.Interaction) {
	m.mapping[ix.SequenceID()] = ix
}

func (m *sequenceIDToIXMap) reset() {
	m.mapping = make(map[uint64]*common.Interaction)
}

func (m *sequenceIDToIXMap) remove(ixns ...*common.Interaction) {
	for _, ix := range ixns {
		delete(m.mapping, ix.SequenceID())
	}
}
