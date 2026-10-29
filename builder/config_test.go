package builder

import (
	"slices"
	"testing"

	gconst "API_Gateway/pkg/constant"
)

// TestBuildSampleConfigs 校验示例配置能走完整的 Build 链路，并产出运行态 pipeline 与插件配置。
func TestBuildSampleConfigs(t *testing.T) {
	cfg, err := Build("../configs")
	if err != nil {
		t.Fatalf("Build sample configs: %v", err)
	}
	if len(cfg.Backend.Service) == 0 {
		t.Fatal("expected services from sample configs")
	}
	if len(cfg.Handler.Filter.Pipelines) == 0 {
		t.Fatal("expected route pipelines from sample configs")
	}
	if _, ok := cfg.Handler.Filter.Configs["auth"]; !ok {
		t.Fatal("expected auth filter config")
	}

	rc, ok := cfg.Handler.Filter.Configs["rate_limit"].(RateLimitFilterConfig)
	if !ok {
		t.Fatalf("rate_limit config = %T, want RateLimitFilterConfig", cfg.Handler.Filter.Configs["rate_limit"])
	}
	if len(rc.Limiters) != 1 || rc.Limiters[0] != gconst.RateLimitByRoute {
		t.Errorf("limiters = %v, want [route]", rc.Limiters)
	}
	if len(rc.Rules) == 0 {
		t.Fatal("expected rate limit rules from sample configs")
	}
	for routeID, rule := range rc.Rules {
		if rule.Dimension != gconst.RateLimitByRoute {
			t.Errorf("rule[%s] dimension = %q, want route", routeID, rule.Dimension)
		}
	}
	if _, ok := rc.Rules["shopA.order#0"]; !ok {
		t.Error("missing rate limit rule for shopA.order#0")
	}
	if _, ok := rc.Rules["shopA.order#1"]; !ok {
		t.Error("missing rate limit rule for shopA.order#1")
	}
	if _, ok := rc.Rules["shopA.user#0"]; ok {
		t.Error("route without rate_limit declaration should not have a rule")
	}
	if pipeline := cfg.Handler.Filter.Pipelines["shopA.order#0"]; !slices.Contains(pipeline, "rate_limit") {
		t.Errorf("pipeline = %v, want rate_limit", pipeline)
	}
}
