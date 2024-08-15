package ixpool

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"golang.org/x/crypto/blake2b"
)

var saltedPool = sync.Pool{
	New: func() interface{} {
		// 2 x MaxAvailableAppProgramLen that covers
		// max approve + clear state programs with max args for app create ixn.
		// other transactions are much smaller.
		return make([]byte, ixMaxSize+100)
	},
}

// digestCache is a rotating cache of size N accepting crypto.Digest as a key
// and keeping up to 2*N elements in memory
type digestCache struct {
	cur  map[common.Hash]struct{}
	prev map[common.Hash]struct{}

	maxSize int
	mu      sync.RWMutex
}

func newDigestCache(size int) *digestCache {
	c := &digestCache{
		cur:     map[common.Hash]struct{}{},
		maxSize: size,
	}

	return c
}

// has if digest d is in a cache.
// locking semantic: write lock must be taken
func (c *digestCache) has(d *common.Hash) bool {
	_, found := c.cur[*d]
	if !found {
		_, found = c.prev[*d]
	}

	return found
}

// swap rotates cache pages.
// locking semantic: write lock must be taken
func (c *digestCache) swap() {
	c.prev = c.cur
	c.cur = map[common.Hash]struct{}{}
}

// insert adds digest d into a cache.
// locking semantic: write lock must be taken
func (c *digestCache) insert(d *common.Hash) {
	if len(c.cur) >= c.maxSize {
		c.swap()
	}

	c.cur[*d] = struct{}{}
}

// CheckAndInsert adds digest d into a cache if not found
func (c *digestCache) CheckAndInsert(d *common.Hash) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.has(d) {
		return true
	}

	c.insert(d)

	return false
}

// Len returns size of a cache
func (c *digestCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.cur) + len(c.prev)
}

// Delete from the cache
func (c *digestCache) Delete(d *common.Hash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cur, *d)
	delete(c.prev, *d)
}

// ixSaltedCache is a digest cache with a rotating salt
// uses blake2b hash function
type ixSaltedCache struct {
	digestCache

	curSalt  [4]byte
	prevSalt [4]byte
	ctx      context.Context
	wg       sync.WaitGroup
}

func newSaltedCache(size int) *ixSaltedCache {
	return &ixSaltedCache{
		digestCache: *newDigestCache(size),
	}
}

// moreSalt updates salt value used for hashing
func (c *ixSaltedCache) moreSalt() {
	r := uint32(utils.RandUint64() % math.MaxUint32)
	binary.LittleEndian.PutUint32(c.curSalt[:], r)
}

// innerSwap rotates cache pages and update the salt used.
// locking semantic: write lock must be held
func (c *ixSaltedCache) innerSwap(scheduled bool) {
	c.prevSalt = c.curSalt
	c.prev = c.cur

	if scheduled {
		// updating by timer, the prev size is a good estimation of a current load => preallocate
		c.cur = make(map[common.Hash]struct{}, len(c.prev))
	} else {
		// otherwise start empty
		c.cur = map[common.Hash]struct{}{}
	}

	c.moreSalt()
}

// Remix is a locked version of innerSwap, called on schedule
func (c *ixSaltedCache) Remix() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.innerSwap(true)
}

// salter is a goroutine refreshing the cache by schedule
func (c *ixSaltedCache) salter(refreshInterval time.Duration) {
	ticker := time.NewTicker(refreshInterval)

	defer c.wg.Done()
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.Remix()
		case <-c.ctx.Done():
			return
		}
	}
}

// innerCheck returns true if exists, and the current salted hash if does not.
// locking semantic: write lock must be held
func (c *ixSaltedCache) innerCheck(msg []byte) (*common.Hash, bool) {
	ptr := saltedPool.Get()
	defer saltedPool.Put(ptr)

	buf, _ := ptr.([]byte)
	toBeHashed := append(buf[:0], msg...) //nolint:gocritic

	toBeHashed = append(toBeHashed, c.curSalt[:]...)
	toBeHashed = toBeHashed[:len(msg)+len(c.curSalt)]

	d := common.Hash(blake2b.Sum256(toBeHashed))

	_, found := c.cur[d]
	if found {
		return nil, true
	}

	toBeHashed = append(toBeHashed[:len(msg)], c.prevSalt[:]...)
	toBeHashed = toBeHashed[:len(msg)+len(c.prevSalt)]
	pd := common.Hash(blake2b.Sum256(toBeHashed))

	_, found = c.prev[pd]
	if found {
		return nil, true
	}

	return &d, false
}

// CheckAndPut adds msg into a cache if not found
// returns a hashing key used for insertion if the message not found.
func (c *ixSaltedCache) CheckAndPut(msg []byte) (*common.Hash, bool) {
	c.mu.RLock()
	d, found := c.innerCheck(msg)
	salt := c.curSalt
	c.mu.RUnlock()
	// fast read-only path: assuming most messages are duplicates, hash msg and check cache
	if found {
		return d, found
	}

	// not found: acquire write lock to add this msg hash to cache
	c.mu.Lock()
	defer c.mu.Unlock()
	// salt may have changed between RUnlock() and Lock(), rehash if needed
	if salt != c.curSalt {
		d, found = c.innerCheck(msg)
		if found {
			// already added to cache between RUnlock() and Lock(), return
			return d, found
		}
	} else {
		// Do another check to see if another copy of the transaction won the race to write it to the cache
		// Only check current to save a lookup since swaps are rare and no need to re-hash
		if _, found := c.cur[*d]; found {
			return d, found
		}
	}

	if len(c.cur) >= c.maxSize {
		c.innerSwap(false)

		ptr := saltedPool.Get()

		defer saltedPool.Put(ptr)

		buf, _ := ptr.([]byte)
		toBeHashed := append(buf[:0], msg...) //nolint:gocritic
		toBeHashed = append(toBeHashed, c.curSalt[:]...)
		toBeHashed = toBeHashed[:len(msg)+len(c.curSalt)]

		dn := common.Hash(blake2b.Sum256(toBeHashed))
		d = &dn
	}

	c.cur[*d] = struct{}{}

	return d, false
}

func (c *ixSaltedCache) Start(ctx context.Context, refreshInterval time.Duration) {
	c.ctx = ctx
	if refreshInterval != 0 {
		c.wg.Add(1)
		go c.salter(refreshInterval)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.moreSalt()
}

func (c *ixSaltedCache) WaitForStop() {
	c.wg.Wait()
}
