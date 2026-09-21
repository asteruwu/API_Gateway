package builder

// GatewayConfig 网关自身参数：端口、插件列表、解码上限等
type GatewayConfig struct {
	Port        string        `yaml:"port"`
	TCPFilters  PluginSet     `yaml:"tcp_filters"`
	HTTPFilters PluginSet     `yaml:"http_filters"`
	Decoder     DecoderConfig `yaml:"decoder"`
	Encoder     EncoderConfig `yaml:"encoder"`
	Transformer PluginSet     `yaml:"transformer"`
}

type PluginSet struct {
	Enabled []string       `yaml:"enabled"`
	Configs map[string]any `yaml:"configs"`
}
