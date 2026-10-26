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

func Hydrate(gw *GatewayConfig, httpActivated []string) error {
	if err := hydrateRegistry(gw.Plugins.TCP, gw.TCPFilters.Enabled, categoryTCPFilter, tcpFilterHydrators); err != nil {
		return err
	}
	if err := hydrateRegistry(gw.Plugins.HTTP, httpActivated, categoryHTTPFilter, httpFilterHydrators); err != nil {
		return err
	}
	return hydrateRegistry(gw.Plugins.Transformer, gw.Transformer.Enabled, categoryTransformer, transformerHydrators)
}

func hydrateRegistry(registry map[string]any, names []string, category string, hydrators map[string]pluginHydrator) error {
	for _, name := range names {
		raw, ok := registry[name]
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
		registry[name] = concrete
	}
	return nil
}
