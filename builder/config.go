package builder

type Config struct {
	Connector ConnectorConfig
	Handler   HandlerConfig
	Backend   BackendConfig
}

type ConnectorConfig struct {
	// 监听端口
	// 连接数限流 policy
	Port string
}

type HandlerConfig struct {
	Decoder     DecoderConfig
	Encoder     EncoderConfig
	Filter      HTTPFilterConfig
	Transformer TransformerConfig
}

type DecoderConfig struct {
	// 解析器配置
}

type EncoderConfig struct {
	// 编码器配置
}

type HTTPFilterConfig struct {
	// 鉴权 Config
	// 限流 Config
	// 路由 Config
	// ...
}

type TransformerConfig struct {
	// gRPC
	// MCP
	// ...
}

type BackendConfig struct {
	// 先写静态配置
	Service []ServiceConfig
}

type ServiceConfig struct {
	// 具体的后端服务配置
}
