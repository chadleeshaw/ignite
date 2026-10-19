package dhcp

import (
	"net"
	"sort"
	"time"
)

// LeaseBucket is the single summary class a lease belongs to.
type LeaseBucket string

const (
	// BucketReserved is a static reservation. It wins over expiry: a
	// reservation still holds its address after the expiry timestamp, and
	// cleanup does not delete it.
	BucketReserved LeaseBucket = "reserved"
	// BucketExpired is a non-reserved lease whose expiry is in the past.
	BucketExpired LeaseBucket = "expired"
	// BucketActive is a non-reserved lease that has not expired. The address
	// is still held, whatever the provisioning state (failed, offline, and
	// complete included).
	BucketActive LeaseBucket = "active"
)

// LeaseCounts is a partition of leases into the three summary buckets.
// Every counted lease contributes to exactly one field.
type LeaseCounts struct {
	Active   int `json:"active"`
	Expired  int `json:"expired"`
	Reserved int `json:"reserved"`
}

// ProvisionCounts totals leases by provisioning state.
//
// InFlight and Failed are independent of ClassifyLease. A lease still
// belongs to exactly one of Active, Reserved, or Expired, and it can also
// be in flight or failed. A reserved lease in StateImaging counts as both
// reserved and in flight. An expired lease in StateFailed counts as both
// expired and failed.
//
// InFlight counts StatePXERequested, StateBooting, StateImaging,
// StateImaged, and StateConfiguring. Failed counts StateFailed.
// Nil leases are skipped.
type ProvisionCounts struct {
	InFlight int `json:"in_flight"`
	Failed   int `json:"failed"`
}

// PoolStat is one DHCP server's address pool.
//
// Server is the server's interface address (its configured IP). Used counts
// leases on that server that occupy an address (reserved, or not expired).
// Total is the server's LeaseRange. Free is Total-Used, floored at zero
// when the stored leases exceed the configured range.
type PoolStat struct {
	Server string `json:"server"`
	Used   int    `json:"used"`
	Free   int    `json:"free"`
	Total  int    `json:"total"`
	// Status is the listener state from CurrentServerState.
	Status ServerState `json:"status"`
	// InFlight and Failed follow CountProvision. They are independent of
	// Used, so a lease can occupy an address and also be in flight or failed.
	InFlight int `json:"in_flight"`
	Failed   int `json:"failed"`
}

// DashboardSummary is the Overview card payload.
// Active, Expired, and Reserved are a partition from CountLeases.
// InFlight and Failed are separate totals from CountProvision: a lease
// can be counted there and in one of the partition fields.
// Counts are global across every server. Pools is one entry per server,
// ordered by interface address. Pools is never nil.
type DashboardSummary struct {
	Active   int        `json:"active"`
	Expired  int        `json:"expired"`
	Reserved int        `json:"reserved"`
	InFlight int        `json:"in_flight"`
	Failed   int        `json:"failed"`
	Pools    []PoolStat `json:"pools"`
	// Mode is the signed-in user's UI density. Page render fills it.
	// It is not part of the JSON summary.
	Mode string `json:"-"`
}

// PoolOccupancy counts leases that occupy an address.
// Used is reserved or unexpired. Expired non-reserved leases do not count.
// Nil entries are skipped. This is the only occupancy rule; the dashboard
// pool row and the DHCP page both call it.
func PoolOccupancy(leases []*Lease, now time.Time) int {
	used := 0
	for _, lease := range leases {
		if lease != nil && lease.OccupiesAddress(now) {
			used++
		}
	}
	return used
}

// ClassifyLease assigns a lease to exactly one bucket.
//
// Precedence, first match wins:
//  1. reserved, when Reserved is true, even if Expiry is already past
//  2. expired, when the lease is not reserved and now is after Expiry
//  3. active, otherwise
func ClassifyLease(lease *Lease, now time.Time) LeaseBucket {
	if lease == nil {
		return BucketExpired
	}
	if lease.Reserved {
		return BucketReserved
	}
	if now.After(lease.Expiry) {
		return BucketExpired
	}
	return BucketActive
}

// CountLeases partitions leases with ClassifyLease. Nil entries are skipped.
func CountLeases(leases []*Lease, now time.Time) LeaseCounts {
	var counts LeaseCounts
	for _, lease := range leases {
		if lease == nil {
			continue
		}
		switch ClassifyLease(lease, now) {
		case BucketReserved:
			counts.Reserved++
		case BucketExpired:
			counts.Expired++
		default:
			counts.Active++
		}
	}
	return counts
}

// CountProvision totals in-flight and failed leases.
// See ProvisionCounts for which states count, and for how these totals
// sit beside the Active, Reserved, and Expired partition.
func CountProvision(leases []*Lease) ProvisionCounts {
	var counts ProvisionCounts
	for _, lease := range leases {
		if lease == nil {
			continue
		}
		if lease.inProgress() {
			counts.InFlight++
		}
		if lease.State == StateFailed {
			counts.Failed++
		}
	}
	return counts
}

// PoolStatForServer builds the pool row for one server.
// leases should be that server's leases; others are ignored.
func PoolStatForServer(server *Server, leases []*Lease, now time.Time) PoolStat {
	stat := PoolStat{Server: serverLabel(server)}
	if server != nil && server.LeaseRange > 0 {
		stat.Total = server.LeaseRange
	}
	var owned []*Lease
	for _, lease := range leases {
		if lease == nil || server == nil || lease.ServerID != server.ID {
			continue
		}
		owned = append(owned, lease)
	}
	stat.Used = PoolOccupancy(owned, now)
	stat.Free = stat.Total - stat.Used
	if stat.Free < 0 {
		stat.Free = 0
	}
	provision := CountProvision(owned)
	stat.InFlight = provision.InFlight
	stat.Failed = provision.Failed
	if server != nil {
		stat.Status = CurrentServerState(server.ID)
	} else {
		stat.Status = ServerStateConfigured
	}
	return stat
}

// BuildDashboardSummary assembles the Overview payload.
// leasesByServer maps a server ID to that server's leases. Missing entries
// are an empty pool. Counts include only leases that belong to a known server.
func BuildDashboardSummary(servers []*Server, leasesByServer map[string][]*Lease, now time.Time) DashboardSummary {
	summary := DashboardSummary{Pools: make([]PoolStat, 0, len(servers))}
	ordered := append([]*Server(nil), servers...)
	sort.Slice(ordered, func(i, j int) bool {
		return compareServerLabels(ordered[i], ordered[j]) < 0
	})

	var all []*Lease
	for _, server := range ordered {
		if server == nil {
			continue
		}
		leases := leasesByServer[server.ID]
		summary.Pools = append(summary.Pools, PoolStatForServer(server, leases, now))
		all = append(all, leases...)
	}
	counts := CountLeases(all, now)
	summary.Active = counts.Active
	summary.Expired = counts.Expired
	summary.Reserved = counts.Reserved
	provision := CountProvision(all)
	summary.InFlight = provision.InFlight
	summary.Failed = provision.Failed
	return summary
}

func serverLabel(server *Server) string {
	if server == nil || server.IP == nil {
		if server == nil {
			return ""
		}
		return server.ID
	}
	if ip4 := server.IP.To4(); ip4 != nil {
		return ip4.String()
	}
	return server.IP.String()
}

func compareServerLabels(a, b *Server) int {
	if a == nil || a.IP == nil {
		if b == nil || b.IP == nil {
			return 0
		}
		return 1
	}
	if b == nil || b.IP == nil {
		return -1
	}
	return compareIPs(a.IP, b.IP)
}

func compareIPs(a, b net.IP) int {
	a4 := a.To4()
	b4 := b.To4()
	if a4 == nil || b4 == nil {
		return 0
	}
	for i := 0; i < 4; i++ {
		if a4[i] < b4[i] {
			return -1
		}
		if a4[i] > b4[i] {
			return 1
		}
	}
	return 0
}
