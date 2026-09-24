package httpfilter

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"API_Gateway/builder"
	"API_Gateway/handler/message"
	"API_Gateway/pkg/tools"
	gerrors "API_Gateway/pkg/errors"
)

type Router struct {
	rules []builder.RouterRule
}

func NewRouter(cfg builder.RouterConfig) (*Router, error) {
	rules := make([]builder.RouterRule, len(cfg.Rules))
	copy(rules, cfg.Rules)

	if errs := validateRules(rules); len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", gerrors.ErrInvalidRouterRule, errors.Join(errs...))
	}

	slices.SortFunc(rules,
		func(a, b builder.RouterRule) int {
			if len(a.PathPrefix) != len(b.PathPrefix) {
				return len(b.PathPrefix) - len(a.PathPrefix)
			}
			aw, bw := 0, 0
			if len(a.Hosts) == 0 {
				aw = 1
			}
			if len(b.Hosts) == 0 {
				bw = 1
			}
			return aw - bw
		})

	return &Router{rules: rules}, nil
}

func validateRules(rules []builder.RouterRule) []error {
	var errs []error
	for i, rule := range rules {
		if rule.Service == "" {
			errs = append(errs, fmt.Errorf("rules[%d]: service is empty", i))
		}
		if !strings.HasPrefix(rule.PathPrefix, "/") {
			errs = append(errs, fmt.Errorf("rules[%d]: path prefix %q must start with \"/\"", i, rule.PathPrefix))
		}
	}
	return errs
}

func (r *Router) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	path := req.URL.Path
	for i := range r.rules {
		rule := &r.rules[i]
		if !tools.StringsContain(rule.Hosts, req.Host) {
			continue
		}
		if !strings.HasPrefix(path, rule.PathPrefix) {
			continue
		}
		if !tools.StringsContain(rule.Methods, req.Method) {
			continue
		}
		req.Service = rule.Service
		return nil, nil
	}
	return nil, fmt.Errorf("%w: path %q host %q", gerrors.ErrRouteNotFound, path, req.Host)
}

