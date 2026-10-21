package builder

type platformFile struct {
	Platforms []PlatformConfig `yaml:"platforms"`
}

type PlatformConfig struct {
	Name    string          `yaml:"name"`
	Host    []string        `yaml:"host"`
	Service []ServiceConfig `yaml:"service"`
}

type ServiceConfig struct {
	Name        string            `yaml:"name"`
	Transformer string            `yaml:"transformer"`
	Instances   []InstanceConfig  `yaml:"instances"`
	Routes      []RouteRuleConfig `yaml:"routes"`
}

type RouteRuleConfig struct {
	PathPrefix string           `yaml:"path_prefix"`
	Methods    []string         `yaml:"methods"`
	Filters    []string         `yaml:"filters"`
	Auth       *RouteAuthConfig `yaml:"auth"`
}

type RouteAuthConfig struct {
	Public     bool             `yaml:"public"`
	Credential string           `yaml:"credential"`
	Require    RouteAuthRequire `yaml:"require"`
}

type RouteAuthRequire struct {
	Roles  []string `yaml:"roles"`
	Scopes []string `yaml:"scopes"`
}
