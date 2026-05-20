package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/michael-conway/irods-keycloak-admin/internal/config"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

func TestHealthRoute(t *testing.T) {
	handler := NewHandler(config.Default(), service.NewNotImplementedServices()).Routes()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestScaffoldedMutationReturnsNotImplemented(t *testing.T) {
	handler := NewHandler(config.Default(), service.NewNotImplementedServices()).Routes()
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/irods/groups", strings.NewReader(`{"group_name":"project-alpha"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, rec.Code)
	}
}
