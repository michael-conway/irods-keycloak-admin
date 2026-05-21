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
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

const (
	modeRepairKeycloak       = "repair-keycloak"
	authorityIRODS           = "irods"
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
}

var _ service.RepairService = (*Service)(nil)

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
	Members map[string]struct{}
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
	if mirrorName == "" && authority != authorityIRODS && !strings.HasPrefix(path, defaultKeycloakGroupRoot+"/") {
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

	if authority != "" && authority != authorityIRODS {
		return "", "", false
	}

	return groupName, groupZone, true
}

func (s *Service) planRepair(realm string, zone string, irodsGroups map[string]irodsGroupSnapshot, keycloakGroups map[string]keycloakGroupSnapshot) domain.SyncPlan {
	plan := domain.SyncPlan{
		PlanID:     newPlanID(),
		Mode:       modeRepairKeycloak,
		Authority:  authorityIRODS,
		Realm:      realm,
		Zone:       zone,
		Summary:    domain.PlanSummary{},
		Operations: []domain.PlanOperation{},
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
			plan.Operations = append(plan.Operations, newOperation(operationIndex, "keycloak.group.create", groupPath, "low", map[string]any{
				"irods_group_name": groupName,
				"irods_zone":       irodsGroup.Zone,
				"keycloak_realm":   realm,
				"keycloak_path":    groupPath,
			}))
			operationIndex++
			plan.Summary.CreateKeycloakGroups++
		}

		for _, username := range sortedSet(irodsGroup.Members) {
			if exists && setContains(keycloakGroup.Members, username) {
				continue
			}
			plan.Operations = append(plan.Operations, newOperation(operationIndex, "keycloak.group.member.add", memberTarget(groupPath, username), "low", map[string]any{
				"irods_group_name": groupName,
				"irods_username":   username,
				"irods_zone":       irodsGroup.Zone,
				"keycloak_realm":   realm,
				"keycloak_path":    groupPath,
			}))
			operationIndex++
			plan.Summary.UpdateKeycloakMemberships++
		}

		if !exists {
			continue
		}
		for _, username := range sortedSet(keycloakGroup.Members) {
			if setContains(irodsGroup.Members, username) {
				continue
			}
			plan.Operations = append(plan.Operations, newOperation(operationIndex, "keycloak.group.member.remove", memberTarget(groupPath, username), "medium", map[string]any{
				"irods_group_name": groupName,
				"keycloak_user":    username,
				"irods_zone":       irodsGroup.Zone,
				"keycloak_realm":   realm,
				"keycloak_path":    groupPath,
			}))
			operationIndex++
			plan.Summary.UpdateKeycloakMemberships++
		}
	}

	for _, groupName := range sortedKeys(keycloakGroups) {
		if _, exists := irodsGroups[groupName]; exists {
			continue
		}
		keycloakGroup := keycloakGroups[groupName]
		plan.Operations = append(plan.Operations, newOperation(operationIndex, "keycloak.group.delete", keycloakGroup.Path, "requires_approval", map[string]any{
			"irods_group_name": groupName,
			"irods_zone":       keycloakGroup.Zone,
			"keycloak_realm":   realm,
			"keycloak_path":    keycloakGroup.Path,
		}))
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

func keycloakMemberSet(members []keycloakadmin.User) map[string]struct{} {
	result := map[string]struct{}{}
	for _, member := range members {
		if name := strings.TrimSpace(member.Username); name != "" {
			result[name] = struct{}{}
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

func stringOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}
