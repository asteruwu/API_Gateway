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
			gw := GatewayConfig{HTTPFilters: PluginActivation{Enabled: c.enabled}}
			plan := Calculate(gw, c.platforms)
			if !slices.Equal(plan.HTTPFilters, c.want) {
				t.Errorf("HTTPFilters = %v, want %v", plan.HTTPFilters, c.want)
			}
		})
	}
}

// TestCalculateFlattensPlan 校验 Calculate 一次遍历同时产出路由、服务、pipeline 与策略
func TestCalculateFlattensPlan(t *testing.T) {
	gw := GatewayConfig{HTTPFilters: PluginActivation{Enabled: []string{"auth"}}}
	platforms := []PlatformConfig{{
		Name: "shopA",
		Host: []string{"api.shopA.com"},
		Service: []ServiceConfig{
			{
				Name:        "order",
				Transformer: "testT",
				Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
				Routes: []RouteRuleConfig{
					{PathPrefix: "/orders"},
					{
						PathPrefix: "/refund",
						Filters:    []string{"rate_limit", "auth"},
						Auth: &RouteAuthConfig{
							Credential: "bearer",
							Require:    RouteAuthRequire{Roles: []string{"support"}},
						},
					},
				},
			},
			{
				Name:        "user",
				Transformer: "testT",
				Instances:   []InstanceConfig{{Addr: "127.0.0.1:8082"}},
				Routes:      []RouteRuleConfig{{PathPrefix: "/users", Filters: []string{}}},
			},
		},
	}}

	plan := Calculate(gw, platforms)

	if want := []string{"auth", "rate_limit"}; !slices.Equal(plan.HTTPFilters, want) {
		t.Errorf("HTTPFilters = %v, want %v", plan.HTTPFilters, want)
	}
	if len(plan.Routes) != 3 {
		t.Fatalf("routes = %+v, want 3 entries", plan.Routes)
	}
	if len(plan.Services) != 2 {
		t.Fatalf("services = %+v, want 2 entries", plan.Services)
	}

	wantRoutes := []struct {
		id       string
		service  string
		pipeline []string
	}{
		{"shopA.order#0", "shopA.order", []string{"auth"}},
		{"shopA.order#1", "shopA.order", []string{"rate_limit", "auth"}},
		{"shopA.user#0", "shopA.user", []string{}},
	}
	for i, want := range wantRoutes {
		got := plan.Routes[i]
		if got.Rule.ID != want.id {
			t.Errorf("routes[%d].ID = %q, want %q", i, got.Rule.ID, want.id)
		}
		if got.Rule.Service != want.service {
			t.Errorf("routes[%d].Service = %q, want %q", i, got.Rule.Service, want.service)
		}
		if !slices.Equal(got.Pipeline, want.pipeline) {
			t.Errorf("routes[%d].Pipeline = %v, want %v", i, got.Pipeline, want.pipeline)
		}
	}
	if plan.Routes[0].Rule.Hosts[0] != "api.shopA.com" {
		t.Errorf("hosts = %v, want [api.shopA.com]", plan.Routes[0].Rule.Hosts)
	}

	policy := plan.Routes[1].Policy
	if policy.Credential != "bearer" || !slices.Equal(policy.Roles, []string{"support"}) {
		t.Errorf("policy = %+v, want credential bearer + roles [support]", policy)
	}
	if p := plan.Routes[0].Policy; p.Public || p.Credential != "" || len(p.Roles) != 0 || len(p.Scopes) != 0 {
		t.Errorf("route without auth policy = %+v, want zero value", p)
	}

	wantServices := []struct {
		name        string
		transformer string
		addr        string
	}{
		{"shopA.order", "testT", "127.0.0.1:8081"},
		{"shopA.user", "testT", "127.0.0.1:8082"},
	}
	for i, want := range wantServices {
		got := plan.Services[i]
		if got.Name != want.name || got.Transformer != want.transformer {
			t.Errorf("services[%d] = %+v, want name %q transformer %q", i, got, want.name, want.transformer)
		}
		if len(got.Instances) != 1 || got.Instances[0].Addr != want.addr {
			t.Errorf("services[%d].Instances = %+v, want addr %q", i, got.Instances, want.addr)
		}
	}
}

// TestCalculatePipelineCopiedPerRoute 继承默认链的 route 必须各自持有独立副本
func TestCalculatePipelineCopiedPerRoute(t *testing.T) {
	gw := GatewayConfig{HTTPFilters: PluginActivation{Enabled: []string{"f1", "f2"}}}
	platforms := []PlatformConfig{{
		Name: "shopA",
		Service: []ServiceConfig{{
			Name:   "order",
			Routes: []RouteRuleConfig{{PathPrefix: "/a"}, {PathPrefix: "/b"}},
		}},
	}}

	plan := Calculate(gw, platforms)
	plan.Routes[0].Pipeline[0] = "mutated"
	if plan.Routes[1].Pipeline[0] != "f1" {
		t.Errorf("pipeline[1] = %v, want untouched [f1 f2]", plan.Routes[1].Pipeline)
	}
}
