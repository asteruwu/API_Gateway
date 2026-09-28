package transformer

import (
	"API_Gateway/handler/message"
	"net"
)

// 每个实现代表一种后端协议（grpc/mcp/...）
type Transformer interface {
	Transform(req *message.Request, conn net.Conn) (*message.Response, error)
}
