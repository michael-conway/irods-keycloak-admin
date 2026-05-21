package service

import (
	"context"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

func NewNotImplementedServices() Services {
	stub := NotImplementedService{}
	return Services{
		User:         stub,
		Provisioning: stub,
		Sync:         stub,
		Bootstrap:    stub,
		Repair:       stub,
		Event:        stub,
		Diagnostics:  stub,
	}
}

type NotImplementedService struct{}

func (NotImplementedService) ProvisionUser(context.Context, domain.ProvisionUserRequest) (domain.MutationResult, error) {
	return domain.MutationResult{}, ErrNotImplemented
}

func (NotImplementedService) DeprovisionUser(context.Context, domain.DeprovisionUserRequest) (domain.MutationResult, error) {
	return domain.MutationResult{}, ErrNotImplemented
}

func (NotImplementedService) PlanUser(context.Context, domain.ProvisionUserRequest) (domain.SyncPlan, error) {
	return domain.SyncPlan{}, ErrNotImplemented
}

func (NotImplementedService) ApplyUser(context.Context, domain.ProvisionUserRequest) (domain.MutationResult, error) {
	return domain.MutationResult{}, ErrNotImplemented
}

func (NotImplementedService) CreateRequest(context.Context, domain.ProvisioningRequest) (domain.MutationResult, error) {
	return domain.MutationResult{}, ErrNotImplemented
}

func (NotImplementedService) ApproveRequest(context.Context, domain.ProvisioningDecisionRequest) (domain.MutationResult, error) {
	return domain.MutationResult{}, ErrNotImplemented
}

func (NotImplementedService) RejectRequest(context.Context, domain.ProvisioningDecisionRequest) (domain.MutationResult, error) {
	return domain.MutationResult{}, ErrNotImplemented
}

func (NotImplementedService) Plan(context.Context, domain.PlanRequest) (domain.SyncPlan, error) {
	return domain.SyncPlan{}, ErrNotImplemented
}

func (NotImplementedService) Apply(context.Context, domain.ApplyRequest) (domain.ApplyResult, error) {
	return domain.ApplyResult{}, ErrNotImplemented
}

func (NotImplementedService) BootstrapKeycloak(context.Context, domain.BootstrapRequest) (domain.ApplyResult, error) {
	return domain.ApplyResult{}, ErrNotImplemented
}

func (NotImplementedService) RepairKeycloak(context.Context, domain.RepairRequest) (domain.SyncPlan, error) {
	return domain.SyncPlan{}, ErrNotImplemented
}

func (NotImplementedService) IngestKeycloakEvent(context.Context, domain.KeycloakEventRequest) (domain.EventResult, error) {
	return domain.EventResult{}, ErrNotImplemented
}

func (NotImplementedService) CheckConfig(context.Context, domain.DiagnosticsRequest) (domain.DiagnosticsResult, error) {
	return domain.DiagnosticsResult{}, ErrNotImplemented
}

func (NotImplementedService) CheckMapping(context.Context, domain.DiagnosticsRequest) (domain.DiagnosticsResult, error) {
	return domain.DiagnosticsResult{}, ErrNotImplemented
}

func (NotImplementedService) CheckDrift(context.Context, domain.DiagnosticsRequest) (domain.DiagnosticsResult, error) {
	return domain.DiagnosticsResult{}, ErrNotImplemented
}
