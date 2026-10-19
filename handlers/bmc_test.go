package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ignite/config"

	"github.com/stmcginnis/gofish/redfish"
	"github.com/stretchr/testify/require"
)

type fakeBMCSystem struct {
	id     string
	power  redfish.PowerState
	boot   *redfish.Boot
	reset  redfish.ResetType
	setErr error
	rstErr error
}

func (s *fakeBMCSystem) ID() string                     { return s.id }
func (s *fakeBMCSystem) PowerState() redfish.PowerState { return s.power }
func (s *fakeBMCSystem) SetBoot(boot redfish.Boot) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.boot = &boot
	return nil
}
func (s *fakeBMCSystem) Reset(resetType redfish.ResetType) error {
	s.reset = resetType
	return s.rstErr
}

type fakeBMCSession struct {
	systems []bmcSystem
	closed  bool
}

func (s *fakeBMCSession) Logout() error                 { s.closed = true; return nil }
func (s *fakeBMCSession) Systems() ([]bmcSystem, error) { return s.systems, nil }

func TestSelectBMCSystem(t *testing.T) {
	one := &fakeBMCSystem{id: "1"}
	two := &fakeBMCSystem{id: "2"}

	got, err := selectBMCSystem([]bmcSystem{one}, "")
	require.NoError(t, err)
	require.Equal(t, "1", got.ID())

	_, err = selectBMCSystem(nil, "")
	require.ErrorIs(t, err, errNoBMCSystems)

	_, err = selectBMCSystem([]bmcSystem{one, two}, "")
	require.ErrorIs(t, err, errAmbiguousBMCSystems)

	got, err = selectBMCSystem([]bmcSystem{one, two}, "2")
	require.NoError(t, err)
	require.Equal(t, "2", got.ID())

	_, err = selectBMCSystem([]bmcSystem{one, two}, "9")
	require.ErrorIs(t, err, errBMCSystemNotFound)
}

func TestResetTypeForPower(t *testing.T) {
	kind, err := resetTypeForPower(redfish.OffPowerState)
	require.NoError(t, err)
	require.Equal(t, redfish.OnResetType, kind)

	kind, err = resetTypeForPower(redfish.OnPowerState)
	require.NoError(t, err)
	require.Equal(t, redfish.ForceRestartResetType, kind)

	_, err = resetTypeForPower(redfish.PoweringOffPowerState)
	require.Error(t, err)
}

func TestSubmitIPMIUsesPowerStateAndSystemID(t *testing.T) {
	off := &fakeBMCSystem{id: "sys-off", power: redfish.OffPowerState}
	on := &fakeBMCSystem{id: "sys-on", power: redfish.OnPowerState}
	session := &fakeBMCSession{systems: []bmcSystem{off, on}}
	var gotInsecure bool
	var gotEndpoint string

	h := &IPMIHandlers{
		container: &Container{Config: &config.Config{BMC: config.BMCConfig{TLSInsecure: false, SystemID: "sys-on"}}},
		dial: func(ctx context.Context, endpoint, username, password string, insecure bool) (bmcSession, error) {
			require.NotNil(t, ctx)
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.False(t, deadline.IsZero())
			gotInsecure = insecure
			gotEndpoint = endpoint
			require.Equal(t, "root", username)
			require.Equal(t, "secret", password)
			return session, nil
		},
	}

	form := url.Values{
		"ip":       {"10.0.0.5"},
		"username": {"root"},
		"password": {"secret"},
		"reboot":   {"on"},
	}
	req := httptest.NewRequest(http.MethodPost, "/pxe/submit_ipmi", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.SubmitIPMI(rec, req)

	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/dhcp", rec.Header().Get("Location"))
	require.False(t, gotInsecure)
	require.Equal(t, "https://10.0.0.5/redfish/v1", gotEndpoint)
	require.Equal(t, redfish.ForceRestartResetType, on.reset)
	require.Empty(t, off.reset)
	require.True(t, session.closed)

	off.reset = ""
	on.reset = ""
	form.Set("system_id", "sys-off")
	req = httptest.NewRequest(http.MethodPost, "/pxe/submit_ipmi", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.SubmitIPMI(rec, req)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, redfish.OnResetType, off.reset)
	require.Empty(t, on.reset)
}

func TestSubmitIPMIAmbiguousSystems(t *testing.T) {
	session := &fakeBMCSession{systems: []bmcSystem{
		&fakeBMCSystem{id: "a", power: redfish.OnPowerState},
		&fakeBMCSystem{id: "b", power: redfish.OffPowerState},
	}}
	h := &IPMIHandlers{
		container: &Container{Config: &config.Config{}},
		dial: func(context.Context, string, string, string, bool) (bmcSession, error) {
			return session, nil
		},
	}
	form := url.Values{"ip": {"10.0.0.8"}, "username": {"root"}, "password": {"secret"}, "reboot": {"on"}}
	req := httptest.NewRequest(http.MethodPost, "/pxe/submit_ipmi", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.SubmitIPMI(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body.Error.UserMessage, "system_id")
}

func TestSubmitIPMIInsecureOptIn(t *testing.T) {
	var gotInsecure bool
	h := &IPMIHandlers{
		container: &Container{Config: &config.Config{BMC: config.BMCConfig{TLSInsecure: true}}},
		dial: func(_ context.Context, _, _, _ string, insecure bool) (bmcSession, error) {
			gotInsecure = insecure
			return &fakeBMCSession{systems: []bmcSystem{&fakeBMCSystem{id: "1", power: redfish.OffPowerState}}}, nil
		},
	}
	form := url.Values{"ip": {"10.1.1.1"}, "username": {"root"}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/pxe/submit_ipmi", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.SubmitIPMI(rec, req)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.True(t, gotInsecure)
}
