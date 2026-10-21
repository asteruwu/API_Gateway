package builder

import (
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

const (
	categoryTCPFilter   = "tcp_filters"
	categoryHTTPFilter  = "http_filters"
	categoryTransformer = "transformer"
)

type pluginHydrator func(raw any) (any, error)

func Hydrate(gw *GatewayConfig) error {
	if err := hydratePluginSet(&gw.TCPFilters, categoryTCPFilter, tcpFilterHydrators); err != nil {
		return err
	}
	if err := hydratePluginSet(&gw.HTTPFilters, categoryHTTPFilter, httpFilterHydrators); err != nil {
		return err
	}
	return hydratePluginSet(&gw.Transformer, categoryTransformer, transformerHydrators)
}

func hydratePluginSet(set *PluginSet, category string, hydrators map[string]pluginHydrator) error {
	if set.Configs == nil {
		return nil
	}
	for _, name := range set.Enabled {
		raw, ok := set.Configs[name]
		if !ok {
			return fmt.Errorf("%w: %s plugin %q enabled but has no config entry", gerrors.ErrFilterConfigNotFound, category, name)
		}
		hydrate, ok := hydrators[name]
		if !ok {
			return fmt.Errorf("%w: unknown plugin %q in %s", gerrors.ErrInvalidConfig, name, category)
		}
		concrete, err := hydrate(raw)
		if err != nil {
			return fmt.Errorf("%w: %s plugin %q: %v", gerrors.ErrInvalidConfig, category, name, err)
		}
		set.Configs[name] = concrete
	}
	return nil
}
