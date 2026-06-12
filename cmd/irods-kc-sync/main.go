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
	"github.com/michael-conway/irods-keycloak-admin/internal/workflow/provisioning"
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
	case "sync", "synch", "repair-keycloak":
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
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	flags.SetOutput(stderr)

	dryRun := flags.Bool("dry-run", false, "produce a sync plan without mutating iRODS or Keycloak")
	target := flags.String("target", domain.SyncTargetKeycloak, "sync target system: keycloak or irods")
	keycloakUserID := flags.String("keycloak-user-id", "", "Keycloak user ID to plan for --target=irods")
	keycloakGroupID := flags.String("keycloak-group-id", "", "Keycloak group ID to plan for --target=irods")
	keycloakGroupPath := flags.String("keycloak-group-path", "", "Keycloak group path to plan for --target=irods")
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
	passwordActionReportPath := flags.String("password-action-report", "", "write optional scenario-3 password-action report JSON to this file; reporting only, no password mutation")

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
		_, _ = fmt.Fprintln(stderr, "sync currently supports planning only; pass --dry-run")
		return 2
	}
	targetSystem, err := normalizeSyncTarget(*target)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
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
	irodsSelector, err := validateIRODSSyncSelector(targetSystem, *keycloakUserID, *keycloakGroupID, *keycloakGroupPath)
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

	var plan domain.SyncPlan
	if targetSystem == domain.SyncTargetIRODS {
		service := provisioning.Service{
			IRODS:        irodsClient,
			Keycloak:     keycloakClient,
			DefaultRealm: *realm,
			DefaultZone:  *zone,
		}
		requestMetadata := domain.RequestMetadata{
			Realm:  *realm,
			Zone:   *zone,
			DryRun: true,
			Source: "sync-cli",
		}
		switch irodsSelector.kind {
		case irodsSyncSelectorUser:
			plan, err = service.PlanUser(ctx, domain.ProvisionUserRequest{
				RequestMetadata: requestMetadata,
				KeycloakUserID:  *keycloakUserID,
			})
		case irodsSyncSelectorGroup:
			plan, err = service.PlanGroup(ctx, domain.ProvisionGroupRequest{
				RequestMetadata:   requestMetadata,
				KeycloakGroupID:   *keycloakGroupID,
				KeycloakGroupPath: *keycloakGroupPath,
			})
		}
	} else {
		service := repair.Service{
			IRODS:        irodsClient,
			Keycloak:     keycloakClient,
			Mapper:       mapper.Mapper{DefaultZone: *zone},
			DefaultRealm: *realm,
			DefaultZone:  *zone,
			MirrorRoot:   *keycloakMirrorRoot,
		}
		plan, err = service.RepairKeycloak(ctx, domain.RepairRequest{
			RequestMetadata: domain.RequestMetadata{
				Realm:  *realm,
				Zone:   *zone,
				DryRun: true,
			},
		})
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sync --dry-run failed: %v\n", err)
		return 1
	}

	if outputPath != "" {
		if err := writePlanFile(outputPath, plan); err != nil {
			_, _ = fmt.Fprintf(stderr, "write plan file: %v\n", err)
			return 1
		}
	}
	if reportPath := strings.TrimSpace(*passwordActionReportPath); reportPath != "" {
		report := buildPasswordActionReport(plan)
		if err := writePasswordActionReportFile(reportPath, report); err != nil {
			_, _ = fmt.Fprintf(stderr, "write password action report: %v\n", err)
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

	planPath := flags.String("plan", "", "sync plan JSON to apply")
	realm := flags.String("realm", firstNonEmpty(cfg.KeycloakRealm, envFirst("IRODS_KC_E2E_KEYCLOAK_REALM")), "expected Keycloak realm for the plan")
	zone := flags.String("zone", firstNonEmpty(cfg.IRODSZone, envFirst("IRODS_KC_E2E_IRODS_ZONE")), "expected iRODS zone for the plan")
	prompts := flags.String("prompts", string(planreview.PromptModeRequired), "prompt policy: required, all, or none")

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

	planTarget := strings.ToLower(strings.TrimSpace(syncPlan.TargetSystem))
	if planTarget == "" {
		planTarget = domain.SyncTargetKeycloak
	}
	if planTarget == domain.SyncTargetIRODS {
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
		service := provisioning.Service{
			IRODS:        irodsClient,
			DefaultRealm: *realm,
			DefaultZone:  *zone,
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

func writePasswordActionReportFile(path string, report domain.PasswordActionReport) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return writeIndentedJSON(file, report)
}

func buildPasswordActionReport(plan domain.SyncPlan) domain.PasswordActionReport {
	report := domain.PasswordActionReport{
		ReportFormatVersion: "irods-keycloak-admin.password-action-report.v1",
		PlanID:              plan.PlanID,
		Realm:               plan.Realm,
		Zone:                plan.Zone,
		TargetSystem:        plan.TargetSystem,
		Notification:        "out_of_scope",
		CredentialPath:      "future_keycloak_to_irods_direct",
		Actions:             []domain.PasswordAction{},
	}
	seenUsers := map[string]struct{}{}
	for _, operation := range plan.Operations {
		switch operation.Action {
		case domain.PlanActionIRODSUserCreate:
			action := passwordActionFromOperation(operation, "password_setup_required", "irods_user_create_planned")
			if action.IRODSUsername == "" {
				action.IRODSUsername = operation.Target
			}
			if passwordActionKey(action) == "" {
				continue
			}
			if _, exists := seenUsers[passwordActionKey(action)]; exists {
				continue
			}
			seenUsers[passwordActionKey(action)] = struct{}{}
			report.Actions = append(report.Actions, action)
		case domain.PlanActionIRODSUserMetadataSync:
			action := passwordActionFromOperation(operation, "credential_state_unknown", "existing_irods_user_metadata_sync_planned")
			if action.IRODSUsername == "" {
				action.IRODSUsername = operation.Target
			}
			key := passwordActionKey(action)
			if key == "" {
				continue
			}
			if _, exists := seenUsers[key]; exists {
				continue
			}
			seenUsers[key] = struct{}{}
			report.Actions = append(report.Actions, action)
		}
	}
	return report
}

func passwordActionFromOperation(operation domain.PlanOperation, action string, reason string) domain.PasswordAction {
	return domain.PasswordAction{
		Action:         action,
		KeycloakUserID: evidenceString(operation.Evidence, "keycloak_user_id"),
		IRODSUsername:  firstNonEmpty(evidenceString(operation.Evidence, "irods_username"), evidenceString(operation.Evidence, "keycloak_username")),
		Reason:         reason,
	}
}

func passwordActionKey(action domain.PasswordAction) string {
	return firstNonEmpty(action.KeycloakUserID, action.IRODSUsername)
}

func writeIndentedJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "usage: irods-kc-sync {plan|apply|bootstrap-keycloak|sync}")
	_, _ = fmt.Fprintln(out, "       irods-kc-sync sync --dry-run [--target keycloak|irods] [--keycloak-user-id USER_ID | --keycloak-group-id GROUP_ID | --keycloak-group-path GROUP_PATH] [--realm REALM] [--zone ZONE] [--out PLAN.json] [--password-action-report REPORT.json]")
	_, _ = fmt.Fprintln(out, "       irods-kc-sync apply --plan PLAN.json [--realm REALM] [--zone ZONE] [--prompts required|all|none]")
}

func normalizeSyncTarget(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", domain.SyncTargetKeycloak:
		return domain.SyncTargetKeycloak, nil
	case domain.SyncTargetIRODS:
		return domain.SyncTargetIRODS, nil
	default:
		return "", fmt.Errorf("sync target must be one of: %s, %s", domain.SyncTargetKeycloak, domain.SyncTargetIRODS)
	}
}

type irodsSyncSelectorKind string

const (
	irodsSyncSelectorNone  irodsSyncSelectorKind = ""
	irodsSyncSelectorUser  irodsSyncSelectorKind = "user"
	irodsSyncSelectorGroup irodsSyncSelectorKind = "group"
)

type irodsSyncSelector struct {
	kind irodsSyncSelectorKind
}

func validateIRODSSyncSelector(targetSystem string, keycloakUserID string, keycloakGroupID string, keycloakGroupPath string) (irodsSyncSelector, error) {
	if targetSystem != domain.SyncTargetIRODS {
		return irodsSyncSelector{}, nil
	}

	hasUser := strings.TrimSpace(keycloakUserID) != ""
	hasGroupID := strings.TrimSpace(keycloakGroupID) != ""
	hasGroupPath := strings.TrimSpace(keycloakGroupPath) != ""
	hasGroup := hasGroupID || hasGroupPath

	switch {
	case hasUser && hasGroup:
		return irodsSyncSelector{}, fmt.Errorf("sync --target=irods accepts either --keycloak-user-id or a group selector, not both")
	case hasUser:
		return irodsSyncSelector{kind: irodsSyncSelectorUser}, nil
	case hasGroup:
		return irodsSyncSelector{kind: irodsSyncSelectorGroup}, nil
	default:
		return irodsSyncSelector{}, fmt.Errorf("sync --target=irods requires --keycloak-user-id, --keycloak-group-id, or --keycloak-group-path")
	}
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
	if cause := evidenceString(operation.Evidence, "change_cause"); cause != "" {
		_, _ = fmt.Fprintf(out, "Cause: %s\n", cause)
	}
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
	sort.Slice(keys, func(i int, j int) bool {
		left := evidenceDisplayPriority(keys[i])
		right := evidenceDisplayPriority(keys[j])
		if left != right {
			return left < right
		}
		return keys[i] < keys[j]
	})
	return keys
}

func evidenceDisplayPriority(key string) int {
	switch key {
	case "change_cause":
		return 0
	case "irods_group_name", "irods_username", "irods_zone":
		return 2
	case "keycloak_path", "keycloak_group_id", "keycloak_user", "keycloak_user_id":
		return 3
	default:
		return 10
	}
}

func evidenceString(evidence map[string]any, key string) string {
	if evidence == nil {
		return ""
	}
	value, ok := evidence[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
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
