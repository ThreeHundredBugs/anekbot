package flowcontrol

import (
	"sync"
	"time"
)

// PerKeyLimiter enforces a sliding-window request quota per key. It also bounds its own
// memory: expired entries are swept periodically, and if distinct keys still exceed MaxKeys
// afterwards, the oldest entries are evicted regardless of expiry.
type PerKeyLimiter[K comparable] struct {
	Limit  int
	Window time.Duration
	// PruneInterval bounds how long a stale entry can outlive its window before being swept.
	PruneInterval time.Duration
	MaxKeys       int
	// Now defaults to time.Now; overridable for tests.
	Now func() time.Time

	mu        sync.Mutex
	windows   map[K]window
	lastPrune time.Time
}

type window struct {
	start time.Time
	count int
}

// Allow reports whether key is still under its per-window quota, counting this call if so.
func (l *PerKeyLimiter[K]) Allow(key K) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.windows == nil {
		l.windows = make(map[K]window)
	}
	now := l.nowLocked()

	if now.Sub(l.lastPrune) >= l.PruneInterval || len(l.windows) >= l.MaxKeys {
		l.expireLocked(now)
		l.lastPrune = now
	}
	if len(l.windows) >= l.MaxKeys {
		l.evictOldestLocked(len(l.windows) - l.MaxKeys + 1)
	}

	w := l.windows[key]
	if now.Sub(w.start) >= l.Window {
		w = window{start: now}
	}
	if w.count >= l.Limit {
		return false
	}
	w.count++
	l.windows[key] = w
	return true
}

func (l *PerKeyLimiter[K]) nowLocked() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// expireLocked removes every window that ended before now. l.mu must be held.
func (l *PerKeyLimiter[K]) expireLocked(now time.Time) {
	for key, w := range l.windows {
		if now.Sub(w.start) >= l.Window {
			delete(l.windows, key)
		}
	}
}

// evictOldestLocked removes the n oldest-started windows regardless of expiry. l.mu must be held.
func (l *PerKeyLimiter[K]) evictOldestLocked(n int) {
	if n <= 0 {
		return
	}
	for n > 0 {
		var oldestKey K
		var oldestStart time.Time
		found := false
		for key, w := range l.windows {
			if !found || w.start.Before(oldestStart) {
				oldestKey, oldestStart, found = key, w.start, true
			}
		}
		if !found {
			return
		}
		delete(l.windows, oldestKey)
		n--
	}
}

// Len is for tests.
func (l *PerKeyLimiter[K]) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.windows)
}
