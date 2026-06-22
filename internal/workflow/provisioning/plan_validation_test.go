package provisioning

import (
	"context"
	"strings"
	"testing"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
)

func TestApplyPlanRejectsIRODSUserOperationWithoutKeycloakMapping(t *testing.T) {
	service := Service{
		IRODS:      &fakeIRODSClient{},
		PromptMode: planreview.PromptModeNone,
	}

	_, err := service.Apply(context.Background(), domain.ApplyRequest{
		Plan: irodsUserPlan([]domain.PlanOperation{{
			OperationID: "op-001",
			Action:      domain.PlanActionIRODSUserMetadataSync,
			Target:      "alice",
			Risk:        "low",
			Evidence: map[string]any{
				"keycloak_realm": "example",
				"irods_username": "alice",
				"irods_zone":     "tempZone",
			},
		}}),
	})
	if err == nil || !strings.Contains(err.Error(), "keycloak_user_id evidence is required") {
		t.Fatalf("expected missing keycloak_user_id validation error, got %v", err)
	}
}
