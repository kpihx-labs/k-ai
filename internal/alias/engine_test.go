package alias_test

import (
	"testing"

	"github.com/kpihx-labs/k-ai/internal/alias"
	"github.com/kpihx-labs/k-ai/internal/config"
)

func TestAliasEngineExact(t *testing.T) {
	engine, err := alias.NewEngine([]alias.Rule{
		{ID: "e1", MatchType: config.MatchExact, Pattern: "fast-model", Rewrite: "gpt-fast", ProviderID: "p1", Priority: 1, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := engine.Resolve("fast-model")
	if !ok {
		t.Fatal("expected match")
	}
	if res.UpstreamModel != "gpt-fast" || res.ProviderID != "p1" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestAliasEngineRegexNamedGroup(t *testing.T) {
	engine, err := alias.NewEngine([]alias.Rule{
		{
			ID: "local", MatchType: config.MatchRegex, Pattern: `^local-(?P<upstream>.+)$`,
			Rewrite: "${upstream}", ProviderID: "ollama-local", Priority: 10, Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := engine.Resolve("local-qwen3-ctx2k")
	if !ok {
		t.Fatal("expected regex match")
	}
	if res.UpstreamModel != "qwen3-ctx2k" {
		t.Fatalf("got upstream %q", res.UpstreamModel)
	}
}

func TestAliasEngineGlobWithPrefix(t *testing.T) {
	engine, err := alias.NewEngine([]alias.Rule{
		{
			ID: "g1", MatchType: config.MatchGlob, Pattern: "ollama-*",
			Rewrite: "${model}", ProviderID: "ollama-local", Prefix: "ollama-", Priority: 5, Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := engine.Resolve("ollama-qwen3-ctx2k")
	if !ok {
		t.Fatal("expected glob match")
	}
	if res.UpstreamModel != "qwen3-ctx2k" {
		t.Fatalf("got upstream %q", res.UpstreamModel)
	}
}

func TestAliasEnginePriority(t *testing.T) {
	engine, err := alias.NewEngine([]alias.Rule{
		{ID: "low", MatchType: config.MatchGlob, Pattern: "*", Rewrite: "low", ProviderID: "a", Priority: 1, Enabled: true},
		{ID: "high", MatchType: config.MatchGlob, Pattern: "*", Rewrite: "high", ProviderID: "b", Priority: 99, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := engine.Resolve("anything")
	if !ok || res.UpstreamModel != "high" {
		t.Fatalf("priority failed: %+v ok=%v", res, ok)
	}
}

func TestAliasEngineCaptureGroupNumber(t *testing.T) {
	engine, err := alias.NewEngine([]alias.Rule{
		{
			ID: "cap", MatchType: config.MatchRegex, Pattern: `^vendor/(.+)$`,
			Rewrite: "${1}", ProviderID: "openrouter", Priority: 1, Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := engine.Resolve("vendor/meta-llama")
	if !ok || res.UpstreamModel != "meta-llama" {
		t.Fatalf("unexpected: %+v ok=%v", res, ok)
	}
}
