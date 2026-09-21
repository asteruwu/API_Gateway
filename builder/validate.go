package builder

import (
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

// validatePlatformNames 校验 platform 名非空且全局唯一
func validatePlatformNames(platforms []PlatformConfig) []error {
	var errs []error
	seen := make(map[string]bool, len(platforms))
	for i, p := range platforms {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("%w: platforms[%d]", gerrors.ErrEmptyPlatformName, i))
			continue
		}
		if seen[p.Name] {
			errs = append(errs, fmt.Errorf("%w: %q", gerrors.ErrDuplicatePlatformName, p.Name))
			continue
		}
		seen[p.Name] = true
	}
	return errs
}

// validateServiceNames 校验单个 platform 内 service 名非空且唯一
func validateServiceNames(p PlatformConfig) []error {
	var errs []error
	seen := make(map[string]bool, len(p.Service))
	for i, s := range p.Service {
		if s.Name == "" {
			errs = append(errs, fmt.Errorf("%w: platform %q service[%d]", gerrors.ErrEmptyServiceName, p.Name, i))
			continue
		}
		if seen[s.Name] {
			errs = append(errs, fmt.Errorf("%w: platform %q service %q", gerrors.ErrDuplicateServiceName, p.Name, s.Name))
			continue
		}
		seen[s.Name] = true
	}
	return errs
}

// validateRouteConflicts 校验展平后的路由表里是否存在会产生歧义的 host+path 组合
func validateRouteConflicts(rules []RouterRule) []error {
	var errs []error
	type key struct{ host, path string }
	seen := make(map[key][]RouterRule)

	for _, r := range rules {
		hosts := r.Hosts
		if len(hosts) == 0 {
			hosts = []string{"*"}
		}
		for _, h := range hosts {
			k := key{h, r.PathPrefix}
			for _, prev := range seen[k] {
				if methodsOverlap(prev.Methods, r.Methods) {
					errs = append(errs, fmt.Errorf("%w: host %q path %q between service %q and %q",
						gerrors.ErrDuplicateRoute, h, r.PathPrefix, prev.Service, r.Service))
				}
			}
			seen[k] = append(seen[k], r)
		}
	}
	return errs
}

// validateServiceInstances 校验展平后的每个 service 至少有一个实例：0 实例的 service
func validateServiceInstances(services []ServiceRef) []error {
	var errs []error
	for _, s := range services {
		if len(s.Instances) == 0 {
			errs = append(errs, fmt.Errorf("%w: service %q", gerrors.ErrEmptyServiceInstances, s.Name))
		}
	}
	return errs
}

// methodsOverlap 判断两组 method 是否有交集；空集合表示「匹配所有方法」，必然与任何非空集合重叠
func methodsOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	set := make(map[string]bool, len(a))
	for _, m := range a {
		set[m] = true
	}
	for _, m := range b {
		if set[m] {
			return true
		}
	}
	return false
}
