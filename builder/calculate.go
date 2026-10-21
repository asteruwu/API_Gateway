package builder

func Calculate(gw GatewayConfig, platforms []PlatformConfig) []string {
	seen := make(map[string]bool)
	names := make([]string, 0, len(gw.HTTPFilters.Enabled))
	add := func(list []string) {
		for _, name := range list {
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}

	add(gw.HTTPFilters.Enabled)
	for _, p := range platforms {
		for _, s := range p.Service {
			for _, r := range s.Routes {
				add(r.Filters)
			}
		}
	}
	return names
}
