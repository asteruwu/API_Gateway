package builder

import (
	"time"

	gconst "API_Gateway/pkg/constant"
)

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

type HTTPFilterConfig struct {
	Router    RouterConfig
	Configs   map[string]any
	Pipelines map[string][]string
}

type RouterConfig struct {
	Rules []RouterRule
}

type RouterRule struct {
	ID         string
	Hosts      []string
	PathPrefix string
	Methods    []string
	Service    string
}

type AuthFilterConfig struct {
	Credentials []string
	JWT         JWTVerifierConfig
	Policies    map[string]AuthPolicy
}

type JWTVerifierConfig struct {
	SecretEnv string
	Issuer    string
	Audience  string
	Leeway    time.Duration
}

type AuthPolicy struct {
	Public     bool
	Credential string
	Roles      []string
	Scopes     []string
}

type RateLimitFilterConfig struct {
	Limiters []gconst.RateLimitDimension
	Rules    map[string]RateLimitRule
}

type RateLimitRule struct {
	Dimension gconst.RateLimitDimension
	Params    RateLimitParams
}

type RateLimitParams struct {
	QPS   float64
	Burst int
}

type TransformerConfig struct {
	Transformers map[string]TransformerEntry
}

type TransformerEntry struct {
	Config   any
	Services []string
}

type TestTransformerConfig struct{}

// ===== backend 后端层 =====

type BackendConfig struct {
	Service []ServiceRef
	Pool    PoolConfig
}

type ServiceRef struct {
	Name      string
	Instances []InstanceConfig
}

// ===== 共用 =====

type InstanceConfig struct {
	Addr string `yaml:"addr"`
}

type DecoderConfig struct {
	// 解析器配置
	MaxBody int64 `yaml:"max_body"`
}

type EncoderConfig struct {
	// 编码器配置
}

type PoolConfig struct {
	MaxConn       int           `yaml:"max_conn"`
	MinIdle       int           `yaml:"min_idle"`
	DialTimeout   time.Duration `yaml:"dial_timeout"`
	BorrowTimeout time.Duration `yaml:"borrow_timeout"`
	IdleTimeout   time.Duration `yaml:"idle_timeout"`
	WaitTimeout   time.Duration `yaml:"wait_timeout"`
}
