package builder

import (
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

func assembleEnabledSet(set PluginSet) (order []string, kept map[string]any, errs []error) {
	kept = make(map[string]any, len(set.Enabled))
	for _, name := range set.Enabled {
		cfg, ok := set.Configs[name]
		if !ok {
			errs = append(errs, fmt.Errorf("%w: %q", gerrors.ErrFilterConfigNotFound, name))
			continue
		}
		kept[name] = cfg
		order = append(order, name)
	}
	return order, kept, errs
}

func assembleEnabledList(set PluginSet) ([]any, []error) {
	order, kept, errs := assembleEnabledSet(set)
	list := make([]any, 0, len(order))
	for _, name := range order {
		list = append(list, kept[name])
	}
	return list, errs
}

func assembleEnabledMap(set PluginSet) (map[string]any, []error) {
	_, kept, errs := assembleEnabledSet(set)
	return kept, errs
}
