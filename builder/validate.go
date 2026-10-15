package builder

import (
	"fmt"
	"slices"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"API_Gateway/pkg/tools"
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

// validateTransformerRefs 校验 service 绑定的 transformer 名都已启用
func validateTransformerRefs(set PluginSet, byTransformer map[string][]string) []error {
	enabled := make(map[string]bool, len(set.Enabled))
	for _, name := range set.Enabled {
		enabled[name] = true
	}

	var errs []error
	for name, services := range byTransformer {
		if !enabled[name] {
			errs = append(errs, fmt.Errorf("%w: services %v bind %q which is not enabled", gerrors.ErrTransformerNotFound, services, name))
		}
	}
	return errs
}

// validateServiceTransformer 校验每个 service 都显式绑定了 transformer
func validateServiceTransformer(platforms []PlatformConfig) []error {
	var errs []error
	for _, p := range platforms {
		for i, s := range p.Service {
			if s.Transformer == "" {
				errs = append(errs, fmt.Errorf("%w: platform %q service[%d] %q has no transformer bound",
					gerrors.ErrTransformerNotFound, p.Name, i, s.Name))
			}
		}
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

// validateRouteAuth 校验路由鉴权与 auth 插件的引用一致性，以及鉴权插件自身的参数
func validateRouteAuth(gw GatewayConfig, platforms []PlatformConfig) []error {
	enabled := pluginEnabled(gw.HTTPFilters, "auth")

	var (
		cfg  AuthFilterConfig
		errs []error
	)
	if enabled {
		c, ok := gw.HTTPFilters.Configs["auth"].(AuthFilterConfig)
		if !ok {
			return []error{fmt.Errorf("%w: auth plugin config missing or of wrong type %T",
				gerrors.ErrInvalidAuthConfig, gw.HTTPFilters.Configs["auth"])}
		}
		cfg = c
		errs = append(errs, validateCredentials(cfg.Credentials)...)
		errs = append(errs, validateJWTVerifier(cfg.JWT)...)
	}

	declared := false
	for _, p := range platforms {
		for _, s := range p.Service {
			for i, r := range s.Routes {
				if r.Auth == nil {
					if enabled {
						errs = append(errs, fmt.Errorf("%w: platform %q service %q route[%d] declares no auth; declare auth or set public: true",
							gerrors.ErrInvalidAuthConfig, p.Name, s.Name, i))
					}
					continue
				}
				declared = true
				if enabled {
					where := fmt.Sprintf("platform %q service %q route[%d]", p.Name, s.Name, i)
					errs = append(errs, validateRouteAuthRule(where, *r.Auth, cfg.Credentials)...)
				}
			}
		}
	}

	if declared && !enabled {
		errs = append(errs, fmt.Errorf("%w: routes declare auth but http_filters %q is not enabled",
			gerrors.ErrInvalidAuthConfig, "auth"))
	}
	return errs
}

// validateRouteAuthRule 校验单条 route 的鉴权声明：
// public 与认证/授权要求互斥；非匿名 route 必须声明被网关接受的 credential。
func validateRouteAuthRule(where string, auth RouteAuthConfig, accepted []string) []error {
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
	if !slices.Contains(accepted, auth.Credential) {
		return []error{fmt.Errorf("%w: %s requires credential %q not accepted by auth filter %v",
			gerrors.ErrInvalidAuthConfig, where, auth.Credential, accepted)}
	}
	return nil
}

// validateCredentials 校验凭证类型都受支持
func validateCredentials(creds []string) []error {
	if len(creds) == 0 {
		return []error{fmt.Errorf("%w: credentials must not be empty", gerrors.ErrInvalidAuthConfig)}
	}
	var errs []error
	for _, c := range creds {
		if c != gconst.AuthCredentialBearer {
			errs = append(errs, fmt.Errorf("%w: unsupported credential %q", gerrors.ErrInvalidAuthConfig, c))
		}
	}
	return errs
}

// validateJWTVerifier 校验 JWT 校验参数：算法受支持、密钥来源已声明、容差非负
func validateJWTVerifier(cfg JWTVerifierConfig) []error {
	var errs []error
	if cfg.SecretEnv == "" {
		errs = append(errs, fmt.Errorf("%w: jwt.secret_env is required", gerrors.ErrInvalidAuthConfig))
	}
	if cfg.Leeway < 0 {
		errs = append(errs, fmt.Errorf("%w: jwt.leeway must not be negative", gerrors.ErrInvalidAuthConfig))
	}
	return errs
}

func pluginEnabled(set PluginSet, name string) bool {
	for _, n := range set.Enabled {
		if n == name {
			return true
		}
	}
	return false
}
