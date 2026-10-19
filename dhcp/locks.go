package dhcp

import (
	"context"
	"sync"
)

// leaseMu guards leaseLocks. A per-MAC mutex serializes read-modify-write of
// one lease. Pool allocation still uses sharedAllocMu. The order is always
// sharedAllocMu and then lockLease; a lease lock must never be held while
// acquiring sharedAllocMu.
var (
	leaseMu    sync.Mutex
	leaseLocks = map[string]*sync.Mutex{}
)

// lockLease locks the canonical MAC and returns the unlock function.
// The wait honors ctx so a cancelled maintenance pass does not sit on the
// mutex until the holder finishes. The mutex is not re-entrant: callers that
// already hold it must not call lockLease again.
func lockLease(ctx context.Context, mac string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := mac
	if canon, err := CanonicalMAC(mac); err == nil {
		key = canon
	}

	leaseMu.Lock()
	mu := leaseLocks[key]
	if mu == nil {
		mu = &sync.Mutex{}
		leaseLocks[key] = mu
	}
	leaseMu.Unlock()

	acquired := make(chan struct{})
	go func() {
		mu.Lock()
		close(acquired)
	}()
	select {
	case <-ctx.Done():
		go func() {
			<-acquired
			mu.Unlock()
		}()
		return nil, ctx.Err()
	case <-acquired:
		return mu.Unlock, nil
	}
}
