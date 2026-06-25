package plan

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

type ApplyValidationOptions struct {
	ExpectedRealm      string
	ExpectedZone       string
	ExpectedMirrorRoot string
	ExpectedTarget     string
}

func ValidateForApply(syncPlan domain.SyncPlan, opts ApplyValidationOptions) error {
	if syncPlan.PlanFormatVersion != domain.SyncPlanFormatVersion {
		return fmt.Errorf("unsupported sync plan format version %q", syncPlan.PlanFormatVersion)
	}
	if strings.TrimSpace(syncPlan.PlanID) == "" {
		return errors.New("plan_id is required")
	}
	if syncPlan.Mode != domain.SyncPlanModeSync {
		return fmt.Errorf("unsupported plan mode %q", syncPlan.Mode)
	}
	planTarget := normalizeTargetSystem(syncPlan.TargetSystem)
	if planTarget == "" {
		planTarget = domain.SyncTargetKeycloak
	}
	expectedTarget := normalizeTargetSystem(opts.ExpectedTarget)
	if expectedTarget != "" && planTarget != expectedTarget {
		return fmt.Errorf("unsupported plan target_system %q", syncPlan.TargetSystem)
	}
	if syncPlan.Authority != domain.SyncPlanAuthorityIRODS {
		return fmt.Errorf("unsupported plan authority %q", syncPlan.Authority)
	}
	if strings.TrimSpace(syncPlan.Realm) == "" {
		return errors.New("plan realm is required")
	}
	if strings.TrimSpace(syncPlan.Zone) == "" {
		return errors.New("plan zone is required")
	}
	if opts.ExpectedRealm != "" && syncPlan.Realm != opts.ExpectedRealm {
		return errors.New("plan realm does not match runtime configuration")
	}
	if opts.ExpectedZone != "" && syncPlan.Zone != opts.ExpectedZone {
		return errors.New("plan zone does not match runtime configuration")
	}
	planMirrorRoot := NormalizeGroupPath(syncPlan.KeycloakMirrorRoot)
	expectedMirrorRoot := NormalizeGroupPath(opts.ExpectedMirrorRoot)
	if planMirrorRoot != "" && expectedMirrorRoot != "" && planMirrorRoot != expectedMirrorRoot {
		return errors.New("plan keycloak mirror root does not match runtime configuration")
	}
	validationMirrorRoot := firstNonEmpty(expectedMirrorRoot, planMirrorRoot)
	validationTarget := planTarget

	seenOperations := map[string]struct{}{}
	for _, operation := range syncPlan.Operations {
		if err := ValidateOperationForApply(syncPlan, operation, validationMirrorRoot, validationTarget); err != nil {
			return err
		}
		operationID := strings.TrimSpace(operation.OperationID)
		if operationID == "" {
			return errors.New("operation_id is required")
		}
		if _, ok := seenOperations[operationID]; ok {
			return fmt.Errorf("duplicate operation_id %q", operationID)
		}
		seenOperations[operationID] = struct{}{}
	}
	return nil
}

func ValidateOperationForApply(syncPlan domain.SyncPlan, operation domain.PlanOperation, expectedMirrorRoot string, targetSystem string) error {
	if strings.TrimSpace(operation.Target) == "" {
		return fmt.Errorf("operation %q target is required", operation.OperationID)
	}
	if EvidenceString(operation, "keycloak_realm") != "" && EvidenceString(operation, "keycloak_realm") != syncPlan.Realm {
		return fmt.Errorf("operation %q keycloak_realm evidence does not match plan realm", operation.OperationID)
	}
	if EvidenceString(operation, "irods_zone") != "" && EvidenceString(operation, "irods_zone") != syncPlan.Zone {
		return fmt.Errorf("operation %q irods_zone evidence does not match plan zone", operation.OperationID)
	}
	switch operation.Action {
	case domain.PlanActionKeycloakUserCreate:
		if targetSystem != domain.SyncTargetKeycloak {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		username, err := IRODSUserTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if evidenceUsername := firstNonEmpty(EvidenceString(operation, "irods_username"), EvidenceString(operation, "keycloak_username")); evidenceUsername != "" && evidenceUsername != username {
			return fmt.Errorf("operation %q username evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionKeycloakGroupCreate:
		if targetSystem != domain.SyncTargetKeycloak {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		groupPath, err := GroupTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if err := validateGroupPathWithinMirrorRoot(operation.OperationID, groupPath, expectedMirrorRoot); err != nil {
			return err
		}
		if evidencePath := EvidenceString(operation, "keycloak_path"); evidencePath != "" && NormalizeGroupPath(evidencePath) != groupPath {
			return fmt.Errorf("operation %q keycloak_path evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionKeycloakGroupMemberAdd, domain.PlanActionKeycloakGroupMemberRemove:
		if targetSystem != domain.SyncTargetKeycloak {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		groupPath, username, err := MemberTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if err := validateGroupPathWithinMirrorRoot(operation.OperationID, groupPath, expectedMirrorRoot); err != nil {
			return err
		}
		if evidencePath := EvidenceString(operation, "keycloak_path"); evidencePath != "" && NormalizeGroupPath(evidencePath) != groupPath {
			return fmt.Errorf("operation %q keycloak_path evidence does not match target", operation.OperationID)
		}
		if strings.Contains(username, "#") || strings.Contains(username, "/") {
			return fmt.Errorf("operation %q member target username has invalid characters", operation.OperationID)
		}
	case domain.PlanActionKeycloakGroupDelete:
		if targetSystem != domain.SyncTargetKeycloak {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		if operation.Risk != domain.PlanRiskRequiresApproval {
			return fmt.Errorf("operation %q group delete must be marked requires_approval", operation.OperationID)
		}
		groupPath, err := GroupTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if err := validateGroupPathWithinMirrorRoot(operation.OperationID, groupPath, expectedMirrorRoot); err != nil {
			return err
		}
		if evidencePath := EvidenceString(operation, "keycloak_path"); evidencePath != "" && NormalizeGroupPath(evidencePath) != groupPath {
			return fmt.Errorf("operation %q keycloak_path evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionIRODSUserCreate:
		if targetSystem != domain.SyncTargetIRODS {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		username, err := IRODSUserTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if EvidenceString(operation, "keycloak_user_id") == "" {
			return fmt.Errorf("operation %q keycloak_user_id evidence is required", operation.OperationID)
		}
		if evidenceUsername := firstNonEmpty(EvidenceString(operation, "irods_username"), EvidenceString(operation, "keycloak_username")); evidenceUsername != "" && evidenceUsername != username {
			return fmt.Errorf("operation %q username evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionIRODSUserMetadataSync:
		if targetSystem != domain.SyncTargetIRODS && targetSystem != domain.SyncTargetKeycloak {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		username, err := IRODSUserTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if targetSystem == domain.SyncTargetIRODS && EvidenceString(operation, "keycloak_user_id") == "" {
			return fmt.Errorf("operation %q keycloak_user_id evidence is required", operation.OperationID)
		}
		if targetSystem == domain.SyncTargetKeycloak && EvidenceString(operation, "keycloak_user_id") == "" && EvidenceString(operation, "keycloak_user_id_source") != "created_or_resolved_by_previous_operation" {
			return fmt.Errorf("operation %q keycloak_user_id evidence or post-create keycloak_user_id_source evidence is required", operation.OperationID)
		}
		if evidenceUsername := firstNonEmpty(EvidenceString(operation, "irods_username"), EvidenceString(operation, "keycloak_username")); evidenceUsername != "" && evidenceUsername != username {
			return fmt.Errorf("operation %q username evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionIRODSGroupCreate, domain.PlanActionIRODSGroupMetadataSync:
		if targetSystem != domain.SyncTargetIRODS {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		groupName, err := IRODSGroupTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if EvidenceString(operation, "keycloak_group_id") == "" {
			return fmt.Errorf("operation %q keycloak_group_id evidence is required", operation.OperationID)
		}
		if evidenceGroupName := firstNonEmpty(EvidenceString(operation, "irods_group_name"), EvidenceString(operation, "keycloak_group_name")); evidenceGroupName != "" && evidenceGroupName != groupName {
			return fmt.Errorf("operation %q group name evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionIRODSGroupMemberAdd, domain.PlanActionIRODSGroupMemberRemove:
		if targetSystem != domain.SyncTargetIRODS {
			return fmt.Errorf("operation %q has unsupported action %q for target %q", operation.OperationID, operation.Action, targetSystem)
		}
		groupName, username, err := IRODSMemberTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if EvidenceString(operation, "keycloak_group_id") == "" {
			return fmt.Errorf("operation %q keycloak_group_id evidence is required", operation.OperationID)
		}
		if EvidenceString(operation, "keycloak_user_id") == "" {
			return fmt.Errorf("operation %q keycloak_user_id evidence is required", operation.OperationID)
		}
		if evidenceGroupName := EvidenceString(operation, "irods_group_name"); evidenceGroupName != "" && evidenceGroupName != groupName {
			return fmt.Errorf("operation %q group name evidence does not match target", operation.OperationID)
		}
		if evidenceUsername := firstNonEmpty(EvidenceString(operation, "irods_username"), EvidenceString(operation, "keycloak_username")); evidenceUsername != "" && evidenceUsername != username {
			return fmt.Errorf("operation %q username evidence does not match target", operation.OperationID)
		}
	default:
		return fmt.Errorf("operation %q has unsupported action %q", operation.OperationID, operation.Action)
	}
	return nil
}

func IRODSUserTarget(operation domain.PlanOperation) (string, error) {
	username := strings.TrimSpace(operation.Target)
	if username == "" {
		return "", errors.New("iRODS user target username is required")
	}
	if strings.Contains(username, "#") || strings.Contains(username, "/") {
		return "", errors.New("iRODS user target username has invalid characters")
	}
	return username, nil
}

func IRODSGroupTarget(operation domain.PlanOperation) (string, error) {
	groupName := strings.TrimSpace(operation.Target)
	if groupName == "" {
		return "", errors.New("iRODS group target name is required")
	}
	if strings.Contains(groupName, "#") || strings.Contains(groupName, "/") {
		return "", errors.New("iRODS group target name has invalid characters")
	}
	return groupName, nil
}

func IRODSMemberTarget(operation domain.PlanOperation) (string, string, error) {
	target := strings.TrimSpace(operation.Target)
	parts := strings.Split(target, "#member:")
	if len(parts) != 2 {
		return "", "", errors.New("iRODS member target must have shape group-name#member:username")
	}
	groupName, err := IRODSGroupTarget(domain.PlanOperation{Target: parts[0]})
	if err != nil {
		return "", "", err
	}
	username, err := IRODSUserTarget(domain.PlanOperation{Target: parts[1]})
	if err != nil {
		return "", "", err
	}
	return groupName, username, nil
}

func GroupTarget(operation domain.PlanOperation) (string, error) {
	target := strings.TrimSpace(operation.Target)
	if strings.Contains(target, "#member:") {
		return "", errors.New("group target must not include a member selector")
	}
	groupPath := NormalizeGroupPath(target)
	if groupPath == "" || groupPath == "/" {
		return "", errors.New("group target must be an absolute Keycloak group path")
	}
	return groupPath, nil
}

func MemberTarget(operation domain.PlanOperation) (string, string, error) {
	target := strings.TrimSpace(operation.Target)
	parts := strings.Split(target, "#member:")
	if len(parts) != 2 {
		return "", "", errors.New("member target must have shape /group/path#member:username")
	}
	groupPath := NormalizeGroupPath(parts[0])
	username := strings.TrimSpace(parts[1])
	if groupPath == "" || groupPath == "/" {
		return "", "", errors.New("member target group must be an absolute Keycloak group path")
	}
	if username == "" {
		return "", "", errors.New("member target username is required")
	}
	return groupPath, username, nil
}

func EvidenceString(operation domain.PlanOperation, key string) string {
	if operation.Evidence == nil {
		return ""
	}
	value, ok := operation.Evidence[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func NormalizeGroupPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func GroupNameFromPath(groupPath string) string {
	groupPath = NormalizeGroupPath(groupPath)
	if groupPath == "" || groupPath == "/" {
		return ""
	}
	return path.Base(groupPath)
}

func validateGroupPathWithinMirrorRoot(operationID string, groupPath string, mirrorRoot string) error {
	mirrorRoot = NormalizeGroupPath(mirrorRoot)
	if mirrorRoot == "" {
		return nil
	}
	if groupPath == mirrorRoot || strings.HasPrefix(groupPath, mirrorRoot+"/") {
		return nil
	}
	return fmt.Errorf("operation %q target does not match runtime keycloak mirror root", operationID)
}

func SummaryCounts(syncPlan domain.SyncPlan) domain.PlanSummary {
	summary := domain.PlanSummary{}
	for _, operation := range syncPlan.Operations {
		switch operation.Action {
		case domain.PlanActionKeycloakUserCreate:
			summary.CreateKeycloakUsers++
		case domain.PlanActionKeycloakGroupCreate:
			summary.CreateKeycloakGroups++
		case domain.PlanActionKeycloakGroupMemberAdd, domain.PlanActionKeycloakGroupMemberRemove:
			summary.UpdateKeycloakMemberships++
		case domain.PlanActionKeycloakGroupDelete:
			summary.DeleteKeycloakMirrors++
		case domain.PlanActionIRODSUserCreate:
			summary.CreateIRODSUsers++
		case domain.PlanActionIRODSUserMetadataSync:
			summary.UpdateIRODSUserMetadata++
		case domain.PlanActionIRODSGroupCreate:
			summary.CreateIRODSGroups++
		case domain.PlanActionIRODSGroupMetadataSync:
			summary.UpdateIRODSGroupMetadata++
		case domain.PlanActionIRODSGroupMemberAdd, domain.PlanActionIRODSGroupMemberRemove:
			summary.UpdateIRODSMemberships++
		}
		if operation.Risk == domain.PlanRiskRequiresApproval {
			summary.RequiresApproval++
		}
	}
	return summary
}

func OperationIDs(syncPlan domain.SyncPlan) []string {
	result := make([]string, 0, len(syncPlan.Operations))
	for _, operation := range syncPlan.Operations {
		result = append(result, operation.OperationID)
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func normalizeTargetSystem(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
