package message

import (
	"net/http"
	"net/url"
)

// 网关统一的 HTTP 请求/响应模型
// decoder、http_filter、transformer 胶水、encoder 都用它

type Request struct {
	Header        http.Header
	Body          []byte
	Method        string
	Proto         string
	Host          string
	URL           *url.URL
	ContentLength int64

	RemoteAddr string

	// —— 网关决策：filter 产出，编排层消费 ——
	Service string
}

type Response struct {
	Header        http.Header
	Body          []byte
	StatusCode    int
	Proto         string
	ContentLength int64
}
