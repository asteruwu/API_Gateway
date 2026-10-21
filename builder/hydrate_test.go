package builder

import (
	"errors"
	"strings"
	"testing"

	gerrors "API_Gateway/pkg/errors"
)

// TestHydrateAuthValid 合法的 auth 插件配置应被水合为 AuthFilterConfig
func TestHydrateAuthValid(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginSet{
			Enabled: []string{"auth"},
			Configs: map[string]any{
				"auth": map[string]any{
					"credentials": []any{"bearer"},
					"jwt": map[string]any{
						"secret_env": "JWT_SECRET",
					},
				},
			},
		},
	}

	if err := Hydrate(&gw, gw.HTTPFilters.Enabled); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cfg, ok := gw.HTTPFilters.Configs["auth"].(AuthFilterConfig)
	if !ok {
		t.Fatalf("config = %T, want AuthFilterConfig", gw.HTTPFilters.Configs["auth"])
	}
	if len(cfg.Credentials) != 1 || cfg.Credentials[0] != "bearer" {
		t.Errorf("credentials = %v, want [bearer]", cfg.Credentials)
	}
	if cfg.JWT.SecretEnv != "JWT_SECRET" {
		t.Errorf("jwt = %+v, want secret_env JWT_SECRET", cfg.JWT)
	}
}

// TestHydrateAuthTypeMismatch auth 插件的可选字段类型不符时必须报错，而不是静默吞掉
func TestHydrateAuthTypeMismatch(t *testing.T) {
	cases := map[string]map[string]any{
		"credentials wrong type": {"credentials": "bearer"},
		"credentials missing":    {},
		"jwt wrong type":         {"credentials": []any{"bearer"}, "jwt": "nope"},
		"leeway wrong type":      {"credentials": []any{"bearer"}, "jwt": map[string]any{"leeway": 5}},
	}
	for name, auth := range cases {
		t.Run(name, func(t *testing.T) {
			gw := GatewayConfig{
				HTTPFilters: PluginSet{
					Enabled: []string{"auth"},
					Configs: map[string]any{"auth": auth},
				},
			}
			err := Hydrate(&gw, gw.HTTPFilters.Enabled)
			if !errors.Is(err, gerrors.ErrInvalidConfig) {
				t.Errorf("err = %v, want wrapping %v", err, gerrors.ErrInvalidConfig)
			}
		})
	}
}

// TestHydrateEnabledWithoutConfig Enabled 声明了插件但 Configs 缺少对应配置时应报错
func TestHydrateEnabledWithoutConfig(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginSet{
			Enabled: []string{"auth"},
			Configs: map[string]any{},
		},
	}
	err := Hydrate(&gw, gw.HTTPFilters.Enabled)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

// TestHydrateTCPLimitValid tcp_limit 正常水合
func TestHydrateTCPLimitValid(t *testing.T) {
	gw := GatewayConfig{
		TCPFilters: PluginSet{
			Enabled: []string{"tcp_limit"},
			Configs: map[string]any{"tcp_limit": map[string]any{"max_conn": 8}},
		},
	}

	if err := Hydrate(&gw, gw.HTTPFilters.Enabled); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cfg, ok := gw.TCPFilters.Configs["tcp_limit"].(TCPLimiterFilterConfig)
	if !ok {
		t.Fatalf("config = %T, want TCPLimiterFilterConfig", gw.TCPFilters.Configs["tcp_limit"])
	}
	if cfg.MaxConn != 8 {
		t.Errorf("max_conn = %d, want 8", cfg.MaxConn)
	}
}

// TestHydrateTestTransformerValid testT 正常水合
func TestHydrateTestTransformerValid(t *testing.T) {
	gw := GatewayConfig{
		Transformer: PluginSet{
			Enabled: []string{"testT"},
			Configs: map[string]any{"testT": map[string]any{}},
		},
	}

	if err := Hydrate(&gw, gw.HTTPFilters.Enabled); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.Transformer.Configs["testT"].(TestTransformerConfig); !ok {
		t.Errorf("config = %T, want TestTransformerConfig", gw.Transformer.Configs["testT"])
	}
}

// TestHydrateCrossCategory 插件写错类别必须在 hydrate 期就报错，并点明所在的类别
func TestHydrateCrossCategory(t *testing.T) {
	cases := []struct {
		name     string
		plugin   string
		category string
		pick     func(*GatewayConfig) *PluginSet
	}{
		{"tcp_limit in http_filters", "tcp_limit", categoryHTTPFilter, func(gw *GatewayConfig) *PluginSet { return &gw.HTTPFilters }},
		{"tcp_limit in transformer", "tcp_limit", categoryTransformer, func(gw *GatewayConfig) *PluginSet { return &gw.Transformer }},
		{"auth in tcp_filters", "auth", categoryTCPFilter, func(gw *GatewayConfig) *PluginSet { return &gw.TCPFilters }},
		{"testT in http_filters", "testT", categoryHTTPFilter, func(gw *GatewayConfig) *PluginSet { return &gw.HTTPFilters }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gw := GatewayConfig{}
			set := c.pick(&gw)
			set.Enabled = []string{c.plugin}
			set.Configs = map[string]any{c.plugin: map[string]any{}}

			err := Hydrate(&gw, gw.HTTPFilters.Enabled)
			if !errors.Is(err, gerrors.ErrInvalidConfig) {
				t.Fatalf("err = %v, want wrapping %v", err, gerrors.ErrInvalidConfig)
			}
			if !strings.Contains(err.Error(), c.category) {
				t.Errorf("err = %v, want mention category %q", err, c.category)
			}
		})
	}
}

// TestHydrateHTTPFollowsNames http filter 只认传入名单：不在 enabled 但被 route 引用的也要水合
func TestHydrateHTTPFollowsNames(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginSet{
			Configs: map[string]any{
				"auth": map[string]any{"credentials": []any{"bearer"}},
			},
		},
	}

	if err := Hydrate(&gw, []string{"auth"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.HTTPFilters.Configs["auth"].(AuthFilterConfig); !ok {
		t.Errorf("config = %T, want AuthFilterConfig", gw.HTTPFilters.Configs["auth"])
	}
}

// TestHydrateHTTPIgnoresUnreferenced 名单外的 Configs 条目不水合
func TestHydrateHTTPIgnoresUnreferenced(t *testing.T) {
	raw := map[string]any{"credentials": "not a list"}
	gw := GatewayConfig{
		HTTPFilters: PluginSet{
			Configs: map[string]any{"auth": raw},
		},
	}

	if err := Hydrate(&gw, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.HTTPFilters.Configs["auth"].(map[string]any); !ok {
		t.Errorf("config = %T, want untouched raw map", gw.HTTPFilters.Configs["auth"])
	}
}

// TestHydrateReferencedWithNilConfigs 引用了 filter 却整段没写 configs，不能静默通过
func TestHydrateReferencedWithNilConfigs(t *testing.T) {
	gw := GatewayConfig{}

	err := Hydrate(&gw, []string{"auth"})
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}
