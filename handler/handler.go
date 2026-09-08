package handler

import (
	"API_Gateway/builder"
	hf "API_Gateway/handler/http_filter"
	"API_Gateway/handler/message"
	"API_Gateway/handler/transformer"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"bufio"
	"errors"
	"fmt"
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
	filters      []hf.HTTPFilter
	transformers map[string]transformer.Transformer
	next         func(service string, payload []byte) ([]byte, error)
}

type ConnHandler struct {
	conn   net.Conn
	reader *bufio.Reader
	close  bool
}

func NewHandler(cfg builder.HandlerConfig, next func(service string, payload []byte) ([]byte, error)) (*HTTPHandler, error) {
	decoder := NewDecoder(cfg.Decoder)
	encoder := NewEncoder(cfg.Encoder)
	// 初始化 filters 和 transformer
	filters, err := buildFilter(cfg.Filter.Filters)
	if err != nil {
		return nil, err
	}
	transformers := buildTransformer(cfg.Transformer)
	return &HTTPHandler{
		decoder:      decoder,
		encoder:      encoder,
		filters:      filters,
		transformers: transformers,
		next:         next,
	}, nil
}

func (h *HTTPHandler) Process(ch *ConnHandler) (*message.Response, error) {
	buf, err := http.ReadRequest(ch.reader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", gerrors.ErrReadRequest, err)
	}
	// TODO
	// 0. 连接判断
	if buf.Close {
		ch.close = true
	}
	// 1. 解析
	msgReq, err := h.decoder.Decode(buf)
	if err != nil {
		return nil, err
	}
	msgReq.RemoteAddr = ch.conn.RemoteAddr().String()
	// 2. filter 编排
	for _, f := range h.filters {
		if _, err := f.HandleHTTPFilt(&msgReq); err != nil {
			return nil, err
		}
	}
	if msgReq.Service == "" {
		return nil, gerrors.ErrServiceNotFound
	}
	// 3. 协议转换
	transformer_0 := h.transformers["testT"]
	tResp, err := transformer_0.Transform(&msgReq)
	if err != nil {
		return nil, err
	}
	// 4. next 调用
	log.Println("[handler]pass message to backend")
	resp, err := h.next(msgReq.Service, tResp)
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
		msg, err := h.Process(&ch)
		if err != nil {
			log.Printf("[handler]failed to process, err message: %s", err.Error())
			if closeConnOrNot(err) {
				return err
			}
			msg = ErrorResponse(err)
		}
		if err := h.handleProcessResponse(msg, &ch); err != nil || ch.close {
			log.Println("[handler]handler ended")
			return err
		}
	}
}

func buildFilter(cfgs []any) ([]hf.HTTPFilter, error) {
	filters := []hf.HTTPFilter{}

	for i, cfg := range cfgs {
		switch filter := cfg.(type) {
		case builder.RouterConfig:
			router, err := hf.NewRouter(filter)
			if err != nil {
				return nil, err
			}
			filters = append(filters, router)
		default:
			log.Printf("[handler]failed to initialize http filter %d, unsupported config type %T", i, cfg)
			return nil, gerrors.ErrInitializeHTTPFiltersFailed
		}
	}
	filters = append(filters, &hf.TestFilter{Name: "testF"})

	return filters, nil
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
		close:  false,
	}
}

func (h *HTTPHandler) handleProcessResponse(msg *message.Response, ch *ConnHandler) error {
	if msg == nil {
		return nil
	}

	res, errE := h.encoder.Encode(msg)
	_, errW := ch.conn.Write(res)

	if shouldClose := closeConnOrNot(errE) || closeConnOrNot(errW); shouldClose {
		return gerrors.ErrHandleProcessResponse
	}
	return nil
}

func closeConnOrNot(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	if errors.Is(err, gerrors.ErrDecodeRequest) ||
		errors.Is(err, gerrors.ErrBodyTooLarge) ||
		errors.Is(err, gerrors.ErrReadRequest) {
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

func ErrorResponse(err error) *message.Response {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, gerrors.ErrBodyTooLarge):
		status = http.StatusRequestEntityTooLarge // 413
	case errors.Is(err, gerrors.ErrDecodeRequest),
		errors.Is(err, gerrors.ErrEmptyRequest):
		status = http.StatusBadRequest // 400
	case errors.Is(err, gerrors.ErrHTTPFilterReject):
		status = http.StatusForbidden // 403
	case errors.Is(err, gerrors.ErrRouteNotFound):
		status = http.StatusNotFound // 404
	case errors.Is(err, gerrors.ErrServiceNotFound),
		errors.Is(err, gerrors.ErrNoInstance),
		errors.Is(err, gerrors.ErrDialFailed):
		status = http.StatusBadGateway // 502
	}

	reason := http.StatusText(status)
	body := []byte(reason)
	if err != nil {
		body = []byte(err.Error())
	}

	return &message.Response{
		Header: http.Header{
			"Content-Type": []string{gconst.DefaultErrorContentType},
		},
		Body:          body,
		StatusCode:    status,
		Proto:         gconst.DefaultHTTPProto,
		ContentLength: int64(len(body)),
	}
}
