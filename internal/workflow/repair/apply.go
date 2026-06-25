package repair

import (
	"context"
	"errors"
	"fmt"
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/avu"
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
	case domain.PlanActionKeycloakUserCreate:
		user, err := keycloakUserForCreate(syncPlan, operation)
		if err != nil {
			return "", err
		}
		_, err = s.Keycloak.CreateOrUpdateUser(ctx, syncPlan.Realm, user)
		if err != nil {
			return "", err
		}
		return keycloakadmin.MutationOutcomeCreated, nil
	case domain.PlanActionIRODSUserMetadataSync:
		return s.applyPostCreateUserMetadataSync(ctx, syncPlan, operation)
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

func (s *Service) applyPostCreateUserMetadataSync(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation) (keycloakadmin.MutationOutcome, error) {
	if s.IRODS == nil {
		return "", errors.New("irods adapter is required for post-create user metadata sync")
	}
	username, err := planvalidator.IRODSUserTarget(operation)
	if err != nil {
		return "", err
	}
	zone := firstNonEmpty(planvalidator.EvidenceString(operation, "irods_zone"), syncPlan.Zone)
	keycloakUserID := strings.TrimSpace(planvalidator.EvidenceString(operation, "keycloak_user_id"))
	if keycloakUserID == "" {
		keycloakUsername := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_username"), username)
		keycloakUser, err := s.Keycloak.FindUserByUsername(ctx, syncPlan.Realm, keycloakUsername)
		if err != nil {
			return "", err
		}
		if keycloakUser == nil || strings.TrimSpace(keycloakUser.ID) == "" {
			return "", &keycloakadmin.UserNotFoundError{Realm: syncPlan.Realm, Ref: keycloakUsername}
		}
		keycloakUserID = strings.TrimSpace(keycloakUser.ID)
	}

	currentMetadata, err := s.IRODS.ListUserMetadata(ctx, username, zone)
	if err != nil {
		return "", err
	}
	missingMetadata := missingIRODSUserMetadata(currentMetadata, postCreateUserMetadata(syncPlan, keycloakUserID))
	if len(missingMetadata) == 0 {
		return keycloakadmin.MutationOutcomeUnchanged, nil
	}
	for _, metadata := range missingMetadata {
		if err := s.IRODS.AddUserMetadata(ctx, username, zone, metadata); err != nil {
			return "", fmt.Errorf("add iRODS AVU %q=%q to user %q in zone %q: %w", metadata.Name, metadata.Value, username, zone, err)
		}
	}
	return keycloakadmin.MutationOutcomeUpdated, nil
}

func postCreateUserMetadata(syncPlan domain.SyncPlan, keycloakUserID string) []*irodstypes.IRODSMeta {
	return []*irodstypes.IRODSMeta{
		{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
		{Name: avu.KeycloakRealmAttribute, Value: syncPlan.Realm},
		{Name: avu.KeycloakUserIDAttribute, Value: keycloakUserID},
		{Name: avu.AuthorityAttribute, Value: domain.SyncPlanAuthorityIRODS},
	}
}

func missingIRODSUserMetadata(current []*irodstypes.IRODSMeta, desired []*irodstypes.IRODSMeta) []*irodstypes.IRODSMeta {
	missing := []*irodstypes.IRODSMeta{}
	for _, metadata := range desired {
		if metadata == nil || strings.TrimSpace(metadata.Name) == "" || strings.TrimSpace(metadata.Value) == "" {
			continue
		}
		if hasIRODSMetadataValue(current, metadata.Name, metadata.Value) {
			continue
		}
		missing = append(missing, &irodstypes.IRODSMeta{Name: metadata.Name, Value: metadata.Value, Units: metadata.Units})
	}
	return missing
}

func hasIRODSMetadataValue(metadata []*irodstypes.IRODSMeta, name string, value string) bool {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	for _, entry := range metadata {
		if entry == nil {
			continue
		}
		if strings.TrimSpace(entry.Name) == name && strings.TrimSpace(entry.Value) == value {
			return true
		}
	}
	return false
}

func keycloakUserForCreate(syncPlan domain.SyncPlan, operation domain.PlanOperation) (keycloakadmin.User, error) {
	username, err := planvalidator.IRODSUserTarget(operation)
	if err != nil {
		return keycloakadmin.User{}, err
	}
	zone := planvalidator.EvidenceString(operation, "irods_zone")
	if zone == "" {
		zone = syncPlan.Zone
	}
	return keycloakadmin.User{
		Username: username,
		Attributes: map[string][]string{
			"irods_username":    {username},
			mirrorAttrZone:      {zone},
			mirrorAttrAuthority: {domain.SyncPlanAuthorityIRODS},
		},
	}, nil
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
	userRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_user_id"), username)
	return groupRef, userRef, nil
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
	if operation.Action == domain.PlanActionIRODSUserMetadataSync {
		mutation.IRODS = &domain.SystemMutationResult{
			User:   username,
			Zone:   syncPlan.Zone,
			Status: "pending",
		}
	} else {
		mutation.KeycloakMirror = &domain.SystemMutationResult{
			Realm:  syncPlan.Realm,
			Group:  groupPath,
			User:   username,
			Zone:   syncPlan.Zone,
			Status: "pending",
		}
	}
	return mutation
}

func mutationTargetParts(operation domain.PlanOperation) (string, string) {
	if operation.Action == domain.PlanActionKeycloakUserCreate || operation.Action == domain.PlanActionIRODSUserMetadataSync {
		username, err := planvalidator.IRODSUserTarget(operation)
		if err == nil {
			return "", username
		}
	}
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
	if mutation.IRODS != nil {
		mutation.IRODS.Status = status
	}
}
