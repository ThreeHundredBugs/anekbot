package flowcontrol

// Semaphore bounds concurrent holders. Unlike a typical semaphore, TryAcquire never blocks:
// it fails if no slot is free, rejecting excess load instead of queueing it.
type Semaphore struct {
	slots chan struct{}
}

func NewSemaphore(n int) *Semaphore {
	return &Semaphore{slots: make(chan struct{}, n)}
}

// TryAcquire's release func, on success, must be called exactly once to free the slot.
func (s *Semaphore) TryAcquire() (release func(), ok bool) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, true
	default:
		return nil, false
	}
}
