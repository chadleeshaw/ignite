package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusRedirectsToOverview(t *testing.T) {
	h := NewStatusHandlers(&Container{})
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	h.HandleStatusPage(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/", rec.Header().Get("Location"))
}
