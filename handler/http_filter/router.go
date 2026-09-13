package httpfilter

import (
	"fmt"
	"slices"
	"sort"
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
	hosts   map[string]struct{}
	prefix  string
	service string
	methods map[string]struct{}
}

func NewRouter(cfg builder.RouterConfig) (*Router, error) {
	rules := make([]routeRule, 0)
	seen := make(map[string]struct{})
	for _, rhc := range cfg.Router {
		var hosts map[string]struct{}
		if len(rhc.Host) > 0 {
			hosts = make(map[string]struct{}, len(rhc.Host))
			for _, h := range rhc.Host {
				hosts[h] = struct{}{}
			}
		}
		for _, rsc := range rhc.Service {
			if rsc.Service == "" {
				return nil, fmt.Errorf(
					"%w: empty service name",
					gerrors.ErrInitializeHTTPFiltersFailed)
			}
			for _, rc := range rsc.Rules {
				if rc.PathPrefix == "" {
					return nil, fmt.Errorf(
						"%w: empty path prefix",
						gerrors.ErrInitializeHTTPFiltersFailed)
				}

				key := routeKey(rc.PathPrefix, rhc.Host)
				if _, dup := seen[key]; dup {
					return nil, fmt.Errorf(
						"%w: duplicate route %q host %v",
						gerrors.ErrInitializeHTTPFiltersFailed, rc.PathPrefix, rhc.Host)
				}
				seen[key] = struct{}{}

				var methods map[string]struct{}
				if len(rc.Methods) > 0 {
					methods = make(map[string]struct{}, len(rc.Methods))
					for _, m := range rc.Methods {
						methods[m] = struct{}{}
					}
				}
				rules = append(rules, routeRule{
					hosts:   hosts,
					prefix:  rc.PathPrefix,
					service: rsc.Service,
					methods: methods})
			}
		}
	}

	slices.SortFunc(rules,
		func(a, b routeRule) int {
			if len(a.prefix) != len(b.prefix) {
				return len(b.prefix) - len(a.prefix)
			}

			aw, bw := 0, 0
			if a.hosts == nil {
				aw = 1
			}
			if b.hosts == nil {
				bw = 1
			}
			return aw - bw
		})

	return &Router{name: "router", rules: rules}, nil
}

// routeKey 查重键：前缀 + 域名集合
func routeKey(prefix string, hosts []string) string {
	list := append([]string(nil), hosts...)
	sort.Strings(list)
	return prefix + "|" + strings.Join(list, ",")
}

func (r *Router) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	path := req.URL.Path
	for i := range r.rules {
		rule := &r.rules[i]
		if rule.hosts != nil {
			if _, ok := rule.hosts[req.Host]; !ok {
				continue
			}
		}
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
	return nil, fmt.Errorf("%w: path %q host %q", gerrors.ErrRouteNotFound, path, req.Host)
}
