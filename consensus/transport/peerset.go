package transport

import (
	"errors"
	"sync"

	id "github.com/sarvalabs/go-legacy-kramaid"
)

var errAlreadyRegistered = errors.New("peer is already registered")

// icsPeerSet is a struct that represents a set of Peers.
// It is used to track a set of active participants
type icsPeerSet struct {
	// Represents a mapping of peerIDs to their Peers
	peers map[id.KramaID]*icsPeer
	// Represents a synchronization mutex on the set of peers
	// A RWMutex allows multiple goroutines to acquire a read
	// lock but only a single goroutine to acquire write lock.
	lock sync.RWMutex
}

// newICSPeerSet is a constructor function that generates and returns a blank icsPeerSet
func newICSPeerSet() *icsPeerSet {
	// Create an empty icsPeerSet and return it
	return &icsPeerSet{
		peers: make(map[id.KramaID]*icsPeer),
		lock:  sync.RWMutex{},
	}
}

// Peer is a method of icsPeerSet that returns a peer from the icsPeerSet for a given peer id
func (ps *icsPeerSet) Peer(peerID id.KramaID) *icsPeer {
	// Read Lock the icsPeerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Retrieve the peer from the working set and return it
	return ps.peers[peerID]
}

// ContainsPeer is a method of icsPeerSet that checks if peer with the
// given peer id exists in the set of clusters
func (ps *icsPeerSet) ContainsPeer(peerID id.KramaID) bool {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Retrieve the ok value for whether the peerID key exists in the mapping and return it
	_, ok := ps.peers[peerID]

	return ok
}

// Len returns the current size of the icsPeerSet
func (ps *icsPeerSet) Len() int {
	// Read Lock the icsPeerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	// Return the size of the working set of clusters
	return len(ps.peers)
}

// List returns the list of krama id's from the icsPeerSet
func (ps *icsPeerSet) List() []id.KramaID {
	// Read Lock the icsPeerSet
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	peers := make([]id.KramaID, 0, len(ps.peers))

	for kramaID := range ps.peers {
		peers = append(peers, kramaID)
	}

	return peers
}

// Register is a method of icsPeerSet that registers a new peer to the working set.
// Returns an errClosed if the icsPeerSet is closed, or an errAlreadyRegistered
// if the peer is already a part of the working set
func (ps *icsPeerSet) Register(p *icsPeer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if _, ok := ps.peers[p.kramaID]; ok {
		return errAlreadyRegistered
	}

	ps.peers[p.kramaID] = p

	return nil
}

// Unregister is a method of icsPeerSet that unregisters a peer by removing it from the working set.
// Returns an errNotRegistered if the peer is not part of the working set.
func (ps *icsPeerSet) Unregister(kramaID id.KramaID) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	delete(ps.peers, kramaID)
}

func (ps *icsPeerSet) ForEach(fn func(kPeer *icsPeer)) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, peer := range ps.peers {
		fn(peer)
	}
}

/*
// peerList represents a list of peers.
type peerList struct {
	mtx       sync.RWMutex
	peers     map[id.KramaID]struct{}
	updatedAt int64
}

// newPeerList initializes and returns a new peerList instance.
func newPeerList() *peerList {
	return &peerList{
		mtx:       sync.RWMutex{},
		peers:     make(map[id.KramaID]struct{}),
		updatedAt: time.Now().Unix(),
	}
}

// addPeer adds a KramaID to the peerList and updates the updatedAt timestamp.
func (pl *peerList) addPeer(kramaID id.KramaID) {
	pl.mtx.Lock()
	defer pl.mtx.Unlock()

	pl.peers[kramaID] = struct{}{}
	pl.updatedAt = time.Now().Unix()
}

// getPeers returns the peerList.
func (pl *peerList) getPeers() []id.KramaID {
	pl.mtx.RLock()
	defer pl.mtx.RUnlock()
	ls := make([]id.KramaID, 0, len(pl.peers))

	for kramaID := range pl.peers {
		ls = append(ls, kramaID)
	}

	return ls
}

// getUpdatedAt returns the updatedAt timestamp.
func (pl *peerList) getUpdatedAt() int64 {
	pl.mtx.RLock()
	defer pl.mtx.RUnlock()

	return pl.updatedAt
}


// transitPeers represents a collection of transit peer information organized by cluster ID.
type transitPeers struct {
	mtx      sync.RWMutex
	clusters map[common.ClusterID]*peerList
}

// newTransitPeers initializes and returns a new transitPeers instance.
func newTransitPeers() *transitPeers {
	return &transitPeers{
		mtx:      sync.RWMutex{},
		clusters: make(map[common.ClusterID]*peerList),
	}
}

// add associates a KramaID with the given cluster ID.
func (tp *transitPeers) add(clusterID common.ClusterID, kramaID id.KramaID) {
	tp.mtx.Lock()
	defer tp.mtx.Unlock()

	if _, ok := tp.clusters[clusterID]; !ok {
		tp.clusters[clusterID] = newPeerList()
	}

	tp.clusters[clusterID].addPeer(kramaID)
}

// get retrieves the list of KramaIDs associated with the given cluster ID.
func (tp *transitPeers) get(clusterID common.ClusterID) *peerList {
	tp.mtx.RLock()
	defer tp.mtx.RUnlock()

	return tp.clusters[clusterID]
}

// list returns a copy of the transitPeers
func (tp *transitPeers) list() map[common.ClusterID]*peerList {
	tp.mtx.RLock()
	defer tp.mtx.RUnlock()

	clusters := make(map[common.ClusterID]*peerList)

	for clusterID, peers := range tp.clusters {
		clusters[clusterID] = peers
	}

	return clusters
}

// remove disassociates the list of KramaIDs associated with the given cluster ID.
func (tp *transitPeers) remove(clusterID common.ClusterID) {
	tp.mtx.Lock()
	defer tp.mtx.Unlock()

	delete(tp.clusters, clusterID)
}
*/
