package gconst

import (
	"net/http"
	"time"
)

// ===== connector 连接层 =====

// DefaultListenerPort 监听端口默认值，未配置时使用
const DefaultConnectorListenerPort = "6666"

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

// DefaultPoolProbeTimeout 借出前失效探测的读超时
const DefaultPoolProbeTimeout = 10 * time.Millisecond

// DefaultPoolReapIntervalMin reaper 扫描间隔下限
const DefaultPoolReapIntervalMin = 100 * time.Millisecond

// DefaultPoolReapIntervalMax reaper 扫描间隔上限
const DefaultPoolReapIntervalMax = 30 * time.Second
