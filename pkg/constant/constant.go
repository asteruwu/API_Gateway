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
