package flowcontrol

import "testing"

func TestSemaphoreLimitsConcurrentHolders(t *testing.T) {
	s := NewSemaphore(2)

	release1, ok := s.TryAcquire()
	if !ok {
		t.Fatal("expected first acquire to succeed")
	}
	release2, ok := s.TryAcquire()
	if !ok {
		t.Fatal("expected second acquire to succeed")
	}
	if _, ok := s.TryAcquire(); ok {
		t.Fatal("expected third acquire to fail: semaphore is at capacity")
	}

	release1()
	if _, ok := s.TryAcquire(); !ok {
		t.Fatal("expected acquire to succeed after a release")
	}
	release2()
}
