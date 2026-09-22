package flowcontrol

import (
	"sync"
	"time"
)

// PerKeyLimiter is a per-key token bucket: each key holds up to Limit tokens, refilling
// continuously at Limit/Window per second, and each Allow call costs one token. It also
// bounds its own memory: expired entries are swept periodically, and if distinct keys still
// exceed MaxKeys afterwards, the oldest entries are evicted regardless of expiry.
type PerKeyLimiter[K comparable] struct {
	Limit  int
	Window time.Duration
	// PruneInterval bounds how long a stale entry can outlive its window before being swept.
	PruneInterval time.Duration
	MaxKeys       int
	// Now defaults to time.Now; overridable for tests.
	Now func() time.Time

	mu        sync.Mutex
	buckets   map[K]bucket
	lastPrune time.Time
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// Allow reports whether key has a token available, spending it if so.
func (l *PerKeyLimiter[K]) Allow(key K) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.buckets == nil {
		l.buckets = make(map[K]bucket)
	}
	now := l.nowLocked()

	if now.Sub(l.lastPrune) >= l.PruneInterval || len(l.buckets) >= l.MaxKeys {
		l.expireLocked(now)
		l.lastPrune = now
	}
	if len(l.buckets) >= l.MaxKeys {
		l.evictOldestLocked(len(l.buckets) - l.MaxKeys + 1)
	}

	b := l.refillLocked(key, now)
	if b.tokens < 1 {
		l.buckets[key] = b
		return false
	}
	b.tokens--
	l.buckets[key] = b
	return true
}

// refillLocked returns key's bucket brought up to date as of now. l.mu must be held.
func (l *PerKeyLimiter[K]) refillLocked(key K, now time.Time) bucket {
	b, ok := l.buckets[key]
	if !ok {
		return bucket{tokens: float64(l.Limit), lastRefill: now}
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	rate := float64(l.Limit) / l.Window.Seconds()
	b.tokens = min(float64(l.Limit), b.tokens+elapsed*rate)
	b.lastRefill = now
	return b
}

func (l *PerKeyLimiter[K]) nowLocked() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// expireLocked removes every bucket that would be full again by now (elapsed >= Window),
// since resuming it then behaves identically to starting fresh. l.mu must be held.
func (l *PerKeyLimiter[K]) expireLocked(now time.Time) {
	for key, b := range l.buckets {
		if now.Sub(b.lastRefill) >= l.Window {
			delete(l.buckets, key)
		}
	}
}

// evictOldestLocked removes the n least-recently-refilled buckets regardless of expiry.
// l.mu must be held.
func (l *PerKeyLimiter[K]) evictOldestLocked(n int) {
	if n <= 0 {
		return
	}
	for n > 0 {
		var oldestKey K
		var oldestRefill time.Time
		found := false
		for key, b := range l.buckets {
			if !found || b.lastRefill.Before(oldestRefill) {
				oldestKey, oldestRefill, found = key, b.lastRefill, true
			}
		}
		if !found {
			return
		}
		delete(l.buckets, oldestKey)
		n--
	}
}

// Len is for tests.
func (l *PerKeyLimiter[K]) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
