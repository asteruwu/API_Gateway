package builder

import (
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

// pluginHydrators 插件注册表
var pluginHydrators = map[string]func(raw any) (any, error){
	"tcp_limit": hydrateTCPLimit,
	"testT":     hydrateTestTransformer,
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
	maxConn, ok := toInt(m["max_conn"])
	if !ok {
		return nil, fmt.Errorf("max_conn: expected integer")
	}
	return TCPLimiterFilterConfig{MaxConn: maxConn}, nil
}

func hydrateTestTransformer(raw any) (any, error) {
	return TestTransformerConfig{}, nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
