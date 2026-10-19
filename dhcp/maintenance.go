package dhcp

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	// DefaultMaintenanceInterval is how often expired leases are removed and
	// quiet hosts are marked offline.
	DefaultMaintenanceInterval = time.Minute
	// DefaultOfflineThreshold is how long a host can go without a heartbeat
	// or DHCP renewal before it is marked offline.
	DefaultOfflineThreshold = 10 * time.Minute
)

// leaseJanitor is the slice of LeaseService the background loop needs.
type leaseJanitor interface {
	CleanupExpiredLeases(ctx context.Context) error
	MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error
}

// LeaseMaintainer periodically runs CleanupExpiredLeases and MarkOfflineLeases.
// Stop cancels the maintainer's context and waits for the loop to return.
type LeaseMaintainer struct {
	leases           leaseJanitor
	interval         time.Duration
	offlineThreshold time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewLeaseMaintainer builds a maintainer. Non-positive durations fall back
// to DefaultMaintenanceInterval and DefaultOfflineThreshold.
func NewLeaseMaintainer(leases leaseJanitor, interval, offlineThreshold time.Duration) *LeaseMaintainer {
	if interval <= 0 {
		interval = DefaultMaintenanceInterval
	}
	if offlineThreshold <= 0 {
		offlineThreshold = DefaultOfflineThreshold
	}
	return &LeaseMaintainer{
		leases:           leases,
		interval:         interval,
		offlineThreshold: offlineThreshold,
	}
}

// Start begins the background loop. The loop stops when parent is cancelled
// or when Stop is called. A second Start while the loop is running does nothing.
func (m *LeaseMaintainer) Start(parent context.Context) {
	if m == nil || m.leases == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.done = make(chan struct{})
	go m.run(ctx, m.done)
}

// Stop cancels the loop and waits until it has returned.
// Stop without a prior Start is a no-op.
func (m *LeaseMaintainer) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	cancel := m.cancel
	done := m.done
	m.cancel = nil
	m.done = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (m *LeaseMaintainer) run(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick(ctx)
		}
	}
}

func (m *LeaseMaintainer) tick(ctx context.Context) {
	if err := m.leases.CleanupExpiredLeases(ctx); err != nil && ctx.Err() == nil {
		log.Printf("lease cleanup: %v", err)
	}
	if ctx.Err() != nil {
		return
	}
	if err := m.leases.MarkOfflineLeases(ctx, m.offlineThreshold); err != nil && ctx.Err() == nil {
		log.Printf("mark offline leases: %v", err)
	}
}
