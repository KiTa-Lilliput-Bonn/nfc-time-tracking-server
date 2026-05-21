package config

import (
	"os"
	"strconv"

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
	JWTSecret        string `yaml:"jwt_secret"`
	TokenExpiryHours int    `yaml:"token_expiry_hours"`
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

// ApplyEnv applies bootstrap environment overrides (same as ApplyBootstrapEnv).
func (c *Config) ApplyEnv() {
	c.ApplyBootstrapEnv()
}
