package builder

// GatewayConfig 网关自身参数：端口、插件列表、解码上限等
type GatewayConfig struct {
	Port        string            `yaml:"port"`
	TCPFilters  []any             `yaml:"tcp_filters"`
	HTTPFilters []any             `yaml:"http_filters"`
	Decoder     DecoderConfig     `yaml:"decoder"`
	Encoder     EncoderConfig     `yaml:"encoder"`
	Transformer TransformerConfig `yaml:"transformer"`
}
