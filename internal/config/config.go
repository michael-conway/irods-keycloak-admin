package config

import (
	"errors"
	"os"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

const ServiceVersion = "0.1.0-dev"

type Config struct {
	ServiceName        string
	ListenAddress      string
	IRODSZone          string
	KeycloakRealm      string
	KeycloakMirrorRoot string
}

func Default() Config {
	return Config{
		ServiceName:        "irods-keycloak-admin",
		ListenAddress:      ":8081",
		KeycloakMirrorRoot: "/irods",
	}
}

func FromEnv() Config {
	cfg := Default()
	cfg.ServiceName = envOrDefault("IRODS_KC_SERVICE_NAME", cfg.ServiceName)
	cfg.ListenAddress = envOrDefault("IRODS_KC_LISTEN_ADDRESS", cfg.ListenAddress)
	cfg.IRODSZone = strings.TrimSpace(os.Getenv("IRODS_KC_IRODS_ZONE"))
	cfg.KeycloakRealm = strings.TrimSpace(os.Getenv("IRODS_KC_KEYCLOAK_REALM"))
	cfg.KeycloakMirrorRoot = envOrDefault("IRODS_KC_KEYCLOAK_MIRROR_ROOT", cfg.KeycloakMirrorRoot)
	return cfg
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ServiceName) == "" {
		return errors.New("service name is required")
	}
	if strings.TrimSpace(c.ListenAddress) == "" {
		return errors.New("listen address is required")
	}
	if strings.TrimSpace(c.KeycloakMirrorRoot) == "" {
		return errors.New("keycloak mirror root is required")
	}
	return nil
}

func (c Config) Summary() domain.ConfigSummary {
	return domain.ConfigSummary{
		ServiceName:        c.ServiceName,
		ListenAddress:      c.ListenAddress,
		IRODSZone:          c.IRODSZone,
		KeycloakRealm:      c.KeycloakRealm,
		KeycloakMirrorRoot: c.KeycloakMirrorRoot,
	}
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
