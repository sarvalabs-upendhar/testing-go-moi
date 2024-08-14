package ixpool

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common"
	"github.com/stretchr/testify/require"
)

func TestIxHandlerDigestCache(t *testing.T) {
	t.Parallel()

	const size = 20
	cache := newDigestCache(size)
	require.Zero(t, cache.Len())

	// add some unique random
	var ds [size]common.Hash
	for i := 0; i < size; i++ {
		_, err := crand.Read(ds[i][:])
		require.NoError(t, err)

		exist := cache.CheckAndInsert(&ds[i])
		require.False(t, exist)

		exist = cache.has(&ds[i])
		require.True(t, exist)
	}

	require.Equal(t, size, cache.Len())

	// try to re-add, ensure not added
	for i := 0; i < size; i++ {
		exist := cache.CheckAndInsert(&ds[i])
		require.True(t, exist)
	}

	require.Equal(t, size, cache.Len())

	// add some more and ensure capacity switch
	var ds2 [size]common.Hash
	for i := 0; i < size; i++ {
		_, err := crand.Read(ds2[i][:])
		require.NoError(t, err)

		exist := cache.CheckAndInsert(&ds2[i])
		require.False(t, exist)

		exist = cache.has(&ds2[i])
		require.True(t, exist)
	}

	require.Equal(t, 2*size, cache.Len())

	var d common.Hash
	_, err := crand.Read(d[:])
	require.NoError(t, err)

	exist := cache.CheckAndInsert(&d)
	require.False(t, exist)
	exist = cache.has(&d)
	require.True(t, exist)

	require.Equal(t, size+1, cache.Len())

	// ensure hashes from the prev batch are still there
	for i := 0; i < size; i++ {
		exist := cache.has(&ds2[i])
		require.True(t, exist)
	}

	// ensure hashes from the first batch are gone
	for i := 0; i < size; i++ {
		exist := cache.has(&ds[i])
		require.False(t, exist)
	}

	// check deletion works
	for i := 0; i < size; i++ {
		cache.Delete(&ds[i])
		cache.Delete(&ds2[i])
	}

	require.Equal(t, 1, cache.Len())

	cache.Delete(&d)
	require.Equal(t, 0, cache.Len())
}

func (c *ixSaltedCache) check(msg []byte) bool {
	_, found := c.innerCheck(msg)

	return found
}

// TestIxHandlerSaltedCacheBasic is the same as TestIxHandlerDigestCache but for the salted cache
func TestIxHandlerSaltedCacheBasic(t *testing.T) {
	t.Parallel()

	const size = 20
	cache := newSaltedCache(size)
	cache.Start(context.Background(), 0)
	require.Zero(t, cache.Len())

	// add some unique random
	var (
		ds    [size][8]byte
		ks    [size]*common.Hash
		exist bool
	)

	for i := 0; i < size; i++ {
		_, err := crand.Read(ds[i][:])
		require.NoError(t, err)

		ks[i], exist = cache.CheckAndPut(ds[i][:])
		require.False(t, exist)
		require.NotEmpty(t, ks[i])

		exist = cache.check(ds[i][:])
		require.True(t, exist)
	}

	require.Equal(t, size, cache.Len())

	// try to re-add, ensure not added
	for i := 0; i < size; i++ {
		k, exist := cache.CheckAndPut(ds[i][:])
		require.True(t, exist)
		require.Empty(t, k)
	}

	require.Equal(t, size, cache.Len())

	// add some more and ensure capacity switch
	var (
		ds2 [size][8]byte
		ks2 [size]*common.Hash
	)

	for i := 0; i < size; i++ {
		_, err := crand.Read(ds2[i][:])
		require.NoError(t, err)

		ks2[i], exist = cache.CheckAndPut(ds2[i][:])
		require.False(t, exist)
		require.NotEmpty(t, ks2[i])

		exist = cache.check(ds2[i][:])
		require.True(t, exist)
	}

	require.Equal(t, 2*size, cache.Len())

	var d [8]byte
	_, err := crand.Read(d[:])
	require.NoError(t, err)

	k, exist := cache.CheckAndPut(d[:])
	require.False(t, exist)
	require.NotEmpty(t, k)

	exist = cache.check(d[:])
	require.True(t, exist)

	require.Equal(t, size+1, cache.Len())

	// ensure hashes from the prev batch are still there
	for i := 0; i < size; i++ {
		exist := cache.check(ds2[i][:])
		require.True(t, exist)
	}

	// ensure hashes from the first batch are gone
	for i := 0; i < size; i++ {
		exist := cache.check(ds[i][:])
		require.False(t, exist)
	}

	// check deletion works
	for i := 0; i < size; i++ {
		cache.Delete(ks[i])
		cache.Delete(ks2[i])
	}

	require.Equal(t, 1, cache.Len())

	cache.Delete(k)
	require.Equal(t, 0, cache.Len())
}

func TestTxHandlerSaltedCacheScheduled(t *testing.T) {
	t.Parallel()

	const size = 20

	updateInterval := 1000 * time.Microsecond
	cache := newSaltedCache(size)
	cache.Start(context.Background(), updateInterval)
	require.Zero(t, cache.Len())

	// add some unique random
	var ds [size][8]byte
	for i := 0; i < size; i++ {
		_, err := crand.Read(ds[i][:])
		require.NoError(t, err)

		k, exist := cache.CheckAndPut(ds[i][:])
		require.False(t, exist)
		require.NotEmpty(t, k)

		if rand.Int()%2 == 0 {
			time.Sleep(updateInterval / 2)
		}
	}

	require.Less(t, cache.Len(), size)
}

func TestTxHandlerSaltedCacheManual(t *testing.T) {
	t.Parallel()

	const size = 20
	cache := newSaltedCache(2 * size)
	cache.Start(context.Background(), 0)
	require.Zero(t, cache.Len())

	// add some unique random
	var ds [size][8]byte
	for i := 0; i < size; i++ {
		_, err := crand.Read(ds[i][:])
		require.NoError(t, err)

		k, exist := cache.CheckAndPut(ds[i][:])
		require.False(t, exist)
		require.NotEmpty(t, k)

		exist = cache.check(ds[i][:])
		require.True(t, exist)
	}

	require.Equal(t, size, cache.Len())

	// rotate and add more data
	cache.Remix()

	var ds2 [size][8]byte
	for i := 0; i < size; i++ {
		_, err := crand.Read(ds2[i][:])
		require.NoError(t, err)

		k, exist := cache.CheckAndPut(ds2[i][:])
		require.False(t, exist)
		require.NotEmpty(t, k)

		exist = cache.check(ds2[i][:])
		require.True(t, exist)
	}
	require.Equal(t, 2*size, cache.Len())

	// ensure the old data still in
	for i := 0; i < size; i++ {
		exist := cache.check(ds[i][:])
		require.True(t, exist)
	}

	// rotate again, check only new data left
	cache.Remix()

	require.Equal(t, size, cache.Len())

	for i := 0; i < size; i++ {
		exist := cache.check(ds[i][:])
		require.False(t, exist)
		exist = cache.check(ds2[i][:])
		require.True(t, exist)
	}
}

func BenchmarkDigestCaches(b *testing.B) {
	digestCacheMaker := digestCacheMaker{}
	saltedCacheMaker := saltedCacheMaker{}
	benchmarks := []struct {
		maker      cacheMaker
		numThreads int
	}{
		{digestCacheMaker, 1},
		{saltedCacheMaker, 1},
		{digestCacheMaker, 4},
		{saltedCacheMaker, 4},
		{digestCacheMaker, 16},
		{saltedCacheMaker, 16},
		{digestCacheMaker, 128},
		{saltedCacheMaker, 128},
	}

	for _, bench := range benchmarks {
		b.Run(fmt.Sprintf("%T/threads=%d", bench.maker, bench.numThreads), func(b *testing.B) {
			benchmarkDigestCache(b, bench.maker, bench.numThreads)
		})
	}
}
