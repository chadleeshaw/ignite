package dhcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"ignite/db"

	"github.com/stretchr/testify/require"
)

func TestServerStateTracksTheListener(t *testing.T) {
	prev := listenDHCP
	t.Cleanup(func() { listenDHCP = prev })

	const id = "listener-state-server"
	t.Cleanup(func() { ClearServerState(id) })

	repo := &memServerRepo{servers: map[string]*Server{
		id: {ID: id, IP: net.ParseIP("10.1.0.1"), Started: true},
	}}
	svc := NewDHCPServerService(repo, nil)
	ctx := context.Background()

	// A persisted Started flag from a previous process is not a live listener.
	require.Equal(t, ServerStateConfigured, CurrentServerState(id))

	listenDHCP = func(string, string) (net.PacketConn, error) {
		return nil, errors.New("bind refused")
	}
	err := svc.StartServer(ctx, id)
	require.Error(t, err)
	require.Equal(t, ServerStateStopped, CurrentServerState(id))

	listenDHCP = func(string, string) (net.PacketConn, error) {
		return net.ListenPacket("udp4", "127.0.0.1:0")
	}
	require.NoError(t, svc.StartServer(ctx, id))
	t.Cleanup(func() { _ = svc.StopServer(context.Background(), id) })
	require.Equal(t, ServerStateRunning, CurrentServerState(id))

	require.NoError(t, svc.StopServer(ctx, id))
	require.Equal(t, ServerStateStopped, CurrentServerState(id))
}

func TestNoteServerStateIgnoresUnknownValues(t *testing.T) {
	const id = "note-state-server"
	t.Cleanup(func() { ClearServerState(id) })
	NoteServerState(id, ServerState("badge-success"))
	require.Equal(t, ServerStateConfigured, CurrentServerState(id))
	NoteServerState("", ServerStateRunning)
	require.Equal(t, ServerStateConfigured, CurrentServerState(""))
}

type memServerRepo struct {
	mu      sync.Mutex
	servers map[string]*Server
}

func (m *memServerRepo) Save(_ context.Context, server *Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *server
	m.servers[server.ID] = &cp
	return nil
}

func (m *memServerRepo) Get(_ context.Context, id string) (*Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	server, ok := m.servers[id]
	if !ok {
		return nil, db.ErrNotFound
	}
	cp := *server
	return &cp, nil
}

func (m *memServerRepo) GetAll(context.Context) ([]*Server, error) { return nil, nil }
func (m *memServerRepo) Delete(context.Context, string) error      { return nil }
func (m *memServerRepo) GetByIP(context.Context, net.IP) (*Server, error) {
	return nil, db.ErrNotFound
}
