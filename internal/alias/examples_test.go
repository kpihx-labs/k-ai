package alias

import (
	"testing"

	"github.com/kpihx-labs/k-ai/internal/config"
)

func TestExampleSlug(t *testing.T) {
	cases := []struct {
		rule Rule
		want string
	}{
		{Rule{MatchType: config.MatchGlob, Pattern: "or-*", Prefix: "or-"}, "or-example-model"},
		{Rule{MatchType: config.MatchRegex, Pattern: `^local-(?P<model>.+)$`, Prefix: "local-"}, "local-example-model"},
		{Rule{MatchType: config.MatchExact, Pattern: "mock-model"}, "mock-model"},
	}
	for _, tc := range cases {
		got := ExampleSlug(tc.rule)
		if got != tc.want {
			t.Fatalf("ExampleSlug(%+v) = %q, want %q", tc.rule, got, tc.want)
		}
	}
}
