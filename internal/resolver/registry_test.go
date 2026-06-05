package resolver_test

import (
	"testing"

	"github.com/kpihx-labs/k-ai/internal/config"
	"github.com/kpihx-labs/k-ai/internal/resolver"
)

func TestRegistryExactGlobRegex(t *testing.T) {
	reg, err := resolver.NewRegistry([]resolver.Provider{
		{
			ID: "p1", Name: "P1", BaseURL: "http://example/v1", Enabled: true,
			Rules: []resolver.ModelRule{
				{MatchType: config.MatchExact, Pattern: "exact-model"},
				{MatchType: config.MatchGlob, Pattern: "gpt-*"},
				{MatchType: config.MatchRegex, Pattern: `^claude-\d+$`},
			},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	cases := []string{"exact-model", "gpt-4o", "claude-3"}
	for _, model := range cases {
		res, err := reg.ResolveModel(model)
		if err != nil {
			t.Fatalf("model %q: %v", model, err)
		}
		if res.Provider.ID != "p1" || res.UpstreamModel != model {
			t.Fatalf("model %q: %+v", model, res)
		}
	}

	if _, err := reg.ResolveModel("unknown-model"); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestRegistryProviderScoped(t *testing.T) {
	reg, err := resolver.NewRegistry([]resolver.Provider{
		{ID: "mock", BaseURL: "http://mock/v1", Enabled: true, Rules: []resolver.ModelRule{
			{MatchType: config.MatchExact, Pattern: "mock-model"},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.ResolveWithProvider("mock", "mock-model")
	if err != nil || res.Provider.ID != "mock" {
		t.Fatalf("unexpected: %+v err=%v", res, err)
	}
}

func TestRegistryPriorityRouting(t *testing.T) {
	// Ollama with higher priority should win for catch-all glob
	reg, err := resolver.NewRegistry([]resolver.Provider{
		{ID: "mistral", BaseURL: "http://mistral/v1", Enabled: true, Priority: 20, Rules: []resolver.ModelRule{
			{MatchType: config.MatchGlob, Pattern: "*"},
		}},
		{ID: "ollama-local", BaseURL: "http://ollama/v1", Enabled: true, Priority: 100, Rules: []resolver.ModelRule{
			{MatchType: config.MatchGlob, Pattern: "*"},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.ResolveModel("qwen3-ctx2k:latest")
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider.ID != "ollama-local" {
		t.Fatalf("expected ollama-local, got %q", res.Provider.ID)
	}
}

func TestRegistryCacheRoutesToCorrectProvider(t *testing.T) {
	cache := resolver.NewModelCache()
	cache.Set("mistral", []string{"mistral-small-latest", "mistral-large-latest"})

	reg, err := resolver.NewRegistry([]resolver.Provider{
		{ID: "ollama-local", BaseURL: "http://ollama/v1", Enabled: true, Priority: 100, Rules: []resolver.ModelRule{
			{MatchType: config.MatchGlob, Pattern: "*"},
		}},
		{ID: "mistral", BaseURL: "http://mistral/v1", Enabled: true, Priority: 20, Rules: []resolver.ModelRule{
			{MatchType: config.MatchGlob, Pattern: "*"},
		}},
	}, cache)
	if err != nil {
		t.Fatal(err)
	}

	// mistral-small-latest is in Mistral's cache, should route there despite lower priority
	res, err := reg.ResolveModel("mistral-small-latest")
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider.ID != "mistral" {
		t.Fatalf("expected mistral, got %q", res.Provider.ID)
	}

	// unknown model not in any cache: falls back to highest-priority glob (ollama)
	res2, err := reg.ResolveModel("qwen3:latest")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Provider.ID != "ollama-local" {
		t.Fatalf("expected ollama-local, got %q", res2.Provider.ID)
	}
}

func TestRegistryExactBeatsGlobCatchAll(t *testing.T) {
	reg, err := resolver.NewRegistry([]resolver.Provider{
		{ID: "cloud", BaseURL: "http://cloud/v1", Enabled: true, Rules: []resolver.ModelRule{
			{MatchType: config.MatchGlob, Pattern: "*"},
		}},
		{ID: "mock", BaseURL: "http://mock/v1", Enabled: true, Rules: []resolver.ModelRule{
			{MatchType: config.MatchExact, Pattern: "mock-model"},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.ResolveModel("mock-model")
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider.ID != "mock" {
		t.Fatalf("expected mock provider, got %q", res.Provider.ID)
	}
}
