package transformer

import "API_Gateway/handler/message"

// 每个实现代表一种后端协议（grpc/mcp/...）
type Transformer interface {
	Transform(req *message.Request) ([]byte, error) // 网关模型 → 后端协议字节
	Restore(resp []byte) (*message.Response, error) // 后端协议字节 → 网关模型
}
