package handler

import (
	"API_Gateway/builder"
	hf "API_Gateway/handler/http_filter"
	"API_Gateway/handler/message"
	"API_Gateway/handler/transformer"
	"io"
	"log"
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

const defaultReadBufferSize = 4096

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
	buf := make([]byte, defaultReadBufferSize)
	_, err := conn.Read(buf)
	if err == io.EOF {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	// TODO
	// 1. 解析
	// 2. filter 编排
	// 3. 协议转换
	// 4. next 调用
	log.Println("[handler]pass message to backend")
	resp, err := h.next("", []byte{})
	if err != nil {
		return nil, err
	}
	// 5. 协议转换
	// return h.transformer["type"].Restore(res)
	return &message.Response{Raw: resp}, nil
}

func (h *HTTPHandler) HandleHTTPConn(conn net.Conn) error {
	for {
		msg, err := h.Process(conn)
		if err == io.EOF {
			log.Println("[handler]completed request")
			return nil
		}
		if err != nil {
			// TODO
			log.Printf("[handler]failed to handle connection, err message: %s", err)
			return err
		}
		res, err := h.encoder.Encode(msg)
		if err != nil {
			// TODO
			return err
		}
		log.Println("[handler]writing response to connection")
		_, err = conn.Write(res)
		if err != nil {
			return err
		}
	}
}
