package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kpihx-labs/k-ai/internal/alias"
	"github.com/kpihx-labs/k-ai/internal/resolver"
)

// WarmUpModelCache fetches upstream model lists for all enabled providers
// and populates the shared model cache for smart routing.
func (g *Gateway) WarmUpModelCache(ctx context.Context, registry *resolver.Registry) {
	for _, p := range registry.List() {
		if !p.Enabled || p.ID == "mock" {
			continue
		}
		models, err := g.fetchUpstreamModels(ctx, p.BaseURL, p.APIKey)
		if err != nil {
			log.Printf("[warmup] %s: failed to fetch models: %v", p.ID, err)
			continue
		}
		g.modelCache.Set(p.ID, models)
		log.Printf("[warmup] %s: cached %d models", p.ID, len(models))
	}
}

func (g *Gateway) collectCatalogModels(ctx context.Context, registry *resolver.Registry, aliasEngine *alias.Engine) []modelObject {
	seen := map[string]struct{}{}
	var data []modelObject
	now := time.Now().Unix()

	add := func(id, owner string) {
		if id == "" || strings.Contains(id, "(?") || strings.HasPrefix(id, "^") {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		data = append(data, modelObject{ID: id, Object: "model", Created: now, OwnedBy: owner})
	}

	aliases, _ := g.store.ListAliases(ctx)
	for _, a := range aliases {
		if !a.Enabled {
			continue
		}
		add(alias.ExampleSlug(alias.Rule{
			ID: a.ID, Name: a.Name, MatchType: a.MatchType, Pattern: a.Pattern,
			Prefix: a.Prefix, Suffix: a.Suffix,
		}), "alias:"+a.ProviderID)
	}

	for _, p := range registry.List() {
		if !p.Enabled {
			continue
		}
		if p.ID == "mock" {
			add("mock-model", "mock")
			continue
		}
		upstream, err := g.fetchUpstreamModels(ctx, p.BaseURL, p.APIKey)
		if err != nil || len(upstream) == 0 {
			add(p.ID+"/default", p.ID)
			continue
		}
		g.modelCache.Set(p.ID, upstream)
		for _, m := range upstream {
			add(m, p.ID)
		}
	}

	_ = aliasEngine
	if len(data) == 0 {
		add("mock-model", "mock")
	}
	return data
}

func (g *Gateway) fetchUpstreamModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("upstream models status %d", resp.StatusCode)
	}
	var payload modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}
