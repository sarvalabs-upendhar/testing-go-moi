package ttlmap

import (
	"math"
	"testing"
	"time"
)

const itemName = "foo"

func TestNewItemWithExpiration(t *testing.T) {
	ttl := 500 * time.Millisecond
	expiration := time.Now().Add(ttl)

	item := NewItem(itemName, WithExpiration(expiration))
	if item.Value() != itemName {
		t.Fatalf("Invalid value")
	}

	if diff := ttl - item.TTL(); diff > 10*time.Millisecond {
		t.Fatalf("Invalid TTL")
	}

	if item.Expiration() != expiration {
		t.Fatalf("Invalid expiration")
	}

	if item.Expired() {
		t.Fatalf("Expecting not expired")
	}

	<-time.After(time.Duration(float64(ttl) * 0.8))

	if item.Expired() {
		t.Fatalf("Expecting not expired")
	}

	<-time.After(time.Duration(float64(ttl) * 0.4))

	if !item.Expired() {
		t.Fatalf("Expecting expired")
	}

	if !item.Expires() {
		t.Fatalf("Expecting expires")
	}
}

func TestNewItemWithTTL(t *testing.T) {
	ttl := 2 * time.Second
	item := NewItem(itemName, WithTTL(ttl))

	if item.Value() != itemName {
		t.Fatalf("Invalid value")
	}

	expectedExpiration := time.Now().Add(ttl)
	diff := item.Expiration().Sub(expectedExpiration)

	if diff > 10*time.Millisecond {
		t.Fatalf("Invalid expiration")
	}

	if !item.Expires() {
		t.Fatalf("Expecting expires")
	}
}

func TestNewItemWithoutExpiration(t *testing.T) {
	item := NewItem(itemName, time.Time{})

	if item.Value() != itemName {
		t.Fatalf("Invalid value")
	}

	if item.TTL() != time.Duration(math.MaxInt64) {
		t.Fatalf("Not expecting TTL")
	}

	time.Sleep(100 * time.Millisecond)

	if item.TTL() != time.Duration(math.MaxInt64) {
		t.Fatalf("Not expecting TTL")
	}

	if item.Expired() {
		t.Fatalf("Not expecting expired")
	}

	if item.Expires() {
		t.Fatalf("Not expecting expires")
	}

	if !item.Expiration().IsZero() {
		t.Fatalf("Not expecting expiration")
	}
}
