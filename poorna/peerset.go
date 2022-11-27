package poorna

import (
	"errors"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/moichain/types"
)

var (
	// Represents an error that occurs when the peer set is closed
	errClosed = errors.New("peer set is closed")
	// Represents an error that occurs when a registered peer is attempted to be registered again
	errAlreadyRegistered = errors.New("peer is already registered")
	// Represents an error that occurs when a peer is not registered
	errNotRegistered = errors.New("peer is not registered")
)

// peerSet is a struct that represents a set of KipPeers.
// It is used to track a set of active participants
type peerSet struct {
	// Represents a mapping of peerids to their KipPeers
	peers map[peer.ID]*KipPeer
	// Represents a synchronization mutex on the set of peers
	// A RWMutex allows multiple goroutines to acquire a read
	// lock but only a single goroutine to acquire a write lock.
	lock sync.RWMutex
	// Represents whether the set of peers is closed
	closed bool
}

// newPeerSet is a constructor function that generates and returns an blank peerSet
func newPeerSet() *peerSet {
	// Create an empty peerSet and return it
	return &peerSet{
		peers:  make(map[peer.ID]*KipPeer),
		lock:   sync.RWMutex{},
		closed: false,
	}
}

// Peer is a method of peerSet that returns a peer from the peerSet for a given peer id
func (ps *peerSet) Peer(id peer.ID) *KipPeer {
	// Read Lock the peerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Retrieve the peer from the working set and return it
	return ps.peers[id]
}

// ContainsPeer is a method of peerSet that checks if peer with the
// given peer id exists in the set of peers
func (ps *peerSet) ContainsPeer(pid peer.ID) bool {
	// Read lock the peerSet and defer the unlock
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Retrieve the ok value for whether the peerid key exists in the mapping and return it
	_, ok := ps.peers[pid]

	return ok
}

// Len is a method of peerSet that returns the current size of the the peerSet
func (ps *peerSet) Len() int {
	// Read Lock the peerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Return the size of the working set of peers
	return len(ps.peers)
}

// Register is a method of peerSet that registers a new peer to the working set.
// Returns an errClosed if the peerSet if closed, or an errAlreadyRegistered
// if the peer is already a part of the working set
func (ps *peerSet) Register(p *KipPeer) error {
	// Return an error if the peerset it closed
	if ps.closed {
		return errClosed
	}

	// Retrieve the string rep of the peerid and return an error if it already exists in the peer set
	peerid := p.networkID
	if ps.ContainsPeer(peerid) {
		return errAlreadyRegistered
	}

	// The lock is acquired just before the peer is added to the working set.
	ps.lock.Lock()
	defer ps.lock.Unlock()

	// Add the peer to the peerSet mapping
	ps.peers[peerid] = p

	return nil
}

// Unregister is a method of peerSet that unregisters a peer by removing it from the working set.
// Returns an errNotRegistered if the peer is not part of the working set.
func (ps *peerSet) Unregister(p *KipPeer) error {
	// Retrieve the string rep of the peerid and return an error if the peer is not part of the peerset
	peerid := p.networkID
	if !ps.ContainsPeer(peerid) {
		return errNotRegistered
	}

	// The lock is acquired just before the peer is removed from the working set.
	ps.lock.Lock()
	defer ps.lock.Unlock()

	// Remove the peer from the working set
	delete(ps.peers, peerid)

	return nil
}

// PeersWithoutIX is a method of peerSet that returns a slice of KipPeers that do not
// contain a given Interaction hash in its set know Interactions
func (ps *peerSet) PeersWithoutIX(hash types.Hash) []*KipPeer {
	// Read Lock the peerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*KipPeer, 0, ps.Len())

	// Collect the peers that do not contain the interaction hash in its known set
	for _, p := range ps.peers {
		if !p.knownIXs.Contains(hash) {
			list = append(list, p)
		}
	}

	// Return the final slice of peers
	return list
}
