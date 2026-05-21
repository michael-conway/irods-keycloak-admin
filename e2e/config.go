package e2e

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

type Config struct {
	Enabled  bool
	Target   string
	IRODS    IRODSConfig
	REST     RESTConfig
	Keycloak KeycloakConfig
	Fixtures FixtureConfig
}

type IRODSConfig struct {
	ProviderHost      string
	ProviderPort      int
	Zone              string
	AdminUser         string
	AdminPassword     string
	ProviderResource  string
	PrimaryUser       string
	PrimaryPassword   string
	SecondaryUser     string
	SecondaryPassword string
}

type RESTConfig struct {
	ProviderBaseURL string
	ResourceBaseURL string
}

type KeycloakConfig struct {
	BaseURL              string
	ManagementURL        string
	Realm                string
	AdminUser            string
	AdminPassword        string
	AdminAPIClientID     string
	AdminAPIClientSecret string
	AdminCLIClientID     string
	AdminCLIClientSecret string
	InsecureSkipVerify   bool
}

type FixtureConfig struct {
	MirrorRoot string
	Groups     []string
}

func LoadConfig() Config {
	return Config{
		Enabled: envBool("IRODS_KC_E2E_ENABLED", false),
		Target:  envString("IRODS_KC_E2E_TARGET", "internal"),
		IRODS: IRODSConfig{
			ProviderHost:      envString("IRODS_KC_E2E_IRODS_PROVIDER_HOST", "127.0.0.1"),
			ProviderPort:      envInt("IRODS_KC_E2E_IRODS_PROVIDER_PORT", 1247),
			Zone:              envString("IRODS_KC_E2E_IRODS_ZONE", "tempZone"),
			AdminUser:         envString("IRODS_KC_E2E_IRODS_ADMIN_USER", "rods"),
			AdminPassword:     envString("IRODS_KC_E2E_IRODS_ADMIN_PASSWORD", "rods"),
			ProviderResource:  envString("IRODS_KC_E2E_IRODS_PROVIDER_RESOURCE", "providerResc"),
			PrimaryUser:       envString("IRODS_KC_E2E_IRODS_PRIMARY_TEST_USER", "test1"),
			PrimaryPassword:   envString("IRODS_KC_E2E_IRODS_PRIMARY_TEST_PASSWORD", "test"),
			SecondaryUser:     envString("IRODS_KC_E2E_IRODS_SECONDARY_TEST_USER", "test2"),
			SecondaryPassword: envString("IRODS_KC_E2E_IRODS_SECONDARY_TEST_PASSWORD", "test"),
		},
		REST: RESTConfig{
			ProviderBaseURL: envString("IRODS_KC_E2E_REST_PROVIDER_BASE_URL", "http://127.0.0.1:8080"),
			ResourceBaseURL: envString("IRODS_KC_E2E_REST_RESOURCE_BASE_URL", "http://127.0.0.1:8082"),
		},
		Keycloak: KeycloakConfig{
			BaseURL:              envString("IRODS_KC_E2E_KEYCLOAK_BASE_URL", "https://127.0.0.1:8443"),
			ManagementURL:        envString("IRODS_KC_E2E_KEYCLOAK_MANAGEMENT_URL", "http://127.0.0.1:19090"),
			Realm:                envString("IRODS_KC_E2E_KEYCLOAK_REALM", "irods"),
			AdminUser:            envString("IRODS_KC_E2E_KEYCLOAK_ADMIN_USER", "admin"),
			AdminPassword:        envString("IRODS_KC_E2E_KEYCLOAK_ADMIN_PASSWORD", "admin"),
			AdminAPIClientID:     envString("IRODS_KC_E2E_KEYCLOAK_ADMIN_API_CLIENT_ID", "irods-kc-admin-api"),
			AdminAPIClientSecret: envString("IRODS_KC_E2E_KEYCLOAK_ADMIN_API_CLIENT_SECRET", "irods-kc-admin-api-secret"),
			AdminCLIClientID:     envString("IRODS_KC_E2E_KEYCLOAK_ADMIN_CLI_CLIENT_ID", "irods-kc-admin-cli"),
			AdminCLIClientSecret: envString("IRODS_KC_E2E_KEYCLOAK_ADMIN_CLI_CLIENT_SECRET", "irods-kc-admin-cli-secret"),
			InsecureSkipVerify:   envBool("IRODS_KC_E2E_KEYCLOAK_INSECURE_SKIP_VERIFY", true),
		},
		Fixtures: FixtureConfig{
			MirrorRoot: envString("IRODS_KC_E2E_KEYCLOAK_MIRROR_ROOT", "/irods"),
			Groups:     envCSV("IRODS_KC_E2E_FIXTURE_GROUPS", []string{"project-alpha", "project-beta", "irods-admins"}),
		},
	}
}

func RequireConfig(t testing.TB) Config {
	t.Helper()

	cfg := LoadConfig()
	if !cfg.Enabled {
		t.Skip("set IRODS_KC_E2E_ENABLED=1 to run e2e tests")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("invalid e2e config: %v", err)
	}
	return cfg
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.IRODS.ProviderHost) == "" {
		return errors.New("IRODS provider host is required")
	}
	if c.IRODS.ProviderPort <= 0 {
		return errors.New("IRODS provider port must be positive")
	}
	if strings.TrimSpace(c.IRODS.Zone) == "" {
		return errors.New("iRODS zone is required")
	}
	for name, rawURL := range map[string]string{
		"REST provider base URL":  c.REST.ProviderBaseURL,
		"REST resource base URL":  c.REST.ResourceBaseURL,
		"Keycloak base URL":       c.Keycloak.BaseURL,
		"Keycloak management URL": c.Keycloak.ManagementURL,
	} {
		if err := validateURL(name, rawURL); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.Keycloak.Realm) == "" {
		return errors.New("Keycloak realm is required")
	}
	return nil
}

func validateURL(name string, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must include scheme and host", name)
	}
	return nil
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envCSV(name string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
