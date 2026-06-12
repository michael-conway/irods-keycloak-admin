package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

func TestKCSyncDryRunPlansMissingKeycloakMirrorE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	groupName := uniqueE2EName("kcdrymissing")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupKeycloakGroupsWithPrefix(t, ctx, keycloak, cfg, "kcdry")
	cleanupIRODSGroup(t, ctx, cfg, groupName)
	cleanupKeycloakGroup(t, ctx, keycloak, cfg, groupName)
	t.Cleanup(func() {
		cleanupIRODSGroup(t, context.Background(), cfg, groupName)
		cleanupKeycloakGroup(t, context.Background(), keycloak, cfg, groupName)
	})

	createIRODSGroupWithMember(t, ctx, cfg, groupName, cfg.IRODS.SecondaryUser)

	plan := runKCSyncDryRun(t, ctx, cfg)

	requireOperation(t, plan, "keycloak.group.create", mirrorPath(cfg, groupName))
	requireOperation(t, plan, "keycloak.group.member.add", memberTarget(cfg, groupName, cfg.IRODS.SecondaryUser))
	if group, err := keycloak.findMirrorGroup(ctx, cfg, groupName); err != nil {
		t.Fatalf("checking Keycloak group after dry-run: %v", err)
	} else if group != nil {
		t.Fatalf("dry-run created Keycloak group unexpectedly: %+v", group)
	}
}

func TestKCSyncDryRunPlansKeycloakMembershipDriftE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	groupName := uniqueE2EName("kcdrydrift")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupKeycloakGroupsWithPrefix(t, ctx, keycloak, cfg, "kcdry")
	cleanupIRODSGroup(t, ctx, cfg, groupName)
	cleanupKeycloakGroup(t, ctx, keycloak, cfg, groupName)
	t.Cleanup(func() {
		cleanupIRODSGroup(t, context.Background(), cfg, groupName)
		cleanupKeycloakGroup(t, context.Background(), keycloak, cfg, groupName)
	})

	createIRODSGroupWithMember(t, ctx, cfg, groupName, cfg.IRODS.SecondaryUser)
	keycloak.createMirrorGroup(t, ctx, cfg, groupName)

	plan := runKCSyncDryRun(t, ctx, cfg)

	forbidOperation(t, plan, "keycloak.group.create", mirrorPath(cfg, groupName))
	requireOperation(t, plan, "keycloak.group.member.add", memberTarget(cfg, groupName, cfg.IRODS.SecondaryUser))

	group, err := keycloak.findMirrorGroup(ctx, cfg, groupName)
	if err != nil {
		t.Fatalf("checking Keycloak group after dry-run: %v", err)
	}
	if group == nil {
		t.Fatalf("expected dry-run to leave existing Keycloak group in place")
	}
	members := keycloak.listGroupMembers(t, ctx, cfg, group.ID)
	for _, member := range members {
		if member.Username == cfg.IRODS.SecondaryUser {
			t.Fatalf("dry-run added Keycloak member unexpectedly: %+v", members)
		}
	}
}

func TestKCSyncDryRunPlansStaleKeycloakMirrorE2E(t *testing.T) {
	cfg := RequireConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	groupName := uniqueE2EName("kcdrystale")
	keycloak := newE2EKeycloakClient(t, cfg)

	cleanupKeycloakGroupsWithPrefix(t, ctx, keycloak, cfg, "kcdry")
	cleanupIRODSGroup(t, ctx, cfg, groupName)
	cleanupKeycloakGroup(t, ctx, keycloak, cfg, groupName)
	t.Cleanup(func() {
		cleanupIRODSGroup(t, context.Background(), cfg, groupName)
		cleanupKeycloakGroup(t, context.Background(), keycloak, cfg, groupName)
	})

	keycloak.createMirrorGroup(t, ctx, cfg, groupName)

	plan := runKCSyncDryRun(t, ctx, cfg)

	operation := requireOperation(t, plan, "keycloak.group.delete", mirrorPath(cfg, groupName))
	if operation.Risk != "requires_approval" {
		t.Fatalf("expected stale mirror delete to require approval, got %+v", operation)
	}
	if group, err := keycloak.findMirrorGroup(ctx, cfg, groupName); err != nil {
		t.Fatalf("checking Keycloak group after dry-run: %v", err)
	} else if group == nil {
		t.Fatal("dry-run deleted stale Keycloak mirror unexpectedly")
	}
}

func runKCSyncDryRun(t *testing.T, ctx context.Context, cfg Config) domain.SyncPlan {
	t.Helper()

	args := []string{
		"run", "./cmd/irods-kc-sync", "repair-keycloak",
		"--dry-run",
		"--realm", cfg.Keycloak.Realm,
		"--zone", cfg.IRODS.Zone,
		"--irods-host", cfg.IRODS.ProviderHost,
		"--irods-port", fmt.Sprintf("%d", cfg.IRODS.ProviderPort),
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
		t.Fatalf("irods-kc-sync repair-keycloak --dry-run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	var plan domain.SyncPlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode dry-run plan: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if plan.Mode != "repair-keycloak" || plan.Authority != "irods" || plan.Realm != cfg.Keycloak.Realm || plan.Zone != cfg.IRODS.Zone {
		t.Fatalf("unexpected plan metadata: %+v", plan)
	}
	return plan
}

func createIRODSGroupWithMember(t *testing.T, ctx context.Context, cfg Config, groupName string, username string) {
	t.Helper()

	ensureIRODSUserExists(t, ctx, cfg, username)
	runIAdmin(t, ctx, cfg, true, "mkgroup", groupName)
	runIAdmin(t, ctx, cfg, true, "atg", groupName, username)
}

func ensureIRODSUserExists(t *testing.T, ctx context.Context, cfg Config, username string) {
	t.Helper()

	username = strings.TrimSpace(username)
	if username == "" {
		t.Fatal("iRODS username is required")
	}

	runIAdmin(t, ctx, cfg, false, "mkuser", username, "rodsuser")
}

func cleanupIRODSGroup(t *testing.T, ctx context.Context, cfg Config, groupName string) {
	t.Helper()

	runIAdmin(t, ctx, cfg, false, "rfg", groupName, cfg.IRODS.PrimaryUser)
	runIAdmin(t, ctx, cfg, false, "rfg", groupName, cfg.IRODS.SecondaryUser)
	runIAdmin(t, ctx, cfg, false, "rmgroup", groupName)
}

func runIAdmin(t *testing.T, ctx context.Context, cfg Config, requireSuccess bool, args ...string) {
	t.Helper()

	commandArgs := append([]string{
		"exec",
		cfg.IRODS.ProviderContainer,
		"env",
		"IRODS_ENVIRONMENT_FILE=/root/.irods/irods_environment.json",
		"iadmin",
	}, args...)
	cmd := exec.CommandContext(ctx, "docker", commandArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if requireSuccess && err != nil {
		t.Fatalf("docker iadmin %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
}

func requireOperation(t *testing.T, plan domain.SyncPlan, action string, target string) domain.PlanOperation {
	t.Helper()

	for _, operation := range plan.Operations {
		if operation.Action == action && operation.Target == target {
			return operation
		}
	}
	t.Fatalf("missing operation action=%q target=%q in plan: %+v", action, target, plan.Operations)
	return domain.PlanOperation{}
}

func forbidOperation(t *testing.T, plan domain.SyncPlan, action string, target string) {
	t.Helper()

	for _, operation := range plan.Operations {
		if operation.Action == action && operation.Target == target {
			t.Fatalf("unexpected operation action=%q target=%q in plan: %+v", action, target, plan.Operations)
		}
	}
}

func uniqueE2EName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

func mirrorPath(cfg Config, groupName string) string {
	return strings.TrimRight(cfg.Fixtures.MirrorRoot, "/") + "/" + groupName
}

func memberTarget(cfg Config, groupName string, username string) string {
	return mirrorPath(cfg, groupName) + "#member:" + username
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Dir(filepath.Dir(filename))
}

type e2eKeycloakClient struct {
	baseURL        *url.URL
	adminUser      string
	adminPassword  string
	httpClient     *http.Client
	token          string
	tokenExpiresAt time.Time
}

type e2eKeycloakGroup struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	Attributes map[string][]string    `json:"attributes,omitempty"`
	SubGroups  []e2eKeycloakGroup     `json:"subGroups,omitempty"`
	raw        map[string]interface{} `json:"-"`
}

type e2eKeycloakUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func newE2EKeycloakClient(t *testing.T, cfg Config) *e2eKeycloakClient {
	t.Helper()

	baseURL, err := url.Parse(cfg.Keycloak.BaseURL)
	if err != nil {
		t.Fatalf("parse Keycloak URL: %v", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Keycloak.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &e2eKeycloakClient{
		baseURL:       baseURL,
		adminUser:     cfg.Keycloak.AdminUser,
		adminPassword: cfg.Keycloak.AdminPassword,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *e2eKeycloakClient) createMirrorGroup(t *testing.T, ctx context.Context, cfg Config, groupName string) {
	t.Helper()

	root, err := c.findGroupByPath(ctx, cfg.Keycloak.Realm, cfg.Fixtures.MirrorRoot)
	if err != nil {
		t.Fatalf("find Keycloak mirror root %q: %v", cfg.Fixtures.MirrorRoot, err)
	}
	if root == nil {
		t.Fatalf("Keycloak mirror root %q not found", cfg.Fixtures.MirrorRoot)
	}

	body := map[string]any{
		"name": groupName,
		"attributes": map[string][]string{
			"irods_group_name": {groupName},
			"irods_zone":       {cfg.IRODS.Zone},
			"managed_by":       {"irods-keycloak-admin"},
			"authority":        {"irods"},
		},
	}
	if err := c.doJSON(ctx, http.MethodPost, c.adminPath(cfg.Keycloak.Realm, "groups", root.ID, "children"), body, nil); err != nil {
		t.Fatalf("create Keycloak mirror group %q: %v", groupName, err)
	}
	for attempt := 0; attempt < 10; attempt++ {
		group, err := c.findMirrorGroup(ctx, cfg, groupName)
		if err != nil {
			t.Fatalf("verify Keycloak mirror group %q: %v", groupName, err)
		}
		if group != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("created Keycloak mirror group %q was not found", groupName)
}

func cleanupKeycloakGroup(t *testing.T, ctx context.Context, client *e2eKeycloakClient, cfg Config, groupName string) {
	t.Helper()

	group, err := client.findMirrorGroup(ctx, cfg, groupName)
	if err != nil || group == nil {
		return
	}
	_ = client.doJSON(ctx, http.MethodDelete, client.adminPath(cfg.Keycloak.Realm, "groups", group.ID), nil, nil)
}

func cleanupKeycloakGroupsWithPrefix(t *testing.T, ctx context.Context, client *e2eKeycloakClient, cfg Config, prefix string) {
	t.Helper()

	groups, err := client.listGroups(ctx, cfg.Keycloak.Realm)
	if err != nil {
		t.Fatalf("list Keycloak groups for cleanup: %v", err)
	}
	for _, group := range groups {
		if strings.HasPrefix(group.Name, prefix) && strings.HasPrefix(group.Path, strings.TrimRight(cfg.Fixtures.MirrorRoot, "/")+"/") {
			_ = client.doJSON(ctx, http.MethodDelete, client.adminPath(cfg.Keycloak.Realm, "groups", group.ID), nil, nil)
		}
	}
}

func (c *e2eKeycloakClient) findMirrorGroup(ctx context.Context, cfg Config, groupName string) (*e2eKeycloakGroup, error) {
	return c.findGroupByPath(ctx, cfg.Keycloak.Realm, mirrorPath(cfg, groupName))
}

func (c *e2eKeycloakClient) findGroupByPath(ctx context.Context, realm string, groupPath string) (*e2eKeycloakGroup, error) {
	groups, err := c.listGroups(ctx, realm)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Path == groupPath {
			return &groups[i], nil
		}
	}
	return nil, nil
}

func (c *e2eKeycloakClient) listGroups(ctx context.Context, realm string) ([]e2eKeycloakGroup, error) {
	values := url.Values{}
	values.Set("briefRepresentation", "false")
	values.Set("first", "0")
	values.Set("max", "500")

	var roots []e2eKeycloakGroup
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "groups")+"?"+values.Encode(), nil, &roots); err != nil {
		return nil, err
	}

	result := make([]e2eKeycloakGroup, 0, len(roots))
	seen := map[string]struct{}{}
	for _, root := range roots {
		var err error
		result, err = c.appendGroupHierarchy(ctx, realm, result, seen, root)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *e2eKeycloakClient) appendGroupHierarchy(ctx context.Context, realm string, result []e2eKeycloakGroup, seen map[string]struct{}, group e2eKeycloakGroup) ([]e2eKeycloakGroup, error) {
	if group.ID != "" {
		if _, ok := seen[group.ID]; ok {
			return result, nil
		}
		seen[group.ID] = struct{}{}
	}
	result = append(result, group)

	children, err := c.listGroupChildren(ctx, realm, group.ID)
	if err != nil {
		return nil, err
	}
	if len(children) == 0 {
		children = group.SubGroups
	}
	for _, subgroup := range children {
		result, err = c.appendGroupHierarchy(ctx, realm, result, seen, subgroup)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *e2eKeycloakClient) listGroupChildren(ctx context.Context, realm string, groupID string) ([]e2eKeycloakGroup, error) {
	if strings.TrimSpace(groupID) == "" {
		return nil, nil
	}

	values := url.Values{}
	values.Set("briefRepresentation", "false")
	values.Set("first", "0")
	values.Set("max", "500")

	var children []e2eKeycloakGroup
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "groups", groupID, "children")+"?"+values.Encode(), nil, &children); err != nil {
		return nil, err
	}
	return children, nil
}

func (c *e2eKeycloakClient) listGroupMembers(t *testing.T, ctx context.Context, cfg Config, groupID string) []e2eKeycloakUser {
	t.Helper()

	values := url.Values{}
	values.Set("briefRepresentation", "false")
	var users []e2eKeycloakUser
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(cfg.Keycloak.Realm, "groups", groupID, "members")+"?"+values.Encode(), nil, &users); err != nil {
		t.Fatalf("list Keycloak group members for %q: %v", groupID, err)
	}
	return users
}

func (c *e2eKeycloakClient) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}

	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		requestBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.resolve(path), requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return e2eKeycloakStatusError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *e2eKeycloakClient) accessToken(ctx context.Context) (string, error) {
	if c.token != "" && time.Now().Before(c.tokenExpiresAt.Add(-30*time.Second)) {
		return c.token, nil
	}

	values := url.Values{}
	values.Set("grant_type", "password")
	values.Set("client_id", "admin-cli")
	values.Set("username", c.adminUser)
	values.Set("password", c.adminPassword)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.realmPath("master", "protocol", "openid-connect", "token"), strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", e2eKeycloakStatusError(resp)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("Keycloak token response did not include access_token")
	}
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 60
	}
	c.token = token.AccessToken
	c.tokenExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return c.token, nil
}

func (c *e2eKeycloakClient) adminPath(realm string, parts ...string) string {
	segments := append([]string{"admin", "realms", realm}, parts...)
	return e2ePathJoin(segments...)
}

func (c *e2eKeycloakClient) realmPath(realm string, parts ...string) string {
	segments := append([]string{"realms", realm}, parts...)
	return c.resolve(e2ePathJoin(segments...))
}

func (c *e2eKeycloakClient) resolve(path string) string {
	ref, err := url.Parse(path)
	if err != nil {
		ref = &url.URL{Path: path}
	}
	if ref.IsAbs() {
		return ref.String()
	}
	base := *c.baseURL
	basePath := strings.TrimRight(base.Path, "/")
	if basePath != "" && strings.HasPrefix(ref.Path, "/") {
		ref.Path = basePath + ref.Path
	}
	return base.ResolveReference(ref).String()
}

func e2ePathJoin(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	return "/" + strings.Join(escaped, "/")
}

func e2eKeycloakStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("Keycloak Admin REST request failed: %s", resp.Status)
	}
	return fmt.Errorf("Keycloak Admin REST request failed: %s: %s", resp.Status, text)
}
