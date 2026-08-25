package handler

import (
	"API_Gateway/builder"
	hf "API_Gateway/handler/http_filter"
	"API_Gateway/handler/message"
	"API_Gateway/handler/transformer"
	gconst "API_Gateway/pkg/constant"
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
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

type ConnHandler struct {
	conn   net.Conn
	reader *bufio.Reader
}

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

func (h *HTTPHandler) Process(ch ConnHandler) (*message.Response, error) {
	buf, err := http.ReadRequest(ch.reader)
	if err != nil {
		return nil, err
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
	ch := buildConnHandler(conn)
	for {
		ch.conn.SetReadDeadline(time.Now().Add(gconst.DefaultHandlerReadTimeout))
		msg, err := h.Process(ch)
		if err != nil {
			// TODO
			log.Printf("[handler]failed to handle connection, err message: %s", err.Error())
			if shouldClose := closeConnOrNot(err); shouldClose {
				log.Printf("[handler]close conn, err message: %s", err.Error())
				return err
			}
			log.Println("[handler]writing response to connection...")
			res, err := h.encoder.Encode(message.ErrorResponse(err))
			if err == nil {
				_, err = ch.conn.Write(res)
			} else {
				log.Printf("[handler]failed to write response to connection, err message: %s", err.Error())
			}
			continue
		}
		res, err := h.encoder.Encode(msg)
		if err != nil {
			// TODO
			return err
		}
		log.Println("[handler]writing response to connection")
		_, err = ch.conn.Write(res)
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

func buildConnHandler(conn net.Conn) ConnHandler {
	return ConnHandler{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

func closeConnOrNot(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		switch {
		case errors.Is(opErr.Err, syscall.ECONNRESET),
			errors.Is(opErr.Err, syscall.ECONNABORTED),
			errors.Is(opErr.Err, syscall.EPIPE):
			return true
		}
	}
	return false
}
