package handler

import (
	"API_Gateway/builder"
	hf "API_Gateway/handler/http_filter"
	"API_Gateway/handler/message"
	"API_Gateway/handler/transformer"
	"net"
)

// 读取配置，初始化
// 主流程

type HTTPHandler struct {
	decoder     *Decoder
	encoder     *Encoder
	filters     map[string]hf.HTTPFilter
	transformer map[string]transformer.Transformer
	next        func(service string, payload []byte) ([]byte, error)
}

func NewHandler(cfg builder.HandlerConfig, next func(service string, payload []byte) ([]byte, error)) *HTTPHandler {
	decoder := NewDecoder(cfg.Decoder)
	encoder := NewEncoder(cfg.Encoder)
	// 初始化 filters 和 transformer
	return &HTTPHandler{
		decoder: decoder,
		encoder: encoder,
		next:    next,
	}
}

func (h *HTTPHandler) Process(conn net.Conn) (*message.Response, error) {
	// TODO
	// 1. 解析
	// 2. filter 编排
	// 3. 协议转换
	// 4. next 调用
	// 5. 协议转换
	return &message.Response{}, nil
}

func (h *HTTPHandler) HandleHTTPConn(conn net.Conn) error {
	for {
		msg, err := h.Process(conn)
		if err != nil {
			// TODO
			return err
		}
		res, err := h.encoder.Encode(msg)
		if err != nil {
			// TODO
			return err
		}
		conn.Write(res)
	}
}
