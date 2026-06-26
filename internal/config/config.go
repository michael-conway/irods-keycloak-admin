package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"gopkg.in/yaml.v2"
)

const ServiceVersion = "0.1.0-dev"

type Config struct {
	ServiceName   string `yaml:"ServiceName"`
	ListenAddress string `yaml:"ListenAddress"`
	PublicURL     string `yaml:"PublicURL"`
	LogLevel      string `yaml:"LogLevel"`

	IRODSHost              string `yaml:"IRODSHost"`
	IRODSPort              int    `yaml:"IRODSPort"`
	IRODSZone              string `yaml:"IRODSZone"`
	IRODSAdminUser         string `yaml:"IRODSAdminUser"`
	IRODSAdminPassword     string `yaml:"IRODSAdminPassword"`
	IRODSAdminPasswordFile string `yaml:"IRODSAdminPasswordFile"`
	IRODSDefaultResource   string `yaml:"IRODSDefaultResource"`
	IRODSAuthScheme        string `yaml:"IRODSAuthScheme"`
	IRODSNegotiationPolicy string `yaml:"IRODSNegotiationPolicy"`

	KeycloakBaseURL               string `yaml:"KeycloakBaseURL"`
	KeycloakRealm                 string `yaml:"KeycloakRealm"`
	KeycloakAdminRealm            string `yaml:"KeycloakAdminRealm"`
	KeycloakAdminUser             string `yaml:"KeycloakAdminUser"`
	KeycloakAdminPassword         string `yaml:"KeycloakAdminPassword"`
	KeycloakAdminPasswordFile     string `yaml:"KeycloakAdminPasswordFile"`
	KeycloakAdminClientID         string `yaml:"KeycloakAdminClientID"`
	KeycloakAdminClientSecret     string `yaml:"KeycloakAdminClientSecret"`
	KeycloakAdminClientSecretFile string `yaml:"KeycloakAdminClientSecretFile"`
	KeycloakInsecureSkipVerify    bool   `yaml:"KeycloakInsecureSkipVerify"`
	KeycloakMirrorRoot            string `yaml:"KeycloakMirrorRoot"`
	KeycloakEventSharedSecret     string `yaml:"KeycloakEventSharedSecret"`
	KeycloakEventSharedSecretFile string `yaml:"KeycloakEventSharedSecretFile"`
}

const (
	DefaultConfigName = "irods-keycloak-admin-config"
	DefaultConfigType = "yaml"
	ConfigFileEnvVar  = "IRODS_KC_CONFIG_FILE"
)

func Default() Config {
	return Config{
		ServiceName:            "irods-keycloak-admin",
		ListenAddress:          ":8081",
		LogLevel:               "info",
		IRODSPort:              1247,
		IRODSAuthScheme:        "native",
		IRODSNegotiationPolicy: "CS_NEG_DONT_CARE",
		KeycloakBaseURL:        "https://127.0.0.1:8443",
		KeycloakMirrorRoot:     "/irods",
	}
}

func Load() (Config, error) {
	cfg := Default()
	if configFilePath := strings.TrimSpace(os.Getenv(ConfigFileEnvVar)); configFilePath != "" {
		fileCfg, err := readConfigFile(configFilePath)
		if err != nil {
			return Config{}, err
		}
		cfg = mergeConfig(cfg, fileCfg)
	}
	cfg = applyEnvOverrides(cfg)
	if err := resolveSecrets(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func FromEnv() Config {
	cfg, err := Load()
	if err != nil {
		cfg = applyEnvOverrides(Default())
	}
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
		ServiceName:                    c.ServiceName,
		ListenAddress:                  c.ListenAddress,
		IRODSZone:                      c.IRODSZone,
		IRODSHost:                      c.IRODSHost,
		IRODSPort:                      c.IRODSPort,
		IRODSAdminUser:                 c.IRODSAdminUser,
		IRODSDefaultResource:           c.IRODSDefaultResource,
		KeycloakBaseURL:                c.KeycloakBaseURL,
		KeycloakRealm:                  c.KeycloakRealm,
		KeycloakAdminRealm:             c.KeycloakAdminRealm,
		KeycloakAdminClientID:          c.KeycloakAdminClientID,
		KeycloakMirrorRoot:             c.KeycloakMirrorRoot,
		KeycloakEventSharedSecretSet:   strings.TrimSpace(c.KeycloakEventSharedSecret) != "",
		KeycloakEventSharedSecretModel: "X-IRODS-KC-Shared-Secret",
	}
}

func ReadConfig(configFilePath string) (Config, error) {
	cfg := Default()
	fileCfg, err := readConfigFile(configFilePath)
	if err != nil {
		return Config{}, err
	}
	cfg = mergeConfig(cfg, fileCfg)
	cfg = applyEnvOverrides(cfg)
	if err := resolveSecrets(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func readConfigFile(configFilePath string) (Config, error) {
	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return Config{}, errors.New("config file path is required")
	}
	configBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return Config{}, fmt.Errorf("fatal error config file %q: %w", configFilePath, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(configBytes, &cfg); err != nil {
		return Config{}, fmt.Errorf("unable to decode config file %q: %w", configFilePath, err)
	}
	return cfg, nil
}

func applyEnvOverrides(cfg Config) Config {
	cfg.ServiceName = envOrDefault("IRODS_KC_SERVICE_NAME", cfg.ServiceName)
	cfg.ListenAddress = envOrDefault("IRODS_KC_LISTEN_ADDRESS", cfg.ListenAddress)
	cfg.PublicURL = envOrDefault("IRODS_KC_PUBLIC_URL", cfg.PublicURL)
	cfg.LogLevel = envOrDefault("IRODS_KC_LOG_LEVEL", cfg.LogLevel)

	cfg.IRODSHost = envOrDefault("IRODS_KC_IRODS_HOST", cfg.IRODSHost)
	cfg.IRODSPort = envIntOrDefault(cfg.IRODSPort, "IRODS_KC_IRODS_PORT")
	cfg.IRODSZone = envOrDefault("IRODS_KC_IRODS_ZONE", cfg.IRODSZone)
	cfg.IRODSAdminUser = envOrDefault("IRODS_KC_IRODS_ADMIN_USER", envOrDefault("IRODS_KC_IRODS_USER", cfg.IRODSAdminUser))
	cfg.IRODSAdminPassword = envOrDefault("IRODS_KC_IRODS_ADMIN_PASSWORD", envOrDefault("IRODS_KC_IRODS_PASSWORD", cfg.IRODSAdminPassword))
	cfg.IRODSAdminPasswordFile = envOrDefault("IRODS_KC_IRODS_ADMIN_PASSWORD_FILE", cfg.IRODSAdminPasswordFile)
	cfg.IRODSDefaultResource = envOrDefault("IRODS_KC_IRODS_DEFAULT_RESOURCE", envOrDefault("IRODS_KC_IRODS_RESOURCE", cfg.IRODSDefaultResource))
	cfg.IRODSAuthScheme = envOrDefault("IRODS_KC_IRODS_AUTH_SCHEME", cfg.IRODSAuthScheme)
	cfg.IRODSNegotiationPolicy = envOrDefault("IRODS_KC_IRODS_NEGOTIATION_POLICY", cfg.IRODSNegotiationPolicy)

	cfg.KeycloakBaseURL = envOrDefault("IRODS_KC_KEYCLOAK_BASE_URL", cfg.KeycloakBaseURL)
	cfg.KeycloakRealm = envOrDefault("IRODS_KC_KEYCLOAK_REALM", cfg.KeycloakRealm)
	cfg.KeycloakAdminRealm = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_REALM", cfg.KeycloakAdminRealm)
	cfg.KeycloakAdminUser = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_USER", cfg.KeycloakAdminUser)
	cfg.KeycloakAdminPassword = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_PASSWORD", cfg.KeycloakAdminPassword)
	cfg.KeycloakAdminPasswordFile = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_PASSWORD_FILE", cfg.KeycloakAdminPasswordFile)
	cfg.KeycloakAdminClientID = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID", cfg.KeycloakAdminClientID)
	cfg.KeycloakAdminClientSecret = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET", cfg.KeycloakAdminClientSecret)
	cfg.KeycloakAdminClientSecretFile = envOrDefault("IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET_FILE", cfg.KeycloakAdminClientSecretFile)
	cfg.KeycloakInsecureSkipVerify = envBoolOrDefault(cfg.KeycloakInsecureSkipVerify, "IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY")
	cfg.KeycloakMirrorRoot = envOrDefault("IRODS_KC_KEYCLOAK_MIRROR_ROOT", cfg.KeycloakMirrorRoot)
	cfg.KeycloakEventSharedSecret = envOrDefault("IRODS_KC_KEYCLOAK_EVENT_SHARED_SECRET", cfg.KeycloakEventSharedSecret)
	cfg.KeycloakEventSharedSecretFile = envOrDefault("IRODS_KC_KEYCLOAK_EVENT_SHARED_SECRET_FILE", cfg.KeycloakEventSharedSecretFile)

	return cfg
}

func resolveSecrets(cfg *Config) error {
	var err error
	cfg.IRODSAdminPassword, err = resolveSecret(cfg.IRODSAdminPassword, cfg.IRODSAdminPasswordFile, "iRODS admin password")
	if err != nil {
		return err
	}
	cfg.KeycloakAdminPassword, err = resolveSecret(cfg.KeycloakAdminPassword, cfg.KeycloakAdminPasswordFile, "Keycloak admin password")
	if err != nil {
		return err
	}
	cfg.KeycloakAdminClientSecret, err = resolveSecret(cfg.KeycloakAdminClientSecret, cfg.KeycloakAdminClientSecretFile, "Keycloak admin client secret")
	if err != nil {
		return err
	}
	cfg.KeycloakEventSharedSecret, err = resolveSecret(cfg.KeycloakEventSharedSecret, cfg.KeycloakEventSharedSecretFile, "Keycloak event shared secret")
	if err != nil {
		return err
	}
	return nil
}

func resolveSecret(secret string, secretFile string, secretName string) (string, error) {
	if strings.TrimSpace(secret) != "" {
		return secret, nil
	}
	secretFile = strings.TrimSpace(secretFile)
	if secretFile == "" {
		return "", nil
	}
	secretBytes, err := os.ReadFile(secretFile)
	if err != nil {
		return "", fmt.Errorf("failed to read %s file %q: %w", secretName, secretFile, err)
	}
	return strings.TrimSpace(string(secretBytes)), nil
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envIntOrDefault(fallback int, names ...string) int {
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

func envBoolOrDefault(fallback bool, names ...string) bool {
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

func mergeConfig(base Config, overlay Config) Config {
	if strings.TrimSpace(overlay.ServiceName) != "" {
		base.ServiceName = overlay.ServiceName
	}
	if strings.TrimSpace(overlay.ListenAddress) != "" {
		base.ListenAddress = overlay.ListenAddress
	}
	if strings.TrimSpace(overlay.PublicURL) != "" {
		base.PublicURL = overlay.PublicURL
	}
	if strings.TrimSpace(overlay.LogLevel) != "" {
		base.LogLevel = overlay.LogLevel
	}
	if strings.TrimSpace(overlay.IRODSHost) != "" {
		base.IRODSHost = overlay.IRODSHost
	}
	if overlay.IRODSPort != 0 {
		base.IRODSPort = overlay.IRODSPort
	}
	if strings.TrimSpace(overlay.IRODSZone) != "" {
		base.IRODSZone = overlay.IRODSZone
	}
	if strings.TrimSpace(overlay.IRODSAdminUser) != "" {
		base.IRODSAdminUser = overlay.IRODSAdminUser
	}
	if strings.TrimSpace(overlay.IRODSAdminPassword) != "" {
		base.IRODSAdminPassword = overlay.IRODSAdminPassword
	}
	if strings.TrimSpace(overlay.IRODSAdminPasswordFile) != "" {
		base.IRODSAdminPasswordFile = overlay.IRODSAdminPasswordFile
	}
	if strings.TrimSpace(overlay.IRODSDefaultResource) != "" {
		base.IRODSDefaultResource = overlay.IRODSDefaultResource
	}
	if strings.TrimSpace(overlay.IRODSAuthScheme) != "" {
		base.IRODSAuthScheme = overlay.IRODSAuthScheme
	}
	if strings.TrimSpace(overlay.IRODSNegotiationPolicy) != "" {
		base.IRODSNegotiationPolicy = overlay.IRODSNegotiationPolicy
	}
	if strings.TrimSpace(overlay.KeycloakBaseURL) != "" {
		base.KeycloakBaseURL = overlay.KeycloakBaseURL
	}
	if strings.TrimSpace(overlay.KeycloakRealm) != "" {
		base.KeycloakRealm = overlay.KeycloakRealm
	}
	if strings.TrimSpace(overlay.KeycloakAdminRealm) != "" {
		base.KeycloakAdminRealm = overlay.KeycloakAdminRealm
	}
	if strings.TrimSpace(overlay.KeycloakAdminUser) != "" {
		base.KeycloakAdminUser = overlay.KeycloakAdminUser
	}
	if strings.TrimSpace(overlay.KeycloakAdminPassword) != "" {
		base.KeycloakAdminPassword = overlay.KeycloakAdminPassword
	}
	if strings.TrimSpace(overlay.KeycloakAdminPasswordFile) != "" {
		base.KeycloakAdminPasswordFile = overlay.KeycloakAdminPasswordFile
	}
	if strings.TrimSpace(overlay.KeycloakAdminClientID) != "" {
		base.KeycloakAdminClientID = overlay.KeycloakAdminClientID
	}
	if strings.TrimSpace(overlay.KeycloakAdminClientSecret) != "" {
		base.KeycloakAdminClientSecret = overlay.KeycloakAdminClientSecret
	}
	if strings.TrimSpace(overlay.KeycloakAdminClientSecretFile) != "" {
		base.KeycloakAdminClientSecretFile = overlay.KeycloakAdminClientSecretFile
	}
	if overlay.KeycloakInsecureSkipVerify {
		base.KeycloakInsecureSkipVerify = overlay.KeycloakInsecureSkipVerify
	}
	if strings.TrimSpace(overlay.KeycloakMirrorRoot) != "" {
		base.KeycloakMirrorRoot = overlay.KeycloakMirrorRoot
	}
	if strings.TrimSpace(overlay.KeycloakEventSharedSecret) != "" {
		base.KeycloakEventSharedSecret = overlay.KeycloakEventSharedSecret
	}
	if strings.TrimSpace(overlay.KeycloakEventSharedSecretFile) != "" {
		base.KeycloakEventSharedSecretFile = overlay.KeycloakEventSharedSecretFile
	}
	return base
}
