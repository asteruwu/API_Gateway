package builder

// 运行态配置

type Config struct {
	Connector ConnectorConfig
	Handler   HandlerConfig
	Backend   BackendConfig
}

// ===== connector 连接层 =====

type ConnectorConfig struct {
	// 监听端口
	Port    string
	Filters []any
}

type TCPLimiterFilterConfig struct {
	MaxConn int
}

// ===== handler 处理层 =====

type HandlerConfig struct {
	Decoder     DecoderConfig
	Encoder     EncoderConfig
	Filter      HTTPFilterConfig
	Transformer TransformerConfig
}

type DecoderConfig struct {
	// 解析器配置
	MaxBody int64
}

type EncoderConfig struct {
	// 编码器配置
}

type HTTPFilterConfig struct {
	Filters []any
}

type RouterConfig struct {
	Rules []RouterRule
}

type RouterRule struct {
	Hosts      []string
	PathPrefix string
	Methods    []string
	Service    string
}

type TransformerConfig struct {
	Transformers map[string]any
}

type TestTransformerConfig struct{}

// ===== backend 后端层 =====

type BackendConfig struct {
	Service []ServiceRef
}

type ServiceRef struct {
	Name      string
	Instances []InstanceConfig
}

// ===== 共用 =====

type InstanceConfig struct {
	Addr string `yaml:"addr"`
}
