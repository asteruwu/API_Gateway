package builder

import (
	"errors"
	"fmt"
	"strconv"

	gerrors "API_Gateway/pkg/errors"
)

func Compile(gw GatewayConfig, platforms []PlatformConfig) (*Config, error) {
	errs := globalValidation(platforms)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", gerrors.ErrInvalidConfig, errors.Join(errs...))
	}

	rules, policies, pipelines := compileRoutes(platforms, gw.HTTPFilters.Enabled)
	errs = append(errs, validateRouteConflicts(rules)...)

	service := flattenServices(platforms)
	errs = append(errs, validateServiceInstances(service)...)

	errs = append(errs, validatePoolConfig(gw.Pool)...)

	tcpFilters, tcpErrs := assembleEnabledList(gw.TCPFilters)
	errs = append(errs, tcpErrs...)

	httpFilters, httpErrs := assembleEnabledMap(PluginSet{
		Enabled: Calculate(gw, platforms),
		Configs: gw.HTTPFilters.Configs,
	})
	errs = append(errs, httpErrs...)

	transformers, transformerErrs := assembleEnabledMap(gw.Transformer)
	errs = append(errs, transformerErrs...)

	byTransformer := groupServicesByTransformer(platforms)
	errs = append(errs, validateTransformerRefs(gw.Transformer, byTransformer)...)
	errs = append(errs, validateServiceTransformer(platforms)...)
	errs = append(errs, validateRouteAuth(gw, platforms)...)

	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", gerrors.ErrInvalidConfig, errors.Join(errs...))
	}

	attachAuthPolicies(httpFilters, policies)
	router := RouterConfig{Rules: rules}

	return &Config{
		Connector: ConnectorConfig{
			Port:    gw.Port,
			Filters: tcpFilters,
		},
		Handler: HandlerConfig{
			Decoder: gw.Decoder,
			Encoder: gw.Encoder,
			Filter: HTTPFilterConfig{
				Filters:   legacyFilterList(router, gw.HTTPFilters.Enabled, httpFilters),
				Router:    router,
				Configs:   httpFilters,
				Pipelines: pipelines,
			},
			Transformer: TransformerConfig{
				Transformers: buildTransformerEntries(transformers, byTransformer),
			},
		},
		Backend: BackendConfig{
			Service: service,
			Pool:    gw.Pool,
		},
	}, nil
}

func globalValidation(platforms []PlatformConfig) []error {
	errs := []error{}
	errs = append(errs, validatePlatformNames(platforms)...)
	for _, p := range platforms {
		errs = append(errs, validateServiceNames(p)...)
	}
	return errs
}

// flattenServices 把各 platform 下的 service 压平成 backend 注册表用的 ServiceRef 列表
func flattenServices(platforms []PlatformConfig) []ServiceRef {
	refs := make([]ServiceRef, 0)
	for _, p := range platforms {
		for _, s := range p.Service {
			instances := make([]InstanceConfig, len(s.Instances))
			copy(instances, s.Instances)
			refs = append(refs, ServiceRef{
				Name:      qualifiedServiceName(p.Name, s.Name),
				Instances: instances,
			})
		}
	}
	return refs
}

// compileRoutes 一次遍历平台配置，同时展平路由规则、聚合鉴权策略、产出各 route 的 pipeline
func compileRoutes(platforms []PlatformConfig, defaultChain []string) ([]RouterRule, map[string]AuthPolicy, map[string][]string) {
	rules := make([]RouterRule, 0)
	policies := make(map[string]AuthPolicy)
	pipelines := make(map[string][]string)
	for _, p := range platforms {
		for _, s := range p.Service {
			service := qualifiedServiceName(p.Name, s.Name)
			for i, r := range s.Routes {
				rule := flattenRoute(service, p.Host, r, i)
				rules = append(rules, rule)
				policies[rule.ID] = flattenAuthPolicy(r.Auth)
				pipelines[rule.ID] = routePipeline(r.Filters, defaultChain)
			}
		}
	}
	return rules, policies, pipelines
}

func routePipeline(declared, defaultChain []string) []string {
	src := declared
	if declared == nil {
		src = defaultChain
	}
	return append(make([]string, 0, len(src)), src...)
}

// flattenRoute 把单条 route 声明展平为 RouterRule，并按 service+下标生成稳定 ID
func flattenRoute(service string, hosts []string, route RouteRuleConfig, idx int) RouterRule {
	return RouterRule{
		ID:         routeID(service, idx),
		Hosts:      hosts,
		PathPrefix: route.PathPrefix,
		Methods:    route.Methods,
		Service:    service,
	}
}

// flattenAuthPolicy 把编写态的分层声明展平为运行态策略；nil 表示该 route 未声明 auth。
func flattenAuthPolicy(auth *RouteAuthConfig) AuthPolicy {
	if auth == nil {
		return AuthPolicy{}
	}
	return AuthPolicy{
		Public:     auth.Public,
		Credential: auth.Credential,
		Roles:      auth.Require.Roles,
		Scopes:     auth.Require.Scopes,
	}
}

func routeID(service string, idx int) string {
	return service + "#" + strconv.Itoa(idx)
}

// attachAuthPolicies 把反向聚合出的策略表装进 auth filter 配置
func attachAuthPolicies(configs map[string]any, policies map[string]AuthPolicy) {
	for name, c := range configs {
		cfg, ok := c.(AuthFilterConfig)
		if !ok {
			continue
		}
		cfg.Policies = policies
		configs[name] = cfg
	}
}

// legacyFilterList 过渡期产出旧链：Router 前置 + enabled 顺序，C 阶段 handler 切换后删除
func legacyFilterList(router RouterConfig, enabled []string, configs map[string]any) []any {
	list := make([]any, 0, len(enabled)+1)
	list = append(list, router)
	for _, name := range enabled {
		list = append(list, configs[name])
	}
	return list
}

func qualifiedServiceName(platform, service string) string {
	return platform + "." + service
}

// groupServicesByTransformer 建立 tranformer 到使用该协议的所有服务的映射表
func groupServicesByTransformer(platforms []PlatformConfig) map[string][]string {
	m := make(map[string][]string)
	for _, p := range platforms {
		for _, s := range p.Service {
			if s.Transformer == "" {
				continue
			}
			m[s.Transformer] = append(m[s.Transformer], qualifiedServiceName(p.Name, s.Name))
		}
	}
	return m
}

// buildTransformerEntries 把「已启用配置」与「service 分组」合成运行态注册表
func buildTransformerEntries(enabled map[string]any, byTransformer map[string][]string) map[string]TransformerEntry {
	entries := make(map[string]TransformerEntry, len(enabled))
	for name, cfg := range enabled {
		entries[name] = TransformerEntry{Config: cfg, Services: byTransformer[name]}
	}
	return entries
}
