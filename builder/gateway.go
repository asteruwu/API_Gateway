package builder

type gatewayFile struct {
	Gateway GatewayConfig `yaml:"gateway"`
}

// GatewayConfig 网关自身参数：端口、插件注册表、链/启用集合、解码上限等
type GatewayConfig struct {
	Port        string           `yaml:"port"`
	Plugins     PluginsConfig    `yaml:"plugins"`
	TCPFilters  PluginActivation `yaml:"tcp_filters"`
	HTTPFilters PluginActivation `yaml:"http_filters"`
	Decoder     DecoderConfig    `yaml:"decoder"`
	Encoder     EncoderConfig    `yaml:"encoder"`
	Transformer PluginActivation `yaml:"transformer"`
	Pool        PoolConfig       `yaml:"pool"`
}

type PluginsConfig struct {
	TCP         map[string]any `yaml:"tcp"`
	HTTP        map[string]any `yaml:"http"`
	Transformer map[string]any `yaml:"transformer"`
}

type PluginActivation struct {
	Enabled []string `yaml:"enabled"`
}
