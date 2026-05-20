package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, domain.ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrNotImplemented) {
		writeError(w, http.StatusNotImplemented, "not_implemented", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
}
