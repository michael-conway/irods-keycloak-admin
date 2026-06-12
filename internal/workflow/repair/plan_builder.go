package repair

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

const (
	changeCauseMissingMirrorGroup = "missing_mirror_group"
	changeCauseMembershipDrift    = "membership_drift"
	changeCauseStaleKeycloakState = "stale_keycloak_state"
)

type repairPlanner struct {
	realm          string
	zone           string
	mirrorPolicy   mirrorPathPolicy
	irodsGroups    map[string]irodsGroupSnapshot
	keycloakGroups map[string]keycloakGroupSnapshot
	plan           domain.SyncPlan
	operationIndex int
}

func newRepairPlanner(realm string, zone string, mirrorPolicy mirrorPathPolicy, irodsGroups map[string]irodsGroupSnapshot, keycloakGroups map[string]keycloakGroupSnapshot) *repairPlanner {
	return &repairPlanner{
		realm:          realm,
		zone:           zone,
		mirrorPolicy:   mirrorPolicy,
		irodsGroups:    irodsGroups,
		keycloakGroups: keycloakGroups,
		plan: domain.SyncPlan{
			PlanFormatVersion:  domain.SyncPlanFormatVersion,
			PlanID:             newPlanID(),
			Mode:               domain.SyncPlanModeSync,
			TargetSystem:       domain.SyncTargetKeycloak,
			Authority:          domain.SyncPlanAuthorityIRODS,
			Realm:              realm,
			Zone:               zone,
			KeycloakMirrorRoot: mirrorPolicy.Root(),
			Summary:            domain.PlanSummary{},
			Operations:         []domain.PlanOperation{},
		},
		operationIndex: 1,
	}
}

func (p *repairPlanner) build() domain.SyncPlan {
	for _, groupName := range sortedKeys(p.irodsGroups) {
		p.appendIRODSGroupOperations(groupName, p.irodsGroups[groupName], p.keycloakGroups[groupName])
	}

	for _, groupName := range sortedKeys(p.keycloakGroups) {
		if _, exists := p.irodsGroups[groupName]; exists {
			continue
		}
		p.appendStaleKeycloakGroupDelete(p.keycloakGroups[groupName])
	}

	return p.plan
}

func (p *repairPlanner) appendIRODSGroupOperations(groupName string, irodsGroup irodsGroupSnapshot, keycloakGroup keycloakGroupSnapshot) {
	groupPath, keycloakExists := p.groupPath(groupName, keycloakGroup)
	if !keycloakExists {
		p.appendGroupCreate(groupName, irodsGroup.Zone, groupPath)
	}

	for _, username := range sortedSet(irodsGroup.Members) {
		if keycloakExists && mapContains(keycloakGroup.Members, username) {
			continue
		}
		p.appendMemberAdd(groupName, irodsGroup, groupPath, keycloakGroup, username, keycloakExists)
	}

	if !keycloakExists {
		return
	}

	for _, username := range sortedKeys(keycloakGroup.Members) {
		if setContains(irodsGroup.Members, username) {
			continue
		}
		p.appendMemberRemove(groupName, irodsGroup, groupPath, keycloakGroup, username, keycloakGroup.Members[username])
	}
}

func (p *repairPlanner) groupPath(groupName string, keycloakGroup keycloakGroupSnapshot) (string, bool) {
	groupPath := p.mirrorPolicy.GroupPath(groupName)
	if strings.TrimSpace(keycloakGroup.Path) != "" {
		return keycloakGroup.Path, true
	}
	return groupPath, strings.TrimSpace(keycloakGroup.Name) != ""
}

func (p *repairPlanner) appendGroupCreate(groupName string, zone string, groupPath string) {
	evidence := map[string]any{
		"change_cause":     changeCauseMissingMirrorGroup,
		"irods_group_name": groupName,
		"irods_zone":       zone,
		"keycloak_realm":   p.realm,
		"keycloak_path":    groupPath,
	}
	addSyncModelEvidence(evidence, domain.SyncDirectionIRODSToKeycloak, domain.SyncClassificationCandidateAddition, true)
	p.plan.Operations = append(p.plan.Operations, newOperation(p.nextOperationID(), domain.PlanActionKeycloakGroupCreate, groupPath, "low", evidence))
	p.plan.Summary.CreateKeycloakGroups++
}

func (p *repairPlanner) appendMemberAdd(groupName string, irodsGroup irodsGroupSnapshot, groupPath string, keycloakGroup keycloakGroupSnapshot, username string, keycloakExists bool) {
	changeCause := changeCauseMembershipDrift
	if !keycloakExists {
		changeCause = changeCauseMissingMirrorGroup
	}
	evidence := map[string]any{
		"change_cause":     changeCause,
		"irods_group_name": groupName,
		"irods_username":   username,
		"irods_zone":       irodsGroup.Zone,
		"keycloak_realm":   p.realm,
		"keycloak_path":    groupPath,
	}
	addNonEmptyEvidence(evidence, "keycloak_group_id", keycloakGroup.ID)
	addSyncModelEvidence(evidence, domain.SyncDirectionIRODSToKeycloak, domain.SyncClassificationCandidateAddition, true)
	p.plan.Operations = append(p.plan.Operations, newOperation(p.nextOperationID(), domain.PlanActionKeycloakGroupMemberAdd, memberTarget(groupPath, username), "low", evidence))
	p.plan.Summary.UpdateKeycloakMemberships++
}

func (p *repairPlanner) appendMemberRemove(groupName string, irodsGroup irodsGroupSnapshot, groupPath string, keycloakGroup keycloakGroupSnapshot, username string, userID string) {
	evidence := map[string]any{
		"change_cause":     changeCauseMembershipDrift,
		"irods_group_name": groupName,
		"irods_zone":       irodsGroup.Zone,
		"keycloak_user":    username,
		"keycloak_realm":   p.realm,
		"keycloak_path":    groupPath,
	}
	addNonEmptyEvidence(evidence, "keycloak_group_id", keycloakGroup.ID)
	addNonEmptyEvidence(evidence, "keycloak_user_id", userID)
	addSyncModelEvidence(evidence, domain.SyncDirectionIRODSToKeycloak, domain.SyncClassificationCandidateRemoval, strings.TrimSpace(userID) != "")
	p.plan.Operations = append(p.plan.Operations, newOperation(p.nextOperationID(), domain.PlanActionKeycloakGroupMemberRemove, memberTarget(groupPath, username), "medium", evidence))
	p.plan.Summary.UpdateKeycloakMemberships++
}

func (p *repairPlanner) appendStaleKeycloakGroupDelete(keycloakGroup keycloakGroupSnapshot) {
	evidence := map[string]any{
		"change_cause":     changeCauseStaleKeycloakState,
		"irods_group_name": keycloakGroup.Name,
		"irods_zone":       keycloakGroup.Zone,
		"keycloak_realm":   p.realm,
		"keycloak_path":    keycloakGroup.Path,
	}
	addNonEmptyEvidence(evidence, "keycloak_group_id", keycloakGroup.ID)
	addSyncModelEvidence(evidence, domain.SyncDirectionIRODSToKeycloak, domain.SyncClassificationCandidateRemoval, strings.TrimSpace(keycloakGroup.ID) != "")
	p.plan.Operations = append(p.plan.Operations, newOperation(p.nextOperationID(), domain.PlanActionKeycloakGroupDelete, keycloakGroup.Path, domain.PlanRiskRequiresApproval, evidence))
	p.plan.Summary.DeleteKeycloakMirrors++
	p.plan.Summary.RequiresApproval++
}

func (p *repairPlanner) nextOperationID() int {
	current := p.operationIndex
	p.operationIndex++
	return current
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

func addSyncModelEvidence(evidence map[string]any, direction string, classification string, mappingIdentityKnown bool) {
	evidence["sync_direction"] = direction
	evidence["sync_classification"] = classification
	evidence["mapping_identity_known"] = mappingIdentityKnown
	evidence["authority_role"] = "directional_repair_policy"
	evidence["conflict_status"] = "none"
}

func newPlanID() string {
	return "plan-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func memberTarget(groupPath string, username string) string {
	return strings.TrimSpace(groupPath) + "#member:" + strings.TrimSpace(username)
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
