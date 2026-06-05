package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type MatchType string

const (
	MatchExact MatchType = "exact"
	MatchGlob  MatchType = "glob"
	MatchRegex MatchType = "regex"
)

type ServerConfig struct {
	Host                string `yaml:"host"`
	Port                int    `yaml:"port"`
	AdminToken          string `yaml:"admin_token"`
	JWTSecret           string `yaml:"jwt_secret"`
	JWTExpiryDays       int    `yaml:"jwt_expiry_days"`
	RegistrationEnabled *bool  `yaml:"registration_enabled"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type ModelRule struct {
	MatchType MatchType `yaml:"match_type" json:"match_type"`
	Pattern   string    `yaml:"pattern" json:"pattern"`
}

type ProviderConfig struct {
	ID       string      `yaml:"id"`
	Name     string      `yaml:"name"`
	BaseURL  string      `yaml:"base_url"`
	APIKey   string      `yaml:"api_key"`
	Enabled  bool        `yaml:"enabled"`
	Models   []ModelRule `yaml:"models"`
	Priority int         `yaml:"priority"`
}

type AliasRuleConfig struct {
	ID         string    `yaml:"id"`
	Name       string    `yaml:"name"`
	MatchType  MatchType `yaml:"match_type"`
	Pattern    string    `yaml:"pattern"`
	Rewrite    string    `yaml:"rewrite"`
	ProviderID string    `yaml:"provider_id"`
	Priority   int       `yaml:"priority"`
	Prefix     string    `yaml:"prefix"`
	Suffix     string    `yaml:"suffix"`
	Enabled    bool      `yaml:"enabled"`
}

type APIKeyConfig struct {
	ID     string   `yaml:"id"`
	Name   string   `yaml:"name"`
	Key    string   `yaml:"key"`
	Scopes []string `yaml:"scopes"`
}

type Config struct {
	Server   ServerConfig      `yaml:"server"`
	Database DatabaseConfig    `yaml:"database"`
	Providers []ProviderConfig `yaml:"providers"`
	Aliases  []AliasRuleConfig `yaml:"aliases"`
	APIKeys  []APIKeyConfig    `yaml:"api_keys"`
}

func DefaultConfigPath() string {
	if p := os.Getenv("K_AI_CONFIG_PATH"); p != "" {
		return p
	}
	return "./config/config.yaml"
}

func DataDir() string {
	if d := os.Getenv("K_AI_DATA_DIR"); d != "" {
		return d
	}
	return "./data"
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyEnvOverrides(&cfg)
	expandConfig(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("K_AI_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("K_AI_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("K_AI_ADMIN_TOKEN"); v != "" {
		cfg.Server.AdminToken = v
	}
	if v := os.Getenv("K_AI_DATA_DIR"); v != "" {
		cfg.Database.Path = filepath.Join(v, "k-ai.db")
	}
	if v := os.Getenv("K_AI_OLLAMA_BASE_URL"); v != "" {
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == "ollama-local" {
				cfg.Providers[i].BaseURL = v
			}
		}
	}
	if v := os.Getenv("K_AI_BASE_URL"); v != "" {
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == "mock" {
				cfg.Providers[i].BaseURL = strings.TrimSuffix(v, "/") + "/mock/v1"
			}
		}
	}
	if v := os.Getenv("K_AI_JWT_SECRET"); v != "" {
		cfg.Server.JWTSecret = v
	}
	if v := os.Getenv("K_AI_REGISTRATION_ENABLED"); v != "" {
		b := v == "true" || v == "1"
		cfg.Server.RegistrationEnabled = &b
	}
	applyProviderEnvKey(cfg, "openrouter", "K_AI_OPENROUTER_API_KEY")
	applyProviderEnvKey(cfg, "opencode", "K_AI_OPENCODE_API_KEY")
	applyProviderEnvKey(cfg, "opencode-go", "K_AI_OPENCODE_GO_API_KEY")
	applyProviderEnvKey(cfg, "venice", "K_AI_VENICE_API_KEY")
	applyProviderEnvKey(cfg, "mistral", "K_AI_MISTRAL_API_KEY")
}

func applyProviderEnvKey(cfg *Config, providerID, envVar string) {
	v := os.Getenv(envVar)
	if v == "" {
		return
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == providerID {
			cfg.Providers[i].APIKey = v
			cfg.Providers[i].Enabled = true
		}
	}
}

func (c *Config) Validate() error {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Database.Path == "" {
		c.Database.Path = filepath.Join(DataDir(), "k-ai.db")
	}
	if c.Server.AdminToken == "" {
		return fmt.Errorf("server.admin_token is required (set K_AI_ADMIN_TOKEN)")
	}
	if c.Server.JWTExpiryDays <= 0 {
		c.Server.JWTExpiryDays = 7
	}
	if c.Server.RegistrationEnabled == nil {
		t := true
		c.Server.RegistrationEnabled = &t
	}
	return nil
}

func (c *Config) IsRegistrationEnabled() bool {
	return c.Server.RegistrationEnabled != nil && *c.Server.RegistrationEnabled
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) Save(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
