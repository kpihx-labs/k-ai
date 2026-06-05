package alias

import (
	"strings"

	"github.com/kpihx-labs/k-ai/internal/config"
)

// ExampleSlug returns a human-friendly routable model slug for catalog display.
func ExampleSlug(r Rule) string {
	switch r.MatchType {
	case config.MatchExact:
		return r.Pattern
	case config.MatchGlob:
		if strings.Contains(r.Pattern, "*") {
			return strings.Replace(r.Pattern, "*", "example-model", 1)
		}
		return r.Pattern
	case config.MatchRegex:
		if r.Prefix != "" {
			return r.Prefix + "example-model"
		}
		// Common local-prefix style: ^local-(?P<model>.+)$
		if strings.Contains(r.Pattern, "local") {
			return "local-example-model"
		}
		if r.Name != "" {
			return strings.ToLower(strings.ReplaceAll(r.Name, " ", "-"))
		}
		return r.ID
	default:
		return r.ID
	}
}
