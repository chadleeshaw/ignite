package dhcp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeJanitor struct {
	cleanupHits atomic.Int32
	offlineHits atomic.Int32
	block       chan struct{}
	release     chan struct{}
}

func (f *fakeJanitor) CleanupExpiredLeases(ctx context.Context) error {
	f.cleanupHits.Add(1)
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.release:
			return nil
		}
	}
	return nil
}

func (f *fakeJanitor) MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.offlineHits.Add(1)
	return nil
}

func TestLeaseMaintainerStopBeforeStart(t *testing.T) {
	m := NewLeaseMaintainer(&fakeJanitor{}, time.Hour, time.Hour)
	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop before Start did not return")
	}
}

func TestLeaseMaintainerStopCancelsBlockedTick(t *testing.T) {
	fake := &fakeJanitor{
		block:   make(chan struct{}),
		release: make(chan struct{}),
	}
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	m := NewLeaseMaintainer(fake, 15*time.Millisecond, time.Minute)
	m.Start(parent)

	select {
	case <-fake.block:
	case <-time.After(time.Second):
		t.Fatal("maintenance tick did not start")
	}

	stopped := make(chan struct{})
	go func() {
		m.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return while a tick was blocked on context")
	}

	// The cancelled tick must not mark hosts offline after shutdown.
	time.Sleep(40 * time.Millisecond)
	if fake.offlineHits.Load() != 0 {
		t.Fatalf("offline marking ran after cancellation: %d", fake.offlineHits.Load())
	}
}

func TestLeaseMaintainerStopsTickingAfterStop(t *testing.T) {
	fake := &fakeJanitor{}
	m := NewLeaseMaintainer(fake, 20*time.Millisecond, time.Minute)
	m.Start(context.Background())

	deadline := time.Now().Add(time.Second)
	for fake.cleanupHits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if fake.cleanupHits.Load() == 0 {
		t.Fatal("expected at least one cleanup tick")
	}

	m.Stop()
	after := fake.cleanupHits.Load()
	time.Sleep(60 * time.Millisecond)
	if got := fake.cleanupHits.Load(); got != after {
		t.Fatalf("cleanup ran after Stop: before %d after %d", after, got)
	}
}
