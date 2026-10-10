package builder

import (
	"errors"
	"fmt"

	gerrors "API_Gateway/pkg/errors"
)

func Compile(gw GatewayConfig, platforms []PlatformConfig) (*Config, error) {
	errs := globalValidation(platforms)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", gerrors.ErrInvalidConfig, errors.Join(errs...))
	}

	rules := flattenRoutes(platforms)
	errs = append(errs, validateRouteConflicts(rules)...)

	service := flattenServices(platforms)
	errs = append(errs, validateServiceInstances(service)...)

	errs = append(errs, validatePoolConfig(gw.Pool)...)

	tcpFilters, tcpErrs := assembleEnabledList(gw.TCPFilters)
	errs = append(errs, tcpErrs...)

	httpFilters, httpErrs := assembleEnabledList(gw.HTTPFilters)
	errs = append(errs, httpErrs...)

	transformers, transformerErrs := assembleEnabledMap(gw.Transformer)
	errs = append(errs, transformerErrs...)

	byTransformer := groupServicesByTransformer(platforms)
	errs = append(errs, validateTransformerRefs(gw.Transformer, byTransformer)...)
	errs = append(errs, validateServiceTransformer(platforms)...)

	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", gerrors.ErrInvalidConfig, errors.Join(errs...))
	}

	httpFilters = append(httpFilters, RouterConfig{Rules: rules})

	return &Config{
		Connector: ConnectorConfig{
			Port:    gw.Port,
			Filters: tcpFilters,
		},
		Handler: HandlerConfig{
			Decoder: gw.Decoder,
			Encoder: gw.Encoder,
			Filter:  HTTPFilterConfig{Filters: httpFilters},
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

// flattenRoutes 把各 platform 下 service 的 routes 压平成 RouterRule 列表
func flattenRoutes(platforms []PlatformConfig) []RouterRule {
	rules := make([]RouterRule, 0)
	for _, p := range platforms {
		for _, s := range p.Service {
			service := qualifiedServiceName(p.Name, s.Name)
			for _, r := range s.Routes {
				rules = append(rules, RouterRule{
					Hosts:      p.Host,
					PathPrefix: r.PathPrefix,
					Methods:    r.Methods,
					Service:    service,
				})
			}
		}
	}
	return rules
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
