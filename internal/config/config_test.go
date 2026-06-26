package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, dir string, name string, contents string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
	return path
}

func TestReadConfigYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTestFile(t, dir, "irods-keycloak-admin-config.yaml", `
ServiceName: test-admin
ListenAddress: :18081
PublicURL: http://localhost:18081
LogLevel: debug
IRODSHost: 127.0.0.1
IRODSPort: 1247
IRODSZone: tempZone
IRODSAdminUser: rods
IRODSAdminPassword: rods
IRODSDefaultResource: providerResc
IRODSAuthScheme: native
IRODSNegotiationPolicy: CS_NEG_DONT_CARE
KeycloakBaseURL: https://127.0.0.1:8443
KeycloakRealm: irods
KeycloakAdminRealm: master
KeycloakAdminUser: admin
KeycloakAdminPassword: admin
KeycloakAdminClientID: irods-kc-admin-api
KeycloakAdminClientSecret: irods-kc-admin-api-secret
KeycloakInsecureSkipVerify: true
KeycloakMirrorRoot: /irods
KeycloakEventSharedSecret: local-event-secret
`)

	cfg, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}

	if cfg.ServiceName != "test-admin" {
		t.Fatalf("expected ServiceName from YAML, got %q", cfg.ServiceName)
	}
	if cfg.ListenAddress != ":18081" {
		t.Fatalf("expected ListenAddress from YAML, got %q", cfg.ListenAddress)
	}
	if cfg.IRODSHost != "127.0.0.1" || cfg.IRODSPort != 1247 || cfg.IRODSZone != "tempZone" {
		t.Fatalf("unexpected iRODS config: %+v", cfg)
	}
	if cfg.KeycloakBaseURL != "https://127.0.0.1:8443" || cfg.KeycloakRealm != "irods" {
		t.Fatalf("unexpected Keycloak config: %+v", cfg)
	}
	if !cfg.KeycloakInsecureSkipVerify {
		t.Fatal("expected KeycloakInsecureSkipVerify from YAML")
	}
	if cfg.KeycloakEventSharedSecret != "local-event-secret" {
		t.Fatalf("expected event shared secret from YAML, got %q", cfg.KeycloakEventSharedSecret)
	}
}

func TestReadConfigEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTestFile(t, dir, "irods-keycloak-admin-config.yaml", `
ListenAddress: :18081
IRODSHost: file-host
IRODSPort: 1247
KeycloakBaseURL: https://file-keycloak.example.org
KeycloakEventSharedSecret: file-secret
`)

	t.Setenv("IRODS_KC_LISTEN_ADDRESS", ":28081")
	t.Setenv("IRODS_KC_IRODS_HOST", "env-host")
	t.Setenv("IRODS_KC_IRODS_PORT", "2247")
	t.Setenv("IRODS_KC_KEYCLOAK_BASE_URL", "https://env-keycloak.example.org")
	t.Setenv("IRODS_KC_KEYCLOAK_EVENT_SHARED_SECRET", "env-secret")

	cfg, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}

	if cfg.ListenAddress != ":28081" {
		t.Fatalf("expected env ListenAddress override, got %q", cfg.ListenAddress)
	}
	if cfg.IRODSHost != "env-host" || cfg.IRODSPort != 2247 {
		t.Fatalf("expected env iRODS override, got host=%q port=%d", cfg.IRODSHost, cfg.IRODSPort)
	}
	if cfg.KeycloakBaseURL != "https://env-keycloak.example.org" {
		t.Fatalf("expected env KeycloakBaseURL override, got %q", cfg.KeycloakBaseURL)
	}
	if cfg.KeycloakEventSharedSecret != "env-secret" {
		t.Fatalf("expected env shared secret override, got %q", cfg.KeycloakEventSharedSecret)
	}
}

func TestLoadUsesConfigFileEnvVar(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTestFile(t, dir, "admin-config.yaml", "ListenAddress: :38081\nIRODSZone: tempZone\n")
	t.Setenv(ConfigFileEnvVar, configPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load from %s: %v", ConfigFileEnvVar, err)
	}

	if cfg.ListenAddress != ":38081" {
		t.Fatalf("expected ListenAddress from config file env var, got %q", cfg.ListenAddress)
	}
	if cfg.IRODSZone != "tempZone" {
		t.Fatalf("expected IRODSZone from config file env var, got %q", cfg.IRODSZone)
	}
}

func TestReadConfigSecretFileSupport(t *testing.T) {
	dir := t.TempDir()
	irodsPasswordFile := writeTestFile(t, dir, "irods-password.txt", "rods\n")
	keycloakPasswordFile := writeTestFile(t, dir, "keycloak-password.txt", "admin\n")
	keycloakClientSecretFile := writeTestFile(t, dir, "keycloak-client-secret.txt", "client-secret\n")
	eventSecretFile := writeTestFile(t, dir, "event-secret.txt", "event-secret\n")
	configPath := writeTestFile(t, dir, "admin-config.yaml", ""+
		"IRODSAdminPasswordFile: "+irodsPasswordFile+"\n"+
		"KeycloakAdminPasswordFile: "+keycloakPasswordFile+"\n"+
		"KeycloakAdminClientSecretFile: "+keycloakClientSecretFile+"\n"+
		"KeycloakEventSharedSecretFile: "+eventSecretFile+"\n")

	cfg, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}

	if cfg.IRODSAdminPassword != "rods" {
		t.Fatalf("expected iRODS password from file, got %q", cfg.IRODSAdminPassword)
	}
	if cfg.KeycloakAdminPassword != "admin" {
		t.Fatalf("expected Keycloak password from file, got %q", cfg.KeycloakAdminPassword)
	}
	if cfg.KeycloakAdminClientSecret != "client-secret" {
		t.Fatalf("expected Keycloak client secret from file, got %q", cfg.KeycloakAdminClientSecret)
	}
	if cfg.KeycloakEventSharedSecret != "event-secret" {
		t.Fatalf("expected event shared secret from file, got %q", cfg.KeycloakEventSharedSecret)
	}
}

func TestPackageSampleConfigLoads(t *testing.T) {
	cfg, err := ReadConfig("keycloak-admin.grid-stack.sample.yaml")
	if err != nil {
		t.Fatalf("expected package sample config to load: %v", err)
	}

	if cfg.IRODSHost != "127.0.0.1" {
		t.Fatalf("expected sample IRODSHost, got %q", cfg.IRODSHost)
	}
	if cfg.KeycloakRealm != "irods" {
		t.Fatalf("expected sample KeycloakRealm, got %q", cfg.KeycloakRealm)
	}
	if cfg.KeycloakEventSharedSecret == "" {
		t.Fatal("expected sample KeycloakEventSharedSecret")
	}
}
