package p2p

import (
	"errors"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/go-moi/common"
)

var (
	// Represents an error that occurs when the peer set is closed
	errClosed = errors.New("peer set is closed")
	// Represents an error that occurs when a registered peer is attempted to be registered again
	errAlreadyRegistered = errors.New("peer is already registered")
	// Represents an error that occurs when a peer is not registered
	errNotRegistered = errors.New("peer is not registered")
)

// peerSet is a struct that represents a set of Peers.
// It is used to track a set of active participants
type peerSet struct {
	// Represents a mapping of peerIDs to their Peers
	peers map[peer.ID]*Peer
	// Represents a synchronization mutex on the set of peers
	// A RWMutex allows multiple goroutines to acquire a read
	// lock but only a single goroutine to acquire write lock.
	lock sync.RWMutex
	// Represents whether the set of peers is closed
	closed bool
}

// newPeerSet is a constructor function that generates and returns a blank peerSet
func newPeerSet() *peerSet {
	// Create an empty peerSet and return it
	return &peerSet{
		peers:  make(map[peer.ID]*Peer),
		lock:   sync.RWMutex{},
		closed: false,
	}
}

// Peer is a method of peerSet that returns a peer from the peerSet for a given peer id
func (ps *peerSet) Peer(id peer.ID) *Peer {
	// Read Lock the peerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Retrieve the peer from the working set and return it
	return ps.peers[id]
}

// ContainsPeer is a method of peerSet that checks if peer with the
// given peer id exists in the set of peers
func (ps *peerSet) ContainsPeer(pid peer.ID) bool {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Retrieve the ok value for whether the peerID key exists in the mapping and return it
	_, ok := ps.peers[pid]

	return ok
}

// Len is a method of peerSet that returns the current size of the peerSet
func (ps *peerSet) Len() int {
	// Read Lock the peerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Return the size of the working set of peers
	return len(ps.peers)
}

// Register is a method of peerSet that registers a new peer to the working set.
// Returns an errClosed if the peerSet is closed, or an errAlreadyRegistered
// if the peer is already a part of the working set
func (ps *peerSet) Register(p *Peer) error {
	// Return an error if the peerset it closed
	if ps.closed {
		return errClosed
	}

	if ps.ContainsPeer(p.networkID) {
		return errAlreadyRegistered
	}

	// Add the peer to the peerSet mapping
	ps.addPeer(p)

	return nil
}

func (ps *peerSet) addPeer(p *Peer) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	ps.peers[p.networkID] = p
}

func (ps *peerSet) removePeer(peerID peer.ID) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	delete(ps.peers, peerID)
}

func (ps *peerSet) getPeers() map[peer.ID]*Peer {
	peers := make(map[peer.ID]*Peer)

	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, p := range ps.peers {
		peers[p.networkID] = p
	}

	return peers
}

// Unregister is a method of peerSet that unregisters a peer by removing it from the working set.
// Returns an errNotRegistered if the peer is not part of the working set.
func (ps *peerSet) Unregister(p *Peer) error {
	if ps.closed {
		return nil
	}

	if !ps.ContainsPeer(p.networkID) {
		return errNotRegistered
	}

	ps.removePeer(p.networkID)

	return nil
}

// PeersWithoutIX is a method of peerSet that returns a slice of active Peers that do not
// contain a given Interaction hash in its set know Interactions
func (ps *peerSet) PeersWithoutIX(hash common.Hash) []*Peer {
	// Read Lock the peerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*Peer, 0, ps.Len())

	// Collect the peers that do not contain the interaction hash in its known set
	for _, p := range ps.peers {
		if !p.knownIXs.Contains(hash) {
			list = append(list, p)
		}
	}

	// Return the final slice of peers
	return list
}
