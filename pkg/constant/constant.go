package gconst

import (
	"net/http"
	"time"
)

// ===== connector 连接层 =====

// DefaultListenerPort 监听端口默认值，未配置时使用
const DefaultConnectorListenerPort = "6666"

// DefaultConnectorAcceptBackoffMin Accept 失败后的首次退避时长
const DefaultConnectorAcceptBackoffMin = 5 * time.Millisecond

// DefaultConnectorAcceptBackoffMax Accept 连续失败时的退避上限
const DefaultConnectorAcceptBackoffMax = 1 * time.Second

// ===== handler 处理层 =====

// DefaultReadTimeout 单次读请求的超时时间
const DefaultHandlerReadTimeout = 30 * time.Second

// DefaultHandlerBodySize Decoder 默认读取大小
const DefaultHandlerBodySize = 4096

// DefaultHTTPStatus 默认状态码
const DefaultHTTPStatus = http.StatusOK

// DefaultHTTPProto 默认协议类型
const DefaultHTTPProto = "HTTP/1.1"

// DefaultErrorContentType 默认文本错误响应
const DefaultErrorContentType = "text/plain; charset=utf-8"

// AuthCredentialBearer 凭证类型：Authorization: Bearer <JWT>
const AuthCredentialBearer = "bearer"

// AuthWWWAuthenticateBearer 401 响应的 WWW-Authenticate 头值
const AuthWWWAuthenticateBearer = "Bearer"

// GatewayHeaderPrefix 网关自产头（身份透传等）的保留命名空间前缀
const GatewayHeaderPrefix = "X-Gateway-"

// AuthAlgorithmHS256 JWT 签名算法 HMAC-SHA256
const AuthAlgorithmHS256 = "HS256"

// AuthMinHS256SecretLen HS256 密钥最短字节数，RFC 7518 3.2 要求不短于哈希输出长度
const AuthMinHS256SecretLen = 32

// ===== backend 后端层 =====

// DefaultPoolMaxConn 连接池全局最大连接数（idle + active）
const DefaultPoolMaxConn = 64

// DefaultPoolMinIdle 连接池最小空闲连接数，reaper 预热到底线
const DefaultPoolMinIdle = 4

// DefaultPoolDialTimeout 连接池拨号超时
const DefaultPoolDialTimeout = 3 * time.Second

// DefaultPoolBorrowTimeout 连接借出超时，持有超过该时长视为故障强制关闭
const DefaultPoolBorrowTimeout = 10 * time.Second

// DefaultPoolIdleTimeout 空闲连接超时，超过回收但不低于 MinIdle
const DefaultPoolIdleTimeout = 60 * time.Second

// DefaultPoolWaitTimeout 池满时等待可用连接的最长时间
const DefaultPoolWaitTimeout = 5 * time.Second

// DefaultPoolReapIntervalMin reaper 扫描间隔下限
const DefaultPoolReapIntervalMin = 100 * time.Millisecond

// DefaultPoolReapIntervalMax reaper 扫描间隔上限
const DefaultPoolReapIntervalMax = 30 * time.Second
