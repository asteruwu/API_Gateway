package builder

import "testing"

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
}
