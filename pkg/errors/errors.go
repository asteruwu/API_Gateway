package gerrors

import "errors"

// 未分类的内部错误
var ErrInternal = errors.New("gateway: internal error")

// connector errors
var (
	// ErrListenFailed 监听端口失败
	ErrListenFailed = errors.New("connector: listen failed")

	// ErrListenerClosed 监听器已关闭
	ErrListenerClosed = errors.New("connector: listener closed")

	// ErrAcceptFailed Accept 连接失败
	ErrAcceptFailed = errors.New("connector: accept failed")

	// ErrInitializeFiltersFailed 初始化 filter 失败
	ErrInitializeTCPFiltersFailed = errors.New("connector: initialize tcp filters failed")

	// ErrInitializeListenerFailed 初始化 listener 失败
	ErrInitializeListenerFailed = errors.New("connector: initialize listener failed")

	// ErrConnLimitExceeded 连接数超过上限，被连接级限流拒绝
	ErrTCPConnLimitExceeded = errors.New("connector: connection limit exceeded")

	// ErrConnAdmission 连接未通过准入过滤（tcp_filter 拒绝），被直接关闭
	ErrConnAdmission = errors.New("connector: connection rejected by admission filter")
)

// handler errors
var (
	// ErrInitializeHTTPFiltersFailed 初始化 filter 失败
	ErrInitializeHTTPFiltersFailed = errors.New("handler: initialize http filters failed")

	// ErrRouteNotFound 路由未命中
	ErrRouteNotFound = errors.New("handler: route not found")

	// ErrReadRequest 读取/解析 HTTP 请求失败
	ErrReadRequest = errors.New("handler: read request failed")

	// ErrDecodeRequest 请求解码失败
	ErrDecodeRequest = errors.New("handler: decode request failed")

	ErrBodyTooLarge = errors.New("handler: request too large")

	// ErrEncodeResponse 响应编码失败
	ErrEncodeResponse = errors.New("handler: encode response failed")

	// ErrFilterReject 请求被 HTTP filter 拒绝（filter 主动中断并产出响应）
	ErrHTTPFilterReject = errors.New("handler: request rejected by filter")

	// ErrTransform 请求协议转换失败
	ErrTransform = errors.New("handler: transform request failed")

	// ErrRestore 响应协议还原失败
	ErrRestore = errors.New("handler: restore response failed")

	// ErrEmptyRequest 读到空请求
	ErrEmptyRequest = errors.New("handler: empty request")

	// ErrHandleProcessResponse 响应处理失败
	ErrHandleProcessResponse = errors.New("handler: handle process response failed")
)

// backend errors
var (
	// ErrServiceNotFound 服务未注册
	ErrServiceNotFound = errors.New("backend: service not found")

	// ErrNoInstance 服务存在但没有可用实例
	ErrNoInstance = errors.New("backend: no available instance")

	// ErrDialFailed 建立到后端的连接失败
	ErrDialFailed = errors.New("backend: dial backend failed")

	// ErrForwardWrite 请求字节写入后端失败
	ErrForwardWrite = errors.New("backend: write to backend failed")

	// ErrReadResponse 读取后端响应失败
	ErrReadResponse = errors.New("backend: read response failed")

	// ErrInstanceUnhealthy 实例健康检查未通过
	ErrInstanceUnhealthy = errors.New("backend: instance unhealthy")

	// ErrPoolExhausted 连接池无可用连接
	ErrPoolExhausted = errors.New("backend: connection pool exhausted")
)
