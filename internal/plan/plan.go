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
	ExpectedRealm string
	ExpectedZone  string
}

func ValidateForApply(syncPlan domain.SyncPlan, opts ApplyValidationOptions) error {
	if syncPlan.PlanFormatVersion != domain.SyncPlanFormatVersion {
		return fmt.Errorf("unsupported sync plan format version %q", syncPlan.PlanFormatVersion)
	}
	if strings.TrimSpace(syncPlan.PlanID) == "" {
		return errors.New("plan_id is required")
	}
	if syncPlan.Mode != domain.SyncPlanModeRepairKeycloak {
		return fmt.Errorf("unsupported plan mode %q", syncPlan.Mode)
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

	seenOperations := map[string]struct{}{}
	for _, operation := range syncPlan.Operations {
		if err := ValidateOperationForApply(syncPlan, operation, opts); err != nil {
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

func ValidateOperationForApply(syncPlan domain.SyncPlan, operation domain.PlanOperation, opts ApplyValidationOptions) error {
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
	case domain.PlanActionKeycloakGroupCreate:
		groupPath, err := GroupTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if evidencePath := EvidenceString(operation, "keycloak_path"); evidencePath != "" && NormalizeGroupPath(evidencePath) != groupPath {
			return fmt.Errorf("operation %q keycloak_path evidence does not match target", operation.OperationID)
		}
	case domain.PlanActionKeycloakGroupMemberAdd, domain.PlanActionKeycloakGroupMemberRemove:
		groupPath, username, err := MemberTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if evidencePath := EvidenceString(operation, "keycloak_path"); evidencePath != "" && NormalizeGroupPath(evidencePath) != groupPath {
			return fmt.Errorf("operation %q keycloak_path evidence does not match target", operation.OperationID)
		}
		if strings.Contains(username, "#") || strings.Contains(username, "/") {
			return fmt.Errorf("operation %q member target username has invalid characters", operation.OperationID)
		}
	case domain.PlanActionKeycloakGroupDelete:
		if operation.Risk != domain.PlanRiskRequiresApproval {
			return fmt.Errorf("operation %q group delete must be marked requires_approval", operation.OperationID)
		}
		groupPath, err := GroupTarget(operation)
		if err != nil {
			return fmt.Errorf("operation %q: %w", operation.OperationID, err)
		}
		if evidencePath := EvidenceString(operation, "keycloak_path"); evidencePath != "" && NormalizeGroupPath(evidencePath) != groupPath {
			return fmt.Errorf("operation %q keycloak_path evidence does not match target", operation.OperationID)
		}
	default:
		return fmt.Errorf("operation %q has unsupported action %q", operation.OperationID, operation.Action)
	}
	return nil
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

func SummaryCounts(syncPlan domain.SyncPlan) domain.PlanSummary {
	summary := domain.PlanSummary{}
	for _, operation := range syncPlan.Operations {
		switch operation.Action {
		case domain.PlanActionKeycloakGroupCreate:
			summary.CreateKeycloakGroups++
		case domain.PlanActionKeycloakGroupMemberAdd, domain.PlanActionKeycloakGroupMemberRemove:
			summary.UpdateKeycloakMemberships++
		case domain.PlanActionKeycloakGroupDelete:
			summary.DeleteKeycloakMirrors++
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
