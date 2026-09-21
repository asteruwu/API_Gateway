package gerrors

import "errors"

// 未分类的内部错误
var ErrInternal = errors.New("gateway: internal error")

// builder errors
var (
	// ErrInvalidConfig 配置校验未通过的顶层包装错误，具体原因见其包裹的错误
	ErrInvalidConfig = errors.New("builder: invalid config")

	// ErrEmptyPlatformName platform 名为空
	ErrEmptyPlatformName = errors.New("builder: platform name is empty")

	// ErrDuplicatePlatformName platform 名重复
	ErrDuplicatePlatformName = errors.New("builder: duplicate platform name")

	// ErrEmptyServiceName service 名为空
	ErrEmptyServiceName = errors.New("builder: service name is empty")

	// ErrDuplicateServiceName 同一 platform 下 service 名重复
	ErrDuplicateServiceName = errors.New("builder: duplicate service name within platform")

	// ErrDuplicateRoute 不同 service 声明了会产生歧义的相同 host+path+method 组合
	ErrDuplicateRoute = errors.New("builder: duplicate or ambiguous route")

	// ErrEmptyServiceInstances service 声明了 0 个实例，注册后必然在运行期触发 ErrNoInstance
	ErrEmptyServiceInstances = errors.New("builder: service has no instances")

	// ErrFilterConfigNotFound PluginSet.Enabled 声明了某个名字要启用，
	// 但 PluginSet.Configs 里找不到同名配置
	ErrFilterConfigNotFound = errors.New("builder: enabled filter has no matching config")
)

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

	// ErrInitializeTransformersFailed 初始化 transformer 失败
	ErrInitializeTransformersFailed = errors.New("handler: initialize transformers failed")

	// ErrInvalidRouterRule 路由规则自身不合法
	ErrInvalidRouterRule = errors.New("handler: invalid router rule")
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
