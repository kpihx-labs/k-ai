package resolver

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/kpihx-labs/k-ai/internal/config"
)

type ModelRule struct {
	MatchType config.MatchType
	Pattern   string
	re        *regexp.Regexp
}

type Provider struct {
	ID       string
	Name     string
	BaseURL  string
	APIKey   string
	Enabled  bool
	Rules    []ModelRule
	Priority int
}

type Resolution struct {
	Provider      *Provider
	UpstreamModel string
}

// ModelCache holds cached upstream model lists, thread-safe.
type ModelCache struct {
	mu   sync.RWMutex
	data map[string]map[string]bool // provider_id -> set of model ids
}

func NewModelCache() *ModelCache {
	return &ModelCache{data: make(map[string]map[string]bool)}
}

func (c *ModelCache) Set(providerID string, models []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	set := make(map[string]bool, len(models))
	for _, m := range models {
		set[m] = true
	}
	c.data[providerID] = set
}

func (c *ModelCache) Has(providerID, model string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[providerID][model]
}

type Registry struct {
	providers []*Provider
	byID      map[string]*Provider
	cache     *ModelCache
}

func NewRegistry(providers []Provider, cache *ModelCache) (*Registry, error) {
	if cache == nil {
		cache = NewModelCache()
	}
	reg := &Registry{
		providers: make([]*Provider, 0, len(providers)),
		byID:      make(map[string]*Provider),
		cache:     cache,
	}
	for i := range providers {
		p := providers[i]
		cp := &Provider{
			ID:       p.ID,
			Name:     p.Name,
			BaseURL:  strings.TrimRight(p.BaseURL, "/"),
			APIKey:   p.APIKey,
			Enabled:  p.Enabled,
			Priority: p.Priority,
		}
		for _, mr := range p.Rules {
			rule := ModelRule{MatchType: mr.MatchType, Pattern: mr.Pattern}
			if mr.MatchType == config.MatchRegex {
				re, err := regexp.Compile(mr.Pattern)
				if err != nil {
					return nil, fmt.Errorf("provider %s: invalid model regex %q: %w", p.ID, mr.Pattern, err)
				}
				rule.re = re
			}
			cp.Rules = append(cp.Rules, rule)
		}
		reg.providers = append(reg.providers, cp)
		reg.byID[cp.ID] = cp
	}
	return reg, nil
}

func (r *Registry) Get(id string) (*Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

func (r *Registry) List() []*Provider {
	out := make([]*Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

func (r *Registry) Cache() *ModelCache { return r.cache }

func (r *Registry) ResolveModel(model string) (*Resolution, error) {
	// Phase 1: check upstream model caches for exact membership.
	// Among matching providers, pick the one with the highest Priority.
	var bestCached *Resolution
	bestCachedPrio := -1
	for _, p := range r.providers {
		if !p.Enabled {
			continue
		}
		if r.cache.Has(p.ID, model) && p.Priority > bestCachedPrio {
			bestCachedPrio = p.Priority
			bestCached = &Resolution{Provider: p, UpstreamModel: model}
		}
	}
	if bestCached != nil {
		return bestCached, nil
	}

	// Phase 2: score-based resolution using config rules
	var best *Resolution
	bestScore := -1

	for _, p := range r.providers {
		if !p.Enabled {
			continue
		}
		if len(p.Rules) == 0 {
			continue
		}
		for _, rule := range p.Rules {
			if !ruleMatches(rule, model) {
				continue
			}
			score := ruleScore(rule) + p.Priority
			if score > bestScore {
				bestScore = score
				best = &Resolution{Provider: p, UpstreamModel: model}
			}
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no provider found for model %q", model)
	}
	return best, nil
}

func ruleScore(rule ModelRule) int {
	switch rule.MatchType {
	case config.MatchExact:
		return 10000 + len(rule.Pattern)
	case config.MatchRegex:
		return 5000 + len(rule.Pattern)
	case config.MatchGlob:
		if rule.Pattern == "*" {
			return 1
		}
		wildcards := strings.Count(rule.Pattern, "*") + strings.Count(rule.Pattern, "?")
		return 100 - wildcards*10 + len(rule.Pattern)
	default:
		return 0
	}
}

func (r *Registry) ResolveWithProvider(providerID, model string) (*Resolution, error) {
	p, ok := r.byID[providerID]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerID)
	}
	if !p.Enabled {
		return nil, fmt.Errorf("provider %q is disabled", providerID)
	}
	if len(p.Rules) == 0 {
		return &Resolution{Provider: p, UpstreamModel: model}, nil
	}
	for _, rule := range p.Rules {
		if ruleMatches(rule, model) {
			return &Resolution{Provider: p, UpstreamModel: model}, nil
		}
	}
	return nil, fmt.Errorf("model %q not allowed for provider %q", model, providerID)
}

func ruleMatches(rule ModelRule, model string) bool {
	switch rule.MatchType {
	case config.MatchExact:
		return model == rule.Pattern
	case config.MatchGlob:
		ok, _ := matchGlob(rule.Pattern, model)
		return ok
	case config.MatchRegex:
		if rule.re == nil {
			return false
		}
		return rule.re.MatchString(model)
	default:
		return false
	}
}

func matchGlob(pattern, value string) (bool, error) {
	return regexp.MatchString(globToRegex(pattern), value)
}

func globToRegex(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return b.String()
}

func FromConfig(in []config.ProviderConfig) []Provider {
	out := make([]Provider, 0, len(in))
	for _, p := range in {
		rules := make([]ModelRule, 0, len(p.Models))
		for _, mr := range p.Models {
			rules = append(rules, ModelRule{MatchType: mr.MatchType, Pattern: mr.Pattern})
		}
		out = append(out, Provider{
			ID:       p.ID,
			Name:     p.Name,
			BaseURL:  p.BaseURL,
			APIKey:   p.APIKey,
			Enabled:  p.Enabled,
			Rules:    rules,
			Priority: p.Priority,
		})
	}
	return out
}
