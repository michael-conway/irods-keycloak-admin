package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	planutil "github.com/michael-conway/irods-keycloak-admin/internal/plan"
)

func TestKCSyncApplyCreatesMirrorAndMembershipE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	groupName := uniqueE2EName("kcapplycreate")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupKeycloakGroupsWithPrefix(t, ctx, keycloak, cfg, "kcapply")
	cleanupIRODSGroup(t, ctx, cfg, groupName)
	cleanupKeycloakGroup(t, ctx, keycloak, cfg, groupName)
	t.Cleanup(func() {
		cleanupIRODSGroup(t, context.Background(), cfg, groupName)
		cleanupKeycloakGroup(t, context.Background(), keycloak, cfg, groupName)
	})

	createIRODSGroupWithMember(t, ctx, cfg, groupName, cfg.IRODS.SecondaryUser)

	plan := filterPlanForGroup(t, runKCSyncDryRun(t, ctx, cfg), cfg, groupName)
	requireOperation(t, plan, domain.PlanActionKeycloakGroupCreate, mirrorPath(cfg, groupName))
	requireOperation(t, plan, domain.PlanActionKeycloakGroupMemberAdd, memberTarget(cfg, groupName, cfg.IRODS.SecondaryUser))

	result := runKCSyncApply(t, ctx, cfg, plan, "none", "", true)
	requireApplyConvergedResult(t, result, len(plan.Operations))

	group := requireMirrorGroup(t, ctx, keycloak, cfg, groupName)
	requireGroupMember(t, ctx, keycloak, cfg, group.ID, cfg.IRODS.SecondaryUser, true)

	repeat := runKCSyncApply(t, ctx, cfg, plan, "none", "", true)
	requireApplyNoOpResult(t, repeat, len(plan.Operations))

	after := filterPlanForGroup(t, runKCSyncDryRun(t, ctx, cfg), cfg, groupName)
	if len(after.Operations) != 0 {
		t.Fatalf("expected target group to converge after apply, got operations: %+v", after.Operations)
	}
}

func TestKCSyncApplyRemovesStaleMembershipE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	groupName := uniqueE2EName("kcapplymember")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupKeycloakGroupsWithPrefix(t, ctx, keycloak, cfg, "kcapply")
	cleanupIRODSGroup(t, ctx, cfg, groupName)
	cleanupKeycloakGroup(t, ctx, keycloak, cfg, groupName)
	t.Cleanup(func() {
		cleanupIRODSGroup(t, context.Background(), cfg, groupName)
		cleanupKeycloakGroup(t, context.Background(), keycloak, cfg, groupName)
	})

	createIRODSGroupWithMember(t, ctx, cfg, groupName, cfg.IRODS.SecondaryUser)
	keycloak.createMirrorGroup(t, ctx, cfg, groupName)
	group := requireMirrorGroup(t, ctx, keycloak, cfg, groupName)
	keycloak.addUserToGroup(t, ctx, cfg, cfg.IRODS.PrimaryUser, group.ID)
	keycloak.addUserToGroup(t, ctx, cfg, cfg.IRODS.SecondaryUser, group.ID)

	plan := filterPlanForGroup(t, runKCSyncDryRun(t, ctx, cfg), cfg, groupName)
	requireOperation(t, plan, domain.PlanActionKeycloakGroupMemberRemove, memberTarget(cfg, groupName, cfg.IRODS.PrimaryUser))
	forbidOperation(t, plan, domain.PlanActionKeycloakGroupMemberAdd, memberTarget(cfg, groupName, cfg.IRODS.SecondaryUser))

	result := runKCSyncApply(t, ctx, cfg, plan, "none", "", true)
	requireApplyConvergedResult(t, result, len(plan.Operations))

	group = requireMirrorGroup(t, ctx, keycloak, cfg, groupName)
	requireGroupMember(t, ctx, keycloak, cfg, group.ID, cfg.IRODS.SecondaryUser, true)
	requireGroupMember(t, ctx, keycloak, cfg, group.ID, cfg.IRODS.PrimaryUser, false)

	repeat := runKCSyncApply(t, ctx, cfg, plan, "none", "", true)
	requireApplyNoOpResult(t, repeat, len(plan.Operations))

	after := filterPlanForGroup(t, runKCSyncDryRun(t, ctx, cfg), cfg, groupName)
	if len(after.Operations) != 0 {
		t.Fatalf("expected target membership to converge after apply, got operations: %+v", after.Operations)
	}
}

func TestKCSyncApplyStaleMirrorDeletePromptPolicyE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	groupName := uniqueE2EName("kcapplystale")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupKeycloakGroupsWithPrefix(t, ctx, keycloak, cfg, "kcapply")
	cleanupIRODSGroup(t, ctx, cfg, groupName)
	cleanupKeycloakGroup(t, ctx, keycloak, cfg, groupName)
	t.Cleanup(func() {
		cleanupIRODSGroup(t, context.Background(), cfg, groupName)
		cleanupKeycloakGroup(t, context.Background(), keycloak, cfg, groupName)
	})

	keycloak.createMirrorGroup(t, ctx, cfg, groupName)

	plan := filterPlanForGroup(t, runKCSyncDryRun(t, ctx, cfg), cfg, groupName)
	operation := requireOperation(t, plan, domain.PlanActionKeycloakGroupDelete, mirrorPath(cfg, groupName))
	if operation.Risk != domain.PlanRiskRequiresApproval {
		t.Fatalf("expected stale mirror delete to require approval, got %+v", operation)
	}

	rejected := runKCSyncApply(t, ctx, cfg, plan, "required", "s\n", true)
	if rejected.Status != "skipped" || rejected.Applied != 0 || rejected.Skipped != 1 || rejected.Failed != 0 {
		t.Fatalf("unexpected skipped delete result: %+v", rejected)
	}
	if group, err := keycloak.findMirrorGroup(ctx, cfg, groupName); err != nil {
		t.Fatalf("checking Keycloak group after skipped delete: %v", err)
	} else if group == nil {
		t.Fatal("stale Keycloak mirror was deleted without approval")
	}

	approved := runKCSyncApply(t, ctx, cfg, plan, "required", "a\n", true)
	if approved.Status != "applied" || approved.Applied != 1 || approved.Failed != 0 || approved.Skipped != 0 {
		t.Fatalf("unexpected approved delete result: %+v", approved)
	}
	if group, err := keycloak.findMirrorGroup(ctx, cfg, groupName); err != nil {
		t.Fatalf("checking Keycloak group after approved delete: %v", err)
	} else if group != nil {
		t.Fatalf("expected approved stale mirror delete to remove group, found %+v", group)
	}

	repeat := runKCSyncApply(t, ctx, cfg, plan, "required", "a\n", true)
	requireApplyNoOpResult(t, repeat, len(plan.Operations))

	after := filterPlanForGroup(t, runKCSyncDryRun(t, ctx, cfg), cfg, groupName)
	if len(after.Operations) != 0 {
		t.Fatalf("expected stale mirror delete to converge after apply, got operations: %+v", after.Operations)
	}
}

func TestKCSyncApplyCreatesIRODSUserFromKeycloakE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	username := uniqueE2EName("kcirodsuser")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupIRODSUser(t, ctx, cfg, username)
	cleanupKeycloakUser(t, ctx, keycloak, cfg, username)
	t.Cleanup(func() {
		cleanupIRODSUser(t, context.Background(), cfg, username)
		cleanupKeycloakUser(t, context.Background(), keycloak, cfg, username)
	})

	user := keycloak.createUser(t, ctx, cfg, username)
	plan := runKCSyncDryRunForIRODSUser(t, ctx, cfg, user.ID)
	requireOperation(t, plan, domain.PlanActionIRODSUserCreate, username)
	requireOperation(t, plan, domain.PlanActionIRODSUserMetadataSync, username)
	if plan.Summary.CreateIRODSUsers != 1 || plan.Summary.UpdateIRODSUserMetadata != 1 {
		t.Fatalf("unexpected iRODS user plan summary: %+v", plan.Summary)
	}

	result := runKCSyncApply(t, ctx, cfg, plan, "none", "", true)
	requireApplyConvergedResult(t, result, len(plan.Operations))
	requireIRODSUserExists(t, ctx, cfg, username)

	repeat := runKCSyncApply(t, ctx, cfg, plan, "none", "", true)
	requireApplyNoOpResult(t, repeat, len(plan.Operations))

	after := runKCSyncDryRunForIRODSUser(t, ctx, cfg, user.ID)
	if len(after.Operations) != 0 {
		t.Fatalf("expected iRODS user to converge after apply, got operations: %+v", after.Operations)
	}
}

func requireApplyResult(t *testing.T, result domain.ApplyResult, status string, applied int, skipped int, failed int) {
	t.Helper()

	if result.Status != status || result.Applied != applied || result.Skipped != skipped || result.Failed != failed {
		t.Fatalf("unexpected apply result: want status=%s applied=%d skipped=%d failed=%d got %+v", status, applied, skipped, failed, result)
	}
}

func requireApplyConvergedResult(t *testing.T, result domain.ApplyResult, operationCount int) {
	t.Helper()

	if result.Failed != 0 {
		t.Fatalf("unexpected apply failures: %+v", result)
	}
	if result.Applied+result.Skipped != operationCount {
		t.Fatalf("unexpected apply accounting: want operations=%d got %+v", operationCount, result)
	}
	if result.Status != "applied" && result.Status != "skipped" {
		t.Fatalf("unexpected apply status for converged result: %+v", result)
	}
}

func requireApplyNoOpResult(t *testing.T, result domain.ApplyResult, operationCount int) {
	t.Helper()

	requireApplyResult(t, result, "skipped", 0, operationCount, 0)
	for _, operation := range result.Operations {
		if operation.Status != "unchanged" && operation.Status != "skipped" {
			t.Fatalf("expected repeat apply to be unchanged or skipped, got %+v", result.Operations)
		}
	}
}

func runKCSyncApply(t *testing.T, ctx context.Context, cfg Config, plan domain.SyncPlan, prompts string, stdin string, requireSuccess bool) domain.ApplyResult {
	t.Helper()

	planPath := filepath.Join(t.TempDir(), "plan.json")
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal apply plan: %v", err)
	}
	if err := os.WriteFile(planPath, append(payload, '\n'), 0o600); err != nil {
		t.Fatalf("write apply plan: %v", err)
	}

	args := []string{
		"run", "./cmd/irods-kc-sync", "apply",
		"--plan", planPath,
		"--realm", cfg.Keycloak.Realm,
		"--zone", cfg.IRODS.Zone,
		"--prompts", prompts,
		"--irods-host", cfg.IRODS.ProviderHost,
		"--irods-port", strconv.Itoa(cfg.IRODS.ProviderPort),
		"--irods-user", cfg.IRODS.AdminUser,
		"--irods-password", cfg.IRODS.AdminPassword,
		"--irods-resource", cfg.IRODS.ProviderResource,
		"--keycloak-url", cfg.Keycloak.BaseURL,
		"--keycloak-admin-user", cfg.Keycloak.AdminUser,
		"--keycloak-admin-password", cfg.Keycloak.AdminPassword,
	}
	if cfg.Keycloak.InsecureSkipVerify {
		args = append(args, "--keycloak-insecure-skip-verify")
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if requireSuccess && err != nil {
		t.Fatalf("irods-kc-sync apply failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !requireSuccess {
		if err == nil {
			t.Fatalf("expected irods-kc-sync apply to fail\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
		}
		return domain.ApplyResult{}
	}

	var result domain.ApplyResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode apply result: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return result
}

func filterPlanForGroup(t *testing.T, plan domain.SyncPlan, cfg Config, groupName string) domain.SyncPlan {
	t.Helper()

	groupPath := mirrorPath(cfg, groupName)
	filtered := plan
	filtered.Operations = []domain.PlanOperation{}
	for _, operation := range plan.Operations {
		if operation.Target == groupPath || strings.HasPrefix(operation.Target, groupPath+"#member:") {
			filtered.Operations = append(filtered.Operations, operation)
		}
	}
	filtered.Summary = planutil.SummaryCounts(filtered)
	return filtered
}

func runKCSyncDryRunForIRODSUser(t *testing.T, ctx context.Context, cfg Config, keycloakUserID string) domain.SyncPlan {
	t.Helper()

	args := []string{
		"run", "./cmd/irods-kc-sync", "sync",
		"--dry-run",
		"--target", domain.SyncTargetIRODS,
		"--keycloak-user-id", keycloakUserID,
		"--realm", cfg.Keycloak.Realm,
		"--zone", cfg.IRODS.Zone,
		"--irods-host", cfg.IRODS.ProviderHost,
		"--irods-port", strconv.Itoa(cfg.IRODS.ProviderPort),
		"--irods-user", cfg.IRODS.AdminUser,
		"--irods-password", cfg.IRODS.AdminPassword,
		"--irods-resource", cfg.IRODS.ProviderResource,
		"--keycloak-url", cfg.Keycloak.BaseURL,
		"--keycloak-admin-user", cfg.Keycloak.AdminUser,
		"--keycloak-admin-password", cfg.Keycloak.AdminPassword,
	}
	if cfg.Keycloak.InsecureSkipVerify {
		args = append(args, "--keycloak-insecure-skip-verify")
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("irods-kc-sync sync --dry-run --target=irods failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	var plan domain.SyncPlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode iRODS dry-run plan: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if plan.Mode != domain.SyncPlanModeSync || plan.TargetSystem != domain.SyncTargetIRODS || plan.Authority != domain.SyncPlanAuthorityIRODS {
		t.Fatalf("unexpected iRODS plan metadata: %+v", plan)
	}
	return plan
}

func requireMirrorGroup(t *testing.T, ctx context.Context, keycloak *e2eKeycloakClient, cfg Config, groupName string) *e2eKeycloakGroup {
	t.Helper()

	for attempt := 0; attempt < 20; attempt++ {
		group, err := keycloak.findMirrorGroup(ctx, cfg, groupName)
		if err != nil {
			t.Fatalf("find Keycloak mirror group %q: %v", groupName, err)
		}
		if group != nil {
			return group
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Keycloak mirror group %q was not found", groupName)
	return nil
}

func requireGroupMember(t *testing.T, ctx context.Context, keycloak *e2eKeycloakClient, cfg Config, groupID string, username string, want bool) {
	t.Helper()

	for attempt := 0; attempt < 20; attempt++ {
		if keycloak.groupHasMember(t, ctx, cfg, groupID, username) == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Keycloak group %q member %q presence mismatch: want %t", groupID, username, want)
}

func (c *e2eKeycloakClient) addUserToGroup(t *testing.T, ctx context.Context, cfg Config, username string, groupID string) {
	t.Helper()

	user := c.requireUserByUsername(t, ctx, cfg, username)
	if err := c.doJSON(ctx, http.MethodPut, c.adminPath(cfg.Keycloak.Realm, "users", user.ID, "groups", groupID), nil, nil); err != nil {
		t.Fatalf("add Keycloak user %q to group %q: %v", username, groupID, err)
	}
}

func (c *e2eKeycloakClient) groupHasMember(t *testing.T, ctx context.Context, cfg Config, groupID string, username string) bool {
	t.Helper()

	for _, member := range c.listGroupMembers(t, ctx, cfg, groupID) {
		if member.Username == username {
			return true
		}
	}
	return false
}

func (c *e2eKeycloakClient) requireUserByUsername(t *testing.T, ctx context.Context, cfg Config, username string) e2eKeycloakUser {
	t.Helper()

	values := url.Values{}
	values.Set("username", username)
	values.Set("exact", "true")
	var users []e2eKeycloakUser
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(cfg.Keycloak.Realm, "users")+"?"+values.Encode(), nil, &users); err != nil {
		t.Fatalf("find Keycloak user %q: %v", username, err)
	}
	if len(users) == 0 {
		t.Fatalf("Keycloak user %q was not found", username)
	}
	return users[0]
}
