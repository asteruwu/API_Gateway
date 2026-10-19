package handler

import (
	"API_Gateway/builder"
	hf "API_Gateway/handler/http_filter"
	"API_Gateway/handler/http_filter/auth"
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
	next         func(service string) (net.Conn, func(error), error)
}

type ConnHandler struct {
	conn    net.Conn
	reader  *bufio.Reader
	limiter *io.LimitedReader
	close   bool
}

func NewHandler(cfg builder.HandlerConfig, next func(service string) (net.Conn, func(error), error)) (*HTTPHandler, error) {
	decoder := NewDecoder(cfg.Decoder)
	encoder := NewEncoder(cfg.Encoder)
	// 初始化 filters 和 transformer
	filters, err := buildFilter(cfg.Filter.Filters)
	if err != nil {
		return nil, err
	}
	transformers, err := buildTransformer(cfg.Transformer)
	if err != nil {
		return nil, err
	}
	return &HTTPHandler{
		decoder:      decoder,
		encoder:      encoder,
		filters:      filters,
		transformers: transformers,
		next:         next,
	}, nil
}

func (h *HTTPHandler) Process(ch *ConnHandler) (resp *message.Response, err error) {
	buf, err := http.ReadRequest(ch.reader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", gerrors.ErrReadRequest, err)
	}
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
		resp, err := f.HandleHTTPFilt(&msgReq)
		if err != nil {
			return resp, err
		}
		if resp != nil {
			return resp, nil
		}
	}
	if msgReq.Service == "" {
		return nil, gerrors.ErrServiceNotFound
	}
	// 3. 按 service 选取 transformer
	tr, err := h.pickTransformer(msgReq.Service)
	if err != nil {
		return nil, err
	}
	// 4. next 调用：借出后端连接，并拿到 release 回调
	log.Println("[handler]pass message to backend")
	conn, release, err := h.next(msgReq.Service)
	if err != nil {
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			release(gerrors.ErrInternal)
			panic(r)
		}
		release(err)
	}()
	// 5. 协议转换
	return tr.Transform(&msgReq, conn)
}

func (h *HTTPHandler) pickTransformer(service string) (transformer.Transformer, error) {
	tr, ok := h.transformers[service]
	if !ok {
		return nil, fmt.Errorf("%w: service %q", gerrors.ErrTransformerNotFound, service)
	}
	return tr, nil
}

func (h *HTTPHandler) HandleHTTPConn(conn net.Conn) error {
	ch := buildConnHandler(conn)
	for {
		ch.resetReadLimit()
		ch.conn.SetReadDeadline(time.Now().Add(gconst.DefaultHandlerReadTimeout))
		msg, err := h.Process(&ch)
		if err != nil {
			log.Printf("[handler]failed to process, err message: %s", err.Error())
			if closeConnOrNot(err) {
				return err
			}
			msg = ErrorResponse(err, msg)
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
				log.Printf("[handler]failed to initialize router: %s", err.Error())
				return nil, err
			}
			filters = append(filters, router)
		case builder.AuthFilterConfig:
			authFilter, err := auth.NewAuthFilter(filter)
			if err != nil {
				log.Printf("[handler]failed to initialize auth filter: %s", err.Error())
				return nil, err
			}
			filters = append(filters, authFilter)
		default:
			log.Printf("[handler]failed to initialize http filter %d, unsupported config type %T", i, cfg)
			return nil, gerrors.ErrInitializeHTTPFiltersFailed
		}
	}

	return filters, nil
}

func buildTransformer(cfg builder.TransformerConfig) (map[string]transformer.Transformer, error) {
	byService := make(map[string]transformer.Transformer)
	for name, entry := range cfg.Transformers {
		var t transformer.Transformer
		switch entry.Config.(type) {
		case builder.TestTransformerConfig:
			t = &transformer.TestTransformer{Name: name}
		default:
			log.Printf("[handler]failed to initialize transformer %q, unsupported config type %T", name, entry.Config)
			return nil, gerrors.ErrInitializeTransformersFailed
		}
		for _, service := range entry.Services {
			byService[service] = t
		}
	}
	return byService, nil
}

func newReadLimiter(r io.Reader) *io.LimitedReader {
	return &io.LimitedReader{R: r, N: gconst.DefaultMaxRequestReadBytes}
}

func (ch *ConnHandler) resetReadLimit() {
	ch.limiter.N = gconst.DefaultMaxRequestReadBytes
}

func buildConnHandler(conn net.Conn) ConnHandler {
	lr := newReadLimiter(conn)
	return ConnHandler{
		conn:    conn,
		reader:  bufio.NewReader(lr),
		limiter: lr,
		close:   false,
	}
}

func (h *HTTPHandler) handleProcessResponse(msg *message.Response, ch *ConnHandler) error {
	if msg == nil {
		return nil
	}

	res, errE := h.encoder.Encode(msg)
	ch.conn.SetWriteDeadline(time.Now().Add(gconst.DefaultHandlerWriteTimeout))
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

// ErrorResponse 映射可写回的错误
func ErrorResponse(err error, partial *message.Response) *message.Response {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, gerrors.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, gerrors.ErrForbidden),
		errors.Is(err, gerrors.ErrHTTPFilterReject):
		status = http.StatusForbidden
	case errors.Is(err, gerrors.ErrRouteNotFound):
		status = http.StatusNotFound
	case errors.Is(err, gerrors.ErrServiceNotFound),
		errors.Is(err, gerrors.ErrNoInstance),
		errors.Is(err, gerrors.ErrDialFailed),
		errors.Is(err, gerrors.ErrForwardWrite),
		errors.Is(err, gerrors.ErrReadResponse),
		errors.Is(err, gerrors.ErrTransform),
		errors.Is(err, gerrors.ErrRestore):
		status = http.StatusBadGateway
	case errors.Is(err, gerrors.ErrPoolExhausted),
		errors.Is(err, gerrors.ErrPoolClosed),
		errors.Is(err, gerrors.ErrInstanceUnhealthy):
		status = http.StatusServiceUnavailable
	}

	if partial == nil {
		partial = &message.Response{}
	}
	partial.StatusCode = status
	if partial.Proto == "" {
		partial.Proto = gconst.DefaultHTTPProto
	}
	if partial.Header == nil {
		partial.Header = http.Header{}
	}
	if partial.Header.Get("Content-Type") == "" {
		partial.Header.Set("Content-Type", gconst.DefaultErrorContentType)
	}
	if len(partial.Body) == 0 {
		partial.Body = []byte(http.StatusText(status))
	}
	partial.ContentLength = int64(len(partial.Body))
	return partial
}
