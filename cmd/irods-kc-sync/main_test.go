package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
		Mode:              "repair-keycloak",
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
