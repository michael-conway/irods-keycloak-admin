package repair

import (
	"context"
	"reflect"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/mapper"
)

func TestRepairKeycloakPlansMissingMirrorGroupAndMembership(t *testing.T) {
	irods := &fakeIRODSClient{
		groups: []*irodstypes.IRODSUser{{
			ID:   100,
			Name: "project-alpha",
			Zone: "tempZone",
			Type: irodstypes.IRODSUserRodsGroup,
		}},
		members: map[string][]*irodstypes.IRODSUser{
			"project-alpha": {
				{ID: 200, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	keycloak := &fakeKeycloakClient{}
	service := Service{
		IRODS:        irods,
		Keycloak:     keycloak,
		Mapper:       mapper.Mapper{DefaultZone: "tempZone"},
		DefaultRealm: "example",
	}

	plan, err := service.RepairKeycloak(context.Background(), domain.RepairRequest{})
	if err != nil {
		t.Fatalf("RepairKeycloak returned error: %v", err)
	}

	if plan.Mode != "repair-keycloak" || plan.Authority != "irods" || plan.Realm != "example" || plan.Zone != "tempZone" {
		t.Fatalf("unexpected plan metadata: %+v", plan)
	}
	if plan.Summary.CreateKeycloakGroups != 1 {
		t.Fatalf("expected one group create, got %+v", plan.Summary)
	}
	if plan.Summary.UpdateKeycloakMemberships != 1 {
		t.Fatalf("expected one membership update, got %+v", plan.Summary)
	}
	assertActions(t, plan, []string{
		"keycloak.group.create",
		"keycloak.group.member.add",
	})
	assertTargets(t, plan, []string{
		"/irods/project-alpha",
		"/irods/project-alpha#member:alice",
	})
	if keycloak.createCalls != 0 || keycloak.addMemberCalls != 0 || keycloak.removeMemberCalls != 0 || keycloak.deleteGroupCalls != 0 {
		t.Fatalf("repair planning must not mutate Keycloak, fake calls: %+v", keycloak)
	}
}

func TestRepairKeycloakPlansMembershipDriftAndStaleMirror(t *testing.T) {
	irods := &fakeIRODSClient{
		groups: []*irodstypes.IRODSUser{{
			ID:   100,
			Name: "project-alpha",
			Zone: "tempZone",
			Type: irodstypes.IRODSUserRodsGroup,
		}},
		members: map[string][]*irodstypes.IRODSUser{
			"project-alpha": {
				{ID: 200, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	keycloak := &fakeKeycloakClient{
		groups: []keycloakadmin.Group{
			{
				ID:   "kc-project-alpha",
				Name: "project-alpha",
				Path: "/irods/project-alpha",
				Attributes: map[string][]string{
					"irods_group_name": {"project-alpha"},
					"irods_zone":       {"tempZone"},
					"authority":        {"irods"},
				},
			},
			{
				ID:   "kc-stale",
				Name: "stale-team",
				Path: "/irods/stale-team",
				Attributes: map[string][]string{
					"irods_group_name": {"stale-team"},
					"irods_zone":       {"tempZone"},
					"authority":        {"irods"},
				},
			},
			{
				ID:   "unmanaged",
				Name: "unmanaged",
				Path: "/unmanaged",
			},
		},
		members: map[string][]keycloakadmin.User{
			"kc-project-alpha": {
				{ID: "kc-bob", Username: "bob"},
			},
			"kc-stale": {
				{ID: "kc-carol", Username: "carol"},
			},
		},
	}
	service := Service{
		IRODS:        irods,
		Keycloak:     keycloak,
		Mapper:       mapper.Mapper{DefaultZone: "tempZone"},
		DefaultRealm: "example",
	}

	plan, err := service.RepairKeycloak(context.Background(), domain.RepairRequest{})
	if err != nil {
		t.Fatalf("RepairKeycloak returned error: %v", err)
	}

	if plan.Summary.CreateKeycloakGroups != 0 {
		t.Fatalf("expected no group creates, got %+v", plan.Summary)
	}
	if plan.Summary.UpdateKeycloakMemberships != 2 {
		t.Fatalf("expected two membership updates, got %+v", plan.Summary)
	}
	if plan.Summary.DeleteKeycloakMirrors != 1 || plan.Summary.RequiresApproval != 1 {
		t.Fatalf("expected one guarded stale mirror delete, got %+v", plan.Summary)
	}
	assertActions(t, plan, []string{
		"keycloak.group.member.add",
		"keycloak.group.member.remove",
		"keycloak.group.delete",
	})
	assertTargets(t, plan, []string{
		"/irods/project-alpha#member:alice",
		"/irods/project-alpha#member:bob",
		"/irods/stale-team",
	})
	if keycloak.listMembersCalls["unmanaged"] != 0 {
		t.Fatal("unmanaged Keycloak groups should be ignored")
	}
}

func TestRepairKeycloakRequiresRealmAndZone(t *testing.T) {
	service := Service{
		IRODS:    &fakeIRODSClient{},
		Keycloak: &fakeKeycloakClient{},
	}

	if _, err := service.RepairKeycloak(context.Background(), domain.RepairRequest{}); err == nil {
		t.Fatal("expected missing realm or zone error")
	}
}

func assertActions(t *testing.T, plan domain.SyncPlan, want []string) {
	t.Helper()
	got := make([]string, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		got = append(got, operation.Action)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected actions:\nwant %+v\ngot  %+v", want, got)
	}
}

func assertTargets(t *testing.T, plan domain.SyncPlan, want []string) {
	t.Helper()
	got := make([]string, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		got = append(got, operation.Target)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected targets:\nwant %+v\ngot  %+v", want, got)
	}
}

type fakeIRODSClient struct {
	groups  []*irodstypes.IRODSUser
	members map[string][]*irodstypes.IRODSUser
}

func (f *fakeIRODSClient) GetUser(context.Context, string, string) (*irodstypes.IRODSUser, error) {
	return nil, nil
}

func (f *fakeIRODSClient) CreateUser(context.Context, string, string, irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	return nil, nil
}

func (f *fakeIRODSClient) RemoveUser(context.Context, string, string, irodstypes.IRODSUserType) error {
	return nil
}

func (f *fakeIRODSClient) ListUsers(_ context.Context, _ string, userType irodstypes.IRODSUserType) ([]*irodstypes.IRODSUser, error) {
	if userType != irodstypes.IRODSUserRodsGroup {
		return nil, nil
	}
	return append([]*irodstypes.IRODSUser(nil), f.groups...), nil
}

func (f *fakeIRODSClient) ListGroupMembers(_ context.Context, _ string, groupName string) ([]*irodstypes.IRODSUser, error) {
	return append([]*irodstypes.IRODSUser(nil), f.members[groupName]...), nil
}

func (f *fakeIRODSClient) AddGroupMember(context.Context, string, string, string) error {
	return nil
}

func (f *fakeIRODSClient) RemoveGroupMember(context.Context, string, string, string) error {
	return nil
}

func (f *fakeIRODSClient) AddUserMetadata(context.Context, string, string, *irodstypes.IRODSMeta) error {
	return nil
}

func (f *fakeIRODSClient) ListUserMetadata(context.Context, string, string) ([]*irodstypes.IRODSMeta, error) {
	return nil, nil
}

type fakeKeycloakClient struct {
	groups            []keycloakadmin.Group
	members           map[string][]keycloakadmin.User
	listMembersCalls  map[string]int
	createCalls       int
	addMemberCalls    int
	removeMemberCalls int
	deleteGroupCalls  int
}

func (f *fakeKeycloakClient) GetUser(context.Context, string, string) (*keycloakadmin.User, error) {
	return nil, nil
}

func (f *fakeKeycloakClient) FindUserByUsername(context.Context, string, string) (*keycloakadmin.User, error) {
	return nil, nil
}

func (f *fakeKeycloakClient) ListGroups(context.Context, string) ([]keycloakadmin.Group, error) {
	return append([]keycloakadmin.Group(nil), f.groups...), nil
}

func (f *fakeKeycloakClient) ListGroupMembers(_ context.Context, _ string, groupID string) ([]keycloakadmin.User, error) {
	if f.listMembersCalls == nil {
		f.listMembersCalls = map[string]int{}
	}
	f.listMembersCalls[groupID]++
	return append([]keycloakadmin.User(nil), f.members[groupID]...), nil
}

func (f *fakeKeycloakClient) CreateOrUpdateUser(context.Context, string, keycloakadmin.User) (*keycloakadmin.User, error) {
	return nil, nil
}

func (f *fakeKeycloakClient) CreateOrUpdateGroup(context.Context, string, keycloakadmin.Group) (*keycloakadmin.Group, error) {
	f.createCalls++
	return nil, nil
}

func (f *fakeKeycloakClient) DeleteGroup(context.Context, string, string) error {
	f.deleteGroupCalls++
	return nil
}

func (f *fakeKeycloakClient) AddUserToGroup(context.Context, string, string, string) error {
	f.addMemberCalls++
	return nil
}

func (f *fakeKeycloakClient) RemoveUserFromGroup(context.Context, string, string, string) error {
	f.removeMemberCalls++
	return nil
}
