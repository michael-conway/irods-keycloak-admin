package repair

import (
	"context"
	"reflect"
	"strings"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/mapper"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
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
	if plan.PlanFormatVersion != domain.SyncPlanFormatVersion {
		t.Fatalf("unexpected plan format version: %q", plan.PlanFormatVersion)
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
	assertEvidenceValue(t, plan.Operations[0], "keycloak_group_id", "kc-project-alpha")
	assertEvidenceValue(t, plan.Operations[1], "keycloak_group_id", "kc-project-alpha")
	assertEvidenceValue(t, plan.Operations[2], "keycloak_group_id", "kc-stale")
	if keycloak.listMembersCalls["unmanaged"] != 0 {
		t.Fatal("unmanaged Keycloak groups should be ignored")
	}
}

func TestRepairKeycloakOrdersOperationsDeterministically(t *testing.T) {
	irods := &fakeIRODSClient{
		groups: []*irodstypes.IRODSUser{
			{ID: 100, Name: "z-team", Zone: "tempZone", Type: irodstypes.IRODSUserRodsGroup},
			{ID: 101, Name: "a-team", Zone: "tempZone", Type: irodstypes.IRODSUserRodsGroup},
		},
		members: map[string][]*irodstypes.IRODSUser{
			"z-team": {
				{ID: 200, Name: "zoe", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
				{ID: 201, Name: "amy", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
			"a-team": {
				{ID: 202, Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
				{ID: 203, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	service := Service{
		IRODS:        irods,
		Keycloak:     &fakeKeycloakClient{},
		Mapper:       mapper.Mapper{DefaultZone: "tempZone"},
		DefaultRealm: "example",
	}

	plan, err := service.RepairKeycloak(context.Background(), domain.RepairRequest{})
	if err != nil {
		t.Fatalf("RepairKeycloak returned error: %v", err)
	}

	assertOperationIDs(t, plan, []string{"op-001", "op-002", "op-003", "op-004", "op-005", "op-006"})
	assertTargets(t, plan, []string{
		"/irods/a-team",
		"/irods/a-team#member:alice",
		"/irods/a-team#member:bob",
		"/irods/z-team",
		"/irods/z-team#member:amy",
		"/irods/z-team#member:zoe",
	})
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

func TestApplyKeycloakRepairPlanMutatesOnlyKeycloak(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{
		{
			OperationID: "op-001",
			Action:      domain.PlanActionKeycloakGroupCreate,
			Target:      "/irods/project-alpha",
			Risk:        "low",
			Evidence: map[string]any{
				"irods_group_name": "project-alpha",
				"irods_zone":       "tempZone",
				"keycloak_realm":   "example",
				"keycloak_path":    "/irods/project-alpha",
			},
		},
		{
			OperationID: "op-002",
			Action:      domain.PlanActionKeycloakGroupMemberAdd,
			Target:      "/irods/project-alpha#member:alice",
			Risk:        "low",
			Evidence: map[string]any{
				"irods_group_name": "project-alpha",
				"irods_username":   "alice",
				"irods_zone":       "tempZone",
				"keycloak_realm":   "example",
				"keycloak_path":    "/irods/project-alpha",
			},
		},
		{
			OperationID: "op-003",
			Action:      domain.PlanActionKeycloakGroupMemberRemove,
			Target:      "/irods/project-alpha#member:bob",
			Risk:        "medium",
			Evidence: map[string]any{
				"irods_group_name":  "project-alpha",
				"keycloak_user":     "bob",
				"keycloak_user_id":  "kc-bob",
				"keycloak_group_id": "kc-project-alpha",
				"irods_zone":        "tempZone",
				"keycloak_realm":    "example",
				"keycloak_path":     "/irods/project-alpha",
			},
		},
	})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "applied" || result.Applied != 3 || result.Skipped != 0 || result.Failed != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(keycloak.createdGroups) != 1 {
		t.Fatalf("expected one group create/update, got %+v", keycloak.createdGroups)
	}
	created := keycloak.createdGroups[0]
	if created.Path != "/irods/project-alpha" || created.Name != "project-alpha" {
		t.Fatalf("unexpected created group: %+v", created)
	}
	if created.Attributes["authority"][0] != "irods" || created.Attributes["irods_zone"][0] != "tempZone" {
		t.Fatalf("unexpected created group attributes: %+v", created.Attributes)
	}
	if len(keycloak.addMemberRequests) != 1 || keycloak.addMemberRequests[0] != "example|alice|/irods/project-alpha" {
		t.Fatalf("unexpected add member requests: %+v", keycloak.addMemberRequests)
	}
	if len(keycloak.removeMemberRequests) != 1 || keycloak.removeMemberRequests[0] != "example|kc-bob|kc-project-alpha" {
		t.Fatalf("unexpected remove member requests: %+v", keycloak.removeMemberRequests)
	}
	if keycloak.deleteGroupCalls != 0 {
		t.Fatalf("apply should not delete without delete operations, got %d", keycloak.deleteGroupCalls)
	}
}

func TestApplyRequiresReviewerForApprovalRequiredOperations(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakGroupDelete,
		Target:      "/irods/stale-team",
		Risk:        domain.PlanRiskRequiresApproval,
		Evidence: map[string]any{
			"irods_group_name":  "stale-team",
			"keycloak_group_id": "kc-stale",
			"irods_zone":        "tempZone",
			"keycloak_realm":    "example",
			"keycloak_path":     "/irods/stale-team",
		},
	}})

	_, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a prompt") {
		t.Fatalf("expected prompt error, got %v", err)
	}
	if keycloak.deleteGroupCalls != 0 {
		t.Fatalf("delete should be rejected before mutation, got %d calls", keycloak.deleteGroupCalls)
	}
}

func TestApplyDeletesMirrorWhenPromptsNone(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak, PromptMode: planreview.PromptModeNone}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakGroupDelete,
		Target:      "/irods/stale-team",
		Risk:        domain.PlanRiskRequiresApproval,
		Evidence: map[string]any{
			"irods_group_name":  "stale-team",
			"keycloak_group_id": "kc-stale",
			"irods_zone":        "tempZone",
			"keycloak_realm":    "example",
			"keycloak_path":     "/irods/stale-team",
		},
	}})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "applied" || result.Applied != 1 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(keycloak.deleteGroupRequests) != 1 || keycloak.deleteGroupRequests[0] != "example|kc-stale" {
		t.Fatalf("unexpected delete requests: %+v", keycloak.deleteGroupRequests)
	}
}

func TestApplySkipsApprovalOperationWhenReviewerSkips(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{
		Keycloak:   keycloak,
		Reviewer:   &fakePlanReviewer{decisions: []planreview.Decision{planreview.DecisionSkip}},
		PromptMode: planreview.PromptModeRequired,
	}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakGroupDelete,
		Target:      "/irods/stale-team",
		Risk:        domain.PlanRiskRequiresApproval,
		Evidence: map[string]any{
			"irods_group_name":  "stale-team",
			"keycloak_group_id": "kc-stale",
			"irods_zone":        "tempZone",
			"keycloak_realm":    "example",
			"keycloak_path":     "/irods/stale-team",
		},
	}})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "skipped" || result.Applied != 0 || result.Skipped != 1 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if keycloak.deleteGroupCalls != 0 {
		t.Fatalf("delete should be skipped, got %d calls", keycloak.deleteGroupCalls)
	}
}

func TestApplyAcceptAllSwitchesToPromptsNone(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	reviewer := &fakePlanReviewer{decisions: []planreview.Decision{planreview.DecisionAcceptAll}}
	service := Service{
		Keycloak:   keycloak,
		Reviewer:   reviewer,
		PromptMode: planreview.PromptModeAll,
	}
	plan := testApplyPlan([]domain.PlanOperation{
		{
			OperationID: "op-001",
			Action:      domain.PlanActionKeycloakGroupCreate,
			Target:      "/irods/project-alpha",
			Risk:        "low",
			Evidence: map[string]any{
				"irods_group_name": "project-alpha",
				"irods_zone":       "tempZone",
				"keycloak_realm":   "example",
				"keycloak_path":    "/irods/project-alpha",
			},
		},
		{
			OperationID: "op-002",
			Action:      domain.PlanActionKeycloakGroupDelete,
			Target:      "/irods/stale-team",
			Risk:        domain.PlanRiskRequiresApproval,
			Evidence: map[string]any{
				"irods_group_name":  "stale-team",
				"keycloak_group_id": "kc-stale",
				"irods_zone":        "tempZone",
				"keycloak_realm":    "example",
				"keycloak_path":     "/irods/stale-team",
			},
		},
	})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "applied" || result.Applied != 2 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(reviewer.reviewed) != 1 || reviewer.reviewed[0] != "op-001" {
		t.Fatalf("unexpected reviewed operations: %+v", reviewer.reviewed)
	}
	if keycloak.deleteGroupCalls != 1 {
		t.Fatalf("delete should be applied after accept_all, got %d calls", keycloak.deleteGroupCalls)
	}
}

func testApplyPlan(operations []domain.PlanOperation) domain.SyncPlan {
	return domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            "plan-test",
		Mode:              domain.SyncPlanModeRepairKeycloak,
		Authority:         domain.SyncPlanAuthorityIRODS,
		Realm:             "example",
		Zone:              "tempZone",
		Operations:        operations,
	}
}

func assertOperationIDs(t *testing.T, plan domain.SyncPlan, want []string) {
	t.Helper()
	got := make([]string, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		got = append(got, operation.OperationID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected operation IDs:\nwant %+v\ngot  %+v", want, got)
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

func assertEvidenceValue(t *testing.T, operation domain.PlanOperation, key string, want any) {
	t.Helper()
	if operation.Evidence[key] != want {
		t.Fatalf("unexpected evidence %q for operation %+v: want %v got %v", key, operation, want, operation.Evidence[key])
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
	groups               []keycloakadmin.Group
	members              map[string][]keycloakadmin.User
	listMembersCalls     map[string]int
	createCalls          int
	createdGroups        []keycloakadmin.Group
	addMemberCalls       int
	addMemberRequests    []string
	removeMemberCalls    int
	removeMemberRequests []string
	deleteGroupCalls     int
	deleteGroupRequests  []string
}

type fakePlanReviewer struct {
	decisions []planreview.Decision
	reviewed  []string
}

func (f *fakePlanReviewer) Review(_ context.Context, _ domain.SyncPlan, operation domain.PlanOperation) (planreview.Decision, error) {
	f.reviewed = append(f.reviewed, operation.OperationID)
	if len(f.decisions) == 0 {
		return planreview.DecisionAccept, nil
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return decision, nil
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

func (f *fakeKeycloakClient) CreateOrUpdateGroup(_ context.Context, _ string, group keycloakadmin.Group) (*keycloakadmin.Group, error) {
	f.createCalls++
	f.createdGroups = append(f.createdGroups, group)
	return &group, nil
}

func (f *fakeKeycloakClient) DeleteGroup(_ context.Context, realm string, groupIDOrPath string) error {
	f.deleteGroupCalls++
	f.deleteGroupRequests = append(f.deleteGroupRequests, realm+"|"+groupIDOrPath)
	return nil
}

func (f *fakeKeycloakClient) AddUserToGroup(_ context.Context, realm string, userIDOrUsername string, groupIDOrPath string) error {
	f.addMemberCalls++
	f.addMemberRequests = append(f.addMemberRequests, realm+"|"+userIDOrUsername+"|"+groupIDOrPath)
	return nil
}

func (f *fakeKeycloakClient) RemoveUserFromGroup(_ context.Context, realm string, userIDOrUsername string, groupIDOrPath string) error {
	f.removeMemberCalls++
	f.removeMemberRequests = append(f.removeMemberRequests, realm+"|"+userIDOrUsername+"|"+groupIDOrPath)
	return nil
}
