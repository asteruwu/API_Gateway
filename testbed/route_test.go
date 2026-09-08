package testbed

import (
	"API_Gateway/builder"
	"API_Gateway/testbed/fakebackend"
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
)

// startFixedBackend 启动固定响应模式的假后端，响应体恒为 resp，
// 用于路由测试中区分请求实际到达了哪个服务。
func startFixedBackend(t *testing.T, resp []byte) string {
	t.Helper()
	fb := &fakebackend.FakeBackend{
		Addr:      "127.0.0.1:0",
		Mode:      fakebackend.ModeFixed,
		FixedResp: resp,
	}
	if err := fb.Start(); err != nil {
		t.Fatalf("fakebackend start: %v", err)
	}
	t.Cleanup(func() { _ = fb.Close() })
	return fb.RealAddr()
}

// postViaKeepAlive 在同一条连接上向指定路径发送 POST 并读回状态码与响应体，
// 连接保持可继续复用。
func postViaKeepAlive(t *testing.T, conn net.Conn, reader *bufio.Reader, path string, body []byte) (int, []byte) {
	t.Helper()
	req := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: gateway\r\nContent-Length: %d\r\n\r\n%s", path, len(body), body)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, got
}

// TestRouteDispatch 路由分发：两条路由指向不同假后端，按路径转发；
// 未命中返回 404，写回后同一连接 keep-alive 继续可用。
func TestRouteDispatch(t *testing.T) {
	svcA := startFixedBackend(t, []byte("resp-from-svcA"))
	svcB := startFixedBackend(t, []byte("resp-from-svcB"))

	const gatewayAddr = "127.0.0.1:9996"
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{Port: "9996"},
		Handler: builder.HandlerConfig{
			Filter: builder.HTTPFilterConfig{
				Filters: []any{routerConfig(map[string][]string{
					"svcA": {"/orders"},
					"svcB": {"/users"}})},
			},
		},
		Backend: backendConfig(map[string][]string{
			"svcA": {svcA},
			"svcB": {svcB},
		}),
	}
	startGateway(t, cfg)

	conn, err := net.Dial("tcp", gatewayAddr)
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// 按路径分发到不同后端
	status, got := postViaKeepAlive(t, conn, reader, "/orders/1", []byte("hello"))
	if status != http.StatusOK || string(got) != "resp-from-svcA" {
		t.Errorf("/orders/1: status=%d body=%q, want 200 resp-from-svcA", status, got)
	}
	status, got = postViaKeepAlive(t, conn, reader, "/users/2", []byte("hello"))
	if status != http.StatusOK || string(got) != "resp-from-svcB" {
		t.Errorf("/users/2: status=%d body=%q, want 200 resp-from-svcB", status, got)
	}

	// 未命中 404，写回后 keep-alive 继续可用
	status, got = postViaKeepAlive(t, conn, reader, "/nope", []byte("hello"))
	if status != http.StatusNotFound {
		t.Errorf("/nope: status=%d, want 404", status)
	}
	status, got = postViaKeepAlive(t, conn, reader, "/orders/3", []byte("hello"))
	if status != http.StatusOK || string(got) != "resp-from-svcA" {
		t.Errorf("after 404 /orders/3: status=%d body=%q, want 200 resp-from-svcA", status, got)
	}
}
