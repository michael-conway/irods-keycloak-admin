package service

import (
	"context"
	"errors"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

var ErrNotImplemented = errors.New("not implemented")

type Services struct {
	User         UserWorkflowService
	Provisioning ProvisioningService
	Sync         SyncService
	Bootstrap    BootstrapService
	Repair       RepairService
	Event        EventService
	Diagnostics  DiagnosticsService
}

type UserWorkflowService interface {
	ProvisionUser(ctx context.Context, req domain.ProvisionUserRequest) (domain.MutationResult, error)
	DeprovisionUser(ctx context.Context, req domain.DeprovisionUserRequest) (domain.MutationResult, error)
}

type ProvisioningService interface {
	PlanUser(ctx context.Context, req domain.ProvisionUserRequest) (domain.SyncPlan, error)
	ApplyUser(ctx context.Context, req domain.ProvisionUserRequest) (domain.MutationResult, error)
	CreateRequest(ctx context.Context, req domain.ProvisioningRequest) (domain.MutationResult, error)
	ApproveRequest(ctx context.Context, req domain.ProvisioningDecisionRequest) (domain.MutationResult, error)
	RejectRequest(ctx context.Context, req domain.ProvisioningDecisionRequest) (domain.MutationResult, error)
}

type SyncService interface {
	Plan(ctx context.Context, req domain.PlanRequest) (domain.SyncPlan, error)
	Apply(ctx context.Context, req domain.ApplyRequest) (domain.ApplyResult, error)
}

type BootstrapService interface {
	BootstrapKeycloak(ctx context.Context, req domain.BootstrapRequest) (domain.ApplyResult, error)
}

type RepairService interface {
	RepairKeycloak(ctx context.Context, req domain.RepairRequest) (domain.SyncPlan, error)
}

type EventService interface {
	IngestKeycloakEvent(ctx context.Context, req domain.KeycloakEventRequest) (domain.EventResult, error)
}

type DiagnosticsService interface {
	CheckConfig(ctx context.Context, req domain.DiagnosticsRequest) (domain.DiagnosticsResult, error)
	CheckMapping(ctx context.Context, req domain.DiagnosticsRequest) (domain.DiagnosticsResult, error)
	CheckDrift(ctx context.Context, req domain.DiagnosticsRequest) (domain.DiagnosticsResult, error)
}
