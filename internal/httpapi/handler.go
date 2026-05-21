package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/config"
	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

type Handler struct {
	cfg      config.Config
	services service.Services
}

func NewHandler(cfg config.Config, services service.Services) *Handler {
	return &Handler{
		cfg:      cfg,
		services: services,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.getHealth)
	mux.HandleFunc("GET /admin/v1/status", h.getStatus)
	mux.HandleFunc("GET /admin/v1/config/summary", h.getConfigSummary)

	mux.HandleFunc("POST /admin/v1/provisioning/users/{keycloak_user_id}/plan", h.postProvisionUserPlan)
	mux.HandleFunc("POST /admin/v1/provisioning/users/{keycloak_user_id}/apply", h.postProvisionUserApply)
	mux.HandleFunc("POST /admin/v1/provisioning/requests", h.postProvisioningRequest)
	mux.HandleFunc("POST /admin/v1/provisioning/requests/{request_id}/approve", h.postProvisioningApprove)
	mux.HandleFunc("POST /admin/v1/provisioning/requests/{request_id}/reject", h.postProvisioningReject)

	mux.HandleFunc("POST /admin/v1/sync/plan", h.postSyncPlan)
	mux.HandleFunc("POST /admin/v1/sync/apply", h.postSyncApply)
	mux.HandleFunc("POST /admin/v1/bootstrap/keycloak", h.postBootstrapKeycloak)
	mux.HandleFunc("POST /admin/v1/repair/keycloak", h.postRepairKeycloak)
	mux.HandleFunc("POST /admin/v1/keycloak/events", h.postKeycloakEvent)

	mux.HandleFunc("POST /admin/v1/diagnostics/check-config", h.postCheckConfig)
	mux.HandleFunc("POST /admin/v1/diagnostics/check-mapping", h.postCheckMapping)
	mux.HandleFunc("POST /admin/v1/diagnostics/check-drift", h.postCheckDrift)

	return mux
}

func (h *Handler) getHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) getStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, domain.StatusResponse{
		Status:  "ok",
		Service: h.cfg.ServiceName,
		Version: config.ServiceVersion,
	})
}

func (h *Handler) getConfigSummary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.cfg.Summary())
}

func (h *Handler) postProvisionUserPlan(w http.ResponseWriter, r *http.Request) {
	var req domain.ProvisionUserRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	req.KeycloakUserID = strings.TrimSpace(r.PathValue("keycloak_user_id"))
	result, err := h.services.Provisioning.PlanUser(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) postProvisionUserApply(w http.ResponseWriter, r *http.Request) {
	var req domain.ProvisionUserRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	req.KeycloakUserID = strings.TrimSpace(r.PathValue("keycloak_user_id"))
	result, err := h.services.Provisioning.ApplyUser(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) postProvisioningRequest(w http.ResponseWriter, r *http.Request) {
	var req domain.ProvisioningRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.services.Provisioning.CreateRequest(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) postProvisioningApprove(w http.ResponseWriter, r *http.Request) {
	h.handleProvisioningDecision(w, r, h.services.Provisioning.ApproveRequest)
}

func (h *Handler) postProvisioningReject(w http.ResponseWriter, r *http.Request) {
	h.handleProvisioningDecision(w, r, h.services.Provisioning.RejectRequest)
}

func (h *Handler) handleProvisioningDecision(w http.ResponseWriter, r *http.Request, fn func(context.Context, domain.ProvisioningDecisionRequest) (domain.MutationResult, error)) {
	var req domain.ProvisioningDecisionRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	req.RequestID = strings.TrimSpace(r.PathValue("request_id"))
	result, err := fn(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) postSyncPlan(w http.ResponseWriter, r *http.Request) {
	var req domain.PlanRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.services.Sync.Plan(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) postSyncApply(w http.ResponseWriter, r *http.Request) {
	var req domain.ApplyRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.services.Sync.Apply(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) postBootstrapKeycloak(w http.ResponseWriter, r *http.Request) {
	var req domain.BootstrapRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.services.Bootstrap.BootstrapKeycloak(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) postRepairKeycloak(w http.ResponseWriter, r *http.Request) {
	var req domain.RepairRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.services.Repair.RepairKeycloak(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) postKeycloakEvent(w http.ResponseWriter, r *http.Request) {
	var req domain.KeycloakEventRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.services.Event.IngestKeycloakEvent(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) postCheckConfig(w http.ResponseWriter, r *http.Request) {
	h.handleDiagnostics(w, r, h.services.Diagnostics.CheckConfig)
}

func (h *Handler) postCheckMapping(w http.ResponseWriter, r *http.Request) {
	h.handleDiagnostics(w, r, h.services.Diagnostics.CheckMapping)
}

func (h *Handler) postCheckDrift(w http.ResponseWriter, r *http.Request) {
	h.handleDiagnostics(w, r, h.services.Diagnostics.CheckDrift)
}

func (h *Handler) handleDiagnostics(w http.ResponseWriter, r *http.Request, fn func(context.Context, domain.DiagnosticsRequest) (domain.DiagnosticsResult, error)) {
	var req domain.DiagnosticsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := fn(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeRequest(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Body == nil {
		return true
	}
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(out)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
	return false
}
