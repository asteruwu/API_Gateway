package builder

import (
	"fmt"
	"time"

	gerrors "API_Gateway/pkg/errors"
	"API_Gateway/pkg/tools"
)

// pluginHydrators 插件注册表
var pluginHydrators = map[string]func(raw any) (any, error){
	"tcp_limit": hydrateTCPLimit,
	"testT":     hydrateTestTransformer,
	"auth":      hydrateAuth,
}

func Hydrate(gw *GatewayConfig) error {
	if err := hydratePluginSet(&gw.TCPFilters); err != nil {
		return fmt.Errorf("hydrate tcp_filters: %w", err)
	}
	if err := hydratePluginSet(&gw.HTTPFilters); err != nil {
		return fmt.Errorf("hydrate http_filters: %w", err)
	}
	if err := hydratePluginSet(&gw.Transformer); err != nil {
		return fmt.Errorf("hydrate transformer: %w", err)
	}
	return nil
}

func hydratePluginSet(set *PluginSet) error {
	if set.Configs == nil {
		return nil
	}
	for _, name := range set.Enabled {
		raw, ok := set.Configs[name]
		if !ok {
			return fmt.Errorf("%w: plugin %q enabled but has no config entry", gerrors.ErrFilterConfigNotFound, name)
		}
		h, ok := pluginHydrators[name]
		if !ok {
			return fmt.Errorf("%w: unknown plugin %q", gerrors.ErrInvalidConfig, name)
		}
		concrete, err := h(raw)
		if err != nil {
			return fmt.Errorf("%w: plugin %q: %v", gerrors.ErrInvalidConfig, name, err)
		}
		set.Configs[name] = concrete
	}
	return nil
}

func hydrateTCPLimit(raw any) (any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", raw)
	}
	maxConn, ok := tools.ToInt(m["max_conn"])
	if !ok {
		return nil, fmt.Errorf("max_conn: expected integer")
	}
	return TCPLimiterFilterConfig{MaxConn: maxConn}, nil
}

func hydrateTestTransformer(raw any) (any, error) {
	return TestTransformerConfig{}, nil
}

func hydrateAuth(raw any) (any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", raw)
	}
	creds, ok := tools.ToStrings(m["credentials"])
	if !ok {
		return nil, fmt.Errorf("credentials: expected string list")
	}
	jwt, err := hydrateJWTVerifier(m["jwt"])
	if err != nil {
		return nil, fmt.Errorf("jwt: %w", err)
	}
	return AuthFilterConfig{Credentials: creds, JWT: jwt}, nil
}

func hydrateJWTVerifier(raw any) (JWTVerifierConfig, error) {
	var cfg JWTVerifierConfig
	if raw == nil {
		return cfg, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return cfg, fmt.Errorf("expected map, got %T", raw)
	}
	var err error
	if cfg.SecretEnv, err = tools.ToStringField(m, "secret_env"); err != nil {
		return cfg, err
	}
	if cfg.Issuer, err = tools.ToStringField(m, "issuer"); err != nil {
		return cfg, err
	}
	if cfg.Audience, err = tools.ToStringField(m, "audience"); err != nil {
		return cfg, err
	}
	if v, ok := m["leeway"]; ok {
		s, ok := v.(string)
		if !ok {
			return cfg, fmt.Errorf("leeway: expected duration string, got %T", v)
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return cfg, fmt.Errorf("leeway: %w", err)
		}
		cfg.Leeway = d
	}
	return cfg, nil
}
