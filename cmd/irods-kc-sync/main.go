package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/config"
	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/irodsadapter"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/mapper"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
	"github.com/michael-conway/irods-keycloak-admin/internal/workflow/repair"
)

const appName = "irods-keycloak-admin"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return 2
	}

	switch args[0] {
	case "repair-keycloak":
		return runRepairKeycloak(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "plan", "bootstrap-keycloak":
		_, _ = fmt.Fprintf(stderr, "irods-kc-sync %s is scaffolded but not implemented yet\n", args[0])
		return 1
	default:
		usage(stderr)
		return 2
	}
}

func runRepairKeycloak(args []string, stdout io.Writer, stderr io.Writer) int {
	cfg := config.FromEnv()
	flags := flag.NewFlagSet("repair-keycloak", flag.ContinueOnError)
	flags.SetOutput(stderr)

	dryRun := flags.Bool("dry-run", false, "produce a repair plan without mutating iRODS or Keycloak")
	realm := flags.String("realm", firstNonEmpty(cfg.KeycloakRealm, envFirst("IRODS_KC_E2E_KEYCLOAK_REALM")), "Keycloak realm to inspect")
	zone := flags.String("zone", firstNonEmpty(cfg.IRODSZone, envFirst("IRODS_KC_E2E_IRODS_ZONE")), "iRODS zone to inspect")

	irodsEnv := flags.String("irods-env", os.Getenv("IRODS_ENVIRONMENT_FILE"), "iCommands environment file; defaults to ~/.irods/irods_environment.json when no direct iRODS host is provided")
	irodsHost := flags.String("irods-host", envFirst("IRODS_KC_IRODS_HOST", "IRODS_KC_E2E_IRODS_PROVIDER_HOST"), "iRODS provider host for direct connection")
	irodsPort := flags.Int("irods-port", envInt(0, "IRODS_KC_IRODS_PORT", "IRODS_KC_E2E_IRODS_PROVIDER_PORT"), "iRODS provider port for direct connection")
	irodsUser := flags.String("irods-user", envFirst("IRODS_KC_IRODS_USER", "IRODS_KC_IRODS_ADMIN_USER", "IRODS_KC_E2E_IRODS_ADMIN_USER"), "iRODS user for direct connection")
	irodsPassword := flags.String("irods-password", envFirst("IRODS_KC_IRODS_PASSWORD", "IRODS_KC_IRODS_ADMIN_PASSWORD", "IRODS_KC_E2E_IRODS_ADMIN_PASSWORD"), "iRODS password for direct connection")
	irodsResource := flags.String("irods-resource", envFirst("IRODS_KC_IRODS_RESOURCE", "IRODS_KC_E2E_IRODS_PROVIDER_RESOURCE"), "default iRODS resource for direct connection")

	keycloakURL := flags.String("keycloak-url", envFirst("IRODS_KC_KEYCLOAK_BASE_URL", "IRODS_KC_E2E_KEYCLOAK_BASE_URL"), "Keycloak base URL")
	keycloakAdminRealm := flags.String("keycloak-admin-realm", envFirst("IRODS_KC_KEYCLOAK_ADMIN_REALM"), "Keycloak realm used to obtain the admin token")
	keycloakClientID := flags.String("keycloak-client-id", envFirst("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID"), "Keycloak admin token client ID")
	keycloakClientSecret := flags.String("keycloak-client-secret", envFirst("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET"), "Keycloak admin token client secret")
	keycloakAdminUser := flags.String("keycloak-admin-user", envFirst("IRODS_KC_KEYCLOAK_ADMIN_USER", "IRODS_KC_E2E_KEYCLOAK_ADMIN_USER", "KEYCLOAK_ADMIN"), "Keycloak admin username")
	keycloakAdminPassword := flags.String("keycloak-admin-password", envFirst("IRODS_KC_KEYCLOAK_ADMIN_PASSWORD", "IRODS_KC_E2E_KEYCLOAK_ADMIN_PASSWORD", "KEYCLOAK_ADMIN_PASSWORD"), "Keycloak admin password")
	keycloakInsecureSkipVerify := flags.Bool("keycloak-insecure-skip-verify", envBool(false, "IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY", "IRODS_KC_E2E_KEYCLOAK_INSECURE_SKIP_VERIFY"), "skip Keycloak TLS certificate verification for local test stacks")
	keycloakMirrorRoot := flags.String("keycloak-mirror-root", firstNonEmpty(cfg.KeycloakMirrorRoot, envFirst("IRODS_KC_E2E_KEYCLOAK_MIRROR_ROOT")), "managed Keycloak mirror group root")
	outPath := flags.String("out", "", "write the dry-run plan JSON to this file while also preserving stdout JSON")
	planPath := flags.String("plan-path", "", "alias for --out")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if !*dryRun {
		_, _ = fmt.Fprintln(stderr, "repair-keycloak currently supports planning only; pass --dry-run")
		return 2
	}
	if strings.TrimSpace(*realm) == "" {
		_, _ = fmt.Fprintln(stderr, "Keycloak realm is required; pass --realm or set IRODS_KC_KEYCLOAK_REALM")
		return 2
	}
	outputPath, err := resolvePlanOutputPath(*outPath, *planPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	irodsClient, account, err := newIRODSClient(*irodsEnv, irodsadapter.ConnectionConfig{
		Host:            *irodsHost,
		Port:            *irodsPort,
		Zone:            firstNonEmpty(*zone, cfg.IRODSZone),
		Username:        *irodsUser,
		Password:        *irodsPassword,
		DefaultResource: *irodsResource,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "initialize iRODS client: %v\n", err)
		return 1
	}
	defer irodsClient.Close()

	if strings.TrimSpace(*zone) == "" && account != nil {
		*zone = account.ClientZone
	}
	if strings.TrimSpace(*zone) == "" {
		_, _ = fmt.Fprintln(stderr, "iRODS zone is required; pass --zone or initialize an iCommands environment")
		return 2
	}

	keycloakClient, err := keycloakadmin.NewHTTPClient(keycloakadmin.HTTPClientConfig{
		BaseURL:            firstNonEmpty(*keycloakURL, "https://127.0.0.1:8443"),
		AdminRealm:         *keycloakAdminRealm,
		ClientID:           *keycloakClientID,
		ClientSecret:       *keycloakClientSecret,
		Username:           *keycloakAdminUser,
		Password:           *keycloakAdminPassword,
		InsecureSkipVerify: *keycloakInsecureSkipVerify,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "initialize Keycloak client: %v\n", err)
		return 1
	}

	service := repair.Service{
		IRODS:        irodsClient,
		Keycloak:     keycloakClient,
		Mapper:       mapper.Mapper{DefaultZone: *zone},
		DefaultRealm: *realm,
		DefaultZone:  *zone,
		MirrorRoot:   *keycloakMirrorRoot,
	}
	plan, err := service.RepairKeycloak(ctx, domain.RepairRequest{
		RequestMetadata: domain.RequestMetadata{
			Realm:  *realm,
			Zone:   *zone,
			DryRun: true,
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "repair-keycloak dry-run failed: %v\n", err)
		return 1
	}

	if outputPath != "" {
		if err := writePlanFile(outputPath, plan); err != nil {
			_, _ = fmt.Fprintf(stderr, "write plan file: %v\n", err)
			return 1
		}
	}
	if err := writePlanJSON(stdout, plan); err != nil {
		_, _ = fmt.Fprintf(stderr, "write plan: %v\n", err)
		return 1
	}
	return 0
}

func runApply(args []string, stdout io.Writer, stderr io.Writer) int {
	cfg := config.FromEnv()
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.SetOutput(stderr)

	planPath := flags.String("plan", "", "repair-keycloak plan JSON to apply")
	realm := flags.String("realm", firstNonEmpty(cfg.KeycloakRealm, envFirst("IRODS_KC_E2E_KEYCLOAK_REALM")), "expected Keycloak realm for the plan")
	zone := flags.String("zone", firstNonEmpty(cfg.IRODSZone, envFirst("IRODS_KC_E2E_IRODS_ZONE")), "expected iRODS zone for the plan")
	prompts := flags.String("prompts", string(planreview.PromptModeRequired), "prompt policy: required, all, or none")

	keycloakURL := flags.String("keycloak-url", envFirst("IRODS_KC_KEYCLOAK_BASE_URL", "IRODS_KC_E2E_KEYCLOAK_BASE_URL"), "Keycloak base URL")
	keycloakAdminRealm := flags.String("keycloak-admin-realm", envFirst("IRODS_KC_KEYCLOAK_ADMIN_REALM"), "Keycloak realm used to obtain the admin token")
	keycloakClientID := flags.String("keycloak-client-id", envFirst("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID"), "Keycloak admin token client ID")
	keycloakClientSecret := flags.String("keycloak-client-secret", envFirst("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET"), "Keycloak admin token client secret")
	keycloakAdminUser := flags.String("keycloak-admin-user", envFirst("IRODS_KC_KEYCLOAK_ADMIN_USER", "IRODS_KC_E2E_KEYCLOAK_ADMIN_USER", "KEYCLOAK_ADMIN"), "Keycloak admin username")
	keycloakAdminPassword := flags.String("keycloak-admin-password", envFirst("IRODS_KC_KEYCLOAK_ADMIN_PASSWORD", "IRODS_KC_E2E_KEYCLOAK_ADMIN_PASSWORD", "KEYCLOAK_ADMIN_PASSWORD"), "Keycloak admin password")
	keycloakInsecureSkipVerify := flags.Bool("keycloak-insecure-skip-verify", envBool(false, "IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY", "IRODS_KC_E2E_KEYCLOAK_INSECURE_SKIP_VERIFY"), "skip Keycloak TLS certificate verification for local test stacks")
	keycloakMirrorRoot := flags.String("keycloak-mirror-root", firstNonEmpty(cfg.KeycloakMirrorRoot, envFirst("IRODS_KC_E2E_KEYCLOAK_MIRROR_ROOT")), "managed Keycloak mirror group root")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if strings.TrimSpace(*planPath) == "" {
		_, _ = fmt.Fprintln(stderr, "plan file is required; pass --plan plan.json")
		return 2
	}
	promptMode := planreview.PromptMode(strings.ToLower(strings.TrimSpace(*prompts)))
	if err := planreview.ValidatePromptMode(promptMode); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	syncPlan, err := readPlanFile(*planPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "read plan file: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*realm) == "" {
		*realm = syncPlan.Realm
	}
	if strings.TrimSpace(*zone) == "" {
		*zone = syncPlan.Zone
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	keycloakClient, err := keycloakadmin.NewHTTPClient(keycloakadmin.HTTPClientConfig{
		BaseURL:            firstNonEmpty(*keycloakURL, "https://127.0.0.1:8443"),
		AdminRealm:         *keycloakAdminRealm,
		ClientID:           *keycloakClientID,
		ClientSecret:       *keycloakClientSecret,
		Username:           *keycloakAdminUser,
		Password:           *keycloakAdminPassword,
		InsecureSkipVerify: *keycloakInsecureSkipVerify,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "initialize Keycloak client: %v\n", err)
		return 1
	}

	service := repair.Service{
		Keycloak:     keycloakClient,
		DefaultRealm: *realm,
		DefaultZone:  *zone,
		MirrorRoot:   *keycloakMirrorRoot,
		PromptMode:   promptMode,
	}
	if promptMode != planreview.PromptModeNone {
		service.Reviewer = newTerminalReviewer(os.Stdin, stderr)
	}
	result, err := service.Apply(ctx, domain.ApplyRequest{
		RequestMetadata: domain.RequestMetadata{
			Realm: *realm,
			Zone:  *zone,
		},
		Plan: &syncPlan,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "apply failed: %v\n", err)
		return 1
	}
	if err := writeApplyResultJSON(stdout, result); err != nil {
		_, _ = fmt.Fprintf(stderr, "write apply result: %v\n", err)
		return 1
	}
	if result.Failed > 0 {
		return 1
	}
	return 0
}

func newIRODSClient(envFile string, cfg irodsadapter.ConnectionConfig) (*irodsadapter.FileSystemClient, *irodstypes.IRODSAccount, error) {
	if strings.TrimSpace(cfg.Host) != "" {
		client, account, err := irodsadapter.NewFromConnectionConfig(cfg, appName)
		return client, account, err
	}
	client, account, err := irodsadapter.NewFromICommandsEnvironment(envFile, appName)
	return client, account, err
}

func resolvePlanOutputPath(outPath string, planPath string) (string, error) {
	outPath = strings.TrimSpace(outPath)
	planPath = strings.TrimSpace(planPath)
	if outPath != "" && planPath != "" && outPath != planPath {
		return "", fmt.Errorf("--out and --plan-path must match when both are provided")
	}
	if outPath != "" {
		return outPath, nil
	}
	return planPath, nil
}

func writePlanFile(path string, plan domain.SyncPlan) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return writePlanJSON(file, plan)
}

func readPlanFile(path string) (domain.SyncPlan, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.SyncPlan{}, err
	}
	defer file.Close()

	var plan domain.SyncPlan
	if err := json.NewDecoder(file).Decode(&plan); err != nil {
		return domain.SyncPlan{}, err
	}
	return plan, nil
}

func writePlanJSON(writer io.Writer, plan domain.SyncPlan) error {
	return writeIndentedJSON(writer, plan)
}

func writeApplyResultJSON(writer io.Writer, result domain.ApplyResult) error {
	return writeIndentedJSON(writer, result)
}

func writeIndentedJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "usage: irods-kc-sync {plan|apply|bootstrap-keycloak|repair-keycloak}")
	_, _ = fmt.Fprintln(out, "       irods-kc-sync repair-keycloak --dry-run [--realm REALM] [--zone ZONE] [--out PLAN.json]")
	_, _ = fmt.Fprintln(out, "       irods-kc-sync apply --plan PLAN.json [--realm REALM] [--zone ZONE] [--prompts required|all|none]")
}

type terminalReviewer struct {
	input  *bufio.Reader
	output io.Writer
}

func newTerminalReviewer(input io.Reader, output io.Writer) *terminalReviewer {
	return &terminalReviewer{
		input:  bufio.NewReader(input),
		output: output,
	}
}

func (r *terminalReviewer) Review(_ context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation) (planreview.Decision, error) {
	printOperationReview(r.output, syncPlan, operation)
	for {
		_, _ = fmt.Fprint(r.output, "Decision [a]ccept, [s]kip, [aa]accept all, [ss]skip all: ")
		line, err := r.input.ReadString('\n')
		if err != nil && !(err == io.EOF && strings.TrimSpace(line) != "") {
			return "", err
		}
		decision, normalizeErr := planreview.NormalizeDecision(planreview.Decision(line))
		if normalizeErr == nil {
			return decision, nil
		}
		_, _ = fmt.Fprintf(r.output, "Invalid decision: %v\n", normalizeErr)
	}
}

func printOperationReview(out io.Writer, syncPlan domain.SyncPlan, operation domain.PlanOperation) {
	_, _ = fmt.Fprintf(out, "\nPlan: %s  Realm: %s  Zone: %s\n", syncPlan.PlanID, syncPlan.Realm, syncPlan.Zone)
	_, _ = fmt.Fprintf(out, "Operation: %s\n", operation.OperationID)
	_, _ = fmt.Fprintf(out, "Action: %s\n", operation.Action)
	_, _ = fmt.Fprintf(out, "Target: %s\n", operation.Target)
	_, _ = fmt.Fprintf(out, "Risk: %s\n", operation.Risk)
	if len(operation.Evidence) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "Evidence:")
	for _, key := range sortedEvidenceKeys(operation.Evidence) {
		_, _ = fmt.Fprintf(out, "  %s: %s\n", key, formatEvidenceValue(operation.Evidence[key]))
	}
}

func sortedEvidenceKeys(evidence map[string]any) []string {
	keys := make([]string, 0, len(evidence))
	for key := range evidence {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatEvidenceValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envFirst(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func envInt(fallback int, names ...string) int {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func envBool(fallback bool, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
