package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/config"
	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/irodsadapter"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/mapper"
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
	case "plan", "apply", "bootstrap-keycloak":
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

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(plan); err != nil {
		_, _ = fmt.Fprintf(stderr, "write plan: %v\n", err)
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

func usage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "usage: irods-kc-sync {plan|apply|bootstrap-keycloak|repair-keycloak}")
	_, _ = fmt.Fprintln(out, "       irods-kc-sync repair-keycloak --dry-run [--realm REALM] [--zone ZONE]")
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
