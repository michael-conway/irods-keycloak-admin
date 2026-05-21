package repair

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/irodsadapter"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/mapper"
	planvalidator "github.com/michael-conway/irods-keycloak-admin/internal/plan"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

const (
	defaultKeycloakGroupRoot = "/irods"

	mirrorAttrGroupName = "irods_group_name"
	mirrorAttrZone      = "irods_zone"
	mirrorAttrAuthority = "authority"
)

type Service struct {
	service.NotImplementedService
	IRODS        irodsadapter.Client
	Keycloak     keycloakadmin.Client
	Mapper       mapper.Mapper
	DefaultRealm string
	DefaultZone  string
	PromptMode   planreview.PromptMode
	Reviewer     planreview.Reviewer
}

var _ service.RepairService = (*Service)(nil)
var _ service.SyncService = (*Service)(nil)

type irodsGroupSnapshot struct {
	Name    string
	Zone    string
	Members map[string]struct{}
}

type keycloakGroupSnapshot struct {
	ID      string
	Name    string
	Path    string
	Zone    string
	Members map[string]string
}

func (s *Service) RepairKeycloak(ctx context.Context, req domain.RepairRequest) (domain.SyncPlan, error) {
	if err := s.validate(); err != nil {
		return domain.SyncPlan{}, err
	}

	realm := s.realmFor(req.Realm)
	if realm == "" {
		return domain.SyncPlan{}, errors.New("realm is required")
	}
	zone := s.zoneFor(req.Zone)
	if zone == "" {
		return domain.SyncPlan{}, errors.New("zone is required")
	}

	irodsGroups, err := s.readIRODSSnapshot(ctx, zone)
	if err != nil {
		return domain.SyncPlan{}, err
	}
	keycloakGroups, err := s.readKeycloakSnapshot(ctx, realm, zone)
	if err != nil {
		return domain.SyncPlan{}, err
	}

	return s.planRepair(realm, zone, irodsGroups, keycloakGroups), nil
}

func (s *Service) Apply(ctx context.Context, req domain.ApplyRequest) (domain.ApplyResult, error) {
	if err := s.validateKeycloak(); err != nil {
		return domain.ApplyResult{}, err
	}
	if req.Plan == nil {
		return domain.ApplyResult{}, errors.New("plan is required")
	}

	syncPlan := *req.Plan
	realm := s.realmFor(firstNonEmpty(req.Realm, syncPlan.Realm))
	zone := s.zoneFor(firstNonEmpty(req.Zone, syncPlan.Zone))
	if err := planvalidator.ValidateForApply(syncPlan, planvalidator.ApplyValidationOptions{
		ExpectedRealm: realm,
		ExpectedZone:  zone,
	}); err != nil {
		return domain.ApplyResult{}, err
	}
	reviewSession, err := planreview.NewSession(s.PromptMode, s.Reviewer)
	if err != nil {
		return domain.ApplyResult{}, err
	}

	result := domain.ApplyResult{
		Status:     "applied",
		PlanID:     syncPlan.PlanID,
		Warnings:   []domain.Warning{},
		Operations: []domain.MutationResult{},
	}
	if len(syncPlan.Operations) == 0 {
		result.Status = "skipped"
		return result, nil
	}

	for _, operation := range syncPlan.Operations {
		mutation := newMutationResult(syncPlan, operation)
		decision, err := reviewSession.Decide(ctx, syncPlan, operation)
		if err != nil {
			return domain.ApplyResult{}, err
		}
		if decision == planreview.DecisionSkip {
			mutation.Status = "skipped"
			if mutation.KeycloakMirror != nil {
				mutation.KeycloakMirror.Status = "skipped"
			}
			result.Skipped++
			result.Operations = append(result.Operations, mutation)
			continue
		}
		if err := s.applyOperation(ctx, syncPlan, operation); err != nil {
			mutation.Status = "failed"
			if mutation.KeycloakMirror != nil {
				mutation.KeycloakMirror.Status = "failed"
			}
			mutation.Warnings = append(mutation.Warnings, domain.Warning{
				Code:    "apply.operation_failed",
				Message: err.Error(),
			})
			result.Failed++
			result.Warnings = append(result.Warnings, mutation.Warnings...)
		} else {
			mutation.Status = "applied"
			if mutation.KeycloakMirror != nil {
				mutation.KeycloakMirror.Status = "applied"
			}
			result.Applied++
		}
		result.Operations = append(result.Operations, mutation)
	}
	result.WarningCount = len(result.Warnings)
	if result.Failed > 0 {
		result.Status = "failed"
	} else if result.Applied == 0 && result.Skipped > 0 {
		result.Status = "skipped"
	}
	return result, nil
}

func (s *Service) applyOperation(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation) error {
	switch operation.Action {
	case domain.PlanActionKeycloakGroupCreate:
		groupPath, err := planvalidator.GroupTarget(operation)
		if err != nil {
			return err
		}
		groupName := planvalidator.EvidenceString(operation, "irods_group_name")
		if groupName == "" {
			groupName = planvalidator.GroupNameFromPath(groupPath)
		}
		zone := planvalidator.EvidenceString(operation, "irods_zone")
		if zone == "" {
			zone = syncPlan.Zone
		}
		_, err = s.Keycloak.CreateOrUpdateGroup(ctx, syncPlan.Realm, keycloakadmin.Group{
			Name: groupName,
			Path: groupPath,
			Attributes: map[string][]string{
				mirrorAttrGroupName: {groupName},
				mirrorAttrZone:      {zone},
				mirrorAttrAuthority: {domain.SyncPlanAuthorityIRODS},
			},
		})
		return err
	case domain.PlanActionKeycloakGroupMemberAdd:
		groupPath, username, err := planvalidator.MemberTarget(operation)
		if err != nil {
			return err
		}
		groupRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_group_id"), groupPath)
		return s.Keycloak.AddUserToGroup(ctx, syncPlan.Realm, username, groupRef)
	case domain.PlanActionKeycloakGroupMemberRemove:
		groupPath, username, err := planvalidator.MemberTarget(operation)
		if err != nil {
			return err
		}
		groupRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_group_id"), groupPath)
		userRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_user_id"), planvalidator.EvidenceString(operation, "keycloak_user"), username)
		return s.Keycloak.RemoveUserFromGroup(ctx, syncPlan.Realm, userRef, groupRef)
	case domain.PlanActionKeycloakGroupDelete:
		groupPath, err := planvalidator.GroupTarget(operation)
		if err != nil {
			return err
		}
		groupRef := firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_group_id"), groupPath)
		return s.Keycloak.DeleteGroup(ctx, syncPlan.Realm, groupRef)
	default:
		return fmt.Errorf("unsupported operation action %q", operation.Action)
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

func (s *Service) validate() error {
	if s == nil {
		return errors.New("repair service is required")
	}
	if s.IRODS == nil {
		return errors.New("irods adapter is required")
	}
	if s.Keycloak == nil {
		return errors.New("keycloak admin client is required")
	}
	return nil
}

func (s *Service) validateKeycloak() error {
	if s == nil {
		return errors.New("repair service is required")
	}
	if s.Keycloak == nil {
		return errors.New("keycloak admin client is required")
	}
	return nil
}

func (s *Service) realmFor(realm string) string {
	realm = strings.TrimSpace(realm)
	if realm != "" {
		return realm
	}
	return strings.TrimSpace(s.DefaultRealm)
}

func (s *Service) zoneFor(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone != "" {
		return zone
	}
	if defaultZone := strings.TrimSpace(s.DefaultZone); defaultZone != "" {
		return defaultZone
	}
	return strings.TrimSpace(s.Mapper.DefaultZone)
}

func (s *Service) readIRODSSnapshot(ctx context.Context, zone string) (map[string]irodsGroupSnapshot, error) {
	groups, err := s.IRODS.ListUsers(ctx, zone, irodstypes.IRODSUserRodsGroup)
	if err != nil {
		return nil, err
	}

	snapshot := make(map[string]irodsGroupSnapshot, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		groupName := strings.TrimSpace(group.Name)
		if groupName == "" {
			continue
		}
		members, err := s.IRODS.ListGroupMembers(ctx, zone, groupName)
		if err != nil {
			return nil, err
		}
		snapshot[groupName] = irodsGroupSnapshot{
			Name:    groupName,
			Zone:    stringOrDefault(strings.TrimSpace(group.Zone), zone),
			Members: irodsMemberSet(members),
		}
	}
	return snapshot, nil
}

func (s *Service) readKeycloakSnapshot(ctx context.Context, realm string, zone string) (map[string]keycloakGroupSnapshot, error) {
	groups, err := s.Keycloak.ListGroups(ctx, realm)
	if err != nil {
		return nil, err
	}

	snapshot := map[string]keycloakGroupSnapshot{}
	for _, group := range groups {
		groupName, groupZone, ok := s.keycloakGroupMapping(realm, zone, group)
		if !ok {
			continue
		}
		groupPath := strings.TrimSpace(group.Path)
		if groupPath == "" {
			groupPath = keycloakMirrorPath(groupName)
		}
		groupID := strings.TrimSpace(group.ID)
		if groupID == "" {
			groupID = groupPath
		}
		members, err := s.Keycloak.ListGroupMembers(ctx, realm, groupID)
		if err != nil {
			return nil, err
		}
		snapshot[groupName] = keycloakGroupSnapshot{
			ID:      groupID,
			Name:    groupName,
			Path:    groupPath,
			Zone:    groupZone,
			Members: keycloakMemberSet(members),
		}
	}
	return snapshot, nil
}

func (s *Service) keycloakGroupMapping(realm string, zone string, group keycloakadmin.Group) (string, string, bool) {
	path := strings.TrimSpace(group.Path)
	mirrorName := firstAttribute(group.Attributes, mirrorAttrGroupName)
	authority := strings.ToLower(firstAttribute(group.Attributes, mirrorAttrAuthority))
	if mirrorName == "" && authority != domain.SyncPlanAuthorityIRODS && !strings.HasPrefix(path, defaultKeycloakGroupRoot+"/") {
		return "", "", false
	}

	groupZone := firstAttribute(group.Attributes, mirrorAttrZone)
	if groupZone == "" {
		groupZone = zone
	}
	if groupZone != zone {
		return "", "", false
	}

	groupName := mirrorName
	if groupName == "" {
		groupName = groupNameFromMirrorPath(group.Path)
	}
	if groupName == "" {
		mapping := s.Mapper.GroupToIRODS(realm, group)
		groupName = mapping.IRODSGroupName
	}
	if groupName == "" {
		return "", "", false
	}

	if authority != "" && authority != domain.SyncPlanAuthorityIRODS {
		return "", "", false
	}

	return groupName, groupZone, true
}

func (s *Service) planRepair(realm string, zone string, irodsGroups map[string]irodsGroupSnapshot, keycloakGroups map[string]keycloakGroupSnapshot) domain.SyncPlan {
	plan := domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            newPlanID(),
		Mode:              domain.SyncPlanModeRepairKeycloak,
		Authority:         domain.SyncPlanAuthorityIRODS,
		Realm:             realm,
		Zone:              zone,
		Summary:           domain.PlanSummary{},
		Operations:        []domain.PlanOperation{},
	}

	operationIndex := 1
	for _, groupName := range sortedKeys(irodsGroups) {
		irodsGroup := irodsGroups[groupName]
		keycloakGroup, exists := keycloakGroups[groupName]
		groupPath := keycloakMirrorPath(groupName)
		if exists && keycloakGroup.Path != "" {
			groupPath = keycloakGroup.Path
		}

		if !exists {
			plan.Operations = append(plan.Operations, newOperation(operationIndex, domain.PlanActionKeycloakGroupCreate, groupPath, "low", map[string]any{
				"irods_group_name": groupName,
				"irods_zone":       irodsGroup.Zone,
				"keycloak_realm":   realm,
				"keycloak_path":    groupPath,
			}))
			operationIndex++
			plan.Summary.CreateKeycloakGroups++
		}

		for _, username := range sortedSet(irodsGroup.Members) {
			if exists && mapContains(keycloakGroup.Members, username) {
				continue
			}
			evidence := map[string]any{
				"irods_group_name": groupName,
				"irods_username":   username,
				"irods_zone":       irodsGroup.Zone,
				"keycloak_realm":   realm,
				"keycloak_path":    groupPath,
			}
			addNonEmptyEvidence(evidence, "keycloak_group_id", keycloakGroup.ID)
			plan.Operations = append(plan.Operations, newOperation(operationIndex, domain.PlanActionKeycloakGroupMemberAdd, memberTarget(groupPath, username), "low", evidence))
			operationIndex++
			plan.Summary.UpdateKeycloakMemberships++
		}

		if !exists {
			continue
		}
		for _, username := range sortedKeys(keycloakGroup.Members) {
			if setContains(irodsGroup.Members, username) {
				continue
			}
			evidence := map[string]any{
				"irods_group_name": groupName,
				"keycloak_user":    username,
				"irods_zone":       irodsGroup.Zone,
				"keycloak_realm":   realm,
				"keycloak_path":    groupPath,
			}
			addNonEmptyEvidence(evidence, "keycloak_group_id", keycloakGroup.ID)
			addNonEmptyEvidence(evidence, "keycloak_user_id", keycloakGroup.Members[username])
			plan.Operations = append(plan.Operations, newOperation(operationIndex, domain.PlanActionKeycloakGroupMemberRemove, memberTarget(groupPath, username), "medium", evidence))
			operationIndex++
			plan.Summary.UpdateKeycloakMemberships++
		}
	}

	for _, groupName := range sortedKeys(keycloakGroups) {
		if _, exists := irodsGroups[groupName]; exists {
			continue
		}
		keycloakGroup := keycloakGroups[groupName]
		evidence := map[string]any{
			"irods_group_name": groupName,
			"irods_zone":       keycloakGroup.Zone,
			"keycloak_realm":   realm,
			"keycloak_path":    keycloakGroup.Path,
		}
		addNonEmptyEvidence(evidence, "keycloak_group_id", keycloakGroup.ID)
		plan.Operations = append(plan.Operations, newOperation(operationIndex, domain.PlanActionKeycloakGroupDelete, keycloakGroup.Path, domain.PlanRiskRequiresApproval, evidence))
		operationIndex++
		plan.Summary.DeleteKeycloakMirrors++
		plan.Summary.RequiresApproval++
	}

	return plan
}

func irodsMemberSet(members []*irodstypes.IRODSUser) map[string]struct{} {
	result := map[string]struct{}{}
	for _, member := range members {
		if member == nil || member.Type != irodstypes.IRODSUserRodsUser {
			continue
		}
		if name := strings.TrimSpace(member.Name); name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

func keycloakMemberSet(members []keycloakadmin.User) map[string]string {
	result := map[string]string{}
	for _, member := range members {
		if name := strings.TrimSpace(member.Username); name != "" {
			result[name] = strings.TrimSpace(member.ID)
		}
	}
	return result
}

func newOperation(index int, action string, target string, risk string, evidence map[string]any) domain.PlanOperation {
	return domain.PlanOperation{
		OperationID: fmt.Sprintf("op-%03d", index),
		Action:      action,
		Target:      target,
		Risk:        risk,
		Evidence:    evidence,
	}
}

func addNonEmptyEvidence(evidence map[string]any, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		evidence[key] = value
	}
}

func newPlanID() string {
	return "plan-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func keycloakMirrorPath(groupName string) string {
	return defaultKeycloakGroupRoot + "/" + strings.Trim(strings.TrimSpace(groupName), "/")
}

func groupNameFromMirrorPath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, defaultKeycloakGroupRoot+"/") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(path, defaultKeycloakGroupRoot+"/"))
}

func memberTarget(groupPath string, username string) string {
	return strings.TrimSpace(groupPath) + "#member:" + strings.TrimSpace(username)
}

func firstAttribute(attributes map[string][]string, name string) string {
	values := attributes[name]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func setContains(values map[string]struct{}, key string) bool {
	_, ok := values[key]
	return ok
}

func mapContains[T any](values map[string]T, key string) bool {
	_, ok := values[key]
	return ok
}

func stringOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
