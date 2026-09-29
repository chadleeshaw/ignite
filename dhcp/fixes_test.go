package dhcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIncrementIP(t *testing.T) {
	// Normal increment
	ip, err := incrementIP(net.ParseIP("192.168.1.1"), 5)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.6", ip.String())
	assert.Len(t, ip, 4, "result should be normalized to 4 bytes")

	// Accepts 16-byte IPv4-in-IPv6 form
	ip, err = incrementIP(net.ParseIP("192.168.1.1").To16(), 1)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.2", ip.String())

	// Rejects IPv6
	_, err = incrementIP(net.ParseIP("2001:db8::1"), 1)
	assert.Error(t, err)

	// Rejects nil
	_, err = incrementIP(nil, 1)
	assert.Error(t, err)

	// Rejects negative increments
	_, err = incrementIP(net.ParseIP("192.168.1.10"), -1)
	assert.Error(t, err)

	// Rejects overflow past 255.255.255.255 instead of wrapping
	_, err = incrementIP(net.ParseIP("255.255.255.255"), 1)
	assert.Error(t, err)
}

func TestIpToInt(t *testing.T) {
	n, err := ipToInt(net.ParseIP("192.168.1.1"))
	require.NoError(t, err)
	assert.Equal(t, uint32(0xC0A80101), n)

	_, err = ipToInt(net.ParseIP("2001:db8::1"))
	assert.Error(t, err)

	_, err = ipToInt(nil)
	assert.Error(t, err)
}

func TestServer_IsInRange(t *testing.T) {
	server := &Server{
		IPStart:    net.ParseIP("192.168.1.100"),
		LeaseRange: 50,
	}

	// Works with both 16-byte and 4-byte forms of the candidate IP
	assert.True(t, server.IsInRange(net.ParseIP("192.168.1.120")))
	assert.True(t, server.IsInRange(net.ParseIP("192.168.1.120").To4()))
	assert.True(t, server.IsInRange(net.ParseIP("192.168.1.100")))
	assert.False(t, server.IsInRange(net.ParseIP("192.168.1.99")))
	assert.False(t, server.IsInRange(net.ParseIP("192.168.1.150")))
	assert.False(t, server.IsInRange(net.ParseIP("10.0.0.1")))
	assert.False(t, server.IsInRange(nil))
	assert.False(t, server.IsInRange(net.ParseIP("2001:db8::1")))

	// A huge LeaseRange cannot wrap around past 255.255.255.255
	wide := &Server{IPStart: net.ParseIP("255.255.255.250"), LeaseRange: 1 << 30}
	assert.False(t, wide.IsInRange(net.ParseIP("0.0.0.5")))
	assert.True(t, wide.IsInRange(net.ParseIP("255.255.255.255")))
}

func TestServer_GetNetworkAddress(t *testing.T) {
	server := &Server{
		IP:      net.ParseIP("192.168.1.10"),
		Options: DHCPOptions{SubnetMask: net.ParseIP("255.255.255.0")},
	}
	assert.Equal(t, "192.168.1.0", server.GetNetworkAddress().String())

	broken := &Server{IP: net.ParseIP("2001:db8::1"), Options: DHCPOptions{SubnetMask: net.ParseIP("255.255.255.0")}}
	assert.Nil(t, broken.GetNetworkAddress())
}

func TestValidateServerConfig(t *testing.T) {
	service := NewDHCPServerService(nil, nil)
	valid := ServerConfig{
		IP:            net.ParseIP("192.168.1.10"),
		SubnetMask:    net.ParseIP("255.255.255.0"),
		Gateway:       net.ParseIP("192.168.1.1"),
		DNS:           net.ParseIP("8.8.8.8"),
		StartIP:       net.ParseIP("192.168.1.100"),
		LeaseRange:    50,
		LeaseDuration: 2 * time.Hour,
	}
	assert.NoError(t, service.validateServerConfig(valid))

	cases := map[string]func(*ServerConfig){
		"IPv6 server IP":      func(c *ServerConfig) { c.IP = net.ParseIP("2001:db8::1") },
		"non-contiguous mask": func(c *ServerConfig) { c.SubnetMask = net.ParseIP("255.0.255.0") },
		"IPv6 gateway":        func(c *ServerConfig) { c.Gateway = net.ParseIP("2001:db8::1") },
		"start IP outside subnet": func(c *ServerConfig) {
			c.StartIP = net.ParseIP("192.168.2.100")
		},
		"range overflows IPv4": func(c *ServerConfig) {
			c.StartIP = net.ParseIP("255.255.255.250")
			c.LeaseRange = 10
		},
		"non-positive range": func(c *ServerConfig) { c.LeaseRange = 0 },
	}
	for name, mutate := range cases {
		cfg := valid
		mutate(&cfg)
		assert.Error(t, service.validateServerConfig(cfg), name)
	}
}

func TestLease_UpdateState(t *testing.T) {
	lease := &Lease{State: StateAssigned}
	before := time.Now()
	lease.UpdateState(StateBooting, "dhcp")
	after := time.Now()

	assert.Equal(t, StateBooting, lease.State)
	require.Len(t, lease.StateHistory, 1)
	tr := lease.StateHistory[0]
	assert.Equal(t, StateAssigned, tr.FromState)
	assert.Equal(t, StateBooting, tr.ToState)
	assert.Equal(t, "dhcp", tr.Source)

	// One timestamp drives the transition, StateUpdatedAt, and LastSeen.
	assert.False(t, tr.Timestamp.Before(before))
	assert.False(t, tr.Timestamp.After(after))
	assert.Equal(t, tr.Timestamp, lease.StateUpdatedAt)
	assert.Equal(t, tr.Timestamp, lease.LastSeen)

	// Re-setting the same state records no new transition but refreshes LastSeen.
	lease.UpdateState(StateBooting, "dhcp")
	assert.Len(t, lease.StateHistory, 1)
}

func TestLease_IsActive(t *testing.T) {
	assert.True(t, (&Lease{State: StateAssigned}).IsActive())
	assert.True(t, (&Lease{State: StateImaging}).IsActive())
	assert.False(t, (&Lease{State: StateComplete}).IsActive(), "complete is not active")
	assert.False(t, (&Lease{State: StateOffline}).IsActive())
	assert.False(t, (&Lease{State: StateFailed}).IsActive())
}

func TestProtocolHandler_StopBeforeStart(t *testing.T) {
	h := NewProtocolHandler(&Server{}, nil)
	err := h.Stop()
	assert.True(t, errors.Is(err, ErrHandlerNotRunning))
}

func TestProtocolHandler_DoubleStart(t *testing.T) {
	h := NewProtocolHandler(&Server{IP: net.ParseIP("127.0.0.1")}, nil)
	if err := h.Start(); err != nil {
		t.Skipf("cannot bind UDP :67 in this environment: %v", err)
	}
	defer func() { _ = h.Stop() }()

	// A second start must fail without disturbing the original listener.
	err := h.Start()
	assert.Error(t, err)
	assert.NoError(t, h.Stop())
}

func TestDHCPLeaseService_AssignLease_RejectsForeignServerLease(t *testing.T) {
	ctx := context.Background()
	mockServerRepo := &MockServerRepository{}
	mockLeaseRepo := &MockLeaseRepository{}

	service := NewDHCPLeaseService(mockLeaseRepo, mockServerRepo)

	server := &Server{
		ID:            "server-a",
		IP:            net.ParseIP("192.168.1.1"),
		IPStart:       net.ParseIP("192.168.1.100"),
		LeaseRange:    50,
		LeaseDuration: 2 * time.Hour,
	}
	foreignLease := &Lease{
		ID:       "lease-1",
		MAC:      "00:11:22:33:44:55",
		IP:       net.ParseIP("10.0.0.50"),
		ServerID: "server-b",
		Expiry:   time.Now().Add(time.Hour),
		State:    StateAssigned,
	}

	mockServerRepo.On("Get", ctx, "server-a").Return(server, nil)
	mockLeaseRepo.On("GetByMAC", ctx, foreignLease.MAC).Return(foreignLease, nil)

	lease, err := service.AssignLease(ctx, "server-a", foreignLease.MAC, nil)
	assert.Error(t, err, "must not extend a lease owned by another server")
	assert.Nil(t, lease)
	mockLeaseRepo.AssertNotCalled(t, "Save", ctx, mock.Anything)
}
