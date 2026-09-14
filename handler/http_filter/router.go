package httpfilter

import (
	"fmt"
	"slices"
	"strings"

	"API_Gateway/builder"
	"API_Gateway/handler/message"
	gerrors "API_Gateway/pkg/errors"
)

type Router struct {
	rules []builder.RouterRule
}

func NewRouter(cfg builder.RouterConfig) *Router {
	rules := make([]builder.RouterRule, len(cfg.Rules))
	copy(rules, cfg.Rules)

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

	return &Router{rules: rules}
}

func (r *Router) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	path := req.URL.Path
	for i := range r.rules {
		rule := &r.rules[i]
		if !matchAny(rule.Hosts, req.Host) {
			continue
		}
		if !strings.HasPrefix(path, rule.PathPrefix) {
			continue
		}
		if !matchAny(rule.Methods, req.Method) {
			continue
		}
		req.Service = rule.Service
		return nil, nil
	}
	return nil, fmt.Errorf("%w: path %q host %q", gerrors.ErrRouteNotFound, path, req.Host)
}

func matchAny(list []string, v string) bool {
	if len(list) == 0 {
		return true
	}
	return slices.Contains(list, v)
}
