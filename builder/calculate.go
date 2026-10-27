package builder

import (
	"strconv"

	"API_Gateway/pkg/tools"
)

type Plan struct {
	HTTPFilters []string
	Routes      []PlannedRoute
	Services    []PlannedService
}

type PlannedRoute struct {
	Rule     RouterRule
	Pipeline []string
	Policy   AuthPolicy
}

type PlannedService struct {
	Name        string
	Transformer string
	Instances   []InstanceConfig
}

func Calculate(gw GatewayConfig, platforms []PlatformConfig) *Plan {
	httpFilters, routes, services := collectPlanData(gw.HTTPFilters.Enabled, platforms)

	return &Plan{
		HTTPFilters: httpFilters,
		Routes:      routes,
		Services:    services,
	}
}

func collectPlanData(enabled []string, platforms []PlatformConfig) (httpFilters []string, routes []PlannedRoute, services []PlannedService) {
	names := make([]string, 0, len(enabled))
	names = append(names, enabled...)

	routes = make([]PlannedRoute, 0)
	services = make([]PlannedService, 0)

	for _, p := range platforms {
		for _, s := range p.Service {
			service := qualifiedServiceName(p.Name, s.Name)
			services = append(services, PlannedService{
				Name:        service,
				Transformer: s.Transformer,
				Instances:   append([]InstanceConfig(nil), s.Instances...),
			})

			for i, r := range s.Routes {
				names = append(names, r.Filters...)
				routes = append(routes, PlannedRoute{
					Rule:     flattenRoute(service, p.Host, r, i),
					Pipeline: tools.CloneOrDefault(r.Filters, enabled),
					Policy:   flattenAuthPolicy(r.Auth),
				})
			}
		}
	}

	httpFilters = tools.UniqueStrings(names)
	return httpFilters, routes, services
}

func qualifiedServiceName(platform, service string) string {
	return platform + "." + service
}

func routeID(service string, idx int) string {
	return service + "#" + strconv.Itoa(idx)
}

func flattenRoute(service string, hosts []string, route RouteRuleConfig, idx int) RouterRule {
	return RouterRule{
		ID:         routeID(service, idx),
		Hosts:      append([]string(nil), hosts...),
		PathPrefix: route.PathPrefix,
		Methods:    append([]string(nil), route.Methods...),
		Service:    service,
	}
}

func flattenAuthPolicy(auth *RouteAuthConfig) AuthPolicy {
	if auth == nil {
		return AuthPolicy{}
	}
	return AuthPolicy{
		Public:     auth.Public,
		Credential: auth.Credential,
		Roles:      append([]string(nil), auth.Require.Roles...),
		Scopes:     append([]string(nil), auth.Require.Scopes...),
	}
}
