package repair

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	planvalidator "github.com/michael-conway/irods-keycloak-admin/internal/plan"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
)

func validateApplyPlan(syncPlan domain.SyncPlan, realm string, zone string, mirrorRoot string) error {
	return planvalidator.ValidateForApply(syncPlan, planvalidator.ApplyValidationOptions{
		ExpectedRealm:      realm,
		ExpectedZone:       zone,
		ExpectedMirrorRoot: mirrorRoot,
		ExpectedTarget:     domain.SyncTargetKeycloak,
	})
}

func (s *Service) applyPlan(ctx context.Context, syncPlan domain.SyncPlan, reviewSession *planreview.Session) (domain.ApplyResult, error) {
	result := newApplyResult(syncPlan)
	if len(syncPlan.Operations) == 0 {
		result.Status = "skipped"
		return result, nil
	}

	for _, operation := range syncPlan.Operations {
		mutation, disposition, err := s.applyReviewedOperation(ctx, syncPlan, operation, reviewSession)
		if err != nil {
			return domain.ApplyResult{}, err
		}
		switch disposition {
		case "skipped", "unchanged":
			result.Skipped++
			result.Operations = append(result.Operations, mutation)
			continue
		case "failed":
			result.Failed++
			result.Warnings = append(result.Warnings, mutation.Warnings...)
		case "applied":
			result.Applied++
		}
		result.Operations = append(result.Operations, mutation)
	}

	finalizeApplyResult(&result)
	return result, nil
}

func (s *Service) applyReviewedOperation(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation, reviewSession *planreview.Session) (domain.MutationResult, string, error) {
	mutation := newMutationResult(syncPlan, operation)
	decision, err := reviewSession.Decide(ctx, syncPlan, operation)
	if err != nil {
		return domain.MutationResult{}, "", err
	}
	if decision == planreview.DecisionSkip {
		markMutationStatus(&mutation, "skipped")
		return mutation, "skipped", nil
	}

	outcome, err := s.applyOperation(ctx, syncPlan, operation)
	if err != nil {
		markMutationStatus(&mutation, "failed")
		mutation.Warnings = append(mutation.Warnings, classifyApplyWarning(err))
		return mutation, "failed", nil
	}
	if outcome == keycloakadmin.MutationOutcomeUnchanged {
		markMutationStatus(&mutation, "unchanged")
		return mutation, "unchanged", nil
	}

	markMutationStatus(&mutation, "applied")
	return mutation, "applied", nil
}

func (s *Service) applyOperation(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation) (keycloakadmin.MutationOutcome, error) {
	switch operation.Action {
	case domain.PlanActionKeycloakGroupCreate:
		group, err := keycloakGroupForCreate(syncPlan, operation)
		if err != nil {
			return "", err
		}
		_, outcome, err := s.Keycloak.CreateOrUpdateGroup(ctx, syncPlan.Realm, group)
		return outcome, err
	case domain.PlanActionKeycloakGroupMemberAdd:
		groupRef, userRef, err := keycloakMemberAddRefs(operation)
		if err != nil {
			return "", err
		}
		return s.Keycloak.AddUserToGroup(ctx, syncPlan.Realm, userRef, groupRef)
	case domain.PlanActionKeycloakGroupMemberRemove:
		groupRef, userRef, err := keycloakMemberRemoveRefs(operation)
		if err != nil {
			return "", err
		}
		return s.Keycloak.RemoveUserFromGroup(ctx, syncPlan.Realm, userRef, groupRef)
	case domain.PlanActionKeycloakGroupDelete:
		groupRef, err := keycloakDeleteRef(operation)
		if err != nil {
			return "", err
		}
		return s.Keycloak.DeleteGroup(ctx, syncPlan.Realm, groupRef)
	default:
		return "", fmt.Errorf("unsupported operation action %q", operation.Action)
	}
}

func keycloakGroupForCreate(syncPlan domain.SyncPlan, operation domain.PlanOperation) (group keycloakadmin.Group, err error) {
	groupPath, err := planvalidator.GroupTarget(operation)
	if err != nil {
		return keycloakadmin.Group{}, err
	}
	groupName := planvalidator.EvidenceString(operation, "irods_group_name")
	if groupName == "" {
		groupName = planvalidator.GroupNameFromPath(groupPath)
	}
	zone := planvalidator.EvidenceString(operation, "irods_zone")
	if zone == "" {
		zone = syncPlan.Zone
	}
	return keycloakadmin.Group{
		Name: groupName,
		Path: groupPath,
		Attributes: map[string][]string{
			mirrorAttrGroupName: {groupName},
			mirrorAttrZone:      {zone},
			mirrorAttrAuthority: {domain.SyncPlanAuthorityIRODS},
		},
	}, nil
}

func keycloakMemberAddRefs(operation domain.PlanOperation) (string, string, error) {
	groupPath, username, err := planvalidator.MemberTarget(operation)
	if err != nil {
		return "", "", err
	}
	groupRef := firstNonEmpty(groupPath, planvalidator.EvidenceString(operation, "keycloak_group_id"))
	return groupRef, username, nil
}

func keycloakMemberRemoveRefs(operation domain.PlanOperation) (string, string, error) {
	groupPath, username, err := planvalidator.MemberTarget(operation)
	if err != nil {
		return "", "", err
	}
	groupRef := firstNonEmpty(groupPath, planvalidator.EvidenceString(operation, "keycloak_group_id"))
	userRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_user_id"), planvalidator.EvidenceString(operation, "keycloak_user"), username)
	return groupRef, userRef, nil
}

func keycloakDeleteRef(operation domain.PlanOperation) (string, error) {
	groupPath, err := planvalidator.GroupTarget(operation)
	if err != nil {
		return "", err
	}
	return firstNonEmpty(groupPath, planvalidator.EvidenceString(operation, "keycloak_group_id")), nil
}

func newApplyResult(syncPlan domain.SyncPlan) domain.ApplyResult {
	return domain.ApplyResult{
		Status:     "applied",
		PlanID:     syncPlan.PlanID,
		Warnings:   []domain.Warning{},
		Operations: []domain.MutationResult{},
	}
}

func finalizeApplyResult(result *domain.ApplyResult) {
	if result == nil {
		return
	}
	result.WarningCount = len(result.Warnings)
	if result.Failed > 0 {
		result.Status = "failed"
		return
	}
	if result.Applied == 0 && result.Skipped > 0 {
		result.Status = "skipped"
	}
}

func classifyApplyWarning(err error) domain.Warning {
	var groupNotFound *keycloakadmin.GroupNotFoundError
	if errors.As(err, &groupNotFound) {
		return domain.Warning{Code: "apply.keycloak.group_not_found", Message: err.Error()}
	}
	var userNotFound *keycloakadmin.UserNotFoundError
	if errors.As(err, &userNotFound) {
		return domain.Warning{Code: "apply.keycloak.user_not_found", Message: err.Error()}
	}
	if strings.Contains(err.Error(), "unsupported operation action") {
		return domain.Warning{Code: "apply.plan.unsupported_operation", Message: err.Error()}
	}
	if strings.Contains(err.Error(), "target") || strings.Contains(err.Error(), "member target") || strings.Contains(err.Error(), "group target") {
		return domain.Warning{Code: "apply.plan.invalid_operation", Message: err.Error()}
	}
	var statusErr *keycloakadmin.StatusError
	if errors.As(err, &statusErr) {
		return domain.Warning{Code: "apply.keycloak.request_failed", Message: err.Error()}
	}
	return domain.Warning{Code: "apply.operation_failed", Message: err.Error()}
}

func newMutationResult(syncPlan domain.SyncPlan, operation domain.PlanOperation) domain.MutationResult {
	mutation := domain.MutationResult{
		OperationID: operation.OperationID,
		Status:      "pending",
		Operation:   operation.Action,
		Target:      operation.Target,
		Warnings:    []domain.Warning{},
	}
	groupPath, username := mutationTargetParts(operation)
	mutation.KeycloakMirror = &domain.SystemMutationResult{
		Realm:  syncPlan.Realm,
		Group:  groupPath,
		User:   username,
		Zone:   syncPlan.Zone,
		Status: "pending",
	}
	return mutation
}

func mutationTargetParts(operation domain.PlanOperation) (string, string) {
	if strings.Contains(operation.Target, "#member:") {
		groupPath, username, err := planvalidator.MemberTarget(operation)
		if err == nil {
			return groupPath, username
		}
	}
	groupPath, err := planvalidator.GroupTarget(operation)
	if err == nil {
		return groupPath, ""
	}
	return operation.Target, ""
}

func markMutationStatus(mutation *domain.MutationResult, status string) {
	if mutation == nil {
		return
	}
	mutation.Status = status
	if mutation.KeycloakMirror != nil {
		mutation.KeycloakMirror.Status = status
	}
}
