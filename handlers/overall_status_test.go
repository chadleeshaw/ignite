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
