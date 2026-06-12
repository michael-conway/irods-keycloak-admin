package repair

import (
	"context"
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
	})
}

func (s *Service) applyPlan(ctx context.Context, syncPlan domain.SyncPlan, reviewSession *planreview.Session) (domain.ApplyResult, error) {
	result := newApplyResult(syncPlan)
	if len(syncPlan.Operations) == 0 {
		result.Status = "skipped"
		return result, nil
	}

	for _, operation := range syncPlan.Operations {
		mutation, skipped, err := s.applyReviewedOperation(ctx, syncPlan, operation, reviewSession)
		if err != nil {
			return domain.ApplyResult{}, err
		}
		if skipped {
			result.Skipped++
			result.Operations = append(result.Operations, mutation)
			continue
		}
		if len(mutation.Warnings) > 0 {
			result.Failed++
			result.Warnings = append(result.Warnings, mutation.Warnings...)
		} else {
			result.Applied++
		}
		result.Operations = append(result.Operations, mutation)
	}

	finalizeApplyResult(&result)
	return result, nil
}

func (s *Service) applyReviewedOperation(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation, reviewSession *planreview.Session) (domain.MutationResult, bool, error) {
	mutation := newMutationResult(syncPlan, operation)
	decision, err := reviewSession.Decide(ctx, syncPlan, operation)
	if err != nil {
		return domain.MutationResult{}, false, err
	}
	if decision == planreview.DecisionSkip {
		markMutationStatus(&mutation, "skipped")
		return mutation, true, nil
	}

	if err := s.applyOperation(ctx, syncPlan, operation); err != nil {
		markMutationStatus(&mutation, "failed")
		mutation.Warnings = append(mutation.Warnings, domain.Warning{
			Code:    "apply.operation_failed",
			Message: err.Error(),
		})
		return mutation, false, nil
	}

	markMutationStatus(&mutation, "applied")
	return mutation, false, nil
}

func (s *Service) applyOperation(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation) error {
	switch operation.Action {
	case domain.PlanActionKeycloakGroupCreate:
		group, err := keycloakGroupForCreate(syncPlan, operation)
		if err != nil {
			return err
		}
		_, err = s.Keycloak.CreateOrUpdateGroup(ctx, syncPlan.Realm, group)
		return err
	case domain.PlanActionKeycloakGroupMemberAdd:
		groupRef, userRef, err := keycloakMemberAddRefs(operation)
		if err != nil {
			return err
		}
		return s.Keycloak.AddUserToGroup(ctx, syncPlan.Realm, userRef, groupRef)
	case domain.PlanActionKeycloakGroupMemberRemove:
		groupRef, userRef, err := keycloakMemberRemoveRefs(operation)
		if err != nil {
			return err
		}
		return s.Keycloak.RemoveUserFromGroup(ctx, syncPlan.Realm, userRef, groupRef)
	case domain.PlanActionKeycloakGroupDelete:
		groupRef, err := keycloakDeleteRef(operation)
		if err != nil {
			return err
		}
		return s.Keycloak.DeleteGroup(ctx, syncPlan.Realm, groupRef)
	default:
		return fmt.Errorf("unsupported operation action %q", operation.Action)
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
	groupRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_group_id"), groupPath)
	return groupRef, username, nil
}

func keycloakMemberRemoveRefs(operation domain.PlanOperation) (string, string, error) {
	groupPath, username, err := planvalidator.MemberTarget(operation)
	if err != nil {
		return "", "", err
	}
	groupRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_group_id"), groupPath)
	userRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_user_id"), planvalidator.EvidenceString(operation, "keycloak_user"), username)
	return groupRef, userRef, nil
}

func keycloakDeleteRef(operation domain.PlanOperation) (string, error) {
	groupPath, err := planvalidator.GroupTarget(operation)
	if err != nil {
		return "", err
	}
	return firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_group_id"), groupPath), nil
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
