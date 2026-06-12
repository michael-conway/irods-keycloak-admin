package planreview

import (
	"context"
	"testing"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

func TestReviewPromptsRequiredOperationsOnly(t *testing.T) {
	reviewer := &fakeReviewer{decisions: []Decision{DecisionSkip}}
	result, err := Review(context.Background(), reviewTestPlan(), PromptModeRequired, reviewer)
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if result.Accepted != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected review counts: %+v", result)
	}
	if len(reviewer.reviewed) != 1 || reviewer.reviewed[0] != "op-002" {
		t.Fatalf("unexpected reviewed operations: %+v", reviewer.reviewed)
	}
	if len(result.Plan.Operations) != 1 || result.Plan.Operations[0].OperationID != "op-001" {
		t.Fatalf("unexpected accepted operations: %+v", result.Plan.Operations)
	}
}

func TestReviewAcceptAllSwitchesToNoFurtherPrompts(t *testing.T) {
	reviewer := &fakeReviewer{decisions: []Decision{DecisionAcceptAll}}
	result, err := Review(context.Background(), reviewTestPlan(), PromptModeAll, reviewer)
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if result.Accepted != 2 || result.Skipped != 0 {
		t.Fatalf("unexpected review counts: %+v", result)
	}
	if len(reviewer.reviewed) != 1 || reviewer.reviewed[0] != "op-001" {
		t.Fatalf("unexpected reviewed operations: %+v", reviewer.reviewed)
	}
}

func TestRequiredPromptModeNeedsReviewerForApprovalOperations(t *testing.T) {
	_, err := Review(context.Background(), reviewTestPlan(), PromptModeRequired, nil)
	if err == nil {
		t.Fatal("expected missing reviewer error")
	}
}

func reviewTestPlan() domain.SyncPlan {
	return domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            "plan-test",
		Mode:              domain.SyncPlanModeSync,
		Authority:         domain.SyncPlanAuthorityIRODS,
		Realm:             "example",
		Zone:              "tempZone",
		Operations: []domain.PlanOperation{
			{
				OperationID: "op-001",
				Action:      domain.PlanActionKeycloakGroupCreate,
				Target:      "/irods/project-alpha",
				Risk:        "low",
			},
			{
				OperationID: "op-002",
				Action:      domain.PlanActionKeycloakGroupDelete,
				Target:      "/irods/stale-team",
				Risk:        domain.PlanRiskRequiresApproval,
			},
		},
	}
}

type fakeReviewer struct {
	decisions []Decision
	reviewed  []string
}

func (f *fakeReviewer) Review(_ context.Context, _ domain.SyncPlan, operation domain.PlanOperation) (Decision, error) {
	f.reviewed = append(f.reviewed, operation.OperationID)
	if len(f.decisions) == 0 {
		return DecisionAccept, nil
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return decision, nil
}
