package repair

import (
	"context"
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	planvalidator "github.com/michael-conway/irods-keycloak-admin/internal/plan"
)

func (s *Service) readRepairSnapshots(ctx context.Context, realm string, zone string) (map[string]irodsGroupSnapshot, map[string]keycloakGroupSnapshot, error) {
	irodsGroups, err := s.readIRODSSnapshot(ctx, zone)
	if err != nil {
		return nil, nil, err
	}
	keycloakGroups, err := s.readKeycloakSnapshot(ctx, realm, zone)
	if err != nil {
		return nil, nil, err
	}
	return irodsGroups, keycloakGroups, nil
}

func (s *Service) readIRODSSnapshot(ctx context.Context, zone string) (map[string]irodsGroupSnapshot, error) {
	groups, err := s.IRODS.ListUsers(ctx, zone, irodstypes.IRODSUserRodsGroup)
	if err != nil {
		return nil, err
	}

	snapshot := make(map[string]irodsGroupSnapshot, len(groups))
	for _, group := range groups {
		groupSnapshot, ok, err := s.buildIRODSGroupSnapshot(ctx, zone, group)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		snapshot[groupSnapshot.Name] = groupSnapshot
	}
	return snapshot, nil
}

func (s *Service) buildIRODSGroupSnapshot(ctx context.Context, zone string, group *irodstypes.IRODSUser) (irodsGroupSnapshot, bool, error) {
	if group == nil {
		return irodsGroupSnapshot{}, false, nil
	}
	groupName := strings.TrimSpace(group.Name)
	if groupName == "" {
		return irodsGroupSnapshot{}, false, nil
	}
	members, err := s.IRODS.ListGroupMembers(ctx, zone, groupName)
	if err != nil {
		return irodsGroupSnapshot{}, false, err
	}
	return irodsGroupSnapshot{
		Name:    groupName,
		Zone:    stringOrDefault(strings.TrimSpace(group.Zone), zone),
		Members: irodsMemberSet(members),
	}, true, nil
}

func (s *Service) readKeycloakSnapshot(ctx context.Context, realm string, zone string) (map[string]keycloakGroupSnapshot, error) {
	groups, err := s.Keycloak.ListGroups(ctx, realm)
	if err != nil {
		return nil, err
	}

	snapshot := map[string]keycloakGroupSnapshot{}
	for _, group := range groups {
		groupSnapshot, ok, err := s.buildKeycloakGroupSnapshot(ctx, realm, zone, group)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		snapshot[groupSnapshot.Name] = groupSnapshot
	}
	return snapshot, nil
}

func (s *Service) buildKeycloakGroupSnapshot(ctx context.Context, realm string, zone string, group keycloakadmin.Group) (keycloakGroupSnapshot, bool, error) {
	groupName, groupZone, ok := s.keycloakGroupMapping(realm, zone, group)
	if !ok {
		return keycloakGroupSnapshot{}, false, nil
	}

	groupPath := planvalidator.NormalizeGroupPath(group.Path)
	if groupPath == "" {
		groupPath = s.mirrorPolicy().GroupPath(groupName)
	}
	groupID := strings.TrimSpace(group.ID)
	if groupID == "" {
		groupID = groupPath
	}
	members, err := s.Keycloak.ListGroupMembers(ctx, realm, groupID)
	if err != nil {
		return keycloakGroupSnapshot{}, false, err
	}
	return keycloakGroupSnapshot{
		ID:      groupID,
		Name:    groupName,
		Path:    groupPath,
		Zone:    groupZone,
		Members: keycloakMemberSet(members),
	}, true, nil
}

func (s *Service) keycloakGroupMapping(realm string, zone string, group keycloakadmin.Group) (string, string, bool) {
	mirrorPolicy := s.mirrorPolicy()
	path := planvalidator.NormalizeGroupPath(group.Path)
	mirrorName := firstAttribute(group.Attributes, mirrorAttrGroupName)
	authority := strings.ToLower(firstAttribute(group.Attributes, mirrorAttrAuthority))
	if mirrorName == "" && authority != domain.SyncPlanAuthorityIRODS && !mirrorPolicy.IsManagedPath(path) {
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
		groupName = mirrorPolicy.GroupNameFromPath(path)
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

func firstAttribute(attributes map[string][]string, name string) string {
	values := attributes[name]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
