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
	flags.Usage = func() {
		syncUsage(stderr)
	}

	dryRun := flags.Bool("dry-run", false, "required for sync today; produce a JSON plan without mutating iRODS or Keycloak")
	target := flags.String("target", domain.SyncTargetKeycloak, "system the plan will mutate: keycloak mirrors iRODS into Keycloak; irods provisions/reconciles selected Keycloak users or groups into iRODS")
	keycloakUserID := flags.String("keycloak-user-id", "", "stable Keycloak user UUID to plan into iRODS; valid only with --target=irods and mutually exclusive with group selectors")
	keycloakGroupID := flags.String("keycloak-group-id", "", "stable Keycloak group UUID to plan into iRODS; valid only with --target=irods and mutually exclusive with --keycloak-user-id")
	keycloakGroupPath := flags.String("keycloak-group-path", "", "Keycloak group path, such as /projects/alpha, to plan into iRODS; valid only with --target=irods and mutually exclusive with --keycloak-user-id")
	realm := flags.String("realm", firstNonEmpty(cfg.KeycloakRealm, envFirst("IRODS_KC_E2E_KEYCLOAK_REALM")), "Keycloak realm containing the users/groups to inspect; required if not configured in environment")
	zone := flags.String("zone", firstNonEmpty(cfg.IRODSZone, envFirst("IRODS_KC_E2E_IRODS_ZONE")), "iRODS zone used for user/group lookup and planned mutations; required unless provided by configuration/environment")

	irodsHost := flags.String("irods-host", firstNonEmpty(cfg.IRODSHost, envFirst("IRODS_KC_E2E_IRODS_PROVIDER_HOST")), "iRODS provider host for direct admin connection")
	irodsPort := flags.Int("irods-port", firstNonZero(cfg.IRODSPort, envInt(0, "IRODS_KC_E2E_IRODS_PROVIDER_PORT")), "iRODS provider port for direct admin connection; required with --irods-host unless provided by environment")
	irodsUser := flags.String("irods-user", firstNonEmpty(cfg.IRODSAdminUser, envFirst("IRODS_KC_E2E_IRODS_ADMIN_USER")), "iRODS admin username for direct connection; must be allowed to create users/groups, change group membership, and write mapping AVUs")
	irodsPassword := flags.String("irods-password", firstNonEmpty(cfg.IRODSAdminPassword, envFirst("IRODS_KC_E2E_IRODS_ADMIN_PASSWORD")), "iRODS password for --irods-user")
	irodsResource := flags.String("irods-resource", firstNonEmpty(cfg.IRODSDefaultResource, envFirst("IRODS_KC_E2E_IRODS_PROVIDER_RESOURCE")), "default iRODS resource for direct connection; used for account initialization, not for sync policy")

	keycloakURL := flags.String("keycloak-url", firstNonEmpty(cfg.KeycloakBaseURL, envFirst("IRODS_KC_E2E_KEYCLOAK_BASE_URL")), "Keycloak base URL, for example https://keycloak.example.org; defaults to https://127.0.0.1:8443 if unset")
	keycloakAdminRealm := flags.String("keycloak-admin-realm", cfg.KeycloakAdminRealm, "Keycloak realm used to obtain the admin token; leave empty for the client default when supported")
	keycloakClientID := flags.String("keycloak-client-id", cfg.KeycloakAdminClientID, "Keycloak admin token client ID for client-credentials or direct-grant authentication")
	keycloakClientSecret := flags.String("keycloak-client-secret", cfg.KeycloakAdminClientSecret, "Keycloak admin token client secret; used with --keycloak-client-id when the client is confidential")
	keycloakAdminUser := flags.String("keycloak-admin-user", firstNonEmpty(cfg.KeycloakAdminUser, envFirst("IRODS_KC_E2E_KEYCLOAK_ADMIN_USER", "KEYCLOAK_ADMIN")), "Keycloak admin username for password-grant authentication when client credentials are not used")
	keycloakAdminPassword := flags.String("keycloak-admin-password", firstNonEmpty(cfg.KeycloakAdminPassword, envFirst("IRODS_KC_E2E_KEYCLOAK_ADMIN_PASSWORD", "KEYCLOAK_ADMIN_PASSWORD")), "Keycloak admin password for --keycloak-admin-user")
	keycloakInsecureSkipVerify := flags.Bool("keycloak-insecure-skip-verify", cfg.KeycloakInsecureSkipVerify || envBool(false, "IRODS_KC_E2E_KEYCLOAK_INSECURE_SKIP_VERIFY"), "skip Keycloak TLS certificate verification; use only for local test stacks with self-signed certificates")
	keycloakMirrorRoot := flags.String("keycloak-mirror-root", firstNonEmpty(cfg.KeycloakMirrorRoot, envFirst("IRODS_KC_E2E_KEYCLOAK_MIRROR_ROOT")), "managed Keycloak group root used for --target=keycloak mirror repair, such as /irods")
	outPath := flags.String("out", "", "write the generated dry-run plan JSON to this file while also writing the same JSON to stdout")
	planPath := flags.String("plan-path", "", "deprecated alias for --out; if both are provided, they must name the same file")
	passwordActionReportPath := flags.String("password-action-report", "", "write scenario-3 password-action report JSON derived from the plan; informational only and never applies, stores, or prints passwords")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected positional arguments for sync: %s\nsync accepts only named flags; run 'irods-kc-sync sync --help' for parameter meanings.\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if !*dryRun {
		_, _ = fmt.Fprintln(stderr, "sync currently supports planning only; pass --dry-run to generate a reviewable JSON plan without mutating iRODS or Keycloak")
		return 2
	}
	targetSystem, err := normalizeSyncTarget(*target)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if strings.TrimSpace(*realm) == "" {
		_, _ = fmt.Fprintln(stderr, "Keycloak realm is required; pass --realm REALM to identify the realm containing the users/groups being synchronized, or set IRODS_KC_KEYCLOAK_REALM")
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

	irodsClient, account, err := newIRODSClient(irodsadapter.ConnectionConfig{
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
		_, _ = fmt.Fprintln(stderr, "iRODS zone is required; pass --zone ZONE or set IRODS_KC_IRODS_ZONE")
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
	flags.Usage = func() {
		applyUsage(stderr)
	}

	planPath := flags.String("plan", "", "path to a JSON plan produced by 'irods-kc-sync sync --dry-run'; apply refuses to run without an explicit plan file")
	realm := flags.String("realm", firstNonEmpty(cfg.KeycloakRealm, envFirst("IRODS_KC_E2E_KEYCLOAK_REALM")), "expected Keycloak realm for the plan; defaults to the plan realm when omitted and no environment value is set")
	zone := flags.String("zone", firstNonEmpty(cfg.IRODSZone, envFirst("IRODS_KC_E2E_IRODS_ZONE")), "expected iRODS zone for the plan; defaults to the plan zone when omitted and no environment value is set")
	prompts := flags.String("prompts", string(planreview.PromptModeRequired), "review prompt policy: required prompts only for risky operations, all prompts for every operation, none applies without interactive confirmation")

	irodsHost := flags.String("irods-host", firstNonEmpty(cfg.IRODSHost, envFirst("IRODS_KC_E2E_IRODS_PROVIDER_HOST")), "iRODS provider host for direct admin connection; used only when applying iRODS-target plans")
	irodsPort := flags.Int("irods-port", firstNonZero(cfg.IRODSPort, envInt(0, "IRODS_KC_E2E_IRODS_PROVIDER_PORT")), "iRODS provider port for direct admin connection; used only when applying iRODS-target plans")
	irodsUser := flags.String("irods-user", firstNonEmpty(cfg.IRODSAdminUser, envFirst("IRODS_KC_E2E_IRODS_ADMIN_USER")), "iRODS admin username for applying iRODS-target plans")
	irodsPassword := flags.String("irods-password", firstNonEmpty(cfg.IRODSAdminPassword, envFirst("IRODS_KC_E2E_IRODS_ADMIN_PASSWORD")), "iRODS password for --irods-user when using direct connection")
	irodsResource := flags.String("irods-resource", firstNonEmpty(cfg.IRODSDefaultResource, envFirst("IRODS_KC_E2E_IRODS_PROVIDER_RESOURCE")), "default iRODS resource for direct connection; used for account initialization, not for sync policy")

	keycloakURL := flags.String("keycloak-url", firstNonEmpty(cfg.KeycloakBaseURL, envFirst("IRODS_KC_E2E_KEYCLOAK_BASE_URL")), "Keycloak base URL used when applying Keycloak-target mirror plans")
	keycloakAdminRealm := flags.String("keycloak-admin-realm", cfg.KeycloakAdminRealm, "Keycloak realm used to obtain the admin token; leave empty for the client default when supported")
	keycloakClientID := flags.String("keycloak-client-id", cfg.KeycloakAdminClientID, "Keycloak admin token client ID for client-credentials or direct-grant authentication")
	keycloakClientSecret := flags.String("keycloak-client-secret", cfg.KeycloakAdminClientSecret, "Keycloak admin token client secret; used with --keycloak-client-id when the client is confidential")
	keycloakAdminUser := flags.String("keycloak-admin-user", firstNonEmpty(cfg.KeycloakAdminUser, envFirst("IRODS_KC_E2E_KEYCLOAK_ADMIN_USER", "KEYCLOAK_ADMIN")), "Keycloak admin username for password-grant authentication when client credentials are not used")
	keycloakAdminPassword := flags.String("keycloak-admin-password", firstNonEmpty(cfg.KeycloakAdminPassword, envFirst("IRODS_KC_E2E_KEYCLOAK_ADMIN_PASSWORD", "KEYCLOAK_ADMIN_PASSWORD")), "Keycloak admin password for --keycloak-admin-user")
	keycloakInsecureSkipVerify := flags.Bool("keycloak-insecure-skip-verify", cfg.KeycloakInsecureSkipVerify || envBool(false, "IRODS_KC_E2E_KEYCLOAK_INSECURE_SKIP_VERIFY"), "skip Keycloak TLS certificate verification; use only for local test stacks with self-signed certificates")
	keycloakMirrorRoot := flags.String("keycloak-mirror-root", firstNonEmpty(cfg.KeycloakMirrorRoot, envFirst("IRODS_KC_E2E_KEYCLOAK_MIRROR_ROOT")), "managed Keycloak group root used to validate/apply Keycloak mirror operations, such as /irods")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected positional arguments for apply: %s\napply accepts only named flags; run 'irods-kc-sync apply --help' for parameter meanings.\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if strings.TrimSpace(*planPath) == "" {
		_, _ = fmt.Fprintln(stderr, "plan file is required; pass --plan PLAN.json with a JSON plan created by 'irods-kc-sync sync --dry-run'")
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
		irodsClient, account, err := newIRODSClient(irodsadapter.ConnectionConfig{
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
	if planRequiresIRODSApplyClient(syncPlan) {
		irodsClient, account, err := newIRODSClient(irodsadapter.ConnectionConfig{
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
			service.DefaultZone = *zone
		}
		service.IRODS = irodsClient
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

func planRequiresIRODSApplyClient(syncPlan domain.SyncPlan) bool {
	for _, operation := range syncPlan.Operations {
		if operation.Action == domain.PlanActionIRODSUserMetadataSync {
			return true
		}
	}
	return false
}

func newIRODSClient(cfg irodsadapter.ConnectionConfig) (*irodsadapter.FileSystemClient, *irodstypes.IRODSAccount, error) {
	client, account, err := irodsadapter.NewFromConnectionConfig(cfg, appName)
	return client, account, err
}

func resolvePlanOutputPath(outPath string, planPath string) (string, error) {
	outPath = strings.TrimSpace(outPath)
	planPath = strings.TrimSpace(planPath)
	if outPath != "" && planPath != "" && outPath != planPath {
		return "", fmt.Errorf("--out and --plan-path both name the dry-run plan output file; provide only --out, or make both values identical")
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
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Run 'irods-kc-sync sync --help' or 'irods-kc-sync apply --help' for parameter meanings.")
}

func syncUsage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "usage: irods-kc-sync sync --dry-run [parameters]")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Purpose:")
	_, _ = fmt.Fprintln(out, "  Build a reviewable JSON synchronization plan. This command does not mutate")
	_, _ = fmt.Fprintln(out, "  iRODS or Keycloak; apply the accepted plan with 'irods-kc-sync apply --plan'.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Target modes:")
	_, _ = fmt.Fprintln(out, "  --target=keycloak mirrors authoritative iRODS users/groups/memberships into Keycloak.")
	_, _ = fmt.Fprintln(out, "  --target=irods provisions or reconciles one selected Keycloak user or group into iRODS.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Selectors:")
	_, _ = fmt.Fprintln(out, "  --target=irods requires exactly one selector: --keycloak-user-id,")
	_, _ = fmt.Fprintln(out, "  --keycloak-group-id, or --keycloak-group-path. Selected-group planning includes")
	_, _ = fmt.Fprintln(out, "  group metadata plus conservative membership drift.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Output:")
	_, _ = fmt.Fprintln(out, "  The plan is always written to stdout. Use --out PLAN.json to also save it.")
	_, _ = fmt.Fprintln(out, "  Use --password-action-report REPORT.json only for scenario-3 reporting; it")
	_, _ = fmt.Fprintln(out, "  never contains password material and never applies credential changes.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Examples:")
	_, _ = fmt.Fprintln(out, "  irods-kc-sync sync --dry-run --target=keycloak --realm irods --zone tempZone --out plan.json")
	_, _ = fmt.Fprintln(out, "  irods-kc-sync sync --dry-run --target=irods --keycloak-user-id 3c5f... --realm irods --zone tempZone --out plan.json")
	_, _ = fmt.Fprintln(out, "  irods-kc-sync sync --dry-run --target=irods --keycloak-group-path /projects/alpha --password-action-report password-actions.json")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Parameters:")
	_, _ = fmt.Fprintln(out, "  --dry-run")
	_, _ = fmt.Fprintln(out, "      Required. Generates a plan only; sync currently has no direct mutation mode.")
	_, _ = fmt.Fprintln(out, "  --target keycloak|irods")
	_, _ = fmt.Fprintln(out, "      Plan destination. keycloak repairs Keycloak mirror state from iRODS.")
	_, _ = fmt.Fprintln(out, "      irods plans selected Keycloak-originating user/group mutations into iRODS.")
	_, _ = fmt.Fprintln(out, "  --keycloak-user-id USER_ID")
	_, _ = fmt.Fprintln(out, "      Stable Keycloak user UUID for --target=irods user provisioning/metadata sync.")
	_, _ = fmt.Fprintln(out, "  --keycloak-group-id GROUP_ID")
	_, _ = fmt.Fprintln(out, "      Stable Keycloak group UUID for --target=irods group and membership planning.")
	_, _ = fmt.Fprintln(out, "  --keycloak-group-path GROUP_PATH")
	_, _ = fmt.Fprintln(out, "      Keycloak group path alternative to --keycloak-group-id, for example /projects/alpha.")
	_, _ = fmt.Fprintln(out, "  --realm REALM")
	_, _ = fmt.Fprintln(out, "      Keycloak realm to inspect. Required unless supplied by configuration/environment.")
	_, _ = fmt.Fprintln(out, "  --zone ZONE")
	_, _ = fmt.Fprintln(out, "      iRODS zone for lookup and planned mutations.")
	_, _ = fmt.Fprintln(out, "  --out PLAN.json")
	_, _ = fmt.Fprintln(out, "      Also write the stdout plan JSON to a file.")
	_, _ = fmt.Fprintln(out, "  --plan-path PLAN.json")
	_, _ = fmt.Fprintln(out, "      Deprecated alias for --out.")
	_, _ = fmt.Fprintln(out, "  --password-action-report REPORT.json")
	_, _ = fmt.Fprintln(out, "      Scenario-3 credential report derived from user operations; reporting only.")
	_, _ = fmt.Fprintln(out, "  --irods-host HOST, --irods-port PORT, --irods-user USER, --irods-password PASSWORD, --irods-resource RESOURCE")
	_, _ = fmt.Fprintln(out, "      Direct iRODS admin connection parameters.")
	_, _ = fmt.Fprintln(out, "  --keycloak-url URL")
	_, _ = fmt.Fprintln(out, "      Keycloak base URL.")
	_, _ = fmt.Fprintln(out, "  --keycloak-admin-realm REALM")
	_, _ = fmt.Fprintln(out, "      Realm used to obtain the Keycloak admin token.")
	_, _ = fmt.Fprintln(out, "  --keycloak-client-id ID, --keycloak-client-secret SECRET")
	_, _ = fmt.Fprintln(out, "      Keycloak admin client credentials.")
	_, _ = fmt.Fprintln(out, "  --keycloak-admin-user USER, --keycloak-admin-password PASSWORD")
	_, _ = fmt.Fprintln(out, "      Keycloak admin username/password fallback.")
	_, _ = fmt.Fprintln(out, "  --keycloak-insecure-skip-verify")
	_, _ = fmt.Fprintln(out, "      Disable Keycloak TLS verification for local test stacks only.")
	_, _ = fmt.Fprintln(out, "  --keycloak-mirror-root PATH")
	_, _ = fmt.Fprintln(out, "      Managed Keycloak group root for --target=keycloak mirror repair, such as /irods.")
}

func applyUsage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "usage: irods-kc-sync apply --plan PLAN.json [parameters]")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Purpose:")
	_, _ = fmt.Fprintln(out, "  Apply a reviewed JSON plan produced by 'irods-kc-sync sync --dry-run'.")
	_, _ = fmt.Fprintln(out, "  The plan target determines whether iRODS or Keycloak clients are initialized.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Examples:")
	_, _ = fmt.Fprintln(out, "  irods-kc-sync apply --plan plan.json --prompts required")
	_, _ = fmt.Fprintln(out, "  irods-kc-sync apply --plan plan.json --prompts none --realm irods --zone tempZone")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Parameters:")
	_, _ = fmt.Fprintln(out, "  --plan PLAN.json")
	_, _ = fmt.Fprintln(out, "      Required. JSON plan file created by sync --dry-run.")
	_, _ = fmt.Fprintln(out, "  --realm REALM")
	_, _ = fmt.Fprintln(out, "      Expected Keycloak realm. Defaults to the plan realm when not configured.")
	_, _ = fmt.Fprintln(out, "  --zone ZONE")
	_, _ = fmt.Fprintln(out, "      Expected iRODS zone. Defaults to the plan zone when not configured.")
	_, _ = fmt.Fprintln(out, "  --prompts required|all|none")
	_, _ = fmt.Fprintln(out, "      Interactive review policy: required prompts only for risky operations,")
	_, _ = fmt.Fprintln(out, "      all prompts for every operation, none applies without interactive confirmation.")
	_, _ = fmt.Fprintln(out, "  --irods-host HOST, --irods-port PORT, --irods-user USER, --irods-password PASSWORD, --irods-resource RESOURCE")
	_, _ = fmt.Fprintln(out, "      Direct iRODS admin connection parameters for iRODS-target plans.")
	_, _ = fmt.Fprintln(out, "  --keycloak-url URL")
	_, _ = fmt.Fprintln(out, "      Keycloak base URL for Keycloak-target mirror plans.")
	_, _ = fmt.Fprintln(out, "  --keycloak-admin-realm REALM")
	_, _ = fmt.Fprintln(out, "      Realm used to obtain the Keycloak admin token.")
	_, _ = fmt.Fprintln(out, "  --keycloak-client-id ID, --keycloak-client-secret SECRET")
	_, _ = fmt.Fprintln(out, "      Keycloak admin client credentials.")
	_, _ = fmt.Fprintln(out, "  --keycloak-admin-user USER, --keycloak-admin-password PASSWORD")
	_, _ = fmt.Fprintln(out, "      Keycloak admin username/password fallback.")
	_, _ = fmt.Fprintln(out, "  --keycloak-insecure-skip-verify")
	_, _ = fmt.Fprintln(out, "      Disable Keycloak TLS verification for local test stacks only.")
	_, _ = fmt.Fprintln(out, "  --keycloak-mirror-root PATH")
	_, _ = fmt.Fprintln(out, "      Managed Keycloak group root used to validate/apply Keycloak mirror operations.")
}

func normalizeSyncTarget(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", domain.SyncTargetKeycloak:
		return domain.SyncTargetKeycloak, nil
	case domain.SyncTargetIRODS:
		return domain.SyncTargetIRODS, nil
	default:
		return "", fmt.Errorf("--target must be one of %q or %q; keycloak mirrors iRODS into Keycloak, irods plans selected Keycloak-originating users/groups into iRODS", domain.SyncTargetKeycloak, domain.SyncTargetIRODS)
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
		return irodsSyncSelector{}, fmt.Errorf("sync --target=irods accepts exactly one selector: use --keycloak-user-id for one Keycloak user, or use --keycloak-group-id/--keycloak-group-path for one Keycloak group; do not combine user and group selectors")
	case hasUser:
		return irodsSyncSelector{kind: irodsSyncSelectorUser}, nil
	case hasGroup:
		return irodsSyncSelector{kind: irodsSyncSelectorGroup}, nil
	default:
		return irodsSyncSelector{}, fmt.Errorf("sync --target=irods requires a selector so the first iRODS mutation slice stays explicit: pass --keycloak-user-id USER_ID, --keycloak-group-id GROUP_ID, or --keycloak-group-path GROUP_PATH")
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

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
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
