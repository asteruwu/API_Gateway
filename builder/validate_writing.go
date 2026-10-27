package builder

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	gerrors "API_Gateway/pkg/errors"
	"API_Gateway/pkg/tools"
)

// ValidateWriting 校验编写态声明：必填/空值、格式、唯一性、冲突、名字引用，
// 以及 pipeline 与 route auth 声明的一致性
func ValidateWriting(gw GatewayConfig, platforms []PlatformConfig, plan *Plan) error {
	var errs []error

	errs = append(errs, validatePlatformNames(platforms)...)
	for _, p := range platforms {
		errs = append(errs, validateServiceNames(p)...)
		errs = append(errs, validateServiceDeclarations(p)...)
		errs = append(errs, validateRouteDeclarations(p)...)
	}

	errs = append(errs, validateQualifiedServiceNames(plan)...)
	errs = append(errs, validateRouteIDs(plan)...)
	errs = append(errs, validateRouteConflicts(plan)...)
	errs = append(errs, validateRouteAuthDeclarations(platforms, plan)...)
	errs = append(errs, validatePluginReferences(gw, plan)...)
	errs = append(errs, validateEnabledUniqueness(gw)...)
	errs = append(errs, validatePoolConfig(gw.Pool)...)

	if len(errs) == 0 {
		return nil
	}
	return configErrors("writing", errs)
}

// configErrors 把某阶段的校验错误聚合为统一的配置错误
func configErrors(phase string, errs []error) error {
	return fmt.Errorf("%w: %s: %w", gerrors.ErrInvalidConfig, phase, errors.Join(errs...))
}

// validatePlatformNames 校验 platform 名非空且全局唯一
func validatePlatformNames(platforms []PlatformConfig) []error {
	var errs []error
	names := make([]string, 0, len(platforms))
	for i, p := range platforms {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("%w: platforms[%d]", gerrors.ErrEmptyPlatformName, i))
			continue
		}
		names = append(names, p.Name)
	}
	for _, name := range tools.DuplicateStrings(names) {
		errs = append(errs, fmt.Errorf("%w: %q", gerrors.ErrDuplicatePlatformName, name))
	}
	return errs
}

// validateServiceNames 校验单个 platform 内 service 名非空且唯一
func validateServiceNames(p PlatformConfig) []error {
	var errs []error
	names := make([]string, 0, len(p.Service))
	for i, s := range p.Service {
		if s.Name == "" {
			errs = append(errs, fmt.Errorf("%w: platform %q service[%d]", gerrors.ErrEmptyServiceName, p.Name, i))
			continue
		}
		names = append(names, s.Name)
	}
	for _, name := range tools.DuplicateStrings(names) {
		errs = append(errs, fmt.Errorf("%w: platform %q service %q", gerrors.ErrDuplicateServiceName, p.Name, name))
	}
	return errs
}

// validateServiceDeclarations 校验 service 的必填字段、实例列表与实例地址唯一性
func validateServiceDeclarations(p PlatformConfig) []error {
	var errs []error
	for i, s := range p.Service {
		where := fmt.Sprintf("platform %q service[%d] %q", p.Name, i, s.Name)
		if s.Transformer == "" {
			errs = append(errs, fmt.Errorf("%w: %s has no transformer bound", gerrors.ErrTransformerNotFound, where))
		}
		if len(s.Instances) == 0 {
			errs = append(errs, fmt.Errorf("%w: %s", gerrors.ErrEmptyServiceInstances, where))
			continue
		}
		addrs := make([]string, 0, len(s.Instances))
		for j, inst := range s.Instances {
			if inst.Addr == "" {
				errs = append(errs, fmt.Errorf("%w: %s instance[%d] addr is empty", gerrors.ErrInvalidConfig, where, j))
				continue
			}
			addrs = append(addrs, inst.Addr)
		}
		for _, addr := range tools.DuplicateStrings(addrs) {
			errs = append(errs, fmt.Errorf("%w: %s duplicates addr %q", gerrors.ErrInvalidConfig, where, addr))
		}
	}
	return errs
}

// validateRouteDeclarations 校验 route 的必填字段、methods 元素与 filters 局部唯一性
func validateRouteDeclarations(p PlatformConfig) []error {
	var errs []error
	for _, s := range p.Service {
		for i, r := range s.Routes {
			where := fmt.Sprintf("platform %q service %q route[%d]", p.Name, s.Name, i)
			if r.PathPrefix == "" {
				errs = append(errs, fmt.Errorf("%w: %s path_prefix is empty", gerrors.ErrInvalidConfig, where))
			} else if !strings.HasPrefix(r.PathPrefix, "/") {
				errs = append(errs, fmt.Errorf("%w: %s path prefix %q must start with \"/\"", gerrors.ErrInvalidConfig, where, r.PathPrefix))
			}
			for j, m := range r.Methods {
				if m == "" {
					errs = append(errs, fmt.Errorf("%w: %s methods[%d] is empty", gerrors.ErrInvalidConfig, where, j))
				}
			}
			for _, name := range tools.DuplicateStrings(r.Filters) {
				errs = append(errs, fmt.Errorf("%w: %s repeats filter %q", gerrors.ErrInvalidConfig, where, name))
			}
		}
	}
	return errs
}

// validateQualifiedServiceNames 校验展平后的限定服务名唯一
func validateQualifiedServiceNames(plan *Plan) []error {
	var errs []error
	names := make([]string, 0, len(plan.Services))
	for i, s := range plan.Services {
		if s.Name == "" {
			errs = append(errs, fmt.Errorf("%w: services[%d] qualified name is empty", gerrors.ErrEmptyServiceName, i))
			continue
		}
		names = append(names, s.Name)
	}
	for _, name := range tools.DuplicateStrings(names) {
		errs = append(errs, fmt.Errorf("%w: %q", gerrors.ErrDuplicateServiceName, name))
	}
	return errs
}

// validateRouteIDs 校验 RouteID 非空且唯一（派生不变量，防御性检查）
func validateRouteIDs(plan *Plan) []error {
	var errs []error
	ids := make([]string, 0, len(plan.Routes))
	for i, r := range plan.Routes {
		if r.Rule.ID == "" {
			errs = append(errs, fmt.Errorf("%w: routes[%d] ID is empty", gerrors.ErrInvalidConfig, i))
			continue
		}
		ids = append(ids, r.Rule.ID)
	}
	for _, id := range tools.DuplicateStrings(ids) {
		errs = append(errs, fmt.Errorf("%w: duplicate route ID %q", gerrors.ErrInvalidConfig, id))
	}
	return errs
}

// validateRouteConflicts 校验展平后的路由表里是否存在会产生歧义的 host+path+method 组合
func validateRouteConflicts(plan *Plan) []error {
	var errs []error
	type key struct{ host, path string }
	seen := make(map[key][]RouterRule)

	for _, pr := range plan.Routes {
		r := pr.Rule
		hosts := r.Hosts
		if len(hosts) == 0 {
			hosts = []string{"*"}
		}
		for _, h := range hosts {
			k := key{h, r.PathPrefix}
			for _, prev := range seen[k] {
				if tools.StringsOverlap(prev.Methods, r.Methods) {
					errs = append(errs, fmt.Errorf("%w: host %q path %q between service %q and %q",
						gerrors.ErrDuplicateRoute, h, r.PathPrefix, prev.Service, r.Service))
				}
			}
			seen[k] = append(seen[k], r)
		}
	}
	return errs
}

// validateRouteAuthDeclarations 校验 pipeline 与 route auth 声明的一致性：
// pipeline 含 auth ⇔ 声明了 auth 块；两边都有时再校验声明自身。
func validateRouteAuthDeclarations(platforms []PlatformConfig, plan *Plan) []error {
	var errs []error
	byID := make(map[string]*PlannedRoute, len(plan.Routes))
	for i := range plan.Routes {
		byID[plan.Routes[i].Rule.ID] = &plan.Routes[i]
	}

	for _, p := range platforms {
		for _, s := range p.Service {
			service := qualifiedServiceName(p.Name, s.Name)
			for i, r := range s.Routes {
				where := fmt.Sprintf("platform %q service %q route[%d]", p.Name, s.Name, i)
				pr, ok := byID[routeID(service, i)]
				if !ok {
					errs = append(errs, fmt.Errorf("%w: %s missing from plan", gerrors.ErrInvalidConfig, where))
					continue
				}

				hasAuth := slices.Contains(pr.Pipeline, "auth")
				switch {
				case hasAuth && r.Auth == nil:
					errs = append(errs, fmt.Errorf("%w: %s pipeline contains %q but declares no auth block",
						gerrors.ErrInvalidAuthConfig, where, "auth"))
				case !hasAuth && r.Auth != nil:
					errs = append(errs, fmt.Errorf("%w: %s declares auth but pipeline does not contain %q",
						gerrors.ErrInvalidAuthConfig, where, "auth"))
				case hasAuth && r.Auth != nil:
					errs = append(errs, validateAuthDeclaration(where, *r.Auth)...)
				}
			}
		}
	}
	return errs
}

// validateAuthDeclaration 校验单条 route 的鉴权声明结构：
// public 与认证/授权要求互斥；非匿名 route 必须声明 credential。
func validateAuthDeclaration(where string, auth RouteAuthConfig) []error {
	if auth.Public {
		var errs []error
		if auth.Credential != "" {
			errs = append(errs, fmt.Errorf("%w: %s is public but also declares credential %q",
				gerrors.ErrInvalidAuthConfig, where, auth.Credential))
		}
		if len(auth.Require.Roles) > 0 || len(auth.Require.Scopes) > 0 {
			errs = append(errs, fmt.Errorf("%w: %s is public but also declares authorization requirements",
				gerrors.ErrInvalidAuthConfig, where))
		}
		return errs
	}

	if auth.Credential == "" {
		return []error{fmt.Errorf("%w: %s declares auth but no credential",
			gerrors.ErrInvalidAuthConfig, where)}
	}
	return nil
}

// validatePluginReferences 校验 enabled / route 引用的插件名在注册表中存在，
// 且每个 service 的 transformer 已启用。
func validatePluginReferences(gw GatewayConfig, plan *Plan) []error {
	var errs []error

	errs = append(errs, validateEnabledPlugins("tcp_filters", gw.TCPFilters.Enabled, gw.Plugins.TCP)...)
	errs = append(errs, validateEnabledPlugins("http_filters", plan.HTTPFilters, gw.Plugins.HTTP)...)
	errs = append(errs, validateEnabledPlugins("transformer", gw.Transformer.Enabled, gw.Plugins.Transformer)...)

	for _, s := range plan.Services {
		if s.Transformer == "" {
			continue // 已由 service 声明校验报告
		}
		if !slices.Contains(gw.Transformer.Enabled, s.Transformer) {
			errs = append(errs, fmt.Errorf("%w: service %q binds %q which is not enabled",
				gerrors.ErrTransformerNotFound, s.Name, s.Transformer))
		}
	}
	return errs
}

// validateEnabledPlugins 校验 enabled 名单里的每个插件名在注册表中都有配置项
func validateEnabledPlugins(category string, enabled []string, registry map[string]any) []error {
	var errs []error
	for _, name := range enabled {
		if _, ok := registry[name]; !ok {
			errs = append(errs, fmt.Errorf("%w: %s plugin %q referenced but has no config entry",
				gerrors.ErrFilterConfigNotFound, category, name))
		}
	}
	return errs
}

// validateEnabledUniqueness 校验三张 enabled 列表内部不重复
func validateEnabledUniqueness(gw GatewayConfig) []error {
	var errs []error
	errs = append(errs, validateUniqueNames("tcp_filters.enabled", gw.TCPFilters.Enabled)...)
	errs = append(errs, validateUniqueNames("http_filters.enabled", gw.HTTPFilters.Enabled)...)
	errs = append(errs, validateUniqueNames("transformer.enabled", gw.Transformer.Enabled)...)
	return errs
}

func validateUniqueNames(where string, names []string) []error {
	var errs []error
	for _, name := range tools.DuplicateStrings(names) {
		errs = append(errs, fmt.Errorf("%w: %s duplicates %q", gerrors.ErrInvalidConfig, where, name))
	}
	return errs
}

// validatePoolConfig 校验网关级连接池配置：零值表示使用默认值，负数与自相矛盾的取值视为非法
func validatePoolConfig(cfg PoolConfig) []error {
	var errs []error
	if cfg.MaxConn < 0 {
		errs = append(errs, fmt.Errorf("%w: pool.max_conn must not be negative", gerrors.ErrInvalidPoolConfig))
	}
	if cfg.MinIdle < 0 {
		errs = append(errs, fmt.Errorf("%w: pool.min_idle must not be negative", gerrors.ErrInvalidPoolConfig))
	}
	if cfg.MaxConn > 0 && cfg.MinIdle > cfg.MaxConn {
		errs = append(errs, fmt.Errorf("%w: pool.min_idle %d exceeds max_conn %d",
			gerrors.ErrInvalidPoolConfig, cfg.MinIdle, cfg.MaxConn))
	}
	if cfg.DialTimeout < 0 || cfg.BorrowTimeout < 0 || cfg.IdleTimeout < 0 || cfg.WaitTimeout < 0 {
		errs = append(errs, fmt.Errorf("%w: pool durations must not be negative", gerrors.ErrInvalidPoolConfig))
	}
	return errs
}
