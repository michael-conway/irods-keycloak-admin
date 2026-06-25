package provisioning

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/avu"
	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
)

func TestPlanUserPlansCreateAndMetadataSyncForMissingIRODSUser(t *testing.T) {
	service := Service{
		IRODS: &fakeIRODSClient{},
		Keycloak: &fakeKeycloakClient{
			usersByID: map[string]*keycloakadmin.User{
				"kc-alice": {ID: "kc-alice", Username: "alice"},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	plan, err := service.PlanUser(context.Background(), domain.ProvisionUserRequest{
		RequestMetadata: domain.RequestMetadata{
			Source: "admin-portal",
		},
		KeycloakUserID: "kc-alice",
	})
	if err != nil {
		t.Fatalf("PlanUser returned error: %v", err)
	}

	if plan.Mode != domain.SyncPlanModeSync || plan.TargetSystem != domain.SyncTargetIRODS {
		t.Fatalf("unexpected plan metadata: %+v", plan)
	}
	if plan.Summary.CreateIRODSUsers != 1 || plan.Summary.UpdateIRODSUserMetadata != 1 {
		t.Fatalf("unexpected plan summary: %+v", plan.Summary)
	}
	if len(plan.Operations) != 2 {
		t.Fatalf("expected two operations, got %+v", plan.Operations)
	}
	if got := plan.Operations[0].Action; got != domain.PlanActionIRODSUserCreate {
		t.Fatalf("unexpected first action %q", got)
	}
	if got := plan.Operations[1].Action; got != domain.PlanActionIRODSUserMetadataSync {
		t.Fatalf("unexpected second action %q", got)
	}
	assertEvidenceValue(t, plan.Operations[0], "request_flow", flowAdministratorProvisioning)
	assertEvidenceValue(t, plan.Operations[0], "irods_user_present", false)
	assertEvidenceValue(t, plan.Operations[0], "sync_direction", domain.SyncDirectionKeycloakToIRODS)
	assertEvidenceValue(t, plan.Operations[0], "sync_classification", domain.SyncClassificationCandidateAddition)
	assertScenario2CredentialEvidence(t, plan.Operations[0])
	assertEvidenceValue(t, plan.Operations[1], "request_flow", flowAdministratorProvisioning)
	assertEvidenceValue(t, plan.Operations[1], "sync_classification", domain.SyncClassificationMappedUpdate)
	assertScenario2CredentialEvidence(t, plan.Operations[1])
	assertEvidenceValue(t, plan.Operations[1], "missing_avu_attributes", []string{
		avu.AuthorityAttribute,
		avu.ManagedByAttribute,
		avu.KeycloakRealmAttribute,
		avu.KeycloakUserIDAttribute,
	})
}

func TestPlanUserPlansMetadataSyncForSelfServiceRequest(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"alice": {Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"alice": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
			},
		},
	}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			usersByID: map[string]*keycloakadmin.User{
				"kc-alice": {ID: "kc-alice", Username: "alice"},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	plan, err := service.PlanUser(context.Background(), domain.ProvisionUserRequest{
		RequestMetadata: domain.RequestMetadata{
			Source: "self-service",
		},
		KeycloakUserID: "kc-alice",
	})
	if err != nil {
		t.Fatalf("PlanUser returned error: %v", err)
	}

	if plan.Summary.CreateIRODSUsers != 0 || plan.Summary.UpdateIRODSUserMetadata != 1 {
		t.Fatalf("unexpected plan summary: %+v", plan.Summary)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Action != domain.PlanActionIRODSUserMetadataSync {
		t.Fatalf("unexpected operations: %+v", plan.Operations)
	}
	assertEvidenceValue(t, plan.Operations[0], "request_flow", flowSelfServiceAccountRequest)
	assertEvidenceValue(t, plan.Operations[0], "irods_user_present", true)
	assertScenario2CredentialEvidence(t, plan.Operations[0])
	assertEvidenceValue(t, plan.Operations[0], "missing_avu_attributes", []string{
		avu.AuthorityAttribute,
		avu.KeycloakRealmAttribute,
		avu.KeycloakUserIDAttribute,
	})
}

func TestPlanUserClassifiesMappedUserSync(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"alice": {Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"alice": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
				{Name: avu.KeycloakRealmAttribute, Value: "example"},
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-alice"},
			},
		},
	}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			usersByID: map[string]*keycloakadmin.User{
				"kc-alice": {ID: "kc-alice", Username: "alice"},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	plan, err := service.PlanUser(context.Background(), domain.ProvisionUserRequest{
		KeycloakUserID: "kc-alice",
	})
	if err != nil {
		t.Fatalf("PlanUser returned error: %v", err)
	}

	if len(plan.Operations) != 1 || plan.Operations[0].Action != domain.PlanActionIRODSUserMetadataSync {
		t.Fatalf("unexpected operations: %+v", plan.Operations)
	}
	assertEvidenceValue(t, plan.Operations[0], "request_flow", flowMappedUserSync)
	assertEvidenceValue(t, plan.Operations[0], "irods_mapping_present", true)
	assertEvidenceValue(t, plan.Operations[0], "missing_avu_attributes", []string{avu.AuthorityAttribute})
}

func TestApplyUserCreatesIRODSUserAndAddsMissingMetadata(t *testing.T) {
	irods := &fakeIRODSClient{}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			usersByID: map[string]*keycloakadmin.User{
				"kc-alice": {ID: "kc-alice", Username: "alice"},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	result, err := service.ApplyUser(context.Background(), domain.ProvisionUserRequest{
		KeycloakUserID: "kc-alice",
	})
	if err != nil {
		t.Fatalf("ApplyUser returned error: %v", err)
	}

	if result.Status != "applied" || result.Operation != operationIRODSUserSync {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.createCalls != 1 {
		t.Fatalf("expected one CreateUser call, got %d", irods.createCalls)
	}
	if len(irods.addedMetadata["alice"]) != 4 {
		t.Fatalf("expected four added AVUs, got %+v", irods.addedMetadata)
	}
}

func TestApplyUserIsUnchangedForMappedUserWithRequiredMetadata(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"alice": {Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"alice": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
				{Name: avu.KeycloakRealmAttribute, Value: "example"},
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-alice"},
				{Name: avu.AuthorityAttribute, Value: domain.SyncPlanAuthorityIRODS},
			},
		},
	}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			usersByID: map[string]*keycloakadmin.User{
				"kc-alice": {ID: "kc-alice", Username: "alice"},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	result, err := service.ApplyUser(context.Background(), domain.ProvisionUserRequest{
		KeycloakUserID: "kc-alice",
	})
	if err != nil {
		t.Fatalf("ApplyUser returned error: %v", err)
	}

	if result.Status != "unchanged" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.createCalls != 0 || len(irods.addedMetadata["alice"]) != 0 {
		t.Fatalf("apply should not mutate converged state: %+v", irods)
	}
}

func TestPlanGroupPlansCreateAndMetadataSyncForMissingIRODSGroup(t *testing.T) {
	service := Service{
		IRODS: &fakeIRODSClient{},
		Keycloak: &fakeKeycloakClient{
			groups: []keycloakadmin.Group{{
				ID:   "kc-group-alpha",
				Name: "project-alpha",
				Path: "/projects/project-alpha",
			}},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	plan, err := service.PlanGroup(context.Background(), domain.ProvisionGroupRequest{
		KeycloakGroupID: "kc-group-alpha",
	})
	if err != nil {
		t.Fatalf("PlanGroup returned error: %v", err)
	}

	if plan.Mode != domain.SyncPlanModeSync || plan.TargetSystem != domain.SyncTargetIRODS {
		t.Fatalf("unexpected plan metadata: %+v", plan)
	}
	if plan.Summary.CreateIRODSGroups != 1 || plan.Summary.UpdateIRODSGroupMetadata != 1 {
		t.Fatalf("unexpected plan summary: %+v", plan.Summary)
	}
	if len(plan.Operations) != 2 {
		t.Fatalf("expected two operations, got %+v", plan.Operations)
	}
	if got := plan.Operations[0].Action; got != domain.PlanActionIRODSGroupCreate {
		t.Fatalf("unexpected first action %q", got)
	}
	if got := plan.Operations[1].Action; got != domain.PlanActionIRODSGroupMetadataSync {
		t.Fatalf("unexpected second action %q", got)
	}
	assertEvidenceValue(t, plan.Operations[0], "keycloak_group_id", "kc-group-alpha")
	assertEvidenceValue(t, plan.Operations[0], "irods_group_name", "project-alpha")
	assertEvidenceValue(t, plan.Operations[0], "irods_group_present", false)
	assertEvidenceValue(t, plan.Operations[0], "sync_direction", domain.SyncDirectionKeycloakToIRODS)
	assertEvidenceValue(t, plan.Operations[0], "sync_classification", domain.SyncClassificationCandidateAddition)
	assertScenario2CredentialEvidence(t, plan.Operations[0])
	assertEvidenceValue(t, plan.Operations[1], "sync_classification", domain.SyncClassificationMappedUpdate)
	assertScenario2CredentialEvidence(t, plan.Operations[1])
	assertEvidenceValue(t, plan.Operations[1], "missing_avu_attributes", []string{
		avu.AuthorityAttribute,
		avu.KeycloakGroupIDAttribute,
		avu.ManagedByAttribute,
		avu.KeycloakRealmAttribute,
	})
}

func TestApplyGroupCreatesIRODSGroupAndAddsMissingMetadata(t *testing.T) {
	irods := &fakeIRODSClient{}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			groups: []keycloakadmin.Group{{
				ID:   "kc-group-alpha",
				Name: "project-alpha",
				Path: "/projects/project-alpha",
			}},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	result, err := service.ApplyGroup(context.Background(), domain.ProvisionGroupRequest{
		KeycloakGroupPath: "/projects/project-alpha",
	})
	if err != nil {
		t.Fatalf("ApplyGroup returned error: %v", err)
	}

	if result.Status != "applied" || result.Operation != operationIRODSGroupSync {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.createCalls != 1 {
		t.Fatalf("expected one CreateUser call, got %d", irods.createCalls)
	}
	if got := irods.users["project-alpha"].Type; got != irodstypes.IRODSUserRodsGroup {
		t.Fatalf("expected group create type, got %q", got)
	}
	if len(irods.addedMetadata["project-alpha"]) != 4 {
		t.Fatalf("expected four added AVUs, got %+v", irods.addedMetadata)
	}
}

func TestPlanGroupPlansConservativeMembershipAddAndRemove(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"project-alpha": {Name: "project-alpha", Zone: "tempZone", Type: irodstypes.IRODSUserRodsGroup},
			"alice":         {Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			"bob":           {Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			"unmanaged":     {Name: "unmanaged", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"project-alpha": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
				{Name: avu.KeycloakRealmAttribute, Value: "example"},
				{Name: avu.KeycloakGroupIDAttribute, Value: "kc-group-alpha"},
				{Name: avu.AuthorityAttribute, Value: domain.SyncPlanAuthorityIRODS},
			},
			"alice": {
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-alice"},
			},
			"bob": {
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-bob"},
			},
		},
		groupMembers: map[string][]*irodstypes.IRODSUser{
			"project-alpha": {
				{Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
				{Name: "unmanaged", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			groups: []keycloakadmin.Group{{
				ID:   "kc-group-alpha",
				Name: "project-alpha",
				Path: "/projects/project-alpha",
			}},
			groupMembers: map[string][]keycloakadmin.User{
				"kc-group-alpha": {
					{ID: "kc-alice", Username: "alice"},
				},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	plan, err := service.PlanGroup(context.Background(), domain.ProvisionGroupRequest{
		KeycloakGroupID: "kc-group-alpha",
	})
	if err != nil {
		t.Fatalf("PlanGroup returned error: %v", err)
	}

	if plan.Summary.UpdateIRODSMemberships != 2 {
		t.Fatalf("unexpected plan summary: %+v", plan.Summary)
	}
	if len(plan.Operations) != 2 {
		t.Fatalf("expected only membership operations, got %+v", plan.Operations)
	}
	if got := plan.Operations[0].Action; got != domain.PlanActionIRODSGroupMemberAdd {
		t.Fatalf("unexpected first action %q", got)
	}
	if got := plan.Operations[0].Target; got != "project-alpha#member:alice" {
		t.Fatalf("unexpected add target %q", got)
	}
	if got := plan.Operations[1].Action; got != domain.PlanActionIRODSGroupMemberRemove {
		t.Fatalf("unexpected second action %q", got)
	}
	if got := plan.Operations[1].Target; got != "project-alpha#member:bob" {
		t.Fatalf("unexpected remove target %q", got)
	}
	assertEvidenceValue(t, plan.Operations[0], "keycloak_user_id", "kc-alice")
	assertEvidenceValue(t, plan.Operations[1], "keycloak_user_id", "kc-bob")
	assertEvidenceValue(t, plan.Operations[0], "sync_classification", domain.SyncClassificationCandidateAddition)
	assertEvidenceValue(t, plan.Operations[1], "sync_classification", domain.SyncClassificationCandidateRemoval)
	assertEvidenceValue(t, plan.Operations[1], "authority_role", "directional_hint")
	assertScenario2CredentialEvidence(t, plan.Operations[0])
	assertScenario2CredentialEvidence(t, plan.Operations[1])
}

func TestPlanGroupDoesNotRemoveIRODSMemberWhenKeycloakMemberIdentityIsAmbiguous(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"project-alpha": {Name: "project-alpha", Zone: "tempZone", Type: irodstypes.IRODSUserRodsGroup},
			"bob":           {Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"project-alpha": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
				{Name: avu.KeycloakRealmAttribute, Value: "example"},
				{Name: avu.KeycloakGroupIDAttribute, Value: "kc-group-alpha"},
				{Name: avu.AuthorityAttribute, Value: domain.SyncPlanAuthorityIRODS},
			},
			"bob": {
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-bob"},
			},
		},
		groupMembers: map[string][]*irodstypes.IRODSUser{
			"project-alpha": {
				{Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			groups: []keycloakadmin.Group{{
				ID:   "kc-group-alpha",
				Name: "project-alpha",
				Path: "/projects/project-alpha",
			}},
			groupMembers: map[string][]keycloakadmin.User{
				"kc-group-alpha": {
					{Username: "bob"},
				},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	plan, err := service.PlanGroup(context.Background(), domain.ProvisionGroupRequest{
		KeycloakGroupID: "kc-group-alpha",
	})
	if err != nil {
		t.Fatalf("PlanGroup returned error: %v", err)
	}

	if len(plan.Operations) != 0 || plan.Summary.UpdateIRODSMemberships != 0 {
		t.Fatalf("expected ambiguous Keycloak member to block removal, got plan %+v", plan)
	}
}

func TestApplyGroupMutatesConservativeMembershipDrift(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"project-alpha": {Name: "project-alpha", Zone: "tempZone", Type: irodstypes.IRODSUserRodsGroup},
			"alice":         {Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			"bob":           {Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"project-alpha": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
				{Name: avu.KeycloakRealmAttribute, Value: "example"},
				{Name: avu.KeycloakGroupIDAttribute, Value: "kc-group-alpha"},
				{Name: avu.AuthorityAttribute, Value: domain.SyncPlanAuthorityIRODS},
			},
			"alice": {
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-alice"},
			},
			"bob": {
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-bob"},
			},
		},
		groupMembers: map[string][]*irodstypes.IRODSUser{
			"project-alpha": {
				{Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	service := Service{
		IRODS: irods,
		Keycloak: &fakeKeycloakClient{
			groups: []keycloakadmin.Group{{
				ID:   "kc-group-alpha",
				Name: "project-alpha",
				Path: "/projects/project-alpha",
			}},
			groupMembers: map[string][]keycloakadmin.User{
				"kc-group-alpha": {
					{ID: "kc-alice", Username: "alice"},
				},
			},
		},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	result, err := service.ApplyGroup(context.Background(), domain.ProvisionGroupRequest{
		KeycloakGroupID: "kc-group-alpha",
	})
	if err != nil {
		t.Fatalf("ApplyGroup returned error: %v", err)
	}

	if result.Status != "applied" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.addMemberCalls != 1 || irods.removeMemberCalls != 1 {
		t.Fatalf("expected one add and one remove, got add=%d remove=%d", irods.addMemberCalls, irods.removeMemberCalls)
	}
	if !fakeGroupHasMember(irods.groupMembers["project-alpha"], "alice") {
		t.Fatalf("expected alice to be a group member: %+v", irods.groupMembers)
	}
	if fakeGroupHasMember(irods.groupMembers["project-alpha"], "bob") {
		t.Fatalf("expected bob to be removed from group: %+v", irods.groupMembers)
	}
}

func TestPlanUserFailsWhenKeycloakUserIsMissing(t *testing.T) {
	service := Service{
		IRODS:        &fakeIRODSClient{},
		Keycloak:     &fakeKeycloakClient{},
		DefaultRealm: "example",
		DefaultZone:  "tempZone",
	}

	_, err := service.PlanUser(context.Background(), domain.ProvisionUserRequest{
		KeycloakUserID: "missing-user",
	})
	if err == nil {
		t.Fatal("expected missing user error")
	}
	var notFound *keycloakadmin.UserNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected keycloak user not found error, got %v", err)
	}
}

func TestApplyPlanCreatesIRODSUserAndSyncsMetadata(t *testing.T) {
	irods := &fakeIRODSClient{}
	service := Service{
		IRODS:      irods,
		PromptMode: planreview.PromptModeNone,
	}

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		Plan: irodsUserPlan([]domain.PlanOperation{
			{
				OperationID: "op-001",
				Action:      domain.PlanActionIRODSUserCreate,
				Target:      "alice",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":   "example",
					"keycloak_user_id": "kc-alice",
					"irods_username":   "alice",
					"irods_zone":       "tempZone",
				},
			},
			{
				OperationID: "op-002",
				Action:      domain.PlanActionIRODSUserMetadataSync,
				Target:      "alice",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":   "example",
					"keycloak_user_id": "kc-alice",
					"irods_username":   "alice",
					"irods_zone":       "tempZone",
					"desired_avus": map[string]any{
						avu.ManagedByAttribute:      defaultManagedByValue,
						avu.KeycloakRealmAttribute:  "example",
						avu.KeycloakUserIDAttribute: "kc-alice",
						avu.AuthorityAttribute:      domain.SyncPlanAuthorityIRODS,
					},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "applied" || result.Applied != 2 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.createCalls != 1 {
		t.Fatalf("expected one CreateUser call, got %d", irods.createCalls)
	}
	if len(irods.addedMetadata["alice"]) != 4 {
		t.Fatalf("expected four added AVUs, got %+v", irods.addedMetadata)
	}
}

func TestApplyPlanSkipsConvergedIRODSUserAndMetadata(t *testing.T) {
	irods := &fakeIRODSClient{
		users: map[string]*irodstypes.IRODSUser{
			"alice": {Name: "alice", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
		},
		metadataByUser: map[string][]*irodstypes.IRODSMeta{
			"alice": {
				{Name: avu.ManagedByAttribute, Value: defaultManagedByValue},
				{Name: avu.KeycloakRealmAttribute, Value: "example"},
				{Name: avu.KeycloakUserIDAttribute, Value: "kc-alice"},
				{Name: avu.AuthorityAttribute, Value: domain.SyncPlanAuthorityIRODS},
			},
		},
	}
	service := Service{
		IRODS:      irods,
		PromptMode: planreview.PromptModeNone,
	}

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		Plan: irodsUserPlan([]domain.PlanOperation{
			{
				OperationID: "op-001",
				Action:      domain.PlanActionIRODSUserCreate,
				Target:      "alice",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":   "example",
					"keycloak_user_id": "kc-alice",
					"irods_username":   "alice",
					"irods_zone":       "tempZone",
				},
			},
			{
				OperationID: "op-002",
				Action:      domain.PlanActionIRODSUserMetadataSync,
				Target:      "alice",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":   "example",
					"keycloak_user_id": "kc-alice",
					"irods_username":   "alice",
					"irods_zone":       "tempZone",
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "skipped" || result.Applied != 0 || result.Skipped != 2 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.createCalls != 0 || len(irods.addedMetadata["alice"]) != 0 {
		t.Fatalf("apply should not mutate converged state: %+v", irods)
	}
}

func TestApplyPlanFailsMetadataSyncWhenIRODSUserIsMissing(t *testing.T) {
	service := Service{
		IRODS:      &fakeIRODSClient{},
		PromptMode: planreview.PromptModeNone,
	}

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		Plan: irodsUserPlan([]domain.PlanOperation{{
			OperationID: "op-001",
			Action:      domain.PlanActionIRODSUserMetadataSync,
			Target:      "alice",
			Risk:        "low",
			Evidence: map[string]any{
				"keycloak_realm":   "example",
				"keycloak_user_id": "kc-alice",
				"irods_username":   "alice",
				"irods_zone":       "tempZone",
			},
		}}),
	})
	if err != nil {
		t.Fatalf("Apply returned unexpected top-level error: %v", err)
	}

	if result.Status != "failed" || result.Failed != 1 || result.WarningCount != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if result.Warnings[0].Code != "apply.irods.operation_failed" {
		t.Fatalf("unexpected warning: %+v", result.Warnings)
	}
	if strings.Contains(strings.ToLower(result.Warnings[0].Message), "password") || strings.Contains(strings.ToLower(result.Warnings[0].Code), "password") {
		t.Fatalf("scenario-2 apply failure must not be reported as password failure: %+v", result.Warnings[0])
	}
}

func TestApplyPlanCreatesIRODSGroupAndSyncsMetadata(t *testing.T) {
	irods := &fakeIRODSClient{}
	service := Service{
		IRODS:      irods,
		PromptMode: planreview.PromptModeNone,
	}

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		Plan: irodsUserPlan([]domain.PlanOperation{
			{
				OperationID: "op-001",
				Action:      domain.PlanActionIRODSGroupCreate,
				Target:      "project-alpha",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":    "example",
					"keycloak_group_id": "kc-group-alpha",
					"irods_group_name":  "project-alpha",
					"irods_zone":        "tempZone",
				},
			},
			{
				OperationID: "op-002",
				Action:      domain.PlanActionIRODSGroupMetadataSync,
				Target:      "project-alpha",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":    "example",
					"keycloak_group_id": "kc-group-alpha",
					"irods_group_name":  "project-alpha",
					"irods_zone":        "tempZone",
					"desired_avus": map[string]any{
						avu.ManagedByAttribute:       defaultManagedByValue,
						avu.KeycloakRealmAttribute:   "example",
						avu.KeycloakGroupIDAttribute: "kc-group-alpha",
						avu.AuthorityAttribute:       domain.SyncPlanAuthorityIRODS,
					},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "applied" || result.Applied != 2 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.createCalls != 1 {
		t.Fatalf("expected one CreateUser call, got %d", irods.createCalls)
	}
	if got := irods.users["project-alpha"].Type; got != irodstypes.IRODSUserRodsGroup {
		t.Fatalf("expected group create type, got %q", got)
	}
	if len(irods.addedMetadata["project-alpha"]) != 4 {
		t.Fatalf("expected four added AVUs, got %+v", irods.addedMetadata)
	}
}

func TestApplyPlanMutatesIRODSGroupMembership(t *testing.T) {
	irods := &fakeIRODSClient{
		groupMembers: map[string][]*irodstypes.IRODSUser{
			"project-alpha": {
				{Name: "bob", Zone: "tempZone", Type: irodstypes.IRODSUserRodsUser},
			},
		},
	}
	service := Service{
		IRODS:      irods,
		PromptMode: planreview.PromptModeNone,
	}

	result, err := service.Apply(context.Background(), domain.ApplyRequest{
		Plan: irodsUserPlan([]domain.PlanOperation{
			{
				OperationID: "op-001",
				Action:      domain.PlanActionIRODSGroupMemberAdd,
				Target:      "project-alpha#member:alice",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_realm":    "example",
					"keycloak_group_id": "kc-group-alpha",
					"keycloak_user_id":  "kc-alice",
					"irods_group_name":  "project-alpha",
					"irods_username":    "alice",
					"irods_zone":        "tempZone",
				},
			},
			{
				OperationID: "op-002",
				Action:      domain.PlanActionIRODSGroupMemberRemove,
				Target:      "project-alpha#member:bob",
				Risk:        "medium",
				Evidence: map[string]any{
					"keycloak_realm":    "example",
					"keycloak_group_id": "kc-group-alpha",
					"keycloak_user_id":  "kc-bob",
					"irods_group_name":  "project-alpha",
					"irods_username":    "bob",
					"irods_zone":        "tempZone",
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "applied" || result.Applied != 2 || result.Failed != 0 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if irods.addMemberCalls != 1 || irods.removeMemberCalls != 1 {
		t.Fatalf("expected one add and one remove, got add=%d remove=%d", irods.addMemberCalls, irods.removeMemberCalls)
	}
	if !fakeGroupHasMember(irods.groupMembers["project-alpha"], "alice") {
		t.Fatalf("expected alice member after apply: %+v", irods.groupMembers)
	}
	if fakeGroupHasMember(irods.groupMembers["project-alpha"], "bob") {
		t.Fatalf("expected bob removed after apply: %+v", irods.groupMembers)
	}
}

type fakeIRODSClient struct {
	users             map[string]*irodstypes.IRODSUser
	metadataByUser    map[string][]*irodstypes.IRODSMeta
	addedMetadata     map[string][]*irodstypes.IRODSMeta
	groupMembers      map[string][]*irodstypes.IRODSUser
	createCalls       int
	addMemberCalls    int
	removeMemberCalls int
}

func (f *fakeIRODSClient) GetUser(_ context.Context, username string, _ string) (*irodstypes.IRODSUser, error) {
	if f.users == nil {
		return nil, nil
	}
	return f.users[username], nil
}

func (f *fakeIRODSClient) CreateUser(_ context.Context, username string, zone string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	f.createCalls++
	if f.users == nil {
		f.users = map[string]*irodstypes.IRODSUser{}
	}
	user := &irodstypes.IRODSUser{Name: username, Zone: zone, Type: userType}
	f.users[username] = user
	return user, nil
}

func (f *fakeIRODSClient) CreateGroup(ctx context.Context, groupName string, zone string) (*irodstypes.IRODSUser, error) {
	return f.CreateUser(ctx, groupName, zone, irodstypes.IRODSUserRodsGroup)
}

func (f *fakeIRODSClient) RemoveUser(context.Context, string, string, irodstypes.IRODSUserType) error {
	return nil
}

func (f *fakeIRODSClient) ListUsers(context.Context, string, irodstypes.IRODSUserType) ([]*irodstypes.IRODSUser, error) {
	return nil, nil
}

func (f *fakeIRODSClient) ListGroupMembers(_ context.Context, _ string, groupName string) ([]*irodstypes.IRODSUser, error) {
	return copyUsers(f.groupMembers[groupName]), nil
}

func (f *fakeIRODSClient) AddGroupMember(_ context.Context, groupName string, username string, zone string) error {
	f.addMemberCalls++
	if f.groupMembers == nil {
		f.groupMembers = map[string][]*irodstypes.IRODSUser{}
	}
	if fakeGroupHasMember(f.groupMembers[groupName], username) {
		return nil
	}
	f.groupMembers[groupName] = append(f.groupMembers[groupName], &irodstypes.IRODSUser{Name: username, Zone: zone, Type: irodstypes.IRODSUserRodsUser})
	return nil
}

func (f *fakeIRODSClient) RemoveGroupMember(_ context.Context, groupName string, username string, _ string) error {
	f.removeMemberCalls++
	members := f.groupMembers[groupName]
	kept := make([]*irodstypes.IRODSUser, 0, len(members))
	for _, member := range members {
		if member == nil || member.Name == username {
			continue
		}
		kept = append(kept, member)
	}
	f.groupMembers[groupName] = kept
	return nil
}

func (f *fakeIRODSClient) AddUserMetadata(_ context.Context, username string, _ string, metadata *irodstypes.IRODSMeta) error {
	if f.addedMetadata == nil {
		f.addedMetadata = map[string][]*irodstypes.IRODSMeta{}
	}
	if f.metadataByUser == nil {
		f.metadataByUser = map[string][]*irodstypes.IRODSMeta{}
	}
	copied := &irodstypes.IRODSMeta{Name: metadata.Name, Value: metadata.Value, Units: metadata.Units}
	f.addedMetadata[username] = append(f.addedMetadata[username], copied)
	f.metadataByUser[username] = append(f.metadataByUser[username], copied)
	return nil
}

func (f *fakeIRODSClient) ListUserMetadata(_ context.Context, username string, _ string) ([]*irodstypes.IRODSMeta, error) {
	return copyMetadata(f.metadataByUser[username]), nil
}

type fakeKeycloakClient struct {
	usersByID    map[string]*keycloakadmin.User
	groups       []keycloakadmin.Group
	groupMembers map[string][]keycloakadmin.User
}

func (f *fakeKeycloakClient) GetUser(_ context.Context, _ string, userID string) (*keycloakadmin.User, error) {
	if f.usersByID == nil {
		return nil, nil
	}
	user, ok := f.usersByID[userID]
	if !ok || user == nil {
		return nil, nil
	}
	copied := *user
	return &copied, nil
}

func (f *fakeKeycloakClient) FindUserByUsername(context.Context, string, string) (*keycloakadmin.User, error) {
	return nil, nil
}

func (f *fakeKeycloakClient) ListGroups(context.Context, string) ([]keycloakadmin.Group, error) {
	result := make([]keycloakadmin.Group, 0, len(f.groups))
	for _, group := range f.groups {
		copied := group
		result = append(result, copied)
	}
	return result, nil
}

func (f *fakeKeycloakClient) ListGroupMembers(_ context.Context, _ string, groupID string) ([]keycloakadmin.User, error) {
	result := make([]keycloakadmin.User, 0, len(f.groupMembers[groupID]))
	for _, user := range f.groupMembers[groupID] {
		copied := user
		result = append(result, copied)
	}
	return result, nil
}

func (f *fakeKeycloakClient) CreateOrUpdateUser(context.Context, string, keycloakadmin.User) (*keycloakadmin.User, error) {
	return nil, nil
}

func (f *fakeKeycloakClient) CreateOrUpdateGroup(context.Context, string, keycloakadmin.Group) (*keycloakadmin.Group, keycloakadmin.MutationOutcome, error) {
	return nil, "", nil
}

func (f *fakeKeycloakClient) DeleteGroup(context.Context, string, string) (keycloakadmin.MutationOutcome, error) {
	return "", nil
}

func (f *fakeKeycloakClient) AddUserToGroup(context.Context, string, string, string) (keycloakadmin.MutationOutcome, error) {
	return "", nil
}

func (f *fakeKeycloakClient) RemoveUserFromGroup(context.Context, string, string, string) (keycloakadmin.MutationOutcome, error) {
	return "", nil
}

func irodsUserPlan(operations []domain.PlanOperation) *domain.SyncPlan {
	return &domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            "plan-test",
		Mode:              domain.SyncPlanModeSync,
		TargetSystem:      domain.SyncTargetIRODS,
		Authority:         domain.SyncPlanAuthorityIRODS,
		Realm:             "example",
		Zone:              "tempZone",
		Summary:           domain.PlanSummary{},
		Operations:        operations,
	}
}

func assertEvidenceValue(t *testing.T, operation domain.PlanOperation, key string, want any) {
	t.Helper()
	got := operation.Evidence[key]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected evidence %q: want=%v got=%v", key, want, got)
	}
}

func assertScenario2CredentialEvidence(t *testing.T, operation domain.PlanOperation) {
	t.Helper()
	assertEvidenceValue(t, operation, "credential_policy", domain.SyncCredentialPolicyExternalAuthority)
	assertEvidenceValue(t, operation, "credential_action", domain.SyncCredentialActionNone)
	assertEvidenceValue(t, operation, "failure_domain", domain.SyncFailureDomainIdentityMapping)
	for key := range operation.Evidence {
		if strings.Contains(strings.ToLower(key), "password") {
			t.Fatalf("scenario-2 plan evidence must not include password evidence key %q in %+v", key, operation)
		}
	}
}

func copyMetadata(metadata []*irodstypes.IRODSMeta) []*irodstypes.IRODSMeta {
	result := make([]*irodstypes.IRODSMeta, 0, len(metadata))
	for _, entry := range metadata {
		if entry == nil {
			continue
		}
		result = append(result, &irodstypes.IRODSMeta{
			Name:  entry.Name,
			Value: entry.Value,
			Units: entry.Units,
		})
	}
	return result
}

func copyUsers(users []*irodstypes.IRODSUser) []*irodstypes.IRODSUser {
	result := make([]*irodstypes.IRODSUser, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		copied := *user
		result = append(result, &copied)
	}
	return result
}

func fakeGroupHasMember(members []*irodstypes.IRODSUser, username string) bool {
	for _, member := range members {
		if member != nil && member.Name == username {
			return true
		}
	}
	return false
}
