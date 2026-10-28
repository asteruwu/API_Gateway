package builder

import (
	"fmt"
	"time"

	gconst "API_Gateway/pkg/constant"
	"API_Gateway/pkg/tools"
)

var httpFilterHydrators = map[string]pluginHydrator{
	"auth":       hydrateAuth,
	"rate_limit": hydrateRateLimit,
}

func hydrateRateLimit(raw any) (any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", raw)
	}
	names, ok := tools.ToStrings(m["limiters"])
	if !ok {
		return nil, fmt.Errorf("limiters: expected string list")
	}
	limiters := make([]gconst.RateLimitDimension, 0, len(names))
	for _, name := range names {
		limiters = append(limiters, gconst.RateLimitDimension(name))
	}
	return RateLimitFilterConfig{Limiters: limiters}, nil
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
