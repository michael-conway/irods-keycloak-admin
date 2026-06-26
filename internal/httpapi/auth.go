package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const keycloakEventSharedSecretHeader = "X-IRODS-KC-Shared-Secret"

func (h *Handler) authorizeKeycloakEvent(w http.ResponseWriter, r *http.Request) bool {
	expected := strings.TrimSpace(h.cfg.KeycloakEventSharedSecret)
	if expected == "" {
		return true
	}
	actual := strings.TrimSpace(r.Header.Get(keycloakEventSharedSecretHeader))
	if actual == "" {
		writeError(w, http.StatusUnauthorized, "missing_shared_secret", "missing Keycloak event shared secret")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		writeError(w, http.StatusForbidden, "invalid_shared_secret", "invalid Keycloak event shared secret")
		return false
	}
	return true
}
