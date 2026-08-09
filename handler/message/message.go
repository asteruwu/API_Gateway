package message

// 网关统一的 HTTP 请求/响应模型
// decoder、http_filter、transformer 胶水、encoder 都用它

type Request struct {
	Raw   []byte
	Proto string
}

type Response struct {
	Raw []byte
}
