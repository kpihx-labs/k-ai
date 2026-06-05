package config

import (
	"os"
	"regexp"
	"strings"
)

var envPattern = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// ExpandEnv replaces {env:VAR} placeholders with os.Getenv values.
func ExpandEnv(value string) string {
	return envPattern.ReplaceAllStringFunc(value, func(match string) string {
		sub := envPattern.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		return os.Getenv(sub[1])
	})
}

func expandConfig(cfg *Config) {
	cfg.Server.AdminToken = ExpandEnv(cfg.Server.AdminToken)
	for i := range cfg.Providers {
		cfg.Providers[i].BaseURL = ExpandEnv(cfg.Providers[i].BaseURL)
		cfg.Providers[i].APIKey = ExpandEnv(cfg.Providers[i].APIKey)
	}
}

func providerEnabled(p ProviderConfig) bool {
	if !p.Enabled {
		return false
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		return false
	}
	// Local Ollama may run without API key.
	if p.ID == "ollama-local" || p.ID == "mock" {
		return true
	}
	return strings.TrimSpace(p.APIKey) != ""
}
