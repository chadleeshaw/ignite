package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

// bmcRequestTimeout bounds a single Redfish connect-and-act sequence.
const bmcRequestTimeout = 20 * time.Second

var (
	errNoBMCSystems        = errors.New("no redfish systems")
	errAmbiguousBMCSystems = errors.New("multiple redfish systems and no system id")
	errBMCSystemNotFound   = errors.New("redfish system not found")
)

// bmcSystem is one Redfish ComputerSystem. Tests use a fake.
type bmcSystem interface {
	ID() string
	PowerState() redfish.PowerState
	SetBoot(redfish.Boot) error
	Reset(redfish.ResetType) error
}

// bmcSession is a connected Redfish service.
type bmcSession interface {
	Logout() error
	Systems() ([]bmcSystem, error)
}

// bmcDialer opens a Redfish session. insecure skips TLS verification.
type bmcDialer func(ctx context.Context, endpoint, username, password string, insecure bool) (bmcSession, error)

// selectBMCSystem picks the ComputerSystem to operate on.
// A non-empty id must match exactly one system. With no id, the BMC must
// expose exactly one system; more than one is an error rather than systems[0].
func selectBMCSystem(systems []bmcSystem, wantID string) (bmcSystem, error) {
	present := make([]bmcSystem, 0, len(systems))
	for _, system := range systems {
		if system != nil {
			present = append(present, system)
		}
	}
	if len(present) == 0 {
		return nil, errNoBMCSystems
	}

	wantID = strings.TrimSpace(wantID)
	if wantID != "" {
		var match bmcSystem
		for _, system := range present {
			if system.ID() == wantID {
				if match != nil {
					return nil, errAmbiguousBMCSystems
				}
				match = system
			}
		}
		if match == nil {
			return nil, fmt.Errorf("%w: %s", errBMCSystemNotFound, wantID)
		}
		return match, nil
	}
	if len(present) == 1 {
		return present[0], nil
	}
	return nil, errAmbiguousBMCSystems
}

// resetTypeForPower returns On when the host is off and ForceRestart only
// when it is on. Any other PowerState is refused so a transitional state
// is not force-restarted.
func resetTypeForPower(state redfish.PowerState) (redfish.ResetType, error) {
	switch state {
	case redfish.OnPowerState:
		return redfish.ForceRestartResetType, nil
	case redfish.OffPowerState:
		return redfish.OnResetType, nil
	default:
		return "", fmt.Errorf("refusing to change power while the host is in state %q", state)
	}
}

type gofishSession struct {
	client *gofish.APIClient
}

func (s *gofishSession) Logout() error {
	if s == nil || s.client == nil {
		return nil
	}
	s.client.Logout()
	return nil
}

func (s *gofishSession) Systems() ([]bmcSystem, error) {
	if s == nil || s.client == nil || s.client.Service == nil {
		return nil, errNoBMCSystems
	}
	systems, err := s.client.Service.Systems()
	if err != nil {
		return nil, err
	}
	out := make([]bmcSystem, 0, len(systems))
	for _, system := range systems {
		if system == nil {
			continue
		}
		out = append(out, gofishSystem{system: system})
	}
	return out, nil
}

type gofishSystem struct {
	system *redfish.ComputerSystem
}

func (s gofishSystem) ID() string {
	if s.system == nil {
		return ""
	}
	return s.system.ID
}

func (s gofishSystem) PowerState() redfish.PowerState {
	if s.system == nil {
		return ""
	}
	return s.system.PowerState
}

func (s gofishSystem) SetBoot(boot redfish.Boot) error {
	return s.system.SetBoot(boot)
}

func (s gofishSystem) Reset(resetType redfish.ResetType) error {
	return s.system.Reset(resetType)
}

// dialGofish connects with the caller's context, so a timeout on ctx bounds
// the handshake and the calls made on the returned session.
func dialGofish(ctx context.Context, endpoint, username, password string, insecure bool) (bmcSession, error) {
	client, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint: endpoint,
		Username: username,
		Password: password,
		Insecure: insecure,
	})
	if err != nil {
		return nil, err
	}
	return &gofishSession{client: client}, nil
}
