package httpapi

import (
	"net/http"
	"net/http/httptest"
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

func TestGenericIRODSGroupRoutesAreNotRegistered(t *testing.T) {
	handler := NewHandler(config.Default(), service.NewNotImplementedServices()).Routes()

	tests := []struct {
		name   string
		method string
		target string
	}{
		{
			name:   "create group",
			method: http.MethodPost,
			target: "/admin/v1/irods/groups",
		},
		{
			name:   "delete group",
			method: http.MethodDelete,
			target: "/admin/v1/irods/groups/project-alpha",
		},
		{
			name:   "add member",
			method: http.MethodPost,
			target: "/admin/v1/irods/groups/project-alpha/members/alice",
		},
		{
			name:   "remove member",
			method: http.MethodDelete,
			target: "/admin/v1/irods/groups/project-alpha/members/alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
			}
		})
	}
}

func TestControlPlaneRoutesRemainRegistered(t *testing.T) {
	handler := NewHandler(config.Default(), service.NewNotImplementedServices()).Routes()

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{
			name:       "sync plan",
			method:     http.MethodPost,
			target:     "/admin/v1/sync/plan",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "bootstrap keycloak",
			method:     http.MethodPost,
			target:     "/admin/v1/bootstrap/keycloak",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "repair keycloak",
			method:     http.MethodPost,
			target:     "/admin/v1/repair/keycloak",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "keycloak events",
			method:     http.MethodPost,
			target:     "/admin/v1/keycloak/events",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "provisioning request",
			method:     http.MethodPost,
			target:     "/admin/v1/provisioning/requests",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "diagnostics",
			method:     http.MethodPost,
			target:     "/admin/v1/diagnostics/check-config",
			wantStatus: http.StatusNotImplemented,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestKeycloakEventRouteRequiresSharedSecretWhenConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.KeycloakEventSharedSecret = "expected-secret"
	handler := NewHandler(cfg, service.NewNotImplementedServices()).Routes()

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/keycloak/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestKeycloakEventRouteRejectsInvalidSharedSecret(t *testing.T) {
	cfg := config.Default()
	cfg.KeycloakEventSharedSecret = "expected-secret"
	handler := NewHandler(cfg, service.NewNotImplementedServices()).Routes()

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/keycloak/events", nil)
	req.Header.Set(keycloakEventSharedSecretHeader, "wrong-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestKeycloakEventRouteAcceptsConfiguredSharedSecret(t *testing.T) {
	cfg := config.Default()
	cfg.KeycloakEventSharedSecret = "expected-secret"
	handler := NewHandler(cfg, service.NewNotImplementedServices()).Routes()

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/keycloak/events", nil)
	req.Header.Set(keycloakEventSharedSecretHeader, "expected-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, rec.Code)
	}
}
