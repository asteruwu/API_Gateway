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
	decoder      *Decoder
	encoder      *Encoder
	filters      map[string]hf.HTTPFilter
	transformers map[string]transformer.Transformer
	next         func(service string, payload []byte) ([]byte, error)
}

const defaultReadBufferSize = 4096

func NewHandler(cfg builder.HandlerConfig, next func(service string, payload []byte) ([]byte, error)) *HTTPHandler {
	decoder := NewDecoder(cfg.Decoder)
	encoder := NewEncoder(cfg.Encoder)
	// 初始化 filters 和 transformer
	filters := buildFilter(cfg.Filter)
	transformers := buildTransformer(cfg.Transformer)
	return &HTTPHandler{
		decoder:      decoder,
		encoder:      encoder,
		filters:      filters,
		transformers: transformers,
		next:         next,
	}
}

func (h *HTTPHandler) Process(conn net.Conn) (*message.Response, error) {
	buf, err := io.ReadAll(conn)
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, io.EOF
	}
	// TODO
	// 1. 解析
	msgReq, err := h.decoder.Decode(buf)
	if err != nil {
		return nil, err
	}
	// 2. filter 编排
	filter_0 := h.filters["testF"]
	msgResp, err := filter_0.HandleHTTPFilt(&msgReq)
	if err != nil {
		return nil, err
	}
	log.Printf("[handler]pass filter, response: %v", msgResp.Raw)
	// 3. 协议转换
	transformer_0 := h.transformers["testT"]
	tResp, err := transformer_0.Transform(&msgReq)
	if err != nil {
		return nil, err
	}
	// 4. next 调用
	log.Println("[handler]pass message to backend")
	resp, err := h.next("testBackend", tResp)
	if err != nil {
		return nil, err
	}
	// 5. 协议转换
	// return h.transformer["type"].Restore(res)
	return transformer_0.Restore(resp)
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

func buildFilter(cfg builder.HTTPFilterConfig) map[string]hf.HTTPFilter {
	fMap := make(map[string]hf.HTTPFilter)
	f := hf.TestFilter{
		Name: "testF",
	}
	fMap[f.Name] = &f
	return fMap
}

func buildTransformer(cfg builder.TransformerConfig) map[string]transformer.Transformer {
	tMap := make(map[string]transformer.Transformer)
	t := transformer.TestTransformer{
		Name: "testT",
	}
	tMap[t.Name] = &t
	return tMap
}
