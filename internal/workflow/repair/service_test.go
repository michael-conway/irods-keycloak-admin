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
		users: []*irodstypes.IRODSUser{
			{ID: 200, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
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
		users: map[string]keycloakadmin.User{
			"alice": {ID: "kc-alice", Username: "alice"},
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

	if plan.Mode != "sync" || plan.Authority != "irods" || plan.Realm != "example" || plan.Zone != "tempZone" {
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
	assertEvidenceValue(t, plan.Operations[0], "change_cause", "missing_mirror_group")
	assertEvidenceValue(t, plan.Operations[1], "change_cause", "missing_mirror_group")
	assertEvidenceValue(t, plan.Operations[0], "sync_direction", domain.SyncDirectionIRODSToKeycloak)
	assertEvidenceValue(t, plan.Operations[0], "sync_classification", domain.SyncClassificationCandidateAddition)
	assertEvidenceValue(t, plan.Operations[1], "sync_classification", domain.SyncClassificationCandidateAddition)
	if keycloak.createCalls != 0 || keycloak.addMemberCalls != 0 || keycloak.removeMemberCalls != 0 || keycloak.deleteGroupCalls != 0 {
		t.Fatalf("repair planning must not mutate Keycloak, fake calls: %+v", keycloak)
	}
}

func TestRepairKeycloakUsesConfiguredMirrorRoot(t *testing.T) {
	irods := &fakeIRODSClient{
		users: []*irodstypes.IRODSUser{
			{ID: 200, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
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
		users: map[string]keycloakadmin.User{
			"alice": {ID: "kc-alice", Username: "alice"},
		},
	}
	service := Service{
		IRODS:        irods,
		Keycloak:     keycloak,
		Mapper:       mapper.Mapper{DefaultZone: "tempZone"},
		DefaultRealm: "example",
		MirrorRoot:   "/kc-irods",
	}

	plan, err := service.RepairKeycloak(context.Background(), domain.RepairRequest{})
	if err != nil {
		t.Fatalf("RepairKeycloak returned error: %v", err)
	}
	if plan.KeycloakMirrorRoot != "/kc-irods" {
		t.Fatalf("unexpected plan mirror root: %q", plan.KeycloakMirrorRoot)
	}

	assertTargets(t, plan, []string{
		"/kc-irods/project-alpha",
		"/kc-irods/project-alpha#member:alice",
	})
}

func TestRepairKeycloakPlansMembershipDriftAndStaleMirror(t *testing.T) {
	irods := &fakeIRODSClient{
		users: []*irodstypes.IRODSUser{
			{ID: 200, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
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
		users: map[string]keycloakadmin.User{
			"alice": {ID: "kc-alice", Username: "alice"},
		},
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
	assertEvidenceValue(t, plan.Operations[0], "change_cause", "membership_drift")
	assertEvidenceValue(t, plan.Operations[1], "change_cause", "membership_drift")
	assertEvidenceValue(t, plan.Operations[2], "change_cause", "stale_keycloak_state")
	assertEvidenceValue(t, plan.Operations[0], "keycloak_group_id", "kc-project-alpha")
	assertEvidenceValue(t, plan.Operations[0], "keycloak_user_id", "kc-alice")
	assertEvidenceValue(t, plan.Operations[1], "keycloak_group_id", "kc-project-alpha")
	assertEvidenceValue(t, plan.Operations[2], "keycloak_group_id", "kc-stale")
	assertEvidenceValue(t, plan.Operations[0], "sync_classification", domain.SyncClassificationCandidateAddition)
	assertEvidenceValue(t, plan.Operations[1], "sync_classification", domain.SyncClassificationCandidateRemoval)
	assertEvidenceValue(t, plan.Operations[2], "sync_classification", domain.SyncClassificationCandidateRemoval)
	assertEvidenceValue(t, plan.Operations[2], "authority_role", "directional_repair_policy")
	if keycloak.listMembersCalls["unmanaged"] != 0 {
		t.Fatal("unmanaged Keycloak groups should be ignored")
	}
}

func TestRepairKeycloakIgnoresMirrorRootContainer(t *testing.T) {
	irods := &fakeIRODSClient{
		users: []*irodstypes.IRODSUser{
			{ID: 200, Name: "anonymous", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		groups: []*irodstypes.IRODSUser{{
			ID:   100,
			Name: "public",
			Zone: "tempZone",
			Type: irodstypes.IRODSUserRodsGroup,
		}},
		members: map[string][]*irodstypes.IRODSUser{
			"public": {
				{ID: 200, Name: "anonymous", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	keycloak := &fakeKeycloakClient{
		users: map[string]keycloakadmin.User{
			"anonymous": {ID: "kc-anonymous", Username: "anonymous"},
		},
		groups: []keycloakadmin.Group{
			{
				ID:   "kc-root",
				Name: "irods",
				Path: "/irods",
			},
			{
				ID:   "kc-public",
				Name: "public",
				Path: "/irods/public",
				Attributes: map[string][]string{
					"irods_group_name": {"public"},
					"irods_zone":       {"tempZone"},
					"authority":        {"irods"},
				},
			},
		},
		members: map[string][]keycloakadmin.User{
			"kc-public": {},
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

	if plan.Summary.DeleteKeycloakMirrors != 0 || plan.Summary.RequiresApproval != 0 {
		t.Fatalf("mirror root container should not be planned for deletion, got %+v", plan.Summary)
	}
	assertActions(t, plan, []string{
		domain.PlanActionKeycloakGroupMemberAdd,
	})
	assertTargets(t, plan, []string{
		"/irods/public#member:anonymous",
	})
	if keycloak.listMembersCalls["kc-root"] != 0 {
		t.Fatal("mirror root container should not be read as an iRODS group")
	}
}

func TestRepairKeycloakPlansMissingMirrorUser(t *testing.T) {
	irods := &fakeIRODSClient{
		users: []*irodstypes.IRODSUser{
			{ID: 200, Name: "frog", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
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

	if plan.Summary.CreateKeycloakUsers != 1 {
		t.Fatalf("expected one user create, got %+v", plan.Summary)
	}
	assertActions(t, plan, []string{
		domain.PlanActionKeycloakUserCreate,
	})
	assertTargets(t, plan, []string{
		"frog",
	})
	assertEvidenceValue(t, plan.Operations[0], "change_cause", "missing_mirror_user")
	assertEvidenceValue(t, plan.Operations[0], "irods_username", "frog")
	assertEvidenceValue(t, plan.Operations[0], "keycloak_username", "frog")
}

func TestRepairKeycloakOrdersOperationsDeterministically(t *testing.T) {
	irods := &fakeIRODSClient{
		users: []*irodstypes.IRODSUser{
			{ID: 200, Name: "zoe", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			{ID: 201, Name: "amy", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			{ID: 202, Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			{ID: 203, Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
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
	keycloak := &fakeKeycloakClient{
		users: map[string]keycloakadmin.User{
			"alice": {ID: "kc-alice", Username: "alice"},
			"amy":   {ID: "kc-amy", Username: "amy"},
			"bob":   {ID: "kc-bob", Username: "bob"},
			"zoe":   {ID: "kc-zoe", Username: "zoe"},
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
	if len(keycloak.removeMemberRequests) != 1 || keycloak.removeMemberRequests[0] != "example|kc-bob|/irods/project-alpha" {
		t.Fatalf("unexpected remove member requests: %+v", keycloak.removeMemberRequests)
	}
	if keycloak.deleteGroupCalls != 0 {
		t.Fatalf("apply should not delete without delete operations, got %d", keycloak.deleteGroupCalls)
	}
}

func TestApplyCreatesKeycloakUser(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakUserCreate,
		Target:      "frog",
		Risk:        "low",
		Evidence: map[string]any{
			"irods_username":    "frog",
			"irods_zone":        "tempZone",
			"keycloak_realm":    "example",
			"keycloak_username": "frog",
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
	if len(keycloak.createdUsers) != 1 {
		t.Fatalf("expected one created user, got %+v", keycloak.createdUsers)
	}
	created := keycloak.createdUsers[0]
	if created.Username != "frog" {
		t.Fatalf("unexpected created user: %+v", created)
	}
	if created.Attributes["irods_username"][0] != "frog" || created.Attributes["irods_zone"][0] != "tempZone" {
		t.Fatalf("unexpected created user attributes: %+v", created.Attributes)
	}
	if result.Operations[0].KeycloakMirror.User != "frog" {
		t.Fatalf("unexpected mutation result: %+v", result.Operations[0])
	}
}

func TestApplyUsesConfiguredMirrorRootForCreateTargets(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak, MirrorRoot: "/kc-irods"}
	plan := testApplyPlan([]domain.PlanOperation{
		{
			OperationID: "op-001",
			Action:      domain.PlanActionKeycloakGroupCreate,
			Target:      "/kc-irods/project-alpha",
			Risk:        "low",
			Evidence: map[string]any{
				"irods_group_name": "project-alpha",
				"irods_zone":       "tempZone",
				"keycloak_realm":   "example",
				"keycloak_path":    "/kc-irods/project-alpha",
			},
		},
	})
	plan.KeycloakMirrorRoot = "/kc-irods"

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "applied" || result.Applied != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(keycloak.createdGroups) != 1 || keycloak.createdGroups[0].Path != "/kc-irods/project-alpha" {
		t.Fatalf("unexpected created groups: %+v", keycloak.createdGroups)
	}
}

func TestApplyRejectsPlanWithMismatchedRuntimeMirrorRoot(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak, MirrorRoot: "/kc-irods"}
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
	})

	_, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err == nil || !strings.Contains(err.Error(), "plan keycloak mirror root does not match runtime configuration") {
		t.Fatalf("expected mirror root mismatch error, got %v", err)
	}
	if len(keycloak.createdGroups) != 0 {
		t.Fatalf("apply should fail before mutation, got %+v", keycloak.createdGroups)
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
	if len(keycloak.deleteGroupRequests) != 1 || keycloak.deleteGroupRequests[0] != "example|/irods/stale-team" {
		t.Fatalf("unexpected delete requests: %+v", keycloak.deleteGroupRequests)
	}
}

func TestApplyMarksConvergedOperationsUnchanged(t *testing.T) {
	keycloak := &fakeKeycloakClient{
		createGroupOutcome: keycloakadmin.MutationOutcomeUnchanged,
	}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
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
	if len(result.Operations) != 1 || result.Operations[0].Status != "unchanged" {
		t.Fatalf("unexpected operation results: %+v", result.Operations)
	}
}

func TestApplyReportsSpecificUserNotFoundFailure(t *testing.T) {
	keycloak := &fakeKeycloakClient{
		addMemberErr: &keycloakadmin.UserNotFoundError{Realm: "example", Ref: "alice"},
	}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
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
	}})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "failed" || result.Applied != 0 || result.Failed != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(result.Operations) != 1 || len(result.Operations[0].Warnings) != 1 {
		t.Fatalf("unexpected operation warnings: %+v", result.Operations)
	}
	if result.Operations[0].Warnings[0].Code != "apply.keycloak.user_not_found" {
		t.Fatalf("unexpected warning code: %+v", result.Operations[0].Warnings)
	}
}

func TestApplyReportsSpecificGroupNotFoundFailure(t *testing.T) {
	keycloak := &fakeKeycloakClient{
		addMemberErr: &keycloakadmin.GroupNotFoundError{Realm: "example", Ref: "/irods/project-alpha"},
	}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
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
	}})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "failed" || result.Applied != 0 || result.Failed != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if result.Operations[0].Warnings[0].Code != "apply.keycloak.group_not_found" {
		t.Fatalf("unexpected warning code: %+v", result.Operations[0].Warnings)
	}
}

func TestApplyUsesPathFallbackWhenGroupIDEvidenceMissing(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakGroupMemberRemove,
		Target:      "/irods/project-alpha#member:bob",
		Risk:        "medium",
		Evidence: map[string]any{
			"irods_group_name": "project-alpha",
			"keycloak_user":    "bob",
			"keycloak_user_id": "kc-bob",
			"irods_zone":       "tempZone",
			"keycloak_realm":   "example",
			"keycloak_path":    "/irods/project-alpha",
		},
	}})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "applied" || result.Applied != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(keycloak.removeMemberRequests) != 1 || keycloak.removeMemberRequests[0] != "example|kc-bob|/irods/project-alpha" {
		t.Fatalf("unexpected remove member requests: %+v", keycloak.removeMemberRequests)
	}
}

func TestApplyReportsPartialFailureAcrossOperations(t *testing.T) {
	keycloak := &fakeKeycloakClient{
		addMemberErr: &keycloakadmin.UserNotFoundError{Realm: "example", Ref: "alice"},
	}
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
	})

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Status != "failed" || result.Applied != 1 || result.Failed != 1 || result.Skipped != 0 || result.WarningCount != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(result.Operations) != 2 {
		t.Fatalf("unexpected operation results: %+v", result.Operations)
	}
	if result.Operations[0].Status != "applied" || result.Operations[1].Status != "failed" {
		t.Fatalf("unexpected operation statuses: %+v", result.Operations)
	}
	if result.Operations[1].Warnings[0].Code != "apply.keycloak.user_not_found" {
		t.Fatalf("unexpected failed operation warning: %+v", result.Operations[1].Warnings)
	}
}

func TestApplyReportsUnsupportedOperationSpecifically(t *testing.T) {
	keycloak := &fakeKeycloakClient{}
	service := Service{Keycloak: keycloak}
	plan := testApplyPlan([]domain.PlanOperation{{
		OperationID: "op-001",
		Action:      "keycloak.group.rename",
		Target:      "/irods/project-alpha",
		Risk:        "low",
	}})

	_, err := service.Apply(context.Background(), domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{Realm: "example", Zone: "tempZone"},
		Plan:            &plan,
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported action "keycloak.group.rename"`) {
		t.Fatalf("expected unsupported operation validation error, got %v", err)
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
		PlanFormatVersion:  domain.SyncPlanFormatVersion,
		PlanID:             "plan-test",
		Mode:               domain.SyncPlanModeSync,
		Authority:          domain.SyncPlanAuthorityIRODS,
		Realm:              "example",
		Zone:               "tempZone",
		KeycloakMirrorRoot: "/irods",
		Operations:         operations,
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
	users   []*irodstypes.IRODSUser
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
	if userType == irodstypes.IRODSUserRodsUser {
		return append([]*irodstypes.IRODSUser(nil), f.users...), nil
	}
	if userType == irodstypes.IRODSUserRodsGroup {
		return append([]*irodstypes.IRODSUser(nil), f.groups...), nil
	}
	return nil, nil
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
	users                map[string]keycloakadmin.User
	groups               []keycloakadmin.Group
	members              map[string][]keycloakadmin.User
	listMembersCalls     map[string]int
	createUserCalls      int
	createdUsers         []keycloakadmin.User
	createUserErr        error
	createCalls          int
	createdGroups        []keycloakadmin.Group
	createGroupOutcome   keycloakadmin.MutationOutcome
	createGroupErr       error
	addMemberCalls       int
	addMemberRequests    []string
	addMemberOutcome     keycloakadmin.MutationOutcome
	addMemberErr         error
	removeMemberCalls    int
	removeMemberRequests []string
	removeMemberOutcome  keycloakadmin.MutationOutcome
	removeMemberErr      error
	deleteGroupCalls     int
	deleteGroupRequests  []string
	deleteGroupOutcome   keycloakadmin.MutationOutcome
	deleteGroupErr       error
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

func (f *fakeKeycloakClient) FindUserByUsername(_ context.Context, _ string, username string) (*keycloakadmin.User, error) {
	user, ok := f.users[strings.TrimSpace(username)]
	if !ok {
		return nil, nil
	}
	userCopy := user
	if userCopy.Username == "" {
		userCopy.Username = strings.TrimSpace(username)
	}
	return &userCopy, nil
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

func (f *fakeKeycloakClient) CreateOrUpdateUser(_ context.Context, _ string, user keycloakadmin.User) (*keycloakadmin.User, error) {
	f.createUserCalls++
	f.createdUsers = append(f.createdUsers, user)
	if f.createUserErr != nil {
		return nil, f.createUserErr
	}
	return &user, nil
}

func (f *fakeKeycloakClient) CreateOrUpdateGroup(_ context.Context, _ string, group keycloakadmin.Group) (*keycloakadmin.Group, keycloakadmin.MutationOutcome, error) {
	f.createCalls++
	f.createdGroups = append(f.createdGroups, group)
	if f.createGroupErr != nil {
		return nil, "", f.createGroupErr
	}
	if f.createGroupOutcome == "" {
		f.createGroupOutcome = keycloakadmin.MutationOutcomeUpdated
	}
	return &group, f.createGroupOutcome, nil
}

func (f *fakeKeycloakClient) DeleteGroup(_ context.Context, realm string, groupIDOrPath string) (keycloakadmin.MutationOutcome, error) {
	f.deleteGroupCalls++
	f.deleteGroupRequests = append(f.deleteGroupRequests, realm+"|"+groupIDOrPath)
	if f.deleteGroupErr != nil {
		return "", f.deleteGroupErr
	}
	if f.deleteGroupOutcome == "" {
		f.deleteGroupOutcome = keycloakadmin.MutationOutcomeDeleted
	}
	return f.deleteGroupOutcome, nil
}

func (f *fakeKeycloakClient) AddUserToGroup(_ context.Context, realm string, userIDOrUsername string, groupIDOrPath string) (keycloakadmin.MutationOutcome, error) {
	f.addMemberCalls++
	f.addMemberRequests = append(f.addMemberRequests, realm+"|"+userIDOrUsername+"|"+groupIDOrPath)
	if f.addMemberErr != nil {
		return "", f.addMemberErr
	}
	if f.addMemberOutcome == "" {
		f.addMemberOutcome = keycloakadmin.MutationOutcomeUpdated
	}
	return f.addMemberOutcome, nil
}

func (f *fakeKeycloakClient) RemoveUserFromGroup(_ context.Context, realm string, userIDOrUsername string, groupIDOrPath string) (keycloakadmin.MutationOutcome, error) {
	f.removeMemberCalls++
	f.removeMemberRequests = append(f.removeMemberRequests, realm+"|"+userIDOrUsername+"|"+groupIDOrPath)
	if f.removeMemberErr != nil {
		return "", f.removeMemberErr
	}
	if f.removeMemberOutcome == "" {
		f.removeMemberOutcome = keycloakadmin.MutationOutcomeUpdated
	}
	return f.removeMemberOutcome, nil
}
