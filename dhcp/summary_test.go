package dhcp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyLeasePrecedence(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name   string
		lease  *Lease
		bucket LeaseBucket
	}{
		{
			name:   "reserved and unexpired",
			lease:  &Lease{Reserved: true, Expiry: future, State: StateAssigned},
			bucket: BucketReserved,
		},
		{
			name:   "reserved wins over a past expiry",
			lease:  &Lease{Reserved: true, Expiry: past, State: StateFailed},
			bucket: BucketReserved,
		},
		{
			name:   "expired non-reserved",
			lease:  &Lease{Reserved: false, Expiry: past, State: StateAssigned},
			bucket: BucketExpired,
		},
		{
			name:   "active holds the address regardless of provisioning state",
			lease:  &Lease{Reserved: false, Expiry: future, State: StateFailed},
			bucket: BucketActive,
		},
		{
			name:   "complete and unexpired is active",
			lease:  &Lease{Reserved: false, Expiry: future, State: StateComplete},
			bucket: BucketActive,
		},
		{
			name:   "expiry equal to now is still active",
			lease:  &Lease{Reserved: false, Expiry: now, State: StateAssigned},
			bucket: BucketActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.bucket, ClassifyLease(tt.lease, now))
		})
	}
}

func TestCountLeasesPartitionsExactlyOnce(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	leases := []*Lease{
		{Reserved: true, Expiry: now.Add(time.Hour)},
		{Reserved: true, Expiry: now.Add(-time.Hour)},
		{Expiry: now.Add(-time.Minute), State: StateAssigned},
		{Expiry: now.Add(time.Hour), State: StateFailed},
		{Expiry: now.Add(time.Hour), State: StateComplete},
		nil,
	}
	counts := CountLeases(leases, now)
	assert.Equal(t, 2, counts.Reserved)
	assert.Equal(t, 1, counts.Expired)
	assert.Equal(t, 2, counts.Active)
	assert.Equal(t, 5, counts.Active+counts.Expired+counts.Reserved)
}

func TestCountProvisionStates(t *testing.T) {
	inFlight := []LeaseState{
		StatePXERequested,
		StateBooting,
		StateImaging,
		StateImaged,
		StateConfiguring,
	}
	var leases []*Lease
	for _, state := range inFlight {
		leases = append(leases, &Lease{State: state})
	}
	leases = append(leases,
		&Lease{State: StateFailed},
		&Lease{State: StateAssigned},
		&Lease{State: StateComplete},
		&Lease{State: StateOffline},
		nil,
	)
	counts := CountProvision(leases)
	assert.Equal(t, len(inFlight), counts.InFlight)
	assert.Equal(t, 1, counts.Failed)
}

func TestProvisionCountsIgnoreOccupancyPrecedence(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	reservedImaging := &Lease{Reserved: true, Expiry: now.Add(-time.Hour), State: StateImaging}
	expiredFailed := &Lease{Expiry: now.Add(-time.Hour), State: StateFailed}
	assert.Equal(t, BucketReserved, ClassifyLease(reservedImaging, now))
	assert.Equal(t, BucketExpired, ClassifyLease(expiredFailed, now))

	counts := CountProvision([]*Lease{reservedImaging, expiredFailed})
	assert.Equal(t, 1, counts.InFlight)
	assert.Equal(t, 1, counts.Failed)

	partition := CountLeases([]*Lease{reservedImaging, expiredFailed}, now)
	assert.Equal(t, 1, partition.Reserved)
	assert.Equal(t, 1, partition.Expired)
	assert.Equal(t, 0, partition.Active)
}

func TestPoolStatForServer(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	server := &Server{
		ID:         "srv-1",
		IP:         net.ParseIP("192.168.1.10"),
		LeaseRange: 10,
	}
	leases := []*Lease{
		{ServerID: "srv-1", Reserved: true, Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.1")},
		{ServerID: "srv-1", Reserved: true, Expiry: now.Add(time.Hour), IP: net.ParseIP("192.168.1.2")},
		{ServerID: "srv-1", Expiry: now.Add(time.Hour), IP: net.ParseIP("192.168.1.3")},
		{ServerID: "srv-1", Expiry: now.Add(time.Hour), IP: net.ParseIP("192.168.1.4")},
		{ServerID: "srv-1", Expiry: now.Add(time.Hour), IP: net.ParseIP("192.168.1.5")},
		{ServerID: "srv-1", Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.6")},
		{ServerID: "other", Expiry: now.Add(time.Hour), IP: net.ParseIP("10.0.0.2")},
	}

	stat := PoolStatForServer(server, leases, now)
	assert.Equal(t, "192.168.1.10", stat.Server)
	assert.Equal(t, 10, stat.Total)
	// Two reservations (one already past expiry) plus three live leases.
	assert.Equal(t, 5, stat.Used)
	assert.Equal(t, 5, stat.Free)
}

func TestBuildDashboardSummaryOrdersPoolsAndCounts(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	low := &Server{ID: "b", IP: net.ParseIP("10.0.0.10"), LeaseRange: 4}
	high := &Server{ID: "a", IP: net.ParseIP("192.168.1.10"), LeaseRange: 8}
	byServer := map[string][]*Lease{
		"a": {
			{ServerID: "a", Reserved: true, Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.20")},
			{ServerID: "a", Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.21")},
		},
		"b": {
			{ServerID: "b", Expiry: now.Add(time.Hour), IP: net.ParseIP("10.0.0.20")},
		},
	}

	summary := BuildDashboardSummary([]*Server{high, low}, byServer, now)
	require.Len(t, summary.Pools, 2)
	assert.Equal(t, "10.0.0.10", summary.Pools[0].Server)
	assert.Equal(t, 1, summary.Pools[0].Used)
	assert.Equal(t, 3, summary.Pools[0].Free)
	assert.Equal(t, 4, summary.Pools[0].Total)
	assert.Equal(t, "192.168.1.10", summary.Pools[1].Server)
	assert.Equal(t, 1, summary.Pools[1].Used)
	assert.Equal(t, 7, summary.Pools[1].Free)

	assert.Equal(t, 1, summary.Active)
	assert.Equal(t, 1, summary.Expired)
	assert.Equal(t, 1, summary.Reserved)
	assert.Equal(t, 0, summary.InFlight)
	assert.Equal(t, 0, summary.Failed)
}

func TestBuildDashboardSummaryCountsInFlightAndFailed(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	server := &Server{ID: "srv", IP: net.ParseIP("10.1.0.1"), LeaseRange: 8}
	owned := []*Lease{
		{ServerID: "srv", Reserved: true, Expiry: now.Add(-time.Hour), State: StateImaging, IP: net.ParseIP("10.1.0.10")},
		{ServerID: "srv", Expiry: now.Add(-time.Hour), State: StateFailed, IP: net.ParseIP("10.1.0.11")},
		{ServerID: "srv", Expiry: now.Add(time.Hour), State: StateBooting, IP: net.ParseIP("10.1.0.12")},
	}
	summary := BuildDashboardSummary([]*Server{server}, map[string][]*Lease{"srv": owned}, now)
	assert.Equal(t, 1, summary.Active)
	assert.Equal(t, 1, summary.Expired)
	assert.Equal(t, 1, summary.Reserved)
	assert.Equal(t, 2, summary.InFlight)
	assert.Equal(t, 1, summary.Failed)
	require.Len(t, summary.Pools, 1)
	assert.Equal(t, 2, summary.Pools[0].InFlight)
	assert.Equal(t, 1, summary.Pools[0].Failed)
	assert.Equal(t, 2, summary.Pools[0].Used)

	withOther := append(append([]*Lease{}, owned...), &Lease{
		ServerID: "other",
		Expiry:   now.Add(time.Hour),
		State:    StateFailed,
		IP:       net.ParseIP("10.9.0.1"),
	})
	stat := PoolStatForServer(server, withOther, now)
	assert.Equal(t, 2, stat.InFlight)
	assert.Equal(t, 1, stat.Failed)
}

func TestPoolFreeFloorsAtZero(t *testing.T) {
	now := time.Now()
	server := &Server{ID: "s", IP: net.ParseIP("192.168.0.1"), LeaseRange: 1}
	leases := []*Lease{
		{ServerID: "s", Expiry: now.Add(time.Hour), IP: net.ParseIP("192.168.0.2")},
		{ServerID: "s", Reserved: true, Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.0.3")},
	}
	stat := PoolStatForServer(server, leases, now)
	assert.Equal(t, 2, stat.Used)
	assert.Equal(t, 0, stat.Free)
	assert.Equal(t, 1, stat.Total)
}

func TestPoolOccupancyIgnoresExpired(t *testing.T) {
	now := time.Now()
	leases := []*Lease{
		{IP: net.ParseIP("10.0.0.1"), Expiry: now.Add(time.Hour)},
		{IP: net.ParseIP("10.0.0.2"), Expiry: now.Add(-time.Hour)},
		{IP: net.ParseIP("10.0.0.3"), Reserved: true, Expiry: now.Add(-time.Hour)},
		nil,
	}
	assert.Equal(t, 2, PoolOccupancy(leases, now))
}

func TestOccupiesAddressReservedPastExpiry(t *testing.T) {
	now := time.Now()
	reserved := &Lease{Reserved: true, Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.9")}
	expired := &Lease{Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.10")}
	assert.True(t, reserved.OccupiesAddress(now))
	assert.False(t, expired.OccupiesAddress(now))
}
