package flowcontrol

// Semaphore bounds how many callers may hold a slot concurrently. Unlike a
// typical semaphore, TryAcquire never blocks: it fails immediately if no slot
// is free, which suits rejecting excess load rather than queueing it.
type Semaphore struct {
	slots chan struct{}
}

// NewSemaphore returns a Semaphore allowing up to n concurrent holders.
func NewSemaphore(n int) *Semaphore {
	return &Semaphore{slots: make(chan struct{}, n)}
}

// TryAcquire attempts to take a slot without blocking. On success, it returns a
// release func that must be called exactly once to free the slot.
func (s *Semaphore) TryAcquire() (release func(), ok bool) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, true
	default:
		return nil, false
	}
}
