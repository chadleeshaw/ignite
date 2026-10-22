package dhcp

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStartServerWhenDHCPDisabledDoesNotBind(t *testing.T) {
	t.Setenv("DHCP_ENABLED", "false")
	repo := &MockServerRepository{}
	repo.On("Get", mock.Anything, "srv").Return(&Server{
		ID: "srv",
		IP: net.ParseIP("127.0.0.1"),
	}, nil)
	svc := NewDHCPServerService(repo, &MockLeaseRepository{})

	err := svc.StartServer(context.Background(), "srv")
	require.ErrorIs(t, err, ErrDHCPDisabled)
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	require.Equal(t, ServerStateConfigured, CurrentServerState("srv"))
}

func TestDisabledListenersStayConfigured(t *testing.T) {
	const id = "stopped-while-off"
	SetListenersEnabled(false)
	t.Cleanup(func() {
		SetListenersEnabled(true)
		ClearServerState(id)
	})
	NoteServerState(id, ServerStateStopped)
	NoteServerState(id+"-run", ServerStateRunning)
	t.Cleanup(func() { ClearServerState(id + "-run") })

	require.Equal(t, ServerStateConfigured, CurrentServerState(id))
	require.Equal(t, ServerStateConfigured, CurrentServerState(id+"-run"))
	require.False(t, ListenersEnabled())

	SetListenersEnabled(true)
	require.Equal(t, ServerStateStopped, CurrentServerState(id))
	require.Equal(t, ServerStateRunning, CurrentServerState(id+"-run"))
}
