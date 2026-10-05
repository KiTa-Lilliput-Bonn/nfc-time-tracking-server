package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Logging  LoggingConfig  `yaml:"logging"`
	// BackupTargetPath is set only from NFC_BACKUP_TARGET_PATH (not from YAML).
	BackupTargetPath string `yaml:"-"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
	// PairingAdvertiseHost optional LAN hostname/IP for QR pairing URL (field u). Env: NFC_PAIRING_ADVERTISE_HOST.
	PairingAdvertiseHost string `yaml:"pairing_advertise_host"`
	TLS  struct {
		Enabled  bool   `yaml:"enabled"`
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
	} `yaml:"tls"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	JWTSecret        string     `yaml:"jwt_secret"`
	TokenExpiryHours int        `yaml:"token_expiry_hours"`
	OIDC             OIDCConfig `yaml:"oidc"`
}

// OIDCConfig: Anmeldung über einen OpenID-Connect-Provider (z. B. Authentik, Authelia, Pocket ID).
type OIDCConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Issuer       string `yaml:"issuer"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	// RedirectURL: öffentliche Adresse des Callbacks, z. B. https://zeit.example.de/api/v1/auth/oidc/callback.
	RedirectURL string   `yaml:"redirect_url"`
	Scopes      []string `yaml:"scopes"`
	// UsernameClaim: Claim, dessen Wert beim ersten Login mit dem App-Benutzernamen verglichen wird.
	UsernameClaim string `yaml:"username_claim"`
	// AutoLink: beim ersten SSO-Login automatisch über den Benutzernamen verknüpfen (Standard: true).
	AutoLink *bool `yaml:"auto_link"`
	// PasswordLogin: "all" (Standard) = alle dürfen sich weiter mit Passwort anmelden,
	// "admins" = nur Leitung und Superadmin (Fallback), "none" = niemand.
	PasswordLogin string `yaml:"password_login"`
	// ButtonLabel: Beschriftung des Buttons auf der Login-Seite.
	ButtonLabel string `yaml:"button_label"`
}

// AutoLinkEnabled ist true, wenn auto_link nicht ausdrücklich auf false steht.
func (o OIDCConfig) AutoLinkEnabled() bool {
	return o.AutoLink == nil || *o.AutoLink
}

type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxAgeDays int    `yaml:"max_age_days"`
	MaxBackups int    `yaml:"max_backups"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
}

func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Database: DatabaseConfig{
			Path: "./data/timetracking.db",
		},
		Auth: AuthConfig{
			TokenExpiryHours: 8,
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "./data/server.log",
			MaxAgeDays: 14,
			MaxBackups: 0,
			MaxSizeMB:  20,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ApplyBootstrapEnv overrides server, database, auth and logging from the environment.
func (c *Config) ApplyBootstrapEnv() {
	if v := os.Getenv("NFC_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
	if v := os.Getenv("NFC_SERVER_HOST"); v != "" {
		c.Server.Host = v
	}
	if v := os.Getenv("NFC_PAIRING_ADVERTISE_HOST"); v != "" {
		c.Server.PairingAdvertiseHost = v
	}
	if v := os.Getenv("NFC_DATABASE_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("NFC_AUTH_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("NFC_AUTH_EXPIRY_HOURS"); v != "" {
		if h, err := strconv.Atoi(v); err == nil {
			c.Auth.TokenExpiryHours = h
		}
	}
	applyOIDCEnv(&c.Auth.OIDC)
	if v := os.Getenv("NFC_LOGGING_FILE"); v != "" {
		c.Logging.File = v
	}
	if v := os.Getenv("NFC_LOGGING_MAX_AGE_DAYS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			c.Logging.MaxAgeDays = d
		}
	}
	if v := os.Getenv("NFC_BACKUP_TARGET_PATH"); v != "" {
		c.BackupTargetPath = v
	}
}

func applyOIDCEnv(o *OIDCConfig) {
	if v := os.Getenv("NFC_OIDC_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			o.Enabled = b
		}
	}
	if v := os.Getenv("NFC_OIDC_ISSUER"); v != "" {
		o.Issuer = v
	}
	if v := os.Getenv("NFC_OIDC_CLIENT_ID"); v != "" {
		o.ClientID = v
	}
	if v := os.Getenv("NFC_OIDC_CLIENT_SECRET"); v != "" {
		o.ClientSecret = v
	}
	if v := os.Getenv("NFC_OIDC_REDIRECT_URL"); v != "" {
		o.RedirectURL = v
	}
	if v := os.Getenv("NFC_OIDC_SCOPES"); v != "" {
		o.Scopes = strings.Fields(strings.ReplaceAll(v, ",", " "))
	}
	if v := os.Getenv("NFC_OIDC_USERNAME_CLAIM"); v != "" {
		o.UsernameClaim = v
	}
	if v := os.Getenv("NFC_OIDC_AUTO_LINK"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			o.AutoLink = &b
		}
	}
	if v := os.Getenv("NFC_OIDC_PASSWORD_LOGIN"); v != "" {
		o.PasswordLogin = v
	}
	if v := os.Getenv("NFC_OIDC_BUTTON_LABEL"); v != "" {
		o.ButtonLabel = v
	}
}

// ApplyEnv applies bootstrap environment overrides (same as ApplyBootstrapEnv).
func (c *Config) ApplyEnv() {
	c.ApplyBootstrapEnv()
}
