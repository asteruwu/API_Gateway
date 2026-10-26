package builder

import (
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

func assembleSelectedSet(names []string, registry map[string]any) (order []string, kept map[string]any, errs []error) {
	kept = make(map[string]any, len(names))
	for _, name := range names {
		cfg, ok := registry[name]
		if !ok {
			errs = append(errs, fmt.Errorf("%w: %q", gerrors.ErrFilterConfigNotFound, name))
			continue
		}
		kept[name] = cfg
		order = append(order, name)
	}
	return order, kept, errs
}

func assembleSelectedList(names []string, registry map[string]any) ([]any, []error) {
	order, kept, errs := assembleSelectedSet(names, registry)
	list := make([]any, 0, len(order))
	for _, name := range order {
		list = append(list, kept[name])
	}
	return list, errs
}

func assembleSelectedMap(names []string, registry map[string]any) (map[string]any, []error) {
	_, kept, errs := assembleSelectedSet(names, registry)
	return kept, errs
}
