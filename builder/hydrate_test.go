package builder

import (
	"errors"
	"strings"
	"testing"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

// TestHydrateAuthValid 合法的 auth 插件配置应被水合为 AuthFilterConfig
func TestHydrateAuthValid(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"auth"}},
		Plugins: PluginsConfig{HTTP: map[string]any{
			"auth": map[string]any{
				"credentials": []any{"bearer"},
				"jwt": map[string]any{
					"secret_env": "JWT_SECRET",
				},
			},
		}},
	}

	if err := Hydrate(&gw, gw.HTTPFilters.Enabled); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cfg, ok := gw.Plugins.HTTP["auth"].(AuthFilterConfig)
	if !ok {
		t.Fatalf("config = %T, want AuthFilterConfig", gw.Plugins.HTTP["auth"])
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
				HTTPFilters: PluginActivation{Enabled: []string{"auth"}},
				Plugins:     PluginsConfig{HTTP: map[string]any{"auth": auth}},
			}
			err := Hydrate(&gw, gw.HTTPFilters.Enabled)
			if !errors.Is(err, gerrors.ErrInvalidConfig) {
				t.Errorf("err = %v, want wrapping %v", err, gerrors.ErrInvalidConfig)
			}
		})
	}
}

// TestHydrateEnabledWithoutConfig 默认链声明了插件但注册表缺少对应配置时应报错
func TestHydrateEnabledWithoutConfig(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"auth"}},
		Plugins:     PluginsConfig{HTTP: map[string]any{}},
	}
	err := Hydrate(&gw, gw.HTTPFilters.Enabled)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

// TestHydrateRateLimitValid rate_limit 正常水合 limiters 列表
func TestHydrateRateLimitValid(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"rate_limit"}},
		Plugins: PluginsConfig{HTTP: map[string]any{
			"rate_limit": map[string]any{"limiters": []any{"route"}},
		}},
	}

	if err := Hydrate(&gw, gw.HTTPFilters.Enabled); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cfg, ok := gw.Plugins.HTTP["rate_limit"].(RateLimitFilterConfig)
	if !ok {
		t.Fatalf("config = %T, want RateLimitFilterConfig", gw.Plugins.HTTP["rate_limit"])
	}
	if len(cfg.Limiters) != 1 || cfg.Limiters[0] != gconst.RateLimitByRoute {
		t.Errorf("limiters = %v, want [route]", cfg.Limiters)
	}
}

// TestHydrateRateLimitTypeMismatch rate_limit 只负责类型转换，不校验语义
func TestHydrateRateLimitTypeMismatch(t *testing.T) {
	cases := map[string]any{
		"nil":                 nil,
		"wrong type":          "nope",
		"missing limiters":    map[string]any{},
		"limiters wrong type": map[string]any{"limiters": "route"},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			gw := GatewayConfig{
				HTTPFilters: PluginActivation{Enabled: []string{"rate_limit"}},
				Plugins:     PluginsConfig{HTTP: map[string]any{"rate_limit": raw}},
			}
			err := Hydrate(&gw, gw.HTTPFilters.Enabled)
			if !errors.Is(err, gerrors.ErrInvalidConfig) {
				t.Errorf("err = %v, want wrapping %v", err, gerrors.ErrInvalidConfig)
			}
		})
	}
}

// TestHydrateRateLimitKeepsDeclarations 支持性、重复与空集合属于后续校验，hydrate 原样保留
func TestHydrateRateLimitKeepsDeclarations(t *testing.T) {
	cases := map[string][]any{
		"duplicate":   {"route", "route"},
		"unsupported": {"ip"},
		"empty":       {},
	}
	for name, names := range cases {
		t.Run(name, func(t *testing.T) {
			gw := GatewayConfig{
				HTTPFilters: PluginActivation{Enabled: []string{"rate_limit"}},
				Plugins: PluginsConfig{HTTP: map[string]any{
					"rate_limit": map[string]any{"limiters": names},
				}},
			}
			if err := Hydrate(&gw, gw.HTTPFilters.Enabled); err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			cfg, ok := gw.Plugins.HTTP["rate_limit"].(RateLimitFilterConfig)
			if !ok {
				t.Fatalf("config = %T, want RateLimitFilterConfig", gw.Plugins.HTTP["rate_limit"])
			}
			if len(cfg.Limiters) != len(names) {
				t.Fatalf("limiters = %v, want %v", cfg.Limiters, names)
			}
			for i, name := range names {
				want := gconst.RateLimitDimension(name.(string))
				if cfg.Limiters[i] != want {
					t.Errorf("limiters[%d] = %q, want %q", i, cfg.Limiters[i], want)
				}
			}
		})
	}
}

// TestHydrateTCPLimitValid tcp_limit 正常水合
func TestHydrateTCPLimitValid(t *testing.T) {
	gw := GatewayConfig{
		TCPFilters: PluginActivation{Enabled: []string{"tcp_limit"}},
		Plugins:    PluginsConfig{TCP: map[string]any{"tcp_limit": map[string]any{"max_conn": 8}}},
	}

	if err := Hydrate(&gw, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cfg, ok := gw.Plugins.TCP["tcp_limit"].(TCPLimiterFilterConfig)
	if !ok {
		t.Fatalf("config = %T, want TCPLimiterFilterConfig", gw.Plugins.TCP["tcp_limit"])
	}
	if cfg.MaxConn != 8 {
		t.Errorf("max_conn = %d, want 8", cfg.MaxConn)
	}
}

// TestHydrateTestTransformerValid testT 正常水合
func TestHydrateTestTransformerValid(t *testing.T) {
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": map[string]any{}}},
	}

	if err := Hydrate(&gw, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.Plugins.Transformer["testT"].(TestTransformerConfig); !ok {
		t.Errorf("config = %T, want TestTransformerConfig", gw.Plugins.Transformer["testT"])
	}
}

// TestHydrateCrossCategory 插件写错类别必须在 hydrate 期就报错，并点明所在的类别
func TestHydrateCrossCategory(t *testing.T) {
	cases := []struct {
		name     string
		plugin   string
		category string
		setup    func(*GatewayConfig) []string
	}{
		{
			name:     "tcp_limit in http_filters",
			plugin:   "tcp_limit",
			category: categoryHTTPFilter,
			setup: func(gw *GatewayConfig) []string {
				gw.HTTPFilters.Enabled = []string{"tcp_limit"}
				gw.Plugins.HTTP = map[string]any{"tcp_limit": map[string]any{}}
				return gw.HTTPFilters.Enabled
			},
		},
		{
			name:     "tcp_limit in transformer",
			plugin:   "tcp_limit",
			category: categoryTransformer,
			setup: func(gw *GatewayConfig) []string {
				gw.Transformer.Enabled = []string{"tcp_limit"}
				gw.Plugins.Transformer = map[string]any{"tcp_limit": map[string]any{}}
				return nil
			},
		},
		{
			name:     "auth in tcp_filters",
			plugin:   "auth",
			category: categoryTCPFilter,
			setup: func(gw *GatewayConfig) []string {
				gw.TCPFilters.Enabled = []string{"auth"}
				gw.Plugins.TCP = map[string]any{"auth": map[string]any{}}
				return nil
			},
		},
		{
			name:     "testT in http_filters",
			plugin:   "testT",
			category: categoryHTTPFilter,
			setup: func(gw *GatewayConfig) []string {
				gw.HTTPFilters.Enabled = []string{"testT"}
				gw.Plugins.HTTP = map[string]any{"testT": map[string]any{}}
				return gw.HTTPFilters.Enabled
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gw := GatewayConfig{}
			httpNames := c.setup(&gw)

			err := Hydrate(&gw, httpNames)
			if !errors.Is(err, gerrors.ErrInvalidConfig) {
				t.Fatalf("err = %v, want wrapping %v", err, gerrors.ErrInvalidConfig)
			}
			if !strings.Contains(err.Error(), c.category) {
				t.Errorf("err = %v, want mention category %q", err, c.category)
			}
		})
	}
}

// TestHydrateHTTPFollowsNames http filter 只认传入名单：不在默认链但被 route 引用的也要水合
func TestHydrateHTTPFollowsNames(t *testing.T) {
	gw := GatewayConfig{
		Plugins: PluginsConfig{HTTP: map[string]any{
			"auth": map[string]any{"credentials": []any{"bearer"}},
		}},
	}

	if err := Hydrate(&gw, []string{"auth"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.Plugins.HTTP["auth"].(AuthFilterConfig); !ok {
		t.Errorf("config = %T, want AuthFilterConfig", gw.Plugins.HTTP["auth"])
	}
}

// TestHydrateHTTPIgnoresUnreferenced 名单外的注册表条目不水合
func TestHydrateHTTPIgnoresUnreferenced(t *testing.T) {
	raw := map[string]any{"credentials": "not a list"}
	gw := GatewayConfig{
		Plugins: PluginsConfig{HTTP: map[string]any{"auth": raw}},
	}

	if err := Hydrate(&gw, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.Plugins.HTTP["auth"].(map[string]any); !ok {
		t.Errorf("config = %T, want untouched raw map", gw.Plugins.HTTP["auth"])
	}
}

// TestHydrateHTTPRouteOnlyActivated route-only filter 也属于激活集合，必须被水合
func TestHydrateHTTPRouteOnlyActivated(t *testing.T) {
	gw := GatewayConfig{
		Plugins: PluginsConfig{HTTP: map[string]any{
			"auth": map[string]any{"credentials": []any{"bearer"}},
		}},
	}
	platforms := []PlatformConfig{{
		Name: "p",
		Service: []ServiceConfig{{
			Name:   "s",
			Routes: []RouteRuleConfig{{PathPrefix: "/a", Filters: []string{"auth"}}},
		}},
	}}

	plan := Calculate(gw, platforms)
	if err := Hydrate(&gw, plan.HTTPFilters); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := gw.Plugins.HTTP["auth"].(AuthFilterConfig); !ok {
		t.Errorf("config = %T, want AuthFilterConfig", gw.Plugins.HTTP["auth"])
	}
}

// TestHydrateOnlyActivatedEntriesValidated 未激活条目允许非法/半填，激活后才校验
func TestHydrateOnlyActivatedEntriesValidated(t *testing.T) {
	gw := GatewayConfig{
		Plugins: PluginsConfig{
			TCP:         map[string]any{"tcp_limit": map[string]any{"max_conn": "bad"}},
			HTTP:        map[string]any{"auth": map[string]any{"credentials": "not a list"}},
			Transformer: map[string]any{"testT": "not-a-map"},
		},
	}

	if err := Hydrate(&gw, nil); err != nil {
		t.Fatalf("inactive entries must not be validated: %v", err)
	}
	if _, ok := gw.Plugins.TCP["tcp_limit"].(map[string]any); !ok {
		t.Errorf("tcp config = %T, want untouched raw map", gw.Plugins.TCP["tcp_limit"])
	}
	if _, ok := gw.Plugins.HTTP["auth"].(map[string]any); !ok {
		t.Errorf("http config = %T, want untouched raw map", gw.Plugins.HTTP["auth"])
	}
	if _, ok := gw.Plugins.Transformer["testT"].(string); !ok {
		t.Errorf("transformer config = %T, want untouched raw value", gw.Plugins.Transformer["testT"])
	}

	gw.HTTPFilters.Enabled = []string{"auth"}
	err := Hydrate(&gw, gw.HTTPFilters.Enabled)
	if !errors.Is(err, gerrors.ErrInvalidConfig) {
		t.Errorf("err = %v, want wrapping %v", err, gerrors.ErrInvalidConfig)
	}
}

// TestHydrateReferencedWithNilConfigs 引用了 filter 却整段没写注册表，不能静默通过
func TestHydrateReferencedWithNilConfigs(t *testing.T) {
	gw := GatewayConfig{}

	err := Hydrate(&gw, []string{"auth"})
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}
