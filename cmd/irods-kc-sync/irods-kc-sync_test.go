package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

func TestResolvePlanOutputPath(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		planPath string
		want     string
		wantErr  bool
	}{
		{name: "empty", want: ""},
		{name: "out", out: "plan.json", want: "plan.json"},
		{name: "plan path", planPath: "plan.json", want: "plan.json"},
		{name: "same", out: "plan.json", planPath: "plan.json", want: "plan.json"},
		{name: "different", out: "one.json", planPath: "two.json", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePlanOutputPath(tt.out, tt.planPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected path: want %q got %q", tt.want, got)
			}
		})
	}
}

func TestWritePlanFileUsesIndentedJSONContract(t *testing.T) {
	plan := domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            "plan-test",
		Mode:              "sync",
		TargetSystem:      domain.SyncTargetKeycloak,
		Authority:         "irods",
		Realm:             "irods",
		Zone:              "tempZone",
		Summary:           domain.PlanSummary{CreateKeycloakGroups: 1},
		Operations: []domain.PlanOperation{{
			OperationID: "op-001",
			Action:      "keycloak.group.create",
			Target:      "/irods/project-alpha",
			Risk:        "low",
			Evidence: map[string]any{
				"irods_group_name": "project-alpha",
				"irods_zone":       "tempZone",
				"keycloak_path":    "/irods/project-alpha",
				"keycloak_realm":   "irods",
			},
		}},
	}

	var stdout bytes.Buffer
	if err := writePlanJSON(&stdout, plan); err != nil {
		t.Fatalf("write stdout JSON: %v", err)
	}

	planPath := filepath.Join(t.TempDir(), "plan.json")
	if err := writePlanFile(planPath, plan); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	fileBytes, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan file: %v", err)
	}
	if string(fileBytes) != stdout.String() {
		t.Fatalf("plan file and stdout JSON differ:\nfile:\n%s\nstdout:\n%s", string(fileBytes), stdout.String())
	}

	var decoded domain.SyncPlan
	if err := json.Unmarshal(fileBytes, &decoded); err != nil {
		t.Fatalf("decode plan JSON: %v", err)
	}
	if decoded.PlanFormatVersion != domain.SyncPlanFormatVersion {
		t.Fatalf("unexpected plan format version: %q", decoded.PlanFormatVersion)
	}
}

func TestBuildPasswordActionReportIsReportingOnly(t *testing.T) {
	plan := domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            "plan-test",
		Mode:              domain.SyncPlanModeSync,
		TargetSystem:      domain.SyncTargetIRODS,
		Authority:         domain.SyncPlanAuthorityIRODS,
		Realm:             "example",
		Zone:              "tempZone",
		Operations: []domain.PlanOperation{
			{
				OperationID: "op-001",
				Action:      domain.PlanActionIRODSUserCreate,
				Target:      "alice",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_user_id": "kc-alice",
					"irods_username":   "alice",
				},
			},
			{
				OperationID: "op-002",
				Action:      domain.PlanActionIRODSUserMetadataSync,
				Target:      "bob",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_user_id": "kc-bob",
					"irods_username":   "bob",
				},
			},
			{
				OperationID: "op-003",
				Action:      domain.PlanActionIRODSGroupMemberAdd,
				Target:      "project-alpha#member:carol",
				Risk:        "low",
				Evidence: map[string]any{
					"keycloak_user_id": "kc-carol",
					"irods_username":   "carol",
				},
			},
		},
	}

	report := buildPasswordActionReport(plan)
	if report.ReportFormatVersion == "" || report.PlanID != "plan-test" || report.Notification != "out_of_scope" {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if report.CredentialPath != "future_keycloak_to_irods_direct" {
		t.Fatalf("report should describe future direct credential path, got %+v", report)
	}
	if len(report.Actions) != 2 {
		t.Fatalf("expected report actions only for user operations, got %+v", report.Actions)
	}
	if report.Actions[0].Action != "password_setup_required" || report.Actions[0].KeycloakUserID != "kc-alice" || report.Actions[0].IRODSUsername != "alice" {
		t.Fatalf("unexpected create-user report action: %+v", report.Actions[0])
	}
	if report.Actions[1].Action != "credential_state_unknown" || report.Actions[1].KeycloakUserID != "kc-bob" || report.Actions[1].IRODSUsername != "bob" {
		t.Fatalf("unexpected metadata-sync report action: %+v", report.Actions[1])
	}

	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	rendered := strings.ToLower(string(reportBytes))
	for _, forbidden := range []string{"password_value", "password_value", "secret", "notification_delivery"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("password action report must not include credential material or delivery hooks: %s", rendered)
		}
	}
}

func TestWritePasswordActionReportFileUsesIndentedJSON(t *testing.T) {
	report := domain.PasswordActionReport{
		ReportFormatVersion: "irods-keycloak-admin.password-action-report.v1",
		PlanID:              "plan-test",
		Realm:               "example",
		Zone:                "tempZone",
		TargetSystem:        domain.SyncTargetIRODS,
		Notification:        "out_of_scope",
		CredentialPath:      "future_keycloak_to_irods_direct",
		Actions: []domain.PasswordAction{{
			Action:         "password_setup_required",
			KeycloakUserID: "kc-alice",
			IRODSUsername:  "alice",
			Reason:         "irods_user_create_planned",
		}},
	}

	reportPath := filepath.Join(t.TempDir(), "password-actions.json")
	if err := writePasswordActionReportFile(reportPath, report); err != nil {
		t.Fatalf("write password action report: %v", err)
	}
	fileBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}
	if !bytes.Contains(fileBytes, []byte("\n  \"report_format_version\"")) {
		t.Fatalf("expected indented report JSON, got:\n%s", string(fileBytes))
	}
	var decoded domain.PasswordActionReport
	if err := json.Unmarshal(fileBytes, &decoded); err != nil {
		t.Fatalf("decode report JSON: %v", err)
	}
	if decoded.Actions[0].Action != "password_setup_required" {
		t.Fatalf("unexpected decoded report: %+v", decoded)
	}
}

func TestNormalizeSyncTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults keycloak", input: "", want: domain.SyncTargetKeycloak},
		{name: "keycloak", input: "keycloak", want: domain.SyncTargetKeycloak},
		{name: "irods", input: "irods", want: domain.SyncTargetIRODS},
		{name: "trim and lower", input: "  IRODS  ", want: domain.SyncTargetIRODS},
		{name: "invalid", input: "both", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSyncTarget(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected target: want %q got %q", tt.want, got)
			}
		})
	}
}

func TestSyncHelpExplainsParameterMeanings(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"sync", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected help exit code 0, got %d; stderr:\n%s", code, stderr.String())
	}
	rendered := stderr.String()
	for _, want := range []string{
		"Target modes:",
		"--target=keycloak mirrors authoritative iRODS",
		"--target=irods requires exactly one selector",
		"--password-action-report REPORT.json",
		"--irods-host HOST, --irods-port PORT",
		"--keycloak-client-id ID, --keycloak-client-secret SECRET",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected sync help to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestApplyHelpExplainsParameterMeanings(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"apply", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected help exit code 0, got %d; stderr:\n%s", code, stderr.String())
	}
	rendered := stderr.String()
	for _, want := range []string{
		"Apply a reviewed JSON plan",
		"--plan PLAN.json",
		"--prompts required|all|none",
		"--keycloak-mirror-root PATH",
		"--irods-env FILE",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected apply help to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestValidationErrorsExplainParameterMeaning(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "sync requires dry run",
			args: []string{"sync"},
			want: "pass --dry-run to generate a reviewable JSON plan",
		},
		{
			name: "invalid target explains choices",
			args: []string{"sync", "--dry-run", "--target", "both"},
			want: "keycloak mirrors iRODS into Keycloak",
		},
		{
			name: "irods target requires selector",
			args: []string{"sync", "--dry-run", "--target", "irods", "--realm", "irods"},
			want: "pass --keycloak-user-id USER_ID, --keycloak-group-id GROUP_ID, or --keycloak-group-path GROUP_PATH",
		},
		{
			name: "apply requires plan",
			args: []string{"apply"},
			want: "JSON plan created by 'irods-kc-sync sync --dry-run'",
		},
		{
			name: "apply rejects positional arguments",
			args: []string{"apply", "plan.json"},
			want: "apply accepts only named flags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := run(tt.args, &stdout, &stderr)
			if code == 0 {
				t.Fatal("expected non-zero exit code")
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("expected stderr to contain %q, got:\n%s", tt.want, stderr.String())
			}
		})
	}
}

func TestValidateIRODSSyncSelector(t *testing.T) {
	tests := []struct {
		name              string
		target            string
		keycloakUserID    string
		keycloakGroupID   string
		keycloakGroupPath string
		wantKind          irodsSyncSelectorKind
		wantErr           bool
	}{
		{name: "keycloak target does not require selector", target: domain.SyncTargetKeycloak},
		{name: "user selector", target: domain.SyncTargetIRODS, keycloakUserID: "kc-alice", wantKind: irodsSyncSelectorUser},
		{name: "group id selector", target: domain.SyncTargetIRODS, keycloakGroupID: "kc-group", wantKind: irodsSyncSelectorGroup},
		{name: "group path selector", target: domain.SyncTargetIRODS, keycloakGroupPath: "/projects/alpha", wantKind: irodsSyncSelectorGroup},
		{name: "missing irods selector", target: domain.SyncTargetIRODS, wantErr: true},
		{name: "mixed selectors", target: domain.SyncTargetIRODS, keycloakUserID: "kc-alice", keycloakGroupID: "kc-group", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateIRODSSyncSelector(tt.target, tt.keycloakUserID, tt.keycloakGroupID, tt.keycloakGroupPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.kind != tt.wantKind {
				t.Fatalf("unexpected selector kind: want %q got %q", tt.wantKind, got.kind)
			}
		})
	}
}

func TestPrintOperationReviewShowsCauseBeforeEvidenceDump(t *testing.T) {
	plan := domain.SyncPlan{
		PlanID: "plan-test",
		Realm:  "example",
		Zone:   "tempZone",
	}
	operation := domain.PlanOperation{
		OperationID: "op-001",
		Action:      domain.PlanActionKeycloakGroupDelete,
		Target:      "/irods/stale-team",
		Risk:        domain.PlanRiskRequiresApproval,
		Evidence: map[string]any{
			"change_cause":  "stale_keycloak_state",
			"keycloak_path": "/irods/stale-team",
		},
	}

	var out bytes.Buffer
	printOperationReview(&out, plan, operation)
	rendered := out.String()

	for _, want := range []string{
		"Cause: stale_keycloak_state",
		"Evidence:",
		"  change_cause: stale_keycloak_state",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected review output to contain %q, got:\n%s", want, rendered)
		}
	}
	if strings.Index(rendered, "Cause:") > strings.Index(rendered, "Evidence:") {
		t.Fatalf("expected cause line before evidence block, got:\n%s", rendered)
	}
}
