package alias

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kpihx-labs/k-ai/internal/config"
)

type Rule struct {
	ID         string
	Name       string
	MatchType  config.MatchType
	Pattern    string
	Rewrite    string
	ProviderID string
	Priority   int
	Prefix     string
	Suffix     string
	Enabled    bool
	glob       string
	re         *regexp.Regexp
}

type MatchResult struct {
	Rule           *Rule
	UpstreamModel  string
	ProviderID     string
	RequestedModel string
}

type Engine struct {
	rules []*Rule
}

func NewEngine(rules []Rule) (*Engine, error) {
	compiled := make([]*Rule, 0, len(rules))
	for i := range rules {
		r := rules[i]
		if !r.Enabled {
			continue
		}
		cr := &Rule{
			ID:         r.ID,
			Name:       r.Name,
			MatchType:  r.MatchType,
			Pattern:    r.Pattern,
			Rewrite:    r.Rewrite,
			ProviderID: r.ProviderID,
			Priority:   r.Priority,
			Prefix:     r.Prefix,
			Suffix:     r.Suffix,
			Enabled:    r.Enabled,
		}
		switch r.MatchType {
		case config.MatchExact:
			// no compile step
		case config.MatchGlob:
			cr.glob = r.Pattern
		case config.MatchRegex:
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return nil, fmt.Errorf("alias %s: invalid regex: %w", r.ID, err)
			}
			cr.re = re
		default:
			return nil, fmt.Errorf("alias %s: unknown match_type %q", r.ID, r.MatchType)
		}
		compiled = append(compiled, cr)
	}
	sort.Slice(compiled, func(i, j int) bool {
		return compiled[i].Priority > compiled[j].Priority
	})
	return &Engine{rules: compiled}, nil
}

func (e *Engine) Resolve(requestedModel string) (*MatchResult, bool) {
	for _, rule := range e.rules {
		if res, ok := rule.match(requestedModel); ok {
			return res, true
		}
	}
	return nil, false
}

func (r *Rule) match(requestedModel string) (*MatchResult, bool) {
	var matched bool
	var groups map[string]string
	var capture []string

	switch r.MatchType {
	case config.MatchExact:
		matched = requestedModel == r.Pattern
	case config.MatchGlob:
		ok, err := matchGlob(r.glob, requestedModel)
		if err != nil {
			return nil, false
		}
		matched = ok
	case config.MatchRegex:
		if r.re == nil {
			return nil, false
		}
		if !r.re.MatchString(requestedModel) {
			return nil, false
		}
		matched = true
		groups = namedGroups(r.re, requestedModel)
		capture = r.re.FindStringSubmatch(requestedModel)
	}

	if !matched {
		return nil, false
	}

	upstream := applyRewrite(r, requestedModel, groups, capture)
	return &MatchResult{
		Rule:           r,
		UpstreamModel:  upstream,
		ProviderID:     r.ProviderID,
		RequestedModel: requestedModel,
	}, true
}

func applyRewrite(r *Rule, requestedModel string, groups map[string]string, capture []string) string {
	tmpl := r.Rewrite
	if tmpl == "" {
		tmpl = "${model}"
	}

	stripped := requestedModel
	if r.Prefix != "" {
		stripped = strings.TrimPrefix(stripped, r.Prefix)
	}
	if r.Suffix != "" {
		stripped = strings.TrimSuffix(stripped, r.Suffix)
	}

	out := tmpl
	for name, val := range groups {
		out = strings.ReplaceAll(out, fmt.Sprintf("${%s}", name), val)
	}
	for i, val := range capture {
		if i == 0 {
			continue
		}
		out = strings.ReplaceAll(out, fmt.Sprintf("${%d}", i), val)
	}
	out = strings.ReplaceAll(out, "${model}", stripped)
	out = strings.ReplaceAll(out, "${provider}", r.ProviderID)

	if out == "" {
		out = stripped
	}
	return out
}

func matchGlob(pattern, value string) (bool, error) {
	// filepath.Match requires forward slashes; model names use dashes.
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

func namedGroups(re *regexp.Regexp, s string) map[string]string {
	out := map[string]string{}
	match := re.FindStringSubmatch(s)
	if match == nil {
		return out
	}
	for i, name := range re.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		out[name] = match[i]
	}
	return out
}

func FromConfigRules(in []config.AliasRuleConfig) []Rule {
	out := make([]Rule, 0, len(in))
	for _, r := range in {
		out = append(out, Rule{
			ID:         r.ID,
			Name:       r.Name,
			MatchType:  r.MatchType,
			Pattern:    r.Pattern,
			Rewrite:    r.Rewrite,
			ProviderID: r.ProviderID,
			Priority:   r.Priority,
			Prefix:     r.Prefix,
			Suffix:     r.Suffix,
			Enabled:    r.Enabled,
		})
	}
	return out
}
