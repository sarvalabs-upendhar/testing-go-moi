package ttlmap

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

type testItem struct {
	key       string
	item      Item
	timestamp time.Time
}

func TestNewMap(t *testing.T) {
	opts := &Options{}
	m := New(opts)

	if m == nil {
		t.Fatalf("Expecting map")
	}

	m.Drain()
}

func TestNewMapWithoutOptions(t *testing.T) {
	m := New(nil)

	if m == nil {
		t.Fatalf("Expecting map")
	}

	m.Drain()
}

func TestMapGetEmpty(t *testing.T) {
	opts := &Options{}
	m := New(opts)

	defer m.Drain()

	if item, err := m.Get("invalid"); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}
}

func TestMapSetGet(t *testing.T) {
	opts := &Options{}
	m := New(opts)

	defer m.Drain()

	foo := NewItem("hello", WithTTL(1*time.Second))

	if err := m.Set("foo", foo, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("foo"); err != nil || item != foo {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar := NewItem("world", WithTTL(1*time.Second))

	if err := m.Set("bar", bar, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}
}

func TestMapSetNXGet(t *testing.T) {
	nx := &SetOptions{KeyExist: KeyExistNotYet}
	opts := &Options{}
	m := New(opts)

	defer m.Drain()

	foo := NewItem("hello", WithTTL(1*time.Second))

	if err := m.Set("foo", foo, nx); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("foo"); err != nil || item != foo {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar := NewItem("world", WithTTL(1*time.Second))

	if err := m.Set("bar", bar, nx); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar2 := NewItem("world2", WithTTL(1*time.Second))

	if err := m.Set("bar", bar2, nx); !errors.Is(err, ErrExist) {
		t.Fatal(err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}
}

func TestMapSetXXGet(t *testing.T) {
	xx := &SetOptions{KeyExist: KeyExistAlready}
	opts := &Options{}
	m := New(opts)

	defer m.Drain()

	foo := NewItem("hello", WithTTL(1*time.Second))

	if err := m.Set("foo", foo, xx); !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}

	if item, err := m.Get("foo"); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}

	bar := NewItem("world", WithTTL(1*time.Second))

	if err := m.Set("bar", bar, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar2 := NewItem("world2", WithTTL(1*time.Second))

	if err := m.Set("bar", bar2, xx); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar2 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}
}

func TestMapUpdateGet(t *testing.T) {
	opts := &Options{}
	m := New(opts)

	defer m.Drain()

	foo := NewItem("hello", WithTTL(1*time.Second))

	if item, err := m.Update("foo", foo, nil); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}

	if item, err := m.Get("foo"); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}

	bar := NewItem("world", WithTTL(1*time.Second))

	if err := m.Set("bar", bar, nil); err != nil {
		t.Fatal(err)
	}

	bar2 := NewItem("world2", WithTTL(2*time.Second))

	if item, err := m.Update("bar", bar2, nil); err != nil || item != bar2 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar2 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar3 := NewItem("world3", WithTTL(3*time.Second))
	keepval := &UpdateOptions{KeepValue: true}
	bar4 := NewItem("world2", WithExpiration(bar3.Expiration()))

	if item, err := m.Update("bar", bar3, keepval); err != nil || item != bar4 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar4 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar5 := NewItem("world5", WithTTL(4*time.Second))
	keepexp := &UpdateOptions{KeepExpiration: true}
	bar6 := NewItem("world5", WithExpiration(bar4.Expiration()))

	if item, err := m.Update("bar", bar5, keepexp); err != nil || item != bar6 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar6 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}
}

func TestMapSetDeleteGet(t *testing.T) {
	opts := &Options{}
	m := New(opts)

	defer m.Drain()

	foo := NewItem("hello", WithTTL(1*time.Second))
	if err := m.Set("foo", foo, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("foo"); err != nil || item != foo {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if m.Len() != 1 {
		t.Fatalf("Invalid length")
	}

	if item, err := m.Delete("foo"); item != foo || err != nil {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if m.Len() != 0 {
		t.Fatalf("Invalid length")
	}

	if item, err := m.Get("foo"); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}

	if item, err := m.Delete("foo"); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}
}

func TestMapPersistency(t *testing.T) {
	var expired, evicted []*testItem

	opts := &Options{
		OnWillExpire: func(key string, item Item) {
			expired = append(expired, &testItem{key, item, time.Now()})
		},
		OnWillEvict: func(key string, item Item) {
			evicted = append(evicted, &testItem{key, item, time.Now()})
		},
	}
	m := New(opts)

	defer m.Drain()

	foo := NewItem("hello", time.Time{})

	if err := m.Set("foo", foo, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("foo"); err != nil || item != foo {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar := NewItem("bar", WithTTL(500*time.Millisecond))

	if err := m.Set("bar", bar, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("bar"); err != nil || item != bar {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	bar2 := NewItem("bar2", time.Time{})

	if err := m.Set("bar2", bar2, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("bar2"); err != nil || item != bar2 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	foo2 := NewItem("foo2", time.Time{})

	if err := m.Set("foo2", foo2, nil); err != nil {
		t.Fatal(err)
	}

	if item, err := m.Get("foo2"); err != nil || item != foo2 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if item, err := m.Delete("foo2"); item != foo2 || err != nil {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	time.Sleep(1 * time.Second)

	if item, err := m.Get("foo"); err != nil || item != foo {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	if item, err := m.Get("bar"); item != zeroItem || !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}

	if item, err := m.Get("bar2"); err != nil || item != bar2 {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	m.Drain()

	if m.store.pq.Len() != 0 {
		t.Fatalf("Invalid length")
	}

	if len(expired) != 1 {
		t.Fatalf("Invalid length")
	}

	if expired[0].key != "bar" || expired[0].item != bar {
		t.Fatalf("Invalid item")
	}

	if len(evicted) != 3 {
		t.Fatalf("Invalid length")
	}
}

func TestMapWaitExpired(t *testing.T) {
	var expired []*testItem

	opts := &Options{
		OnWillExpire: func(key string, item Item) {
			expired = append(expired, &testItem{key, item, time.Now()})
		},
	}
	m := New(opts)

	defer m.Drain()

	start := time.Now()
	n, min := 100, 500

	testMapSetNIncreasing(t, m, n, min, start)

	time.Sleep(1 * time.Second)

	if m.Len() != 0 {
		t.Fatalf("Invalid length")
	}

	m.Drain()

	if len(expired) != n {
		t.Fatalf("Expecting %d expired items\n", n)
	}

	for i, eitem := range expired {
		diff := eitem.timestamp.Sub(start)
		diff -= time.Duration(i+min) * time.Millisecond
		key := fmt.Sprintf("%d", i)

		if eitem.key != key {
			t.Fatalf("Wrong expiration key")
		}

		value := fmt.Sprintf("value for %s", key)

		if eitem.item.Value() != value {
			t.Fatalf("Wrong expiration value")
		}

		if diff < 0 || diff > 10*time.Millisecond {
			t.Fatalf("Wrong expiration time")
		}
	}
}

func testMapSetNIncreasing(t *testing.T, m *Map, n, min int, start time.Time) {
	t.Helper()

	nx := &SetOptions{KeyExist: KeyExistNotYet}

	for i := 0; i < n; i++ {
		key := fmt.Sprintf("%d", i)
		value := fmt.Sprintf("value for %s", key)
		ttl := time.Duration(i+min) * time.Millisecond
		expiration := start.Add(ttl)
		item := NewItem(value, WithExpiration(expiration))

		if err := m.Set(key, item, nx); err != nil {
			t.Fatal(err)
		}
	}

	if m.Len() != n {
		t.Fatalf("Invalid length")
	}
}

func TestMapDrain(t *testing.T) {
	nx := &SetOptions{KeyExist: KeyExistNotYet}
	opts := &Options{}
	m := New(opts)

	defer m.Drain()
	testMapSetN(t, m, 100, 100*time.Millisecond)
	select {
	case <-m.Draining():
		t.Fatalf("Expecting not draining")
	default:
	}
	m.Drain()
	select {
	case <-m.Draining():
	default:
		t.Fatalf("Expecting draining")
	}

	if m.Len() != 0 {
		t.Fatalf("Invalid length")
	}

	if item, err := m.Get("1"); item != zeroItem || !errors.Is(err, ErrDrained) {
		t.Fatalf("Invalid item=%v err=%v", item, err)
	}

	item := NewItem("value", WithTTL(100*time.Millisecond))

	if err := m.Set("1", item, nil); !errors.Is(err, ErrDrained) {
		t.Fatal(err)
	}

	if err := m.Set("1", item, nx); !errors.Is(err, ErrDrained) {
		t.Fatal(err)
	}

	if item2, err := m.Update("1", item, nil); item2 != zeroItem || !errors.Is(err, ErrDrained) {
		t.Fatalf("Invalid item=%v err=%v", item2, err)
	}

	if item2, err := m.Delete("1"); item2 != zeroItem || !errors.Is(err, ErrDrained) {
		t.Fatalf("Invalid item=%v err=%v", item2, err)
	}
}

func TestMapSetItemReuseEvict(t *testing.T) {
	var evicted []*testItem

	opts := &Options{
		OnWillEvict: func(key string, item Item) {
			evicted = append(evicted, &testItem{key, item, time.Now()})
		},
	}
	m := New(opts)
	value := NewItem("bar", WithTTL(30*time.Minute))

	for i := 0; i < 1000; i++ {
		if err := m.Set(fmt.Sprintf("%d", i), value, nil); err != nil {
			t.Fatal(err)
		}
	}

	if len(evicted) != 0 {
		t.Fatalf("Invalid length")
	}

	m.Drain()

	if len(evicted) != 1000 {
		t.Fatalf("Invalid length")
	}
}

func testMapSetN(t *testing.T, m *Map, n int, d time.Duration) {
	t.Helper()

	nx := &SetOptions{KeyExist: KeyExistNotYet}

	for i := 0; i < n; i++ {
		item := NewItem("value", WithTTL(d))
		if err := m.Set(fmt.Sprintf("%d", i), item, nx); err != nil {
			t.Fatal(err)
		}
	}

	if m.Len() != n {
		t.Fatalf("Invalid length")
	}
}

func TestMapSetSetEvict(t *testing.T) {
	var evicted []*testItem

	opts := &Options{
		OnWillEvict: func(key string, item Item) {
			evicted = append(evicted, &testItem{key, item, time.Now()})
		},
	}
	m := New(opts)

	defer m.Drain()

	item := NewItem("hello", WithTTL(1*time.Second))

	if err := m.Set("foo", item, nil); err != nil {
		t.Fatal(err)
	}

	if len(evicted) != 0 {
		t.Fatalf("Invalid length")
	}

	item = NewItem("world", WithTTL(2*time.Second))

	if err := m.Set("foo", item, nil); err != nil {
		t.Fatal(err)
	}

	if len(evicted) != 1 {
		t.Fatalf("Invalid length")
	}
}

func TestMapExpireAlreadyExpired(t *testing.T) {
	var expired []*testItem

	opts := &Options{
		OnWillExpire: func(key string, item Item) {
			expired = append(expired, &testItem{key, item, time.Now()})
		},
	}
	m := New(opts)

	defer m.Drain()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	expiration := start.Add(-1 * time.Second)
	item := NewItem("bar", WithExpiration(expiration))

	if err := m.Set("foo", item, nil); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if m.Len() != 0 {
		t.Fatalf("Invalid length")
	}

	m.Drain()

	if len(expired) != 1 {
		t.Fatalf("Expecting %d expired items\n", 1)
	}

	eitem := expired[0]
	diff := eitem.timestamp.Sub(start)

	if diff < 0 || diff > 10*time.Millisecond {
		t.Fatalf("Wrong expiration time")
	}
}

func TestMapGetAlreadyExpired(t *testing.T) {
	opts := &Options{}
	m := New(opts)

	defer m.Drain()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	expiration := start.Add(-1 * time.Second)
	done := false

	for i := 0; i < 1000 && !done; i++ {
		item := NewItem("bar", WithExpiration(expiration))

		if err := m.Set("foo", item, nil); err != nil {
			t.Fatal(err)
		}

		if _, err := m.Get("foo"); err == nil {
			done = true

			break
		}

		if m.Len() != 0 {
			t.Fatalf("Invalid length")
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !done {
		t.Fatalf("Expecting get to succeed")
	}
}

func BenchmarkMapGet1(b *testing.B) {
	b.StopTimer()

	m := New(nil)

	if err := m.Set("foo", NewItem("bar", WithTTL(30*time.Minute)), nil); err != nil {
		b.Fatal(err)
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Get("foo"); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	m.Drain()
}

func BenchmarkMapSet1(b *testing.B) {
	b.StopTimer()

	m := New(nil)
	value := NewItem("bar", WithTTL(30*time.Minute))

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Set("foo", value, nil); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	m.Drain()
}

func BenchmarkMapSetNX1(b *testing.B) {
	nx := &SetOptions{KeyExist: KeyExistNotYet}

	b.StopTimer()

	m := New(nil)
	value := NewItem("bar", WithTTL(30*time.Minute))

	if err := m.Set("foo", value, nx); err != nil {
		b.Fatal(err)
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Set("foo", value, nx); !errors.Is(err, ErrExist) {
			b.Fatal("Expecting already exists")
		}
	}
	b.StopTimer()
	m.Drain()
}

func BenchmarkMapSetXX1(b *testing.B) {
	xx := &SetOptions{KeyExist: KeyExistAlready}

	b.StopTimer()

	m := New(nil)
	value := NewItem("bar", WithTTL(30*time.Minute))

	if err := m.Set("foo", value, nil); err != nil {
		b.Fatal(err)
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Set("foo", value, xx); err != nil {
			b.Fatal("Expecting already exists")
		}
	}
	b.StopTimer()
	m.Drain()
}

func BenchmarkMapDelete1(b *testing.B) {
	b.StopTimer()

	m := New(nil)

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Delete("foo"); !errors.Is(err, ErrNotExist) {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	m.Drain()
}

func BenchmarkMapSetDelete1(b *testing.B) {
	b.StopTimer()

	m := New(nil)
	value := NewItem("bar", WithTTL(30*time.Minute))

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Set("foo", value, nil); err != nil {
			b.Fatal(err)
		}

		if _, err := m.Delete("foo"); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	m.Drain()
}

func BenchmarkMapSetDrainN(b *testing.B) {
	b.StopTimer()

	opts := &Options{
		InitialCapacity: b.N,
		OnWillEvict: func(key string, item Item) {
			// do nothing
		},
	}
	m := New(opts)
	value := NewItem("bar", WithTTL(30*time.Minute))

	for i := 0; i < b.N; i++ {
		if err := m.Set(fmt.Sprintf("%d", i), value, nil); err != nil {
			b.Fatal(err)
		}
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		m.Drain()
	}
	b.StopTimer()
}
