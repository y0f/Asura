package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Monitor  MonitorConfig  `yaml:"monitor"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Listen          string        `yaml:"listen"`
	TLSCert         string        `yaml:"tls_cert"`
	TLSKey          string        `yaml:"tls_key"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	MaxBodySize     int64         `yaml:"max_body_size"`
	CORSOrigins     []string      `yaml:"cors_origins"`
	RateLimitPerSec float64       `yaml:"rate_limit_per_sec"`
	RateLimitBurst  int           `yaml:"rate_limit_burst"`
}

type DatabaseConfig struct {
	Path            string        `yaml:"path"`
	MaxReadConns    int           `yaml:"max_read_conns"`
	RetentionDays   int           `yaml:"retention_days"`
	RetentionPeriod time.Duration `yaml:"retention_period"`
}

type AuthConfig struct {
	APIKeys []APIKeyConfig `yaml:"api_keys"`
}

type APIKeyConfig struct {
	Name     string `yaml:"name"`
	Hash     string `yaml:"hash"`
	Role     string `yaml:"role"` // "admin" or "readonly"
	rawKey   string // only set during HashPlaintext
}

type MonitorConfig struct {
	Workers            int           `yaml:"workers"`
	DefaultTimeout     time.Duration `yaml:"default_timeout"`
	DefaultInterval    time.Duration `yaml:"default_interval"`
	FailureThreshold   int           `yaml:"failure_threshold"`
	SuccessThreshold   int           `yaml:"success_threshold"`
	MaxConcurrentDNS   int           `yaml:"max_concurrent_dns"`
	CommandTimeout     time.Duration `yaml:"command_timeout"`
	CommandAllowlist   []string      `yaml:"command_allowlist"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // "text" or "json"
}

func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:          ":8080",
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			MaxBodySize:     1 << 20, // 1MB
			RateLimitPerSec: 10,
			RateLimitBurst:  20,
		},
		Database: DatabaseConfig{
			Path:            "asura.db",
			MaxReadConns:    4,
			RetentionDays:   90,
			RetentionPeriod: 1 * time.Hour,
		},
		Auth: AuthConfig{},
		Monitor: MonitorConfig{
			Workers:          10,
			DefaultTimeout:   10 * time.Second,
			DefaultInterval:  60 * time.Second,
			FailureThreshold: 3,
			SuccessThreshold: 1,
			CommandTimeout:   30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if c.Server.MaxBodySize <= 0 {
		return fmt.Errorf("server.max_body_size must be positive")
	}
	if c.Server.RateLimitPerSec <= 0 {
		return fmt.Errorf("server.rate_limit_per_sec must be positive")
	}
	if c.Server.RateLimitBurst <= 0 {
		return fmt.Errorf("server.rate_limit_burst must be positive")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.Database.MaxReadConns <= 0 {
		return fmt.Errorf("database.max_read_conns must be positive")
	}
	if c.Database.RetentionDays <= 0 {
		return fmt.Errorf("database.retention_days must be positive")
	}
	if c.Monitor.Workers <= 0 {
		return fmt.Errorf("monitor.workers must be positive")
	}
	if c.Monitor.DefaultTimeout <= 0 {
		return fmt.Errorf("monitor.default_timeout must be positive")
	}
	if c.Monitor.DefaultInterval < 5*time.Second {
		return fmt.Errorf("monitor.default_interval must be at least 5s")
	}
	if c.Monitor.FailureThreshold <= 0 {
		return fmt.Errorf("monitor.failure_threshold must be positive")
	}
	if c.Monitor.SuccessThreshold <= 0 {
		return fmt.Errorf("monitor.success_threshold must be positive")
	}

	for i, key := range c.Auth.APIKeys {
		if key.Name == "" {
			return fmt.Errorf("auth.api_keys[%d].name is required", i)
		}
		if key.Hash == "" {
			return fmt.Errorf("auth.api_keys[%d].hash is required", i)
		}
		if key.Role != "admin" && key.Role != "readonly" {
			return fmt.Errorf("auth.api_keys[%d].role must be 'admin' or 'readonly'", i)
		}
	}

	level := strings.ToLower(c.Logging.Level)
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}

	return nil
}

// HashAPIKey computes the SHA-256 hash of an API key for comparison.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// LookupAPIKey checks if the given key matches any configured API key
// and returns the key config if found.
func (c *Config) LookupAPIKey(key string) (*APIKeyConfig, bool) {
	hash := HashAPIKey(key)
	for i := range c.Auth.APIKeys {
		if c.Auth.APIKeys[i].Hash == hash {
			return &c.Auth.APIKeys[i], true
		}
	}
	return nil, false
}
