package builder

import (
	"slices"
	"testing"
)

func calcPlatforms(routes ...[]string) []PlatformConfig {
	rs := make([]RouteRuleConfig, 0, len(routes))
	for _, f := range routes {
		rs = append(rs, RouteRuleConfig{PathPrefix: "/", Filters: f})
	}
	return []PlatformConfig{{Name: "p", Service: []ServiceConfig{{Name: "s", Routes: rs}}}}
}

func TestCalculate(t *testing.T) {
	cases := []struct {
		name      string
		enabled   []string
		platforms []PlatformConfig
		want      []string
	}{
		{"enabled only", []string{"auth", "rate_limit"}, calcPlatforms(nil), []string{"auth", "rate_limit"}},
		{"route only", nil, calcPlatforms([]string{"rate_limit", "auth"}), []string{"rate_limit", "auth"}},
		{"enabled first then routes", []string{"auth"}, calcPlatforms([]string{"rate_limit", "auth"}), []string{"auth", "rate_limit"}},
		{"dedup across routes", nil, calcPlatforms([]string{"auth"}, []string{"auth", "rate_limit"}, []string{"rate_limit"}), []string{"auth", "rate_limit"}},
		{"explicit empty adds nothing", []string{"auth"}, calcPlatforms([]string{}), []string{"auth"}},
		{"nothing", nil, nil, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gw := GatewayConfig{HTTPFilters: PluginSet{Enabled: c.enabled}}
			got := Calculate(gw, c.platforms)
			if !slices.Equal(got, c.want) {
				t.Errorf("Calculate = %v, want %v", got, c.want)
			}
		})
	}
}
