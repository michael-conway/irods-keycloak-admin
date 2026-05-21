package e2e

import "testing"

func TestLoadConfigDefaultsMatchGridStackHostPorts(t *testing.T) {
	cfg := LoadConfig()

	if cfg.IRODS.ProviderHost != "127.0.0.1" || cfg.IRODS.ProviderPort != 1247 {
		t.Fatalf("unexpected iRODS provider endpoint: %+v", cfg.IRODS)
	}
	if cfg.REST.ProviderBaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected provider REST URL: %q", cfg.REST.ProviderBaseURL)
	}
	if cfg.REST.ResourceBaseURL != "http://127.0.0.1:8082" {
		t.Fatalf("unexpected resource REST URL: %q", cfg.REST.ResourceBaseURL)
	}
	if cfg.Keycloak.BaseURL != "https://127.0.0.1:8443" {
		t.Fatalf("unexpected Keycloak URL: %q", cfg.Keycloak.BaseURL)
	}
	if cfg.Keycloak.ManagementURL != "http://127.0.0.1:19090" {
		t.Fatalf("unexpected Keycloak management URL: %q", cfg.Keycloak.ManagementURL)
	}
}

func TestLoadConfigAcceptsTargetOverrides(t *testing.T) {
	t.Setenv("IRODS_KC_E2E_ENABLED", "true")
	t.Setenv("IRODS_KC_E2E_TARGET", "grid-stack")
	t.Setenv("IRODS_KC_E2E_KEYCLOAK_REALM", "drs")
	t.Setenv("IRODS_KC_E2E_FIXTURE_GROUPS", "project-alpha, project-beta")

	cfg := LoadConfig()
	if !cfg.Enabled {
		t.Fatal("expected e2e config to be enabled")
	}
	if cfg.Target != "grid-stack" {
		t.Fatalf("unexpected target: %q", cfg.Target)
	}
	if cfg.Keycloak.Realm != "drs" {
		t.Fatalf("unexpected realm: %q", cfg.Keycloak.Realm)
	}
	if len(cfg.Fixtures.Groups) != 2 || cfg.Fixtures.Groups[0] != "project-alpha" || cfg.Fixtures.Groups[1] != "project-beta" {
		t.Fatalf("unexpected fixture groups: %+v", cfg.Fixtures.Groups)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}
