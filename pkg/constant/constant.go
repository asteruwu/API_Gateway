package gconst

import "time"

// ===== connector 连接层 =====

// DefaultListenerPort 监听端口默认值，未配置时使用
const DefaultConnectorListenerPort = "6666"

// ===== handler 处理层 =====

// DefaultReadTimeout 单次读请求的超时时间
const DefaultHandlerReadTimeout = 30 * time.Second

// ===== backend 后端层 =====
