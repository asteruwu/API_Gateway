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

	tcpFilters, tcpErrs := assembleEnabledList(gw.TCPFilters)
	errs = append(errs, tcpErrs...)

	httpFilters, httpErrs := assembleEnabledList(gw.HTTPFilters)
	errs = append(errs, httpErrs...)

	transformers, transformerErrs := assembleEnabledMap(gw.Transformer)
	errs = append(errs, transformerErrs...)

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
			Decoder:     gw.Decoder,
			Encoder:     gw.Encoder,
			Filter:      HTTPFilterConfig{Filters: httpFilters},
			Transformer: TransformerConfig{Transformers: transformers},
		},
		Backend: BackendConfig{
			Service: service,
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
