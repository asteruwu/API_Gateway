package httpfilter

import (
	"errors"
	"fmt"
	"net"
	"path"
	"slices"
	"strings"

	"API_Gateway/builder"
	"API_Gateway/handler/message"
	gerrors "API_Gateway/pkg/errors"
	"API_Gateway/pkg/tools"
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
	path := cleanPath(req.URL.Path)
	req.URL.Path = path
	host := normalizeHost(req.Host)
	for i := range r.rules {
		rule := &r.rules[i]
		if !hostMatches(rule.Hosts, host) {
			continue
		}
		if !pathMatchesPrefix(path, rule.PathPrefix) {
			continue
		}
		if !tools.StringsContain(rule.Methods, req.Method) {
			continue
		}
		req.Service = rule.Service
		req.RouteID = rule.ID
		return nil, nil
	}
	return &message.Response{
		Body: fmt.Appendf([]byte{}, "no route for %s %s (host %q)", req.Method, path, req.Host),
	}, fmt.Errorf("%w: path %q host %q", gerrors.ErrRouteNotFound, path, req.Host)
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	return path.Clean(p)
}

func normalizeHost(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

func hostMatches(hosts []string, host string) bool {
	if len(hosts) == 0 {
		return true
	}
	for _, h := range hosts {
		if strings.EqualFold(normalizeHost(h), host) {
			return true
		}
	}
	return false
}

func pathMatchesPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	if strings.HasSuffix(prefix, "/") {
		return true
	}
	return path[len(prefix)] == '/'
}
