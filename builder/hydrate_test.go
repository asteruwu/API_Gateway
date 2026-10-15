package builder

import (
	"errors"
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

	if err := Hydrate(&gw); err != nil {
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
			err := Hydrate(&gw)
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
	err := Hydrate(&gw)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}
