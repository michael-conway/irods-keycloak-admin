package plan

import (
	"strings"
	"testing"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

func TestValidateForApplyRejectsInvalidPlanBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*domain.SyncPlan)
		want string
	}{
		{
			name: "format version",
			edit: func(plan *domain.SyncPlan) {
				plan.PlanFormatVersion = "old"
			},
			want: "unsupported sync plan format version",
		},
		{
			name: "mode",
			edit: func(plan *domain.SyncPlan) {
				plan.Mode = "bootstrap"
			},
			want: "unsupported plan mode",
		},
		{
			name: "authority",
			edit: func(plan *domain.SyncPlan) {
				plan.Authority = "keycloak"
			},
			want: "unsupported plan authority",
		},
		{
			name: "duplicate operation id",
			edit: func(plan *domain.SyncPlan) {
				plan.Operations = append(plan.Operations, plan.Operations[0])
			},
			want: "duplicate operation_id",
		},
		{
			name: "delete risk marker",
			edit: func(plan *domain.SyncPlan) {
				plan.Operations[0].Action = domain.PlanActionKeycloakGroupDelete
				plan.Operations[0].Risk = "low"
			},
			want: "group delete must be marked requires_approval",
		},
		{
			name: "member target",
			edit: func(plan *domain.SyncPlan) {
				plan.Operations[0].Action = domain.PlanActionKeycloakGroupMemberAdd
				plan.Operations[0].Target = "/irods/project-alpha"
			},
			want: "member target must have shape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := validPlan()
			tt.edit(&plan)
			err := ValidateForApply(plan, ApplyValidationOptions{ExpectedRealm: "example", ExpectedZone: "tempZone"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateForApplyAcceptsRequiresApprovalDelete(t *testing.T) {
	plan := validPlan()
	plan.Operations[0] = domain.PlanOperation{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakGroupDelete,
		Target:      "/irods/project-alpha",
		Risk:        domain.PlanRiskRequiresApproval,
		Evidence: map[string]any{
			"keycloak_realm": "example",
			"irods_zone":     "tempZone",
			"keycloak_path":  "/irods/project-alpha",
		},
	}

	err := ValidateForApply(plan, ApplyValidationOptions{
		ExpectedRealm: "example",
		ExpectedZone:  "tempZone",
	})
	if err != nil {
		t.Fatalf("ValidateForApply returned error: %v", err)
	}
}

func validPlan() domain.SyncPlan {
	return domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            "plan-test",
		Mode:              domain.SyncPlanModeRepairKeycloak,
		Authority:         domain.SyncPlanAuthorityIRODS,
		Realm:             "example",
		Zone:              "tempZone",
		Operations: []domain.PlanOperation{{
			OperationID: "op-001",
			Action:      domain.PlanActionKeycloakGroupCreate,
			Target:      "/irods/project-alpha",
			Risk:        "low",
			Evidence: map[string]any{
				"keycloak_realm": "example",
				"irods_zone":     "tempZone",
				"keycloak_path":  "/irods/project-alpha",
			},
		}},
	}
}
