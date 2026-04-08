package p2p

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	cacheSize       = 1024
	coolDownPeriod  = 2 * time.Minute
	cleanupInterval = 1 * time.Minute
)

type coolDownCache struct {
	mu     sync.Mutex
	timers map[string]time.Time
}

// newCoolDownCache returns a new instance of coolDownCache and initiates the cleanup routine
func newCoolDownCache() *coolDownCache {
	c := &coolDownCache{
		timers: make(map[string]time.Time),
	}

	go c.startCleanup()

	return c
}

// Has checks if a peer is present in the cache.
func (cache *coolDownCache) Has(id peer.ID) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if t, exists := cache.timers[id.String()]; exists && time.Since(t) < coolDownPeriod {
		return true
	}

	// Peer has exceeded the cooldown period or is not in the cache
	return false
}

// Add adds a peer to the cache.
func (cache *coolDownCache) Add(id peer.ID) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.timers[id.String()] = time.Now()

	if len(cache.timers) > cacheSize {
		cache.pruneOldest()
	}
}

// Reset clears the cache.
func (cache *coolDownCache) Reset() {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.timers = make(map[string]time.Time)
}

// startCleanup initiates the periodic cleanup process to remove expired entries from the cache.
func (cache *coolDownCache) startCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		cache.cleanup()
	}
}

// cleanup removes expired entries from the cache.
func (cache *coolDownCache) cleanup() {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	now := time.Now()
	for id, t := range cache.timers {
		if now.Sub(t) >= coolDownPeriod {
			delete(cache.timers, id)
		}
	}
}

// pruneOldest removes the entry with the oldest timestamp from the timers map.
func (cache *coolDownCache) pruneOldest() {
	var (
		oldestTime   time.Time
		oldestPeerID string
	)

	for peerID, t := range cache.timers {
		if oldestTime.IsZero() || t.Before(oldestTime) {
			oldestTime = t
			oldestPeerID = peerID
		}
	}

	delete(cache.timers, oldestPeerID)
}
