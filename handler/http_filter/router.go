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
	name  string
	rules []routeRule
}

type routeRule struct {
	prefix  string
	service string
	methods map[string]struct{}
}

func NewRouter(cfg builder.RouterConfig) (*Router, error) {
	rules := make([]routeRule, 0)
	seen := make(map[string]struct{})
	for _, rsc := range cfg.Router {
		for _, rc := range rsc.Rules {
			if rc.PathPrefix == "" {
				return nil, fmt.Errorf(
					"%w: empty path prefix",
					gerrors.ErrInitializeHTTPFiltersFailed)
			}

			if _, dup := seen[rc.PathPrefix]; dup {
				return nil, fmt.Errorf(
					"%w: duplicate path prefix %q",
					gerrors.ErrInitializeHTTPFiltersFailed, rc.PathPrefix)
			}
			seen[rc.PathPrefix] = struct{}{}

			var methods map[string]struct{}
			if len(rc.Methods) > 0 {
				methods = make(map[string]struct{}, len(rc.Methods))
				for _, m := range rc.Methods {
					methods[m] = struct{}{}
				}
			}
			rules = append(rules, routeRule{
				prefix:  rc.PathPrefix,
				service: rsc.Service,
				methods: methods})
		}
	}

	slices.SortFunc(rules,
		func(a, b routeRule) int {
			return len(b.prefix) - len(a.prefix)
		})

	return &Router{name: "router", rules: rules}, nil
}

func (r *Router) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	path := req.URL.Path
	for i := range r.rules {
		rule := &r.rules[i]
		if !strings.HasPrefix(path, rule.prefix) {
			continue
		}
		if rule.methods != nil {
			if _, ok := rule.methods[req.Method]; !ok {
				continue
			}
		}
		req.Service = rule.service
		return nil, nil
	}
	return nil, fmt.Errorf("%w: path %q", gerrors.ErrRouteNotFound, path)
}
