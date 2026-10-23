package dhcp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ignite/db"

	"github.com/krolaw/dhcp4"
	"github.com/stretchr/testify/require"
)

func TestMenuNameUsesOSAndVersion(t *testing.T) {
	require.Equal(t, "ubuntu 24.04", MenuName(BootMenu{OS: "ubuntu", Version: "24.04"}))
	require.Equal(t, "ubuntu", MenuName(BootMenu{OS: "ubuntu"}))
	require.Equal(t, "", MenuName(BootMenu{}))
	require.Equal(t, "", MenuName(BootMenu{
		Filename: "/tmp/ignite-tftp.UPoTst/pxelinux.cfg/01-00-11-22-33-44-55",
	}))
	require.Equal(t, "", MenuName(BootMenu{OS: "/tmp/ignite-tftp.UPoTst/menu"}))
	require.Equal(t, "custom", MenuName(BootMenu{TemplateName: "custom"}))
}

func TestMenuNameFromPXE(t *testing.T) {
	body := "default ubuntu\nLABEL ubuntu\n  MENU LABEL Install ubuntu\n  KERNEL ubuntu/24.04/vmlinuz\n"
	require.Equal(t, "ubuntu 24.04", MenuNameFromPXE(body))
	require.Equal(t, "debian", MenuNameFromPXE("LABEL debian\n"))
	require.Equal(t, "", MenuNameFromPXE("KERNEL /tmp/ignite-tftp.UPoTst/vmlinuz\n"))
}

func TestMenuNameForLeaseReadsCurrentRootOnly(t *testing.T) {
	root := t.TempDir()
	mac := "00:11:22:33:44:66"
	dir := filepath.Join(root, "pxelinux.cfg")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := "LABEL ubuntu\n  KERNEL ubuntu/24.04/vmlinuz\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "01-00-11-22-33-44-66"), []byte(body), 0o644))

	stale := filepath.Join(t.TempDir(), "pxelinux.cfg", "01-00-11-22-33-44-66")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte("LABEL ignored\n"), 0o644))

	name := MenuNameForLease(BootMenu{Filename: stale}, root, mac, stale)
	require.Equal(t, "ubuntu 24.04", name)

	outside := MenuNameForLease(BootMenu{}, root, "aa:bb:cc:dd:ee:ff", stale)
	require.Equal(t, "", outside)
}

func TestRequestForNewIPKeepsMenuThroughImaging(t *testing.T) {
	mac, err := net.ParseMAC("00:11:22:33:44:55")
	require.NoError(t, err)
	ctx := context.Background()
	repo := newMemLeases()
	server := &Server{
		ID:            "srv",
		IP:            net.ParseIP("10.0.1.1"),
		IPStart:       net.ParseIP("10.0.1.100"),
		LeaseRange:    20,
		LeaseDuration: time.Hour,
	}
	old := &Lease{
		ID:       "old",
		MAC:      mac.String(),
		IP:       net.ParseIP("10.0.1.50"),
		ServerID: server.ID,
		Expiry:   time.Now().Add(time.Hour),
		State:    StateAssigned,
		Menu: BootMenu{
			Filename: "/tmp/ignite-tftp.UPoTst/pxelinux.cfg/01-00-11-22-33-44-55",
			OS:       "ubuntu",
			Version:  "24.04",
		},
		ProvisionToken: "tok",
	}
	require.NoError(t, repo.Save(ctx, old))

	handler := NewProtocolHandler(server, repo)
	pkt := dhcp4.RequestPacket(dhcp4.Request, mac, old.IP, []byte{1, 2, 3, 4}, false, []dhcp4.Option{
		{Code: dhcp4.OptionRequestedIPAddress, Value: net.ParseIP("10.0.1.100").To4()},
	})
	reply := handler.ServeDHCP(pkt, dhcp4.Request, pkt.ParseOptions())
	require.NotNil(t, reply)

	got, err := repo.GetByMAC(ctx, mac.String())
	require.NoError(t, err)
	require.Equal(t, "10.0.1.100", got.IP.String())
	require.Equal(t, "ubuntu", got.Menu.OS)
	require.Equal(t, "24.04", got.Menu.Version)
	require.Equal(t, "tok", got.ProvisionToken)

	svc := NewDHCPLeaseService(repo, &memServerRepo{servers: map[string]*Server{server.ID: server}})
	require.NoError(t, svc.UpdateLeaseState(ctx, mac.String(), string(StateImaging), "kickstart"))
	got, err = repo.GetByMAC(ctx, mac.String())
	require.NoError(t, err)
	require.Equal(t, StateImaging, got.State)
	require.Equal(t, "ubuntu 24.04", MenuName(got.Menu))
	require.NotContains(t, MenuName(got.Menu), "ignite-tftp")
}

type memLeases struct {
	mu    sync.Mutex
	byID  map[string]*Lease
	byMAC map[string]string
}

func newMemLeases() *memLeases {
	return &memLeases{byID: map[string]*Lease{}, byMAC: map[string]string{}}
}

func (m *memLeases) Save(_ context.Context, lease *Lease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *lease
	m.byID[lease.ID] = &cp
	m.byMAC[lease.MAC] = lease.ID
	return nil
}

func (m *memLeases) Get(_ context.Context, id string) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.byID[id]
	if !ok {
		return nil, db.ErrNotFound
	}
	cp := *lease
	return &cp, nil
}

func (m *memLeases) GetByMAC(_ context.Context, mac string) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byMAC[mac]
	if !ok {
		return nil, db.ErrNotFound
	}
	cp := *m.byID[id]
	return &cp, nil
}

func (m *memLeases) GetByServerID(_ context.Context, serverID string) ([]*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Lease
	for _, lease := range m.byID {
		if lease.ServerID == serverID {
			cp := *lease
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *memLeases) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lease, ok := m.byID[id]; ok {
		delete(m.byMAC, lease.MAC)
	}
	delete(m.byID, id)
	return nil
}

func (m *memLeases) DeleteByMAC(_ context.Context, mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byMAC[mac]; ok {
		delete(m.byID, id)
	}
	delete(m.byMAC, mac)
	return nil
}

func (m *memLeases) DeleteByServerID(context.Context, string) error { return nil }
func (m *memLeases) GetExpired(context.Context) ([]*Lease, error)   { return nil, nil }
func (m *memLeases) CleanupExpired(context.Context) error           { return nil }
