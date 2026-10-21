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

func Hydrate(gw *GatewayConfig, httpFilters []string) error {
	if err := hydratePluginSet(&gw.TCPFilters, gw.TCPFilters.Enabled, categoryTCPFilter, tcpFilterHydrators); err != nil {
		return err
	}
	if err := hydratePluginSet(&gw.HTTPFilters, httpFilters, categoryHTTPFilter, httpFilterHydrators); err != nil {
		return err
	}
	return hydratePluginSet(&gw.Transformer, gw.Transformer.Enabled, categoryTransformer, transformerHydrators)
}

func hydratePluginSet(set *PluginSet, names []string, category string, hydrators map[string]pluginHydrator) error {
	for _, name := range names {
		raw, ok := set.Configs[name]
		if !ok {
			return fmt.Errorf("%w: %s plugin %q referenced but has no config entry", gerrors.ErrFilterConfigNotFound, category, name)
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
