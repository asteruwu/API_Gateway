package builder

type PlatformConfig struct {
	Name    string          `yaml:"name"`
	Host    []string        `yaml:"host"`
	Service []ServiceConfig `yaml:"service"`
}

type ServiceConfig struct {
	Name      string            `yaml:"name"`
	Instances []InstanceConfig  `yaml:"instances"`
	Routes    []RouteRuleConfig `yaml:"routes"`
}

type RouteRuleConfig struct {
	PathPrefix string   `yaml:"path_prefix"`
	Methods    []string `yaml:"methods"`
}
