package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNonOverviewPageGetsOverallStatus(t *testing.T) {
	chdirRepoRoot(t)
	SetPageHealth(func(r *http.Request) string {
		return "partial"
	})
	t.Cleanup(func() { SetPageHealth(nil) })

	data := struct {
		Title         string
		Mode          string
		CSRFToken     string
		OverallStatus string
		Servers       []DHCPServerView
	}{Title: "DHCP Management"}

	req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	rec := httptest.NewRecorder()
	renderCachedTemplate(rec, req, "dhcp", data, "Unable to render the DHCP page")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `data-overall-status="partial"`)
}

func TestApplyUIModeKeepsComputedOverallStatus(t *testing.T) {
	SetPageHealth(nil)
	t.Cleanup(func() { SetPageHealth(nil) })

	data := &StatusPageData{OverallStatus: "partial"}
	req := httptest.NewRequest(http.MethodGet, "/status/content", nil)
	got := applyUIMode(req, data).(*StatusPageData)
	require.Equal(t, "partial", got.OverallStatus)

	SetPageHealth(func(*http.Request) string { return "down" })
	got = applyUIMode(req, data).(*StatusPageData)
	require.Equal(t, "down", got.OverallStatus)
}
