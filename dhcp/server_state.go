package dhcp

import "sync"

// ServerState is the listener state of one DHCP server.
// It is the only status value the HTTP APIs return.
//
//	running    the listener is bound and serving
//	stopped    it was started and then stopped, or the last start failed
//	configured it is defined and has not been started since this process booted
type ServerState string

const (
	ServerStateRunning    ServerState = "running"
	ServerStateStopped    ServerState = "stopped"
	ServerStateConfigured ServerState = "configured"
)

func (s ServerState) valid() bool {
	switch s {
	case ServerStateRunning, ServerStateStopped, ServerStateConfigured:
		return true
	default:
		return false
	}
}

// serverStates is the process-wide record of what each DHCP listener is
// doing. The persisted Server.Started flag is not a substitute: it survives
// a restart in which the socket was never bound, and the HTTP handlers used
// to keep a second copy that the service did not update.
var serverStates = struct {
	sync.RWMutex
	byID    map[string]ServerState
	enabled bool
}{byID: make(map[string]ServerState), enabled: true}

// SetListenersEnabled records whether this process may run DHCP listeners.
// When disabled, CurrentServerState reports configured for every server.
// A running or stopped mark cannot make overall health partial or down
// while DHCP is off.
func SetListenersEnabled(enabled bool) {
	serverStates.Lock()
	serverStates.enabled = enabled
	serverStates.Unlock()
}

// ListenersEnabled reports the last value from SetListenersEnabled.
// The default is true.
func ListenersEnabled() bool {
	serverStates.RLock()
	defer serverStates.RUnlock()
	return serverStates.enabled
}

// NoteServerState records the listener state for serverID.
// An empty id or a value other than the three ServerState constants is ignored.
func NoteServerState(serverID string, state ServerState) {
	if serverID == "" || !state.valid() {
		return
	}
	serverStates.Lock()
	serverStates.byID[serverID] = state
	serverStates.Unlock()
}

// ClearServerState forgets a deleted server so it is not reported again.
func ClearServerState(serverID string) {
	if serverID == "" {
		return
	}
	serverStates.Lock()
	delete(serverStates.byID, serverID)
	serverStates.Unlock()
}

// CurrentServerState returns the listener state recorded for serverID.
// A server this process has not started, stopped, or failed is configured.
func CurrentServerState(serverID string) ServerState {
	serverStates.RLock()
	enabled := serverStates.enabled
	state, ok := serverStates.byID[serverID]
	serverStates.RUnlock()
	if !enabled || !ok || !state.valid() {
		return ServerStateConfigured
	}
	return state
}
