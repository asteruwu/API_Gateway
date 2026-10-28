package builder

import (
	"errors"
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

// Compile 只负责把 Plan 与水合后的插件配置装配成运行态 Config，不做语义校验。
func Compile(gw GatewayConfig, plan *Plan) (*Config, error) {
	if plan == nil {
		return nil, fmt.Errorf("%w: plan is nil", gerrors.ErrInvalidConfig)
	}

	tcpFilters, tcpErrs := assembleSelectedList(gw.TCPFilters.Enabled, gw.Plugins.TCP)
	httpFilters, httpErrs := assembleSelectedMap(plan.HTTPFilters, gw.Plugins.HTTP)
	transformers, transformerErrs := assembleSelectedMap(gw.Transformer.Enabled, gw.Plugins.Transformer)

	errs := append(tcpErrs, httpErrs...)
	errs = append(errs, transformerErrs...)
	if len(errs) > 0 {
		return nil, fmt.Errorf("compile: %w", errors.Join(errs...))
	}

	rules, pipelines, policies := assembleRoutes(plan.Routes)
	services, servicesByTransformer := buildServiceTables(plan.Services)
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
				Router:    router,
				Configs:   httpFilters,
				Pipelines: pipelines,
			},
			Transformer: TransformerConfig{
				Transformers: buildTransformerEntries(transformers, servicesByTransformer),
			},
		},
		Backend: BackendConfig{
			Service: services,
			Pool:    gw.Pool,
		},
	}, nil
}

// assembleRoutes 从 Plan 派生 Router 规则、pipeline 与策略三张运行态表
func assembleRoutes(routes []PlannedRoute) ([]RouterRule, map[string][]string, map[string]AuthPolicy) {
	rules := make([]RouterRule, 0, len(routes))
	pipelines := make(map[string][]string, len(routes))
	policies := make(map[string]AuthPolicy, len(routes))
	for _, r := range routes {
		rules = append(rules, r.Rule)
		pipelines[r.Rule.ID] = append([]string(nil), r.Pipeline...)
		policies[r.Rule.ID] = r.Policy
	}
	return rules, pipelines, policies
}

// buildServiceTables 一次遍历 Plan 中的服务，同时产出 backend 的 ServiceRef 表
// 和 transformer → service 的绑定表。
func buildServiceTables(services []PlannedService) ([]ServiceRef, map[string][]string) {
	refs := make([]ServiceRef, 0, len(services))
	byTransformer := make(map[string][]string)
	for _, s := range services {
		instances := make([]InstanceConfig, len(s.Instances))
		copy(instances, s.Instances)
		refs = append(refs, ServiceRef{Name: s.Name, Instances: instances})
		if s.Transformer != "" {
			byTransformer[s.Transformer] = append(byTransformer[s.Transformer], s.Name)
		}
	}
	return refs, byTransformer
}

// attachAuthPolicies 把按 RouteID 聚合的策略表装进 auth filter 配置
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

// buildTransformerEntries 把「已启用配置」与「service 分组」合成运行态注册表
func buildTransformerEntries(enabled map[string]any, byTransformer map[string][]string) map[string]TransformerEntry {
	entries := make(map[string]TransformerEntry, len(enabled))
	for name, cfg := range enabled {
		entries[name] = TransformerEntry{Config: cfg, Services: byTransformer[name]}
	}
	return entries
}
