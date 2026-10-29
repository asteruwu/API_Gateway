package handler

import (
	"API_Gateway/builder"
	hf "API_Gateway/handler/http_filter"
	"API_Gateway/handler/http_filter/auth"
	"API_Gateway/handler/http_filter/ratelimit"
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
	router       *hf.Router
	pipelines    map[string][]hf.HTTPFilter
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
	router, filtertable, err := buildFilter(cfg.Filter)
	if err != nil {
		return nil, err
	}
	pipelines, err := buildPipelines(cfg.Filter.Pipelines, filtertable)
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
		router:       router,
		pipelines:    pipelines,
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
	// 2. Router 查找 RouteID / Service
	resp, err = h.router.HandleHTTPFilt(&msgReq)
	if err != nil || resp != nil {
		return resp, err
	}
	if msgReq.Service == "" {
		return nil, gerrors.ErrServiceNotFound
	}
	// 3. 执行 pipeline
	chain, ok := h.pipelines[msgReq.RouteID]
	if !ok {
		return nil, fmt.Errorf("%w: route %q", gerrors.ErrPipelineNotFound, msgReq.RouteID)
	}
	for _, f := range chain {
		resp, err = f.HandleHTTPFilt(&msgReq)
		if err != nil || resp != nil {
			return resp, err
		}
	}
	// 4. 按 service 选取 transformer
	tr, err := h.pickTransformer(msgReq.Service)
	if err != nil {
		return nil, err
	}
	// 5. next 调用：借出后端连接，并拿到 release 回调
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
	// 6. 协议转换
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

// buildFilter 从运行态配置构造 Router 实例和「名字 → filter 实例」表
func buildFilter(cfg builder.HTTPFilterConfig) (*hf.Router, map[string]hf.HTTPFilter, error) {
	router, err := hf.NewRouter(cfg.Router)
	if err != nil {
		log.Printf("[handler]failed to initialize router: %s", err.Error())
		return nil, nil, err
	}

	table := make(map[string]hf.HTTPFilter, len(cfg.Configs))
	for name, raw := range cfg.Configs {
		switch filter := raw.(type) {
		case builder.AuthFilterConfig:
			authFilter, err := auth.NewAuthFilter(filter)
			if err != nil {
				log.Printf("[handler]failed to initialize auth filter: %s", err.Error())
				return nil, nil, err
			}
			table[name] = authFilter
		case builder.RateLimitFilterConfig:
			rateLimiter, err := ratelimit.NewRateLimiter(filter)
			if err != nil {
				log.Printf("[handler]failed to initialize rate limit filter: %s", err.Error())
				return nil, nil, fmt.Errorf("%w: %v", gerrors.ErrInitializeHTTPFiltersFailed, err)
			}
			table[name] = rateLimiter
		default:
			log.Printf("[handler]failed to initialize http filter %q, unsupported config type %T", name, raw)
			return nil, nil, gerrors.ErrInitializeHTTPFiltersFailed
		}
	}
	return router, table, nil
}

// buildPipelines 把 route pipeline 的名字预解析为共享的 filter 实例
func buildPipelines(pipelines map[string][]string, table map[string]hf.HTTPFilter) (map[string][]hf.HTTPFilter, error) {
	result := make(map[string][]hf.HTTPFilter, len(pipelines))
	for routeID, names := range pipelines {
		chain := make([]hf.HTTPFilter, 0, len(names))
		for _, name := range names {
			f, ok := table[name]
			if !ok {
				return nil, fmt.Errorf("%w: route %q pipeline references unknown filter %q",
					gerrors.ErrInitializeHTTPFiltersFailed, routeID, name)
			}
			chain = append(chain, f)
		}
		result[routeID] = chain
	}
	return result, nil
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
	case errors.Is(err, gerrors.ErrRateLimited):
		status = http.StatusTooManyRequests
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
