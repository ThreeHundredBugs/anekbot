package flowcontrol

import (
	"testing"
	"time"
)

func newTestLimiter() *PerKeyLimiter[int] {
	return &PerKeyLimiter[int]{
		Limit:         3,
		Window:        time.Minute,
		PruneInterval: 30 * time.Minute,
		MaxKeys:       5,
	}
}

func TestPerKeyLimiterEnforcesPerKeyQuota(t *testing.T) {
	l := newTestLimiter()
	for i := 0; i < l.Limit; i++ {
		if !l.Allow(1) {
			t.Fatalf("call %d: expected Allow to succeed", i)
		}
	}
	if l.Allow(1) {
		t.Fatal("expected Allow to fail once the key's quota is exhausted")
	}
	if !l.Allow(2) {
		t.Fatal("a different key must not be limited by key 1's quota")
	}
}

func TestPerKeyLimiterQuotaResetsAfterWindow(t *testing.T) {
	l := newTestLimiter()
	now := time.Now()
	l.Now = func() time.Time { return now }

	for i := 0; i < l.Limit; i++ {
		l.Allow(1)
	}
	now = now.Add(l.Window)
	if !l.Allow(1) {
		t.Fatal("quota should reset once the window has elapsed")
	}
}

func TestPerKeyLimiterRefillsGraduallyNotAllAtOnce(t *testing.T) {
	l := newTestLimiter()
	now := time.Now()
	l.Now = func() time.Time { return now }

	for i := 0; i < l.Limit; i++ {
		l.Allow(1)
	}

	// Halfway through the window, only ~half the tokens (1 of 3) should be back.
	now = now.Add(l.Window / 2)
	if !l.Allow(1) {
		t.Fatal("expected one token to be available halfway through the window")
	}
	if l.Allow(1) {
		t.Fatal("expected only one token to have refilled halfway through the window")
	}
}

func TestPerKeyLimiterSweepsExpiredEntriesAfterPruneInterval(t *testing.T) {
	l := newTestLimiter()
	now := time.Now()
	l.Now = func() time.Time { return now }

	l.Allow(1)
	if got := l.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}

	now = now.Add(l.PruneInterval)
	l.Allow(2) // triggers the sweep as a side effect
	if got := l.Len(); got != 1 {
		t.Fatalf("Len() after sweep = %d, want 1 (only key 2 should remain)", got)
	}
}

func TestPerKeyLimiterEvictsOldestWhenMaxKeysExceeded(t *testing.T) {
	l := newTestLimiter()
	now := time.Now()
	l.Now = func() time.Time { return now }

	for key := 0; key < l.MaxKeys; key++ {
		if !l.Allow(key) {
			t.Fatalf("key %d: expected Allow to succeed", key)
		}
		now = now.Add(time.Millisecond)
	}
	if got := l.Len(); got != l.MaxKeys {
		t.Fatalf("Len() = %d, want %d", got, l.MaxKeys)
	}

	// One more distinct key should evict the oldest entry (key 0) rather than grow unbounded.
	if !l.Allow(l.MaxKeys) {
		t.Fatal("expected Allow to succeed for the new key")
	}
	if got := l.Len(); got != l.MaxKeys {
		t.Fatalf("Len() after eviction = %d, want %d", got, l.MaxKeys)
	}
	l.mu.Lock()
	_, stillThere := l.buckets[0]
	l.mu.Unlock()
	if stillThere {
		t.Fatal("oldest key (0) should have been evicted")
	}
}
